package review

import (
	"context"
	"contract_review/app/internal/agent"
	"contract_review/app/internal/contract"
	"contract_review/app/internal/global"
	"contract_review/app/internal/llm"
	"contract_review/app/internal/middleware/redis"
	"contract_review/app/internal/rag"
	"contract_review/app/internal/tools"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	fmodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// SessionQuerier 查询会话的最小接口，避免 review ↔ session 循环引用
type SessionQuerier interface {
	SessionExists(ctx context.Context, sessionID uint64) (fileID uint64, err error)
}

// ReviewService 审阅服务
type ReviewService struct {
	reviewRepo        *ReviewRepo
	reviewResultRepo  *ReviewResultRepo
	db                *gorm.DB
	cache             *redis.RedisClient
	llm               fmodel.BaseChatModel
	orchestrator      *agent.ReviewOrchestrator
	basePrompt        string
	contractPrompts   map[string]string
	vectorComponentMu sync.Mutex
	vectorStore       rag.VectorStore
	vectorEmbedder    rag.EmbeddingModel
	vectorConfigKey   string
	vectorIndexMu     sync.Mutex
	vectorIndexHashes map[string]string
}

// NewReviewService 创建审阅服务
func NewReviewService(
	reviewRepo *ReviewRepo,
	reviewResultRepo *ReviewResultRepo,
	db *gorm.DB,
	cache *redis.RedisClient,
) *ReviewService {
	return &ReviewService{
		reviewRepo:        reviewRepo,
		reviewResultRepo:  reviewResultRepo,
		db:                db,
		cache:             cache,
		contractPrompts:   make(map[string]string),
		vectorIndexHashes: make(map[string]string),
	}
}

// InitLLM 初始化LLM客户端
func (s *ReviewService) InitLLM(ctx context.Context) error {
	chatModel, err := llm.NewChatModel(ctx)
	if err != nil {
		global.Log.Error("初始化LLM客户端失败", zap.Error(err))
		return err
	}
	s.llm = chatModel

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
func (s *ReviewService) CreateReviewTask(ctx context.Context, account string, req *CreateReviewTaskRequest) (*ReviewTask, error) {
	// 验证会话存在并获取 FileID
	var sessRecord struct {
		ID     uint64 `gorm:"column:id"`
		FileID uint64 `gorm:"column:file_id"`
		UserID uint64 `gorm:"column:user_id"`
	}
	if err := s.db.WithContext(ctx).Table("sessions").
		Where("id = ?", req.SessionID).First(&sessRecord).Error; err != nil {
		return nil, fmt.Errorf("获取会话信息失败: %w", err)
	}
	var userRecord struct {
		ID uint64 `gorm:"column:id"`
	}
	if err := s.db.WithContext(ctx).Table("users").Select("id").Where("account = ?", account).First(&userRecord).Error; err != nil {
		return nil, fmt.Errorf("获取用户信息失败: %w", err)
	}
	if sessRecord.UserID != userRecord.ID {
		return nil, fmt.Errorf("无权访问该会话")
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
		FileID:       sessRecord.FileID,
		UserID:       userRecord.ID,
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

// UpdateReviewResult 更新审阅结果
func (s *ReviewService) UpdateReviewResult(ctx context.Context, result *ReviewResult) error {
	return s.reviewResultRepo.Update(ctx, result)
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

// ============ Agent 模式审阅 (新架构) ============

// InitOrchestrator 初始化 Agent 编排器
func (s *ReviewService) InitOrchestrator(ctx context.Context) error {
	if s.llm == nil {
		if err := s.InitLLM(ctx); err != nil {
			return fmt.Errorf("初始化 LLM 失败: %w", err)
		}
	}

	llmGenerate := func(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
		return s.llm.Generate(ctx, messages)
	}

	keywordIndex := rag.NewSimpleKeywordIndex()
	retrieverConfig := rag.DefaultRetrieverConfig()
	retrieverConfig.FinalTopK = 8
	// RRF 融合后的分数天然较小，仅关键词通道不再用固定阈值过滤，避免中文命中被误删。
	retrieverConfig.MinRelevance = 0

	var knowledgeChunks []rag.Chunk
	if global.DB != nil {
		chunks, loadErr := rag.LoadKnowledgeChunksFromDB(ctx, global.DB)
		if loadErr != nil {
			global.Log.Warn("加载审阅知识库失败，rag_search 将无检索结果", zap.Error(loadErr))
		} else if len(chunks) > 0 {
			knowledgeChunks = chunks
			if err := keywordIndex.Index(chunks); err != nil {
				global.Log.Warn("审阅知识库入索引失败", zap.Error(err))
			} else {
				global.Log.Info("审阅知识库已加载到关键词索引", zap.Int("chunks", len(chunks)))
			}
		} else {
			global.Log.Warn("审阅知识库无已索引分块：请在库表 review_knowledge_* 写入数据并置 status=indexed，或执行 docs/sql/review_knowledge.sql")
		}
	} else {
		global.Log.Warn("global.DB 未初始化，跳过审阅知识库加载")
	}
	if len(keywordIndex.SearchableChunks()) == 0 {
		knowledgeChunks = defaultReviewKnowledgeChunks()
		if err := keywordIndex.Index(knowledgeChunks); err != nil {
			global.Log.Warn("内置审阅知识入索引失败", zap.Error(err))
		} else {
			global.Log.Warn("使用内置审阅知识兜底；建议执行 docs/sql/review_knowledge.sql 写入数据库知识库")
		}
	}

	vectorStore, embedder := s.initVectorRetrieval(ctx, knowledgeChunks)
	ragRetriever := rag.NewRAGRetriever(vectorStore, keywordIndex, embedder, retrieverConfig)

	ragSearchTool := tools.NewRAGSearchTool(ragRetriever)
	contractContextTool := tools.NewContractContextTool(agent.ContractMeta{}, "")

	suggestionTools := []agent.Tool{ragSearchTool, contractContextTool}

	orchConfig := agent.DefaultOrchestratorConfig()
	s.orchestrator = agent.NewReviewOrchestrator(llmGenerate, ragRetriever, suggestionTools, orchConfig)

	global.Log.Info("Agent 编排器初始化完成")
	return nil
}

func (s *ReviewService) initVectorRetrieval(ctx context.Context, chunks []rag.Chunk) (rag.VectorStore, rag.EmbeddingModel) {
	if global.Config == nil || global.Config.Vector == nil || !global.Config.Vector.Enabled {
		return nil, nil
	}
	vectorCfg := global.Config.Vector
	if vectorCfg.Embedding == nil || !vectorCfg.Embedding.Enabled {
		global.Log.Warn("向量检索已开启，但 embedding 配置未启用，跳过 Milvus 向量检索")
		return nil, nil
	}
	if vectorCfg.Milvus == nil || !vectorCfg.Milvus.Enabled {
		global.Log.Warn("向量检索已开启，但 Milvus 配置未启用，跳过 Milvus 向量检索")
		return nil, nil
	}

	timeout := time.Duration(vectorCfg.Embedding.TimeoutSeconds) * time.Second
	embeddingAPIBase := vectorCfg.Embedding.APIBase
	embeddingAPIKey := vectorCfg.Embedding.APIKey
	if embeddingAPIBase == "" && global.Config.LLMConfig != nil {
		embeddingAPIBase = global.Config.LLMConfig.APIBase
	}
	if embeddingAPIKey == "" && global.Config.LLMConfig != nil {
		embeddingAPIKey = global.Config.LLMConfig.APIKey
	}
	if strings.TrimSpace(vectorCfg.Embedding.Model) == "" || strings.TrimSpace(embeddingAPIBase) == "" {
		global.Log.Warn("向量检索已开启，但 embedding model/api_url 未配置，跳过 Milvus 向量检索")
		return nil, nil
	}
	embedder := rag.NewOpenAIEmbeddingModel(
		vectorCfg.Embedding.Model,
		embeddingAPIBase,
		embeddingAPIKey,
		timeout,
	)
	dimension := vectorCfg.Milvus.Dimension
	if dimension <= 0 {
		dimension = vectorCfg.Embedding.Dimension
	}
	if dimension <= 0 {
		dimension = 1536
	}
	configKey := strings.Join([]string{
		vectorCfg.Milvus.Address,
		vectorCfg.Milvus.DBName,
		vectorCfg.Milvus.Collection,
		fmt.Sprintf("%d", dimension),
		vectorCfg.Milvus.MetricType,
		vectorCfg.Embedding.Model,
		embeddingAPIBase,
	}, "|")

	s.vectorComponentMu.Lock()
	if s.vectorStore != nil && s.vectorEmbedder != nil && s.vectorConfigKey == configKey {
		store := s.vectorStore
		embedder := s.vectorEmbedder
		s.vectorComponentMu.Unlock()
		if err := s.indexKnowledgeChunksToVectorStore(ctx, chunks, store, embedder, dimension); err != nil {
			global.Log.Warn("同步审阅知识到 Milvus 失败，本次审阅仍使用关键词检索兜底", zap.Error(err))
		}
		return store, embedder
	}

	store, err := rag.NewMilvusVectorStore(ctx, rag.MilvusVectorStoreConfig{
		Address:    vectorCfg.Milvus.Address,
		Username:   vectorCfg.Milvus.Username,
		Password:   vectorCfg.Milvus.Password,
		DBName:     vectorCfg.Milvus.DBName,
		Collection: vectorCfg.Milvus.Collection,
		Dimension:  dimension,
		MetricType: vectorCfg.Milvus.MetricType,
		UseTLS:     vectorCfg.Milvus.UseTLS,
	})
	if err != nil {
		s.vectorComponentMu.Unlock()
		global.Log.Warn("初始化 Milvus 向量检索失败，将退回关键词检索", zap.Error(err))
		return nil, nil
	}

	if closer, ok := s.vectorStore.(interface{ Close() }); ok && s.vectorStore != store {
		closer.Close()
	}
	s.vectorStore = store
	s.vectorEmbedder = embedder
	s.vectorConfigKey = configKey
	s.vectorIndexHashes = make(map[string]string)
	s.vectorComponentMu.Unlock()

	if err := s.indexKnowledgeChunksToVectorStore(ctx, chunks, store, embedder, dimension); err != nil {
		global.Log.Warn("同步审阅知识到 Milvus 失败，本次审阅仍使用关键词检索兜底", zap.Error(err))
	}
	return store, embedder
}

func (s *ReviewService) indexKnowledgeChunksToVectorStore(ctx context.Context, chunks []rag.Chunk, store rag.VectorStore, embedder rag.EmbeddingModel, dimension int) error {
	if len(chunks) == 0 || store == nil || embedder == nil {
		return nil
	}

	s.vectorIndexMu.Lock()
	pending := make([]rag.Chunk, 0, len(chunks))
	for _, chunk := range chunks {
		hash := knowledgeChunkHash(chunk)
		if s.vectorIndexHashes[chunk.ID] == hash {
			continue
		}
		pending = append(pending, chunk)
	}
	s.vectorIndexMu.Unlock()

	if len(pending) == 0 {
		return nil
	}

	texts := make([]string, 0, len(pending))
	for _, chunk := range pending {
		texts = append(texts, chunk.Content)
	}
	embeddings, err := embedder.EmbedBatch(texts)
	if err != nil {
		return err
	}
	indexable := make([]rag.Chunk, 0, len(pending))
	for i := range pending {
		if i >= len(embeddings) || len(embeddings[i]) != dimension {
			continue
		}
		pending[i].Embedding = embeddings[i]
		indexable = append(indexable, pending[i])
	}
	if len(indexable) == 0 {
		return nil
	}
	if err := store.Insert(indexable); err != nil {
		return err
	}

	s.vectorIndexMu.Lock()
	for _, chunk := range indexable {
		s.vectorIndexHashes[chunk.ID] = knowledgeChunkHash(chunk)
	}
	s.vectorIndexMu.Unlock()

	global.Log.Info("审阅知识库已同步到 Milvus",
		zap.Int("chunks", len(indexable)),
		zap.Int("dimension", dimension))
	return nil
}

func knowledgeChunkHash(chunk rag.Chunk) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(chunk.ID))
	_, _ = h.Write([]byte(chunk.Content))
	for _, key := range []string{"title", "category", "sub_category", "source", "vector_id"} {
		_, _ = h.Write([]byte(chunk.Metadata[key]))
	}
	return fmt.Sprintf("%x", h.Sum64())
}

func defaultReviewKnowledgeChunks() []rag.Chunk {
	items := []struct {
		id       string
		category string
		subCat   string
		source   string
		content  string
	}{
		{"builtin-1", "规范", "通用", "内置审阅指引", "合同应明确服务范围、交付成果、验收标准、付款节点、违约责任、解除条件、知识产权归属、保密义务、争议解决和管辖。缺少上述核心条款或表述不清，通常构成履约争议风险。"},
		{"builtin-2", "规范", "服务类合同", "内置服务合同审阅指引", "服务类合同应明确服务内容、服务期限、质量标准、验收流程、整改期限、费用支付条件、成果知识产权归属和逾期违约责任。仅列附件或笼统描述服务内容，容易导致验收和付款争议。"},
		{"builtin-3", "法规", "通用", "《中华人民共和国民法典》合同编通用规则", "当事人应当按照约定全面履行自己的义务。一方不履行合同义务或者履行不符合约定的，应承担继续履行、采取补救措施或者赔偿损失等违约责任。"},
		{"builtin-4", "规范", "通用", "内置违约责任审阅指引", "违约责任应与主要义务对应，至少覆盖逾期付款、逾期交付、质量不合格、擅自解除、保密违约和知识产权侵权等场景。仅约定一方责任或责任过轻，可能对审查立场不利。"},
		{"builtin-5", "规范", "通用", "内置争议解决审阅指引", "争议解决条款应明确适用法律、管辖法院或仲裁机构。管辖地、仲裁机构名称不明或同时约定诉讼与仲裁，可能导致争议解决成本和效力风险。"},
	}
	chunks := make([]rag.Chunk, 0, len(items))
	for i, item := range items {
		chunks = append(chunks, rag.Chunk{
			ID:      item.id,
			DocID:   "builtin",
			Content: item.content,
			Metadata: map[string]string{
				"title":        item.source,
				"category":     item.category,
				"sub_category": item.subCat,
				"source":       item.source,
				"chunk_index":  fmt.Sprintf("%d", i),
			},
		})
	}
	return chunks
}

// AgentReviewContract 使用 Agent 架构审阅合同
// 替代原有的 ProcessContractReview 暴力分块 + 并发 LLM 调用
func (s *ReviewService) AgentReviewContract(
	ctx context.Context,
	task *ReviewTask,
	contractInfo *contract.Contract,
	contractContent string,
	resultChan chan<- ChunkResult,
) error {
	// 每次审阅前重新加载知识库，确保刚在 setting/risk 配置的风险点能立即进入 RAG。
	if err := s.InitOrchestrator(ctx); err != nil {
		return fmt.Errorf("初始化 Agent 编排器失败: %w", err)
	}

	meta := agent.ContractMeta{
		ContractType: task.ContractType,
		Stance:       task.Stance,
		Intensity:    task.Intensity,
	}
	if contractInfo != nil {
		meta.PartyA = contractInfo.PartyA
		meta.PartyB = contractInfo.PartyB
		if contractInfo.Amount > 0 {
			meta.Amount = fmt.Sprintf("%.2f", contractInfo.Amount)
		}
		if meta.ContractType == "" && contractInfo.ContractType != nil {
			meta.ContractType = contractInfo.ContractType.Name
		}
	}

	if err := s.UpdateTaskStatus(ctx, task.ID, "processing"); err != nil {
		return err
	}

	s.orchestrator.SetProgressCallback(func(event agent.ProgressEvent) {
		global.Log.Info("Agent 进度",
			zap.String("phase", event.Phase),
			zap.String("agent", event.Agent),
			zap.String("status", event.Status),
			zap.String("message", event.Message),
			zap.Float64("progress", event.Progress))

		resultChan <- ChunkResult{
			Progress: &ReviewSSEProgressData{
				Phase:     event.Phase,
				Agent:     event.Agent,
				Status:    event.Status,
				Message:   event.Message,
				Progress:  event.Progress,
				Timestamp: event.Timestamp.Format("2006-01-02 15:04:05"),
				Data:      sanitizeProgressData(event.Data),
			},
		}
	})
	s.orchestrator.SetFindingCallback(func(finding agent.RiskFinding) {
		suggestedContent := finding.SuggestedText
		reason := finding.SuggestionReason
		if suggestedContent == "" {
			suggestedContent = "修改建议生成中..."
		}
		if reason == "" {
			reason = "AI 已识别风险，正在生成可执行修改建议。"
		}
		resultChan <- ChunkResult{
			Modifications: []ModificationItem{
				modificationFromFinding(finding, suggestedContent, reason),
			},
		}
	})

	report, err := s.orchestrator.ReviewContract(ctx, contractContent, meta)
	if err != nil {
		global.Log.Error("Agent 审阅失败", zap.Error(err))
		return fmt.Errorf("Agent 审阅失败: %w", err)
	}

	for i, finding := range report.Findings {
		var suggestedContent, reason string
		for _, sug := range report.Suggestions {
			if sug.RiskFindingID == finding.FindingID || sug.RiskFindingID == finding.ClauseID {
				suggestedContent = sug.SuggestedText
				reason = sug.Reason
				if sug.LegalReference != "" {
					reason += "\n法律依据: " + sug.LegalReference
				}
				break
			}
		}

		if suggestedContent == "" && len(report.Suggestions) > i {
			suggestedContent = report.Suggestions[i].SuggestedText
			reason = report.Suggestions[i].Reason
		}

		mod := modificationFromFinding(finding, suggestedContent, reason)

		resultChan <- ChunkResult{
			Index:         i,
			Modifications: []ModificationItem{mod},
		}
	}

	return nil
}

func sanitizeProgressData(data interface{}) interface{} {
	switch value := data.(type) {
	case nil:
		return nil
	case *agent.QualityEvaluation:
		if value == nil {
			return nil
		}
		return map[string]interface{}{
			"overall_score": value.OverallScore,
			"critical_gaps": value.CriticalGaps,
			"should_retry":  value.ShouldRetry,
		}
	case map[string]interface{}:
		return value
	case map[string]int:
		out := make(map[string]interface{}, len(value))
		for k, v := range value {
			out[k] = v
		}
		return out
	case []agent.Clause:
		return map[string]interface{}{
			"clause_count": len(value),
		}
	case *agent.ReviewReport:
		if value == nil {
			return nil
		}
		return map[string]interface{}{
			"clause_count":     len(value.Clauses),
			"finding_count":    len(value.Findings),
			"suggestion_count": len(value.Suggestions),
			"quality_score":    value.QualityScore,
			"overall_risk":     value.OverallRisk,
		}
	default:
		return nil
	}
}

func modificationFromFinding(finding agent.RiskFinding, suggestedContent, reason string) ModificationItem {
	return ModificationItem{
		Position:         finding.ClauseID,
		OriginalContent:  finding.OriginalText,
		RiskAnalysis:     buildRiskAnalysis(finding),
		RiskLevel:        finding.RiskLevel,
		SuggestedContent: suggestedContent,
		Reason:           reason,
		RiskType:         finding.RiskType,
		StableKey:        findingStableKey(finding),
	}
}

func buildRiskAnalysis(finding agent.RiskFinding) string {
	var sb strings.Builder
	sb.WriteString(finding.RiskDescription)
	if len(finding.CandidateIDs) > 0 {
		sb.WriteString("\n\n知识库候选: ")
		sb.WriteString(strings.Join(finding.CandidateIDs, "、"))
	}
	if len(finding.LegalBasis) == 0 {
		if finding.Verified {
			sb.WriteString("\n\n依据命中: 已通过规则验证，具体依据待补全。")
		} else {
			sb.WriteString("\n\n依据命中: 知识库未命中，已标记为待人工确认风险。")
		}
		return sb.String()
	}

	sb.WriteString("\n\n法律依据:\n")
	for _, lb := range finding.LegalBasis {
		sb.WriteString(fmt.Sprintf("%s %s: %s\n", lb.Source, lb.Article, lb.Content))
	}
	return sb.String()
}

func findingStableKey(finding agent.RiskFinding) string {
	if strings.TrimSpace(finding.FindingID) != "" {
		return strings.TrimSpace(finding.FindingID)
	}
	parts := []string{
		finding.ClauseID,
		finding.RiskType,
		truncateForKey(finding.OriginalText, 80),
		truncateForKey(finding.RiskDescription, 80),
	}
	return strings.Join(parts, "|")
}

func truncateForKey(value string, max int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= max {
		return string(runes)
	}
	return string(runes[:max])
}

// 默认基础提示词（保留用于旧模式兼容）
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
