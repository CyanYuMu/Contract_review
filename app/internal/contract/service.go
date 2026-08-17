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

// FileLoad 上传并解析合同文件（异步版本）。
// 1. 提取文档文本并持久化，审阅和QA不再重复提取。
// 2. 立即保存 contract（status="processing"），HTTP 响应不等待 LLM。
// 3. 后台 goroutine 调用 LLM 解析元数据（甲方/乙方/金额/合同类型），完成后更新。
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

	// 2. 获取文件类型和默认合同类型
	fileType := utils.GetFileType(filename)
	defaultType, err := cs.ensureDefaultContractType(ctx)
	if err != nil {
		global.Log.Error("获取默认合同类型失败", zap.Error(err))
		return Contract{}, errors.New("获取默认合同类型失败：" + err.Error())
	}

	// 3. 立即保存 contract（status=processing），不等待 LLM 元数据提取。
	contract := Contract{
		Account:    account,
		TypeID:     defaultType.ID,
		Title:      limitString(filename, 256),
		FilePath:   limitString(filePath, 512),
		FileType:   limitString(fileType, 16),
		RawText:    content,
		UploadTime: time.Now(),
		Status:     "processing",
		PartyA:     "",
		PartyB:     "",
		Amount:     0,
		IsAccepted: 0,
	}

	if err := cs.contractRepo.CreateContract(ctx, &contract); err != nil {
		global.Log.Error("保存合同记录失败", zap.Error(err))
		return Contract{}, errors.New("保存合同记录失败：" + err.Error())
	}

	global.Log.Info("合同记录已创建（元数据后台解析中）",
		zap.Uint64("contractID", contract.ID),
		zap.Int("textLength", len(content)))

	// 4. 后台异步解析元数据
	go cs.asyncParseMetadata(contract.ID, content)

	return contract, nil
}

// asyncParseMetadata 在后台 goroutine 中调用 LLM 解析合同元数据，
// 完成后更新 contracts 表的 party_a/party_b/amount/type_id/status 字段。
// 解析失败时标记 status="parse_failed"，合同仍可用于审阅。
func (cs *ContractService) asyncParseMetadata(contractID uint64, content string) {
	bgCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	global.Log.Info("后台开始解析合同元数据", zap.Uint64("contractID", contractID))

	raw, err := agent.LLMContractParse(bgCtx, content)
	if err != nil {
		global.Log.Error("后台LLM解析合同失败", zap.Uint64("contractID", contractID), zap.Error(err))
		_ = cs.contractRepo.UpdateContractByID(bgCtx, contractID, map[string]interface{}{
			"status": "parse_failed",
		})
		return
	}

	info, err := ParseContractInfo(raw)
	if err != nil {
		global.Log.Warn("后台解析合同信息JSON失败，使用默认值", zap.Uint64("contractID", contractID), zap.Error(err))
		info = ContractInfo{}
	}

	amount := ParseAmount(info.Amount)
	typeID, err := cs.ensureContractTypeID(bgCtx, info.Type)
	if err != nil || typeID == 0 {
		global.Log.Warn("后台获取合同类型失败，使用默认类型",
			zap.Uint64("contractID", contractID), zap.Error(err))
		if dt, dtErr := cs.ensureDefaultContractType(bgCtx); dtErr == nil && dt != nil {
			typeID = dt.ID
		}
	}

	updates := map[string]interface{}{
		"party_a": limitString(info.PartyA, 128),
		"party_b": limitString(info.PartyB, 128),
		"amount":  amount,
		"type_id": typeID,
		"status":  "uploaded",
	}

	if err := cs.contractRepo.UpdateContractByID(bgCtx, contractID, updates); err != nil {
		global.Log.Error("后台更新合同元数据失败", zap.Uint64("contractID", contractID), zap.Error(err))
		return
	}

	global.Log.Info("后台合同元数据解析完成",
		zap.Uint64("contractID", contractID),
		zap.String("partyA", info.PartyA),
		zap.String("partyB", info.PartyB),
		zap.Float64("amount", amount),
		zap.String("type", info.Type))
}

func (cs *ContractService) ensureContractTypeID(ctx context.Context, typeName string) (uint64, error) {
	name := classifyContractType(typeName)
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

// 标准合同分类（七大类 + 通用兜底），全量归一化映射使用。
var standardContractTypes = []string{
	"买卖合同", "服务合同", "劳动合同", "租赁合同",
	"借款合同", "合作合同", "知识产权合同", "通用",
}

// classifyContractType 将 LLM 解析出的自由文本合同类型归一化到七大类之一，
// 无法匹配时回退到 "其他"（由 ensureDefaultContractType 兜底）。
func classifyContractType(raw string) string {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "其他"
	}
	name = limitString(name, 64)

	// 关键词 -> 标准分类映射表（按优先级顺序匹配）
	rules := []struct {
		keywords []string
		standard string
	}{
		{[]string{"买卖", "采购", "购销", "销售", "经销", "供货", "货物", "商品", "购货"}, "买卖合同"},
		{[]string{"服务", "外包", "咨询", "运维", "维护", "技术服", "委托开发"}, "服务合同"},
		{[]string{"劳动", "劳务", "用工", "聘用", "雇佣", "竞业"}, "劳动合同"},
		{[]string{"租赁", "租", "承租", "出租"}, "租赁合同"},
		{[]string{"借款", "贷款", "融资", "民间借贷", "还款"}, "借款合同"},
		{[]string{"合作", "合伙", "合资", "联营", "共同"}, "合作合同"},
		{[]string{"知识产权", "专利", "商标", "著作权", "版权", "许可", "技术转让", "技术开发"}, "知识产权合同"},
		{[]string{"通用", "一般", "其他", "综合"}, "通用"},
	}

	lower := strings.ToLower(name)
	for _, rule := range rules {
		for _, kw := range rule.keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				return rule.standard
			}
		}
	}
	return "其他"
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

// GetContractByIDForAccount returns a contract only when account owns it.
func (cs *ContractService) GetContractByIDForAccount(ctx context.Context, account string, id uint64) (*Contract, error) {
	return cs.contractRepo.GetContractByIDForAccount(ctx, id, account)
}

// GetContractByIDWithTypeForAccount returns a typed contract only to its owner.
func (cs *ContractService) GetContractByIDWithTypeForAccount(ctx context.Context, account string, id uint64) (*Contract, error) {
	return cs.contractRepo.GetContractByIDWithTypeForAccount(ctx, id, account)
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
func (cs *ContractService) UpdateContractTypeID(ctx context.Context, account string, contractID uint64, typeID uint64) error {
	// 验证类型是否存在
	exists, err := cs.contractRepo.ExistsContractType(ctx, typeID)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("合同类型不存在")
	}

	return cs.contractRepo.UpdateContractByIDForAccount(ctx, contractID, account, map[string]interface{}{
		"type_id": typeID,
	})
}

// DeleteContract 删除合同
func (cs *ContractService) DeleteContract(ctx context.Context, account string, id uint64) error {
	// 获取合同信息用于删除文件
	contract, err := cs.contractRepo.GetContractByIDForAccount(ctx, id, account)
	if err != nil {
		return err
	}

	// 删除数据库记录
	if err := cs.contractRepo.DeleteContractForAccount(ctx, id, account); err != nil {
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
func (cs *ContractService) GetContractFilePath(ctx context.Context, account string, id uint64) (string, string, string, error) {
	contract, err := cs.contractRepo.GetContractByIDForAccount(ctx, id, account)
	if err != nil {
		return "", "", "", err
	}

	// 检查文件是否存在
	filePath := LocalFilePath(contract.FilePath)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return "", "", "", errors.New("文件不存在")
	}

	return filePath, contract.Title, contract.FileType, nil
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
