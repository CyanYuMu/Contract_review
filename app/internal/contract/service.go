package contract

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"contract_review/app/internal/agent"
	"contract_review/app/internal/global"
	"contract_review/app/internal/middleware/redis"
	"contract_review/app/pkg/utils"

	"go.uber.org/zap"
)

type ContractService struct {
	contractRepo *ContractRepo
	cache        *redis.RedisClient
}

// NewContractService 创建合同服务
func NewContractService(repo *ContractRepo, cache *redis.RedisClient) *ContractService {
	return &ContractService{
		contractRepo: repo,
		cache:        cache,
	}
}

// ContractInfo 合同信息结构（LLM返回的JSON结构）
type ContractInfo struct {
	PartyA string `json:"party_a"`
	PartyB string `json:"party_b"`
	Amount string `json:"amount"`
	Type   string `json:"type"`
}

// ParseContractInfo 解析LLM返回的合同信息JSON
func ParseContractInfo(raw string) (ContractInfo, error) {
	var info ContractInfo

	// 尝试直接JSON解析
	if err := json.Unmarshal([]byte(raw), &info); err != nil {
		// 正则兜底：提取JSON部分
		re := regexp.MustCompile(`\{[\s\S]*\}`)
		match := re.FindString(raw)
		if match == "" {
			return ContractInfo{}, errors.New("无法解析合同信息：未找到有效JSON")
		}
		if err := json.Unmarshal([]byte(match), &info); err != nil {
			return ContractInfo{}, errors.New("无法解析合同信息：JSON格式错误")
		}
	}

	// 清洗数据
	clean := func(s string) string {
		if strings.Contains(s, "未识别") || len(s) > 300 {
			return ""
		}
		return strings.TrimSpace(s)
	}

	info.PartyA = clean(info.PartyA)
	info.PartyB = clean(info.PartyB)
	info.Amount = clean(info.Amount)
	info.Type = clean(info.Type)

	return info, nil
}

// ParseAmount 解析金额字符串为float64
func ParseAmount(amountStr string) float64 {
	if amountStr == "" {
		return 0.0
	}

	// 移除常见的金额单位
	amountStr = strings.ReplaceAll(amountStr, "元", "")
	amountStr = strings.ReplaceAll(amountStr, "人民币", "")
	amountStr = strings.ReplaceAll(amountStr, "￥", "")
	amountStr = strings.ReplaceAll(amountStr, "¥", "")
	amountStr = strings.ReplaceAll(amountStr, ",", "")
	amountStr = strings.ReplaceAll(amountStr, "，", "")
	amountStr = strings.TrimSpace(amountStr)

	// 处理万/亿等单位
	multiplier := 1.0
	if strings.Contains(amountStr, "万") {
		multiplier = 10000.0
		amountStr = strings.ReplaceAll(amountStr, "万", "")
	} else if strings.Contains(amountStr, "亿") {
		multiplier = 100000000.0
		amountStr = strings.ReplaceAll(amountStr, "亿", "")
	}

	// 提取数字部分
	re := regexp.MustCompile(`[\d.]+`)
	match := re.FindString(amountStr)
	if match == "" {
		return 0.0
	}

	amount, err := strconv.ParseFloat(match, 64)
	if err != nil {
		return 0.0
	}

	return amount * multiplier
}

// ============ Contract 相关服务 ============

// FileLoad 上传并解析合同文件
// 1. 提取文档文本
// 2. 调用LLM解析合同信息
// 3. 保存合同记录到数据库
func (cs *ContractService) FileLoad(ctx context.Context, account string, filePath string, filename string) (Contract, error) {
	global.Log.Info("开始处理合同文件",
		zap.String("filePath", filePath),
		zap.String("filename", filename),
		zap.String("account", account))

	// 1. 提取文档文本
	content, err := utils.ExtractText(filePath)
	if err != nil {
		global.Log.Error("提取文档文本失败", zap.Error(err))
		return Contract{}, errors.New("无法提取文档内容：" + err.Error())
	}

	if strings.TrimSpace(content) == "" {
		global.Log.Warn("文档内容为空")
		return Contract{}, errors.New("文档内容为空，无法解析")
	}

	global.Log.Info("文档文本提取成功", zap.Int("contentLength", len(content)))

	// 2. 调用LLM解析合同信息
	raw, err := agent.LLMContractParse(ctx, content)
	if err != nil {
		global.Log.Error("LLM解析合同失败", zap.Error(err))
		return Contract{}, errors.New("AI解析合同信息失败：" + err.Error())
	}

	global.Log.Info("LLM返回结果", zap.String("raw", raw))

	// 3. 解析JSON响应
	info, err := ParseContractInfo(raw)
	if err != nil {
		global.Log.Warn("解析合同信息JSON失败，使用默认值", zap.Error(err))
		// 不完全失败，继续处理
		info = ContractInfo{}
	}

	global.Log.Info("解析出的合同信息",
		zap.String("partyA", info.PartyA),
		zap.String("partyB", info.PartyB),
		zap.String("amount", info.Amount),
		zap.String("type", info.Type))

	// 4. 金额标准化
	amount := ParseAmount(info.Amount)

	// 5. 获取文件类型
	fileType := utils.GetFileType(filename)

	// 6. 尝试匹配合同类型
	var typeID uint64 = 0
	if info.Type != "" {
		contractType, err := cs.contractRepo.GetContractTypeByName(ctx, info.Type)
		if err == nil && contractType != nil {
			typeID = contractType.ID
		}
	}

	// 7. 构建合同记录
	contract := Contract{
		Account:    account,
		TypeID:     typeID,
		Title:      filename,
		FilePath:   filePath,
		FileType:   fileType,
		UploadTime: time.Now(),
		Status:     "uploaded",
		PartyA:     info.PartyA,
		PartyB:     info.PartyB,
		Amount:     amount,
		IsAccepted: 0,
	}

	// 8. 保存到数据库
	if err := cs.contractRepo.CreateContract(ctx, &contract); err != nil {
		global.Log.Error("保存合同记录失败", zap.Error(err))
		return Contract{}, errors.New("保存合同记录失败：" + err.Error())
	}

	global.Log.Info("合同文件处理完成",
		zap.Uint64("contractID", contract.ID),
		zap.String("partyA", contract.PartyA),
		zap.String("partyB", contract.PartyB),
		zap.Float64("amount", contract.Amount))

	return contract, nil
}

// GetContractByID 根据ID获取合同
func (cs *ContractService) GetContractByID(ctx context.Context, id uint64) (*Contract, error) {
	return cs.contractRepo.GetContractByID(ctx, id)
}

// GetContractByIDWithType 根据ID获取合同（包含类型信息）
func (cs *ContractService) GetContractByIDWithType(ctx context.Context, id uint64) (*Contract, error) {
	return cs.contractRepo.GetContractByIDWithType(ctx, id)
}

// GetContractsByAccount 获取用户的合同列表
func (cs *ContractService) GetContractsByAccount(ctx context.Context, account string) ([]Contract, error) {
	return cs.contractRepo.GetContractsByAccount(ctx, account)
}

// ListContractsByAccount 分页获取用户合同列表
func (cs *ContractService) ListContractsByAccount(ctx context.Context, account string, page, pageSize int) ([]Contract, int64, error) {
	offset := (page - 1) * pageSize
	return cs.contractRepo.ListContractsByAccount(ctx, account, offset, pageSize)
}

// ListContractsByAccountWithType 分页获取用户合同列表（包含类型信息）
func (cs *ContractService) ListContractsByAccountWithType(ctx context.Context, account string, page, pageSize int) ([]Contract, int64, error) {
	offset := (page - 1) * pageSize
	return cs.contractRepo.ListContractsByAccountWithType(ctx, account, offset, pageSize)
}

// UpdateContract 更新合同信息
func (cs *ContractService) UpdateContract(ctx context.Context, contract *Contract) error {
	return cs.contractRepo.UpdateContract(ctx, contract)
}

// UpdateContractTypeID 更新合同的类型ID
func (cs *ContractService) UpdateContractTypeID(ctx context.Context, contractID uint64, typeID uint64) error {
	// 验证类型是否存在
	exists, err := cs.contractRepo.ExistsContractType(ctx, typeID)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("合同类型不存在")
	}

	return cs.contractRepo.UpdateContractByID(ctx, contractID, map[string]interface{}{
		"type_id": typeID,
	})
}

// DeleteContract 删除合同
func (cs *ContractService) DeleteContract(ctx context.Context, id uint64) error {
	// 获取合同信息用于删除文件
	contract, err := cs.contractRepo.GetContractByID(ctx, id)
	if err != nil {
		return err
	}

	// 删除数据库记录
	if err := cs.contractRepo.DeleteContract(ctx, id); err != nil {
		return err
	}

	// 尝试删除文件（不影响返回结果）
	if contract.FilePath != "" {
		if err := os.Remove(contract.FilePath); err != nil {
			global.Log.Warn("删除合同文件失败", zap.String("filePath", contract.FilePath), zap.Error(err))
		}
	}

	return nil
}

// GetContractFilePath 获取合同文件路径用于下载
func (cs *ContractService) GetContractFilePath(ctx context.Context, id uint64) (string, string, error) {
	contract, err := cs.contractRepo.GetContractByID(ctx, id)
	if err != nil {
		return "", "", err
	}

	// 检查文件是否存在
	if _, err := os.Stat(contract.FilePath); os.IsNotExist(err) {
		return "", "", errors.New("文件不存在")
	}

	return contract.FilePath, contract.Title, nil
}

// ============ ContractType 相关服务 ============

// CreateContractType 创建合同类型
func (cs *ContractService) CreateContractType(ctx context.Context, name string) (*ContractType, error) {
	// 检查名称是否已存在
	exists, err := cs.contractRepo.ExistsContractTypeByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, errors.New("合同类型名称已存在")
	}

	contractType := &ContractType{
		Name:      name,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := cs.contractRepo.CreateContractType(ctx, contractType); err != nil {
		return nil, err
	}

	return contractType, nil
}

// GetContractTypeByID 根据ID获取合同类型
func (cs *ContractService) GetContractTypeByID(ctx context.Context, id uint64) (*ContractType, error) {
	return cs.contractRepo.GetContractTypeByID(ctx, id)
}

// GetContractTypeByName 根据名称获取合同类型
func (cs *ContractService) GetContractTypeByName(ctx context.Context, name string) (*ContractType, error) {
	return cs.contractRepo.GetContractTypeByName(ctx, name)
}

// ListContractTypes 获取所有合同类型
func (cs *ContractService) ListContractTypes(ctx context.Context) ([]ContractType, error) {
	return cs.contractRepo.ListContractTypes(ctx)
}

// ListContractTypesPaginated 分页获取合同类型
func (cs *ContractService) ListContractTypesPaginated(ctx context.Context, page, pageSize int) ([]ContractType, int64, error) {
	offset := (page - 1) * pageSize
	return cs.contractRepo.ListContractTypesPaginated(ctx, offset, pageSize)
}

// UpdateContractType 更新合同类型
func (cs *ContractService) UpdateContractType(ctx context.Context, id uint64, name string) error {
	// 检查类型是否存在
	exists, err := cs.contractRepo.ExistsContractType(ctx, id)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("合同类型不存在")
	}

	// 检查新名称是否与其他类型重复
	existingType, err := cs.contractRepo.GetContractTypeByName(ctx, name)
	if err == nil && existingType != nil && existingType.ID != id {
		return errors.New("合同类型名称已存在")
	}

	return cs.contractRepo.UpdateContractTypeByID(ctx, id, map[string]interface{}{
		"name":       name,
		"updated_at": time.Now(),
	})
}

// DeleteContractType 删除合同类型
func (cs *ContractService) DeleteContractType(ctx context.Context, id uint64) error {
	return cs.contractRepo.DeleteContractType(ctx, id)
}

// GetContractTypeUsageCount 获取合同类型使用数量
func (cs *ContractService) GetContractTypeUsageCount(ctx context.Context, typeID uint64) (int64, error) {
	return cs.contractRepo.CountContractsByTypeID(ctx, typeID)
}
