package review

import (
	"context"
	"contract_review/app/internal/agent"
	"contract_review/app/internal/contract"
	"contract_review/app/internal/gateway"
	"contract_review/app/internal/global"
	"contract_review/app/internal/knowledge"
	"contract_review/app/internal/llm"
	"contract_review/app/internal/middleware/redis"
	"contract_review/app/internal/rag"
	"contract_review/app/internal/tools"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	fmodel "github.com/cloudwego/eino/components/model"
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
	reviewRepo         *ReviewRepo
	reviewResultRepo   *ReviewResultRepo
	db                 *gorm.DB
	cache              *redis.RedisClient
	llm                fmodel.BaseChatModel
	orchestrator       *agent.ReviewOrchestrator
	basePrompt         string
	contractPrompts    map[string]string
	vectorComponentMu  sync.Mutex
	vectorStore        rag.VectorStore
	vectorEmbedder     rag.EmbeddingModel
	vectorConfigKey    string
	vectorIndexMu      sync.Mutex
	vectorIndexHashes  map[string]string
	orchMu             sync.Mutex
	knowledgeSignature string
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

	// 加载不同合同类型的提示词（七大类标准分类 + 通用兜底）
	contractTypePrompts := map[string]string{
		"买卖合同":   "contract_reviewer_prompt_sale.txt",
		"服务合同":   "contract_reviewer_prompt_service.txt",
		"劳动合同":   "contract_reviewer_prompt_labor.txt",
		"租赁合同":   "contract_reviewer_prompt_lease.txt",
		"借款合同":   "contract_reviewer_prompt_loan.txt",
		"合作合同":   "contract_reviewer_prompt_coop.txt",
		"知识产权合同": "contract_reviewer_prompt_ip.txt",
		"通用":     "contract_reviewer_prompt_base.txt",
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
func (s *ReviewService) CreateReviewTask(ctx context.Context, account string, userID uint64, req *CreateReviewTaskRequest) (*ReviewTask, error) {
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
	if sessRecord.UserID != userID {
		return nil, fmt.Errorf("无权访问该会话")
	}
	var ownedContract struct {
		ID uint64 `gorm:"column:id"`
	}
	if err := s.db.WithContext(ctx).Table("contracts").
		Select("id").
		Where("id = ? AND account = ?", sessRecord.FileID, account).
		First(&ownedContract).Error; err != nil {
		return nil, fmt.Errorf("无权访问该合同")
	}

	// 检查是否已存在任务，存在则删除旧任务
	existingTask, err := s.reviewRepo.GetBySessionIDAndUserID(ctx, req.SessionID, userID)
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
func (s *ReviewService) GetReviewTask(ctx context.Context, userID, sessionID uint64) (*ReviewTask, error) {
	return s.reviewRepo.GetBySessionIDAndUserID(ctx, sessionID, userID)
}

// GetReviewTaskByID 根据ID获取审阅任务
func (s *ReviewService) GetReviewTaskByID(ctx context.Context, userID, id uint64) (*ReviewTask, error) {
	return s.reviewRepo.GetByIDAndUserID(ctx, id, userID)
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
func (s *ReviewService) DeleteReviewTask(ctx context.Context, userID, taskID uint64) error {
	task, err := s.reviewRepo.GetByIDAndUserID(ctx, taskID, userID)
	if err != nil {
		return err
	}
	if task == nil {
		return gorm.ErrRecordNotFound
	}
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
func (s *ReviewService) GetReviewResults(ctx context.Context, userID, taskID uint64) ([]ReviewResult, error) {
	task, err := s.reviewRepo.GetByIDAndUserID(ctx, taskID, userID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return s.reviewResultRepo.GetByTaskID(ctx, taskID)
}

// GetReviewResultsBySessionID 根据会话ID获取审阅结果
func (s *ReviewService) GetReviewResultsBySessionID(ctx context.Context, userID, sessionID uint64) ([]ReviewResult, error) {
	task, err := s.reviewRepo.GetBySessionIDAndUserID(ctx, sessionID, userID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return s.reviewResultRepo.GetBySessionID(ctx, sessionID)
}

// DeleteReviewResults 删除审阅结果
func (s *ReviewService) DeleteReviewResults(ctx context.Context, taskID uint64) error {
	return s.reviewResultRepo.DeleteByTaskID(ctx, taskID)
}

// ============ Contract Review Core Logic ============

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
func (s *ReviewService) InitOrchestrator(ctx context.Context) (*agent.ReviewOrchestrator, error) {
	// Serialize cache refresh and return the exact immutable orchestrator
	// snapshot selected for this run. A later knowledge refresh may replace the
	// service pointer, but cannot mutate the snapshot already in use.
	s.orchMu.Lock()
	defer s.orchMu.Unlock()

	if s.llm == nil {
		if err := s.InitLLM(ctx); err != nil {
			return nil, fmt.Errorf("初始化 LLM 失败: %w", err)
		}
	}

	// 知识库陈旧判断：签名未变且编排器已存在时直接复用，避免每次审阅全量重载知识库。
	// 签名查询失败时回退到全量加载（保持原行为）。sig 为空表示未能获取签名，不参与复用判断。
	var pendingSig string
	if sig, err := knowledge.NewRepo(global.DB).KnowledgeSignature(ctx); err == nil {
		pendingSig = sig
		reuse := s.orchestrator != nil && s.knowledgeSignature == sig
		if reuse {
			global.Log.Debug("知识库签名未变化，复用已构建的 Agent 编排器", zap.String("signature", sig))
			return s.orchestrator, nil
		}
	} else {
		global.Log.Warn("知识库签名查询失败，执行全量加载", zap.Error(err))
	}

	llmGenerate := func(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
		// DevMode：跳过真实 LLM 调用，返回 mock 响应用于前端开发迭代
		if os.Getenv("REVIEW_MOCK_LLM") == "true" {
			return &schema.Message{
				Role:    schema.Assistant,
				Content: `{"status":"ok","message":"DevMode mock response"}`,
			}, nil
		}
		// 优先走大模型网关（统一入口 + 限流/配额/成本/语义缓存）；网关不可用时回退原 LLM
		if global.Gateway != nil {
			return global.Gateway.Generate(ctx, messages)
		}
		return s.llm.Generate(ctx, messages)
	}

	// ======== Phase 1: 检索配置优化 ========
	keywordIndex := rag.NewSimpleKeywordIndex()

	// 为合同审阅优化检索配置
	retrieverConfig := rag.DefaultRetrieverConfig()
	retrieverConfig.FinalTopK = 8
	retrieverConfig.TopK = 10
	// RRF 三路权重: 向量 0.5, BM25 0.3, 关键词 0.2
	retrieverConfig.VectorWeight = 0.5
	retrieverConfig.BM25Weight = 0.3
	retrieverConfig.KeywordWeight = 0.2
	retrieverConfig.RRFK = 30 // 较小集合用较小 K 值
	retrieverConfig.MinRelevance = 0
	retrieverConfig.OversampleMultiplier = 5
	retrieverConfig.OversampleMin = 25
	retrieverConfig.OversampleMax = 100
	retrieverConfig.EnableBM25 = false // 由 initVectorRetrieval 自动检测
	retrieverConfig.EnableRerank = false
	retrieverConfig.MMRLambda = 0.7 // MMR 70% 相关性 + 30% 多样性

	var knowledgeChunks []rag.Chunk
	if global.DB != nil {
		chunks, loadErr := rag.LoadKnowledgeChunksFromDB(ctx, global.DB)
		if loadErr != nil {
			global.Log.Warn("加载审阅知识库失败，rag_search 将无检索结果", zap.Error(loadErr))
		} else if len(chunks) > 0 {
			knowledgeChunks = chunks
		} else {
			global.Log.Warn("审阅知识库无已索引分块：请在库表 review_knowledge_* 写入数据并置 status=indexed，或执行 docs/sql/review_knowledge.sql")
		}
	} else {
		global.Log.Warn("global.DB 未初始化，跳过审阅知识库加载")
	}
	if len(knowledgeChunks) == 0 {
		knowledgeChunks = defaultReviewKnowledgeChunks()
		global.Log.Warn("使用内置审阅知识兜底；建议执行 docs/sql/review_knowledge.sql 写入数据库知识库")
	}

	// 父子分块：parent 分块不参与检索，仅用于 child 命中后回填上下文。
	retrievableChunks, parentChunks := rag.PartitionChunks(knowledgeChunks)
	metaCovered, childChunks := 0, 0
	for _, c := range retrievableChunks {
		if c.Metadata["risk_type"] != "" {
			metaCovered++
		}
		if c.ChunkType == rag.ChunkTypeChild {
			childChunks++
		}
	}
	if err := keywordIndex.Index(retrievableChunks); err != nil {
		global.Log.Warn("审阅知识库入关键词索引失败", zap.Error(err))
	} else {
		// 诊断日志：parents=0 表示父子分块当前无数据驱动（空转）；
		// meta_covered 表示多少分块带结构化 metadata（P0 生效范围）。
		global.Log.Info("审阅知识库已加载到关键词索引",
			zap.Int("chunks", len(retrievableChunks)),
			zap.Int("parents", len(parentChunks)),
			zap.Int("meta_covered", metaCovered),
			zap.Int("child_chunks", childChunks))
	}

	// ======== Phase 1: 向量检索 + BM25 初始化 ========
	vectorStore, embedder := s.initVectorRetrieval(ctx, retrievableChunks)

	// ======== Phase 1: Reranker 初始化 ========
	var retrieverOpts []rag.RAGRetrieverOption
	if global.Config != nil && global.Config.Vector != nil &&
		global.Config.Vector.Rerank != nil && global.Config.Vector.Rerank.Enabled {
		rerankCfg := s.buildRerankerConfig()
		reranker := rag.NewOpenAIReranker(rerankCfg)
		retrieverOpts = append(retrieverOpts, rag.WithReranker(reranker, rerankCfg))
		retrieverConfig.EnableRerank = true
		global.Log.Info("Reranker 已启用",
			zap.String("model", rerankCfg.Model),
			zap.Float64("threshold", rerankCfg.Threshold))
	}

	ragRetriever := rag.NewRAGRetriever(vectorStore, keywordIndex, embedder, retrieverConfig, retrieverOpts...)
	ragRetriever.SetParents(parentChunks)

	// ======== Phase 1: Embedding 缓存命中率日志 ========
	if cachedEmbedder, ok := embedder.(*rag.CachedEmbedder); ok {
		global.Log.Info("Embedding 缓存已启用，审阅中相同条款内容将跳过重复计算")
		_ = cachedEmbedder
	}

	ragSearchTool := tools.NewRAGSearchTool(ragRetriever)
	contractContextTool := tools.NewContractContextTool(agent.ContractMeta{}, "")

	suggestionTools := []agent.Tool{ragSearchTool, contractContextTool}

	orchConfig := agent.DefaultOrchestratorConfig()
	// 质量评估：仅单次评估产出评分/缺口，不进行 Reflection 空转重试（真正的带反馈重审尚未实现）。
	orchConfig.MaxReflectionRetries = 0

	s.orchestrator = agent.NewReviewOrchestrator(llmGenerate, ragRetriever, suggestionTools, orchConfig)

	// 编排器构建成功后，记录知识库签名，供后续审阅复用判断。
	if pendingSig != "" {
		s.knowledgeSignature = pendingSig
	}

	global.Log.Info("Agent 编排器初始化完成",
		zap.Bool("vector_enabled", vectorStore != nil),
		zap.Bool("bm25_enabled", retrieverConfig.EnableBM25),
		zap.Bool("rerank_enabled", retrieverConfig.EnableRerank),
		zap.Int("keyword_chunks", len(keywordIndex.SearchableChunks())))
	return s.orchestrator, nil
}

// buildRerankerConfig 从全局配置构建 RerankerConfig
func (s *ReviewService) buildRerankerConfig() rag.RerankerConfig {
	cfg := rag.DefaultRerankerConfig()
	if global.Config == nil || global.Config.Vector == nil || global.Config.Vector.Rerank == nil {
		return cfg
	}
	rc := global.Config.Vector.Rerank
	if rc.Model != "" {
		cfg.Model = rc.Model
	}
	if rc.APIBase != "" {
		cfg.APIBase = rc.APIBase
	}
	if rc.APIKey != "" {
		cfg.APIKey = rc.APIKey
	}
	if rc.TopK > 0 {
		cfg.TopK = rc.TopK
	}
	if rc.Threshold > 0 {
		cfg.Threshold = rc.Threshold
	}
	if rc.TimeoutSeconds > 0 {
		cfg.TimeoutSeconds = rc.TimeoutSeconds
	}
	return cfg
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
	rawEmbedder := rag.NewOpenAIEmbeddingModel(
		vectorCfg.Embedding.Model,
		embeddingAPIBase,
		embeddingAPIKey,
		timeout,
	)
	// Phase 1: CachedEmbedder - 同一 session 内相同文本不重复计算 embedding
	embedder := rag.NewCachedEmbedder(rawEmbedder, rag.NewEmbeddingCache())
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
		cachedEmbedder := rag.NewCachedEmbedder(s.vectorEmbedder, rag.NewEmbeddingCache())
		s.vectorComponentMu.Unlock()
		if err := s.indexKnowledgeChunksToVectorStore(ctx, chunks, store, cachedEmbedder, dimension); err != nil {
			global.Log.Warn("同步审阅知识到 Milvus 失败，本次审阅仍使用关键词检索兜底", zap.Error(err))
		}
		return store, cachedEmbedder
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
	s.vectorEmbedder = rawEmbedder
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
		{"builtin-2", "规范", "服务合同", "内置服务合同审阅指引", "服务合同应明确服务内容、服务期限、质量标准、验收流程、整改期限、费用支付条件、成果知识产权归属和逾期违约责任。仅列附件或笼统描述服务内容，容易导致验收和付款争议。"},
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
	orchestrator, err := s.InitOrchestrator(ctx)
	if err != nil {
		return fmt.Errorf("初始化 Agent 编排器失败: %w", err)
	}

	// 注入调用方信息，供网关成本归因与限流配额使用（功能=review）
	ctx = gateway.WithCaller(ctx, task.UserID, "", gateway.FeatureReview)

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

	callbacks := agent.ReviewCallbacks{
		OnProgress: func(event agent.ProgressEvent) {
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
		},
		OnFinding: func(finding agent.RiskFinding) {
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
		},
	}

	report, err := orchestrator.ReviewContract(ctx, contractContent, meta, callbacks)
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
