package review

import (
	"context"
	"contract_review/app/internal/global"
	"contract_review/app/internal/middleware/redis"
	"contract_review/app/internal/session"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/cloudwego/eino-ext/components/model/arkbot"
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

// ReviewService 审阅服务
type ReviewService struct {
	reviewRepo       *ReviewRepo
	reviewResultRepo *ReviewResultRepo
	sessionRepo      *session.SessionRepo
	cache            *redis.RedisClient
	llm              *arkbot.ChatModel
	basePrompt       string            // 基础审阅提示词
	contractPrompts  map[string]string // 不同合同类型的提示词
}

// NewReviewService 创建审阅服务
func NewReviewService(
	reviewRepo *ReviewRepo,
	reviewResultRepo *ReviewResultRepo,
	sessionRepo *session.SessionRepo,
	cache *redis.RedisClient,
) *ReviewService {
	return &ReviewService{
		reviewRepo:       reviewRepo,
		reviewResultRepo: reviewResultRepo,
		sessionRepo:      sessionRepo,
		cache:            cache,
		contractPrompts:  make(map[string]string),
	}
}

// InitLLM 初始化LLM客户端
func (s *ReviewService) InitLLM(ctx context.Context) error {
	llm, err := arkbot.NewChatModel(ctx,
		&arkbot.Config{
			Model:  global.Config.LLMConfig.Model,
			APIKey: global.Config.LLMConfig.APIKey,
		})
	if err != nil {
		global.Log.Error("初始化LLM客户端失败", zap.Error(err))
		return err
	}
	s.llm = llm

	// 加载提示词模板
	if err := s.loadPromptTemplates(); err != nil {
		global.Log.Warn("加载提示词模板失败，将使用默认提示词", zap.Error(err))
	}

	return nil
}

// loadPromptTemplates 加载提示词模板
func (s *ReviewService) loadPromptTemplates() error {
	// 获取提示词文件目录
	_, filename, _, _ := runtime.Caller(0)
	promptDir := filepath.Join(filepath.Dir(filename), "..", "..", "prompts")

	// 加载基础提示词
	basePromptPath := filepath.Join(promptDir, "contract_reviewer_prompt_unified.txt")
	if data, err := os.ReadFile(basePromptPath); err == nil {
		s.basePrompt = string(data)
	} else {
		s.basePrompt = defaultBasePrompt
	}

	// 加载不同合同类型的提示词
	contractTypePrompts := map[string]string{
		"基建类合同": "contract_reviewer_prompt_build.txt",
		"货物类合同": "contract_reviewer_prompt_sales.txt",
		"服务类合同": "contract_reviewer_prompt_service.txt",
		"通用":    "contract_reviewer_prompt_base.txt",
	}

	for contractType, filename := range contractTypePrompts {
		promptPath := filepath.Join(promptDir, filename)
		if data, err := os.ReadFile(promptPath); err == nil {
			s.contractPrompts[contractType] = string(data)
		}
	}

	return nil
}

// ============ Task CRUD Operations ============

// CreateReviewTask 创建审阅任务
func (s *ReviewService) CreateReviewTask(ctx context.Context, userID uint64, req *CreateReviewTaskRequest) (*ReviewTask, error) {
	// 获取Session信息
	sessionInfo, err := s.sessionRepo.GetByID(ctx, uint(req.SessionID))
	if err != nil {
		return nil, fmt.Errorf("获取会话信息失败: %w", err)
	}
	if sessionInfo == nil {
		return nil, fmt.Errorf("会话不存在")
	}

	// 检查是否已存在任务，存在则删除旧任务
	existingTask, err := s.reviewRepo.GetBySessionID(ctx, req.SessionID)
	if err != nil {
		return nil, fmt.Errorf("检查现有任务失败: %w", err)
	}
	if existingTask != nil {
		// 删除旧任务及其结果
		if err := s.reviewResultRepo.DeleteByTaskID(ctx, existingTask.ID); err != nil {
			global.Log.Warn("删除旧审阅结果失败", zap.Error(err))
		}
		if err := s.reviewRepo.Delete(ctx, existingTask.ID); err != nil {
			global.Log.Warn("删除旧审阅任务失败", zap.Error(err))
		}
	}

	// 创建新任务
	task := &ReviewTask{
		SessionID:    req.SessionID,
		FileID:       uint64(sessionInfo.FileID),
		UserID:       userID,
		Stance:       req.Stance,
		Intensity:    req.Intensity,
		ContractType: req.ContractType,
		Description:  req.Description,
		Status:       "pending",
	}

	if err := s.reviewRepo.Create(ctx, task); err != nil {
		return nil, fmt.Errorf("创建审阅任务失败: %w", err)
	}

	return task, nil
}

// GetReviewTask 获取审阅任务
func (s *ReviewService) GetReviewTask(ctx context.Context, sessionID uint64) (*ReviewTask, error) {
	return s.reviewRepo.GetBySessionID(ctx, sessionID)
}

// GetReviewTaskByID 根据ID获取审阅任务
func (s *ReviewService) GetReviewTaskByID(ctx context.Context, id uint64) (*ReviewTask, error) {
	return s.reviewRepo.GetByID(ctx, id)
}

// ListUserReviewTasks 获取用户的审阅任务列表
func (s *ReviewService) ListUserReviewTasks(ctx context.Context, userID uint64, page, pageSize int) ([]ReviewTask, int64, error) {
	offset := (page - 1) * pageSize
	return s.reviewRepo.ListByUserID(ctx, userID, offset, pageSize)
}

// UpdateTaskStatus 更新任务状态
func (s *ReviewService) UpdateTaskStatus(ctx context.Context, taskID uint64, status string) error {
	return s.reviewRepo.UpdateStatus(ctx, taskID, status)
}

// DeleteReviewTask 删除审阅任务
func (s *ReviewService) DeleteReviewTask(ctx context.Context, taskID uint64) error {
	// 先删除关联的审阅结果
	if err := s.reviewResultRepo.DeleteByTaskID(ctx, taskID); err != nil {
		global.Log.Warn("删除审阅结果失败", zap.Error(err))
	}
	return s.reviewRepo.Delete(ctx, taskID)
}

// ============ Result CRUD Operations ============

// CreateReviewResult 创建审阅结果
func (s *ReviewService) CreateReviewResult(ctx context.Context, result *ReviewResult) error {
	return s.reviewResultRepo.Create(ctx, result)
}

// GetReviewResults 获取审阅结果列表
func (s *ReviewService) GetReviewResults(ctx context.Context, taskID uint64) ([]ReviewResult, error) {
	return s.reviewResultRepo.GetByTaskID(ctx, taskID)
}

// GetReviewResultsBySessionID 根据会话ID获取审阅结果
func (s *ReviewService) GetReviewResultsBySessionID(ctx context.Context, sessionID uint64) ([]ReviewResult, error) {
	return s.reviewResultRepo.GetBySessionID(ctx, sessionID)
}

// DeleteReviewResults 删除审阅结果
func (s *ReviewService) DeleteReviewResults(ctx context.Context, taskID uint64) error {
	return s.reviewResultRepo.DeleteByTaskID(ctx, taskID)
}

// ============ Contract Review Core Logic ============

// ReviewContract 审阅合同分块
func (s *ReviewService) ReviewContract(
	ctx context.Context,
	chunkText string,
	stance string,
	intensity string,
	chunkContext string,
	contractType string,
) ([]ModificationItem, error) {
	if s.llm == nil {
		if err := s.InitLLM(ctx); err != nil {
			return nil, fmt.Errorf("初始化LLM失败: %w", err)
		}
	}

	// 构建审阅提示词
	reviewPrompt := s.buildReviewPrompt(chunkText, stance, intensity, chunkContext, contractType)

	// 创建消息
	template := prompt.FromMessages(schema.GoTemplate,
		&schema.Message{
			Role:    schema.System,
			Content: "你是一个专业的合同审查律师，请严格按照提示词要求进行合同审阅。",
		},
		&schema.Message{
			Role:    schema.User,
			Content: reviewPrompt,
		},
	)

	messages, err := template.Format(ctx, map[string]any{})
	if err != nil {
		global.Log.Error("格式化提示词失败", zap.Error(err))
		return nil, fmt.Errorf("格式化提示词失败: %w", err)
	}

	// 调用LLM
	response, err := s.llm.Generate(ctx, messages)
	if err != nil {
		global.Log.Error("LLM调用失败", zap.Error(err))
		return nil, fmt.Errorf("LLM调用失败: %w", err)
	}

	// 解析审阅结果
	modifications := s.parseReviewResult(response.Content)

	return modifications, nil
}

// buildReviewPrompt 构建审阅提示词
func (s *ReviewService) buildReviewPrompt(chunkText, stance, intensity, chunkContext, contractType string) string {
	// 获取合同类型对应的提示词
	contractTypePrompt := s.contractPrompts[contractType]
	if contractTypePrompt == "" {
		contractTypePrompt = s.contractPrompts["通用"]
	}

	// 上下文信息
	contextInfo := ""
	if chunkContext != "" {
		contextInfo = fmt.Sprintf(`
## 审阅上下文
%s

请结合上述上下文信息，确保审阅的连续性和一致性。
`, chunkContext)
	}

	// 强度描述
	intensityDescMap := map[string]string{
		"严格": "请进行严格审阅，覆盖全部审查维度，识别所有潜在法律与履约风险，包括措辞模糊、权利不对等等细节问题。",
		"标准": "请进行标准审阅，重点关注7个核心风险领域（交付、质量、违约、知识产权、保密、争议解决、生效要件）。",
		"宽松": "请进行宽松审阅，仅指出重大法律风险（如无效免责、管辖不明、主体缺失、违约无责等），忽略一般性模糊表述。",
	}
	intensityDesc := intensityDescMap[intensity]
	if intensityDesc == "" {
		intensityDesc = intensityDescMap["标准"]
	}

	// 使用基础提示词模板
	basePrompt := s.basePrompt
	if basePrompt == "" {
		basePrompt = defaultBasePrompt
	}
	basePrompt = strings.ReplaceAll(basePrompt, "{stance}", stance)

	// 构建完整提示词
	return fmt.Sprintf(`%s

## 任务要求
- 用户立场：%s
- 审查强度：%s
- 合同类型：%s
- 审阅要点：%s
- %s

%s

## 合同内容
%s

请严格按照上述格式要求输出审阅结果。每个修改点必须包含：
1. 【修改点X】
2. 【原文】（100字以内）
3. 【风险分析】
4. 【风险等级】
5. 【修改后的内容】
6. 【修改理由】
7. 【风险类型】（15字内）
确保分析专业、建议可行、格式规范。
`, basePrompt, stance, intensity, contractType, contractTypePrompt, intensityDesc, contextInfo, chunkText)
}

// parseReviewResult 解析审阅结果
func (s *ReviewService) parseReviewResult(reviewResult string) []ModificationItem {
	var modifications []ModificationItem

	global.Log.Info("开始解析审阅结果", zap.Int("contentLength", len(reviewResult)))

	// 多种格式的匹配模式
	patterns := []string{
		// 模式1: 【修改点X】格式
		`【修改点(\d+)】[^\n]*\n(.*?)(?=【修改点\d+】|$)`,
		// 模式2: ## 修改点X 格式
		`##\s*修改点(\d+)[^\n]*\n(.*?)(?=##\s*修改点\d+|$)`,
		// 模式3: ### 修改点X 格式
		`###\s*修改点(\d+)[^\n]*\n(.*?)(?=###\s*修改点\d+|$)`,
		// 模式4: 数字编号格式
		`(\d+)\.\s*修改点[^\n]*\n(.*?)(?=\d+\.\s*修改点|$)`,
		// 模式5: 通用标题格式
		`[#]*\s*修改点(\d+)[^\n]*\n(.*?)(?=[#]*\s*修改点\d+|$)`,
	}

	var matches [][]string
	for _, pattern := range patterns {
		re := regexp.MustCompile(`(?s)` + pattern)
		matches = re.FindAllStringSubmatch(reviewResult, -1)
		if len(matches) > 0 {
			global.Log.Info("使用模式匹配到修改点", zap.Int("count", len(matches)))
			break
		}
	}

	if len(matches) == 0 {
		global.Log.Warn("未匹配到标准格式，尝试宽松匹配")
		return s.parseLooseFormat(reviewResult)
	}

	// 处理匹配到的修改点
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		pointNum := match[1]
		content := strings.TrimSpace(match[2])

		modification := ModificationItem{
			Position: fmt.Sprintf("修改点%s", pointNum),
		}

		// 提取原文
		modification.OriginalContent = s.extractField(content, []string{
			`【?\s*原文\s*】?[：:\s]*([\s\S]*?)(?=【?\s*风险分析\s*】?|【?\s*风险等级\s*】?|【?\s*修改后的内容\s*】?|$)`,
			`原文[：:]\s*(.*?)(?=风险|修改)`,
		}, "未找到原文内容")

		// 提取风险分析
		modification.RiskAnalysis = s.extractField(content, []string{
			`【?\s*风险分析\s*】?[：:\s]*([\s\S]*?)(?=【?\s*风险等级\s*】?|【?\s*修改后的内容\s*】?|$)`,
			`风险分析?[：:]\s*(.*?)(?=风险等级|修改)`,
		}, "未找到风险分析")

		// 提取风险等级
		modification.RiskLevel = s.extractField(content, []string{
			`【风险等级】[：:\s]*([\s\S]*?)(?=【修改后的内容】|【修改理由】|$)`,
			`风险等级[：:\s]*([\s\S]*?)(?=【修改后的内容】|修改|【修改理由】|$)`,
		}, "未知")

		// 提取修改后内容
		modification.SuggestedContent = s.extractField(content, []string{
			`【?\s*修改后的内容\s*】?[：:\s]*([\s\S]*?)(?=【?\s*修改理由\s*】?|$)`,
			`修改后?[：:]\s*(.*?)(?=\n|$)`,
		}, "未找到修改建议")

		// 提取修改理由
		modification.Reason = s.extractField(content, []string{
			`【修改理由】[：:\s]*([\s\S]*?)(?=\n*【|$)`,
			`修改理由[：:\s]*([\s\S]*?)(?=\n*【|$)`,
		}, "未找到修改理由")

		// 提取风险类型
		modification.RiskType = s.extractField(content, []string{
			`【风险类型】[：:\s]*([\s\S]*?)(?=\n*【|$)`,
			`风险类型[：:\s]*([\s\S]*?)(?=\n*【|$)`,
		}, "未找到风险类型")

		modifications = append(modifications, modification)
	}

	if len(modifications) == 0 {
		global.Log.Warn("未找到任何修改点，返回空列表")
	} else {
		global.Log.Info("解析完成", zap.Int("modificationCount", len(modifications)))
	}

	return modifications
}

// extractField 提取字段内容
func (s *ReviewService) extractField(content string, patterns []string, defaultValue string) string {
	for _, pattern := range patterns {
		re := regexp.MustCompile(`(?s)` + pattern)
		match := re.FindStringSubmatch(content)
		if len(match) > 1 {
			result := strings.TrimSpace(match[1])
			if result != "" {
				return result
			}
		}
	}
	return defaultValue
}

// parseLooseFormat 宽松格式解析
func (s *ReviewService) parseLooseFormat(reviewResult string) []ModificationItem {
	var modifications []ModificationItem

	// 按段落分割
	sections := regexp.MustCompile(`\n\s*\n`).Split(reviewResult, -1)
	var currentMod *ModificationItem

	for _, section := range sections {
		section = strings.TrimSpace(section)
		if section == "" {
			continue
		}

		// 检查是否是新的修改点
		modPointRe := regexp.MustCompile(`(?i)修改点\s*(\d+)`)
		if modPointRe.MatchString(section) {
			if currentMod != nil {
				modifications = append(modifications, *currentMod)
			}
			match := modPointRe.FindStringSubmatch(section)
			currentMod = &ModificationItem{
				Position: fmt.Sprintf("修改点%s", match[1]),
			}
		}

		// 提取内容
		if currentMod != nil {
			if regexp.MustCompile(`(?i)原文`).MatchString(section) && currentMod.OriginalContent == "" {
				currentMod.OriginalContent = section
			} else if regexp.MustCompile(`(?i)风险分析|风险`).MatchString(section) && currentMod.RiskAnalysis == "" {
				currentMod.RiskAnalysis = section
			} else if regexp.MustCompile(`(?i)修改`).MatchString(section) && currentMod.SuggestedContent == "" {
				currentMod.SuggestedContent = section
			}
		}
	}

	if currentMod != nil {
		modifications = append(modifications, *currentMod)
	}

	return modifications
}

// ProcessContractReview 处理合同审阅（并发处理分块）
func (s *ReviewService) ProcessContractReview(
	ctx context.Context,
	task *ReviewTask,
	contractContent string,
	maxConcurrent int,
	resultChan chan<- ChunkResult,
) error {
	// 分割合同内容
	chunks := splitTextByLength(contractContent, 4000)
	totalChunks := len(chunks)

	global.Log.Info("开始处理合同审阅",
		zap.Int("totalChunks", totalChunks),
		zap.Int("maxConcurrent", maxConcurrent))

	// 更新任务状态为处理中
	if err := s.UpdateTaskStatus(ctx, task.ID, "processing"); err != nil {
		return err
	}

	// 创建信号量控制并发
	semaphore := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

	// 存储结果的通道
	internalResults := make(chan ChunkResult, totalChunks)

	// 并发处理每个分块
	for idx, chunk := range chunks {
		wg.Add(1)
		go func(index int, content string) {
			defer wg.Done()

			// 获取信号量
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// 检查上下文是否已取消
			select {
			case <-ctx.Done():
				internalResults <- ChunkResult{
					Index: index,
					Error: ctx.Err(),
				}
				return
			default:
			}

			// 构建上下文信息
			chunkContext := fmt.Sprintf("这是第 %d 个分块，共 %d 个。", index+1, totalChunks)

			// 调用审阅
			mods, err := s.ReviewContract(ctx, content, task.Stance, task.Intensity, chunkContext, task.ContractType)
			internalResults <- ChunkResult{
				Index:         index,
				Modifications: mods,
				Error:         err,
			}
		}(idx, chunk)
	}

	// 等待所有任务完成并关闭通道
	go func() {
		wg.Wait()
		close(internalResults)
	}()

	// 收集结果并按顺序发送
	receivedResults := make(map[int]ChunkResult)
	nextToEmit := 0

	for result := range internalResults {
		receivedResults[result.Index] = result

		// 按顺序发送已完成的结果
		for {
			if res, ok := receivedResults[nextToEmit]; ok {
				resultChan <- res
				delete(receivedResults, nextToEmit)
				nextToEmit++
			} else {
				break
			}
		}
	}

	return nil
}

// splitTextByLength 按长度分割文本
func splitTextByLength(text string, maxLength int) []string {
	var chunks []string
	runes := []rune(text)

	for i := 0; i < len(runes); i += maxLength {
		end := i + maxLength
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[i:end]))
	}

	return chunks
}

// CalculateOverallRisk 计算整体风险等级
func (s *ReviewService) CalculateOverallRisk(totalIssues int) string {
	if totalIssues > 10 {
		return "高"
	} else if totalIssues > 5 {
		return "中"
	}
	return "低"
}

// 默认基础提示词
const defaultBasePrompt = `你是一名专业的法律顾问，专注于合同审查与风险分析。请站在{stance}的角度，对以下合同内容进行全面审阅。

你需要：
1. 识别对{stance}不利的条款
2. 分析每个风险点的严重程度
3. 提供具体的修改建议

输出格式要求：
对于每个发现的风险点，请按以下格式输出：

【修改点X】
【原文】原始条款内容（不超过100字）
【风险分析】详细分析该条款对{stance}的潜在风险
【风险等级】高/中/低
【修改后的内容】建议修改后的条款内容
【修改理由】说明为什么需要这样修改
【风险类型】风险分类（不超过15字）
`
