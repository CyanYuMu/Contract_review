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
	"gorm.io/gorm"
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

	// 6. 尝试匹配合同类型；未匹配时使用默认类型，避免 type_id=0 触发外键失败。
	typeID, err := cs.ensureContractTypeID(ctx, info.Type)
	if err != nil {
		global.Log.Error("获取合同类型失败", zap.Error(err))
		return Contract{}, errors.New("保存合同类型失败：" + err.Error())
	}

	// 7. 构建合同记录
	contract := Contract{
		Account:    account,
		TypeID:     typeID,
		Title:      limitString(filename, 256),
		FilePath:   limitString(filePath, 512),
		FileType:   limitString(fileType, 16),
		UploadTime: time.Now(),
		Status:     "uploaded",
		PartyA:     limitString(info.PartyA, 128),
		PartyB:     limitString(info.PartyB, 128),
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

func (cs *ContractService) ensureContractTypeID(ctx context.Context, typeName string) (uint64, error) {
	name := strings.TrimSpace(typeName)
	if name == "" {
		name = "其他"
	}
	name = limitString(name, 64)

	contractType, err := cs.contractRepo.GetContractTypeByName(ctx, name)
	if err == nil && contractType != nil {
		return contractType.ID, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}

	contractType = &ContractType{Name: name}
	if err := cs.contractRepo.CreateContractType(ctx, contractType); err != nil {
		existing, getErr := cs.contractRepo.GetContractTypeByName(ctx, name)
		if getErr == nil && existing != nil {
			return existing.ID, nil
		}
		defaultType, defaultErr := cs.ensureDefaultContractType(ctx)
		if defaultErr != nil {
			return 0, err
		}
		return defaultType.ID, nil
	}
	return contractType.ID, nil
}

func (cs *ContractService) ensureDefaultContractType(ctx context.Context) (*ContractType, error) {
	const defaultTypeName = "其他"
	contractType, err := cs.contractRepo.GetContractTypeByName(ctx, defaultTypeName)
	if err == nil && contractType != nil {
		return contractType, nil
	}
	contractType = &ContractType{Name: defaultTypeName}
	if err := cs.contractRepo.CreateContractType(ctx, contractType); err != nil {
		return nil, err
	}
	return contractType, nil
}

func limitString(value string, max int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= max {
		return string(runes)
	}
	return string(runes[:max])
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
		filePath := LocalFilePath(contract.FilePath)
		if err := os.Remove(filePath); err != nil {
			global.Log.Warn("删除合同文件失败", zap.String("filePath", filePath), zap.Error(err))
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
	filePath := LocalFilePath(contract.FilePath)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return "", "", errors.New("文件不存在")
	}

	return filePath, contract.Title, nil
}

// ============ ContractType 相关服务 ============

// CreateContractType 创建合同类型
func (cs *ContractService) CreateContractType(ctx context.Context, name, templateContent, creator string) (*ContractType, error) {
	name = limitString(name, 64)
	templateContent = limitString(templateContent, 5000)
	creator = limitString(creator, 64)

	// 检查名称是否已存在
	exists, err := cs.contractRepo.ExistsContractTypeByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, errors.New("合同类型名称已存在")
	}

	contractType := &ContractType{
		Name:            name,
		TemplateContent: templateContent,
		Creator:         creator,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
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

// ListContractTypesFiltered 分页筛选合同类型
func (cs *ContractService) ListContractTypesFiltered(ctx context.Context, name, creator, startDate, endDate string, page, pageSize int) ([]ContractType, int64, error) {
	offset := (page - 1) * pageSize
	return cs.contractRepo.ListContractTypesFiltered(ctx, name, creator, startDate, endDate, offset, pageSize)
}

// UpdateContractType 更新合同类型
func (cs *ContractService) UpdateContractType(ctx context.Context, id uint64, name, templateContent string) error {
	name = limitString(name, 64)
	templateContent = limitString(templateContent, 5000)

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
		"name":             name,
		"template_content": templateContent,
		"updated_at":       time.Now(),
	})
}

// DeleteContractType 删除合同类型
func (cs *ContractService) DeleteContractType(ctx context.Context, id uint64) error {
	return cs.contractRepo.DeleteContractType(ctx, id)
}

// DeleteContractTypes 批量删除合同类型
func (cs *ContractService) DeleteContractTypes(ctx context.Context, ids []uint64) error {
	return cs.contractRepo.DeleteContractTypes(ctx, ids)
}

// ListContractTypeCreators 获取合同类型创建者列表
func (cs *ContractService) ListContractTypeCreators(ctx context.Context) ([]string, error) {
	return cs.contractRepo.ListContractTypeCreators(ctx)
}

// GetContractTypeUsageCount 获取合同类型使用数量
func (cs *ContractService) GetContractTypeUsageCount(ctx context.Context, typeID uint64) (int64, error) {
	return cs.contractRepo.CountContractsByTypeID(ctx, typeID)
}
