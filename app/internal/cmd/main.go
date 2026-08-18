package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"contract_review/app/config"
	"contract_review/app/internal/Initialize"
	"contract_review/app/internal/comparison"
	"contract_review/app/internal/contract"
	"contract_review/app/internal/gateway"
	"contract_review/app/internal/global"
	"contract_review/app/internal/knowledge"
	"contract_review/app/internal/knowledgeapi"
	"contract_review/app/internal/middleware"
	"contract_review/app/internal/middleware/jwt_auth"
	"contract_review/app/internal/middleware/redis"
	"contract_review/app/internal/modelconfig"
	"contract_review/app/internal/prompts"
	"contract_review/app/internal/qa"
	"contract_review/app/internal/rag"
	"contract_review/app/internal/review"
	"contract_review/app/internal/riskconfig"
	"contract_review/app/internal/session"
	"contract_review/app/internal/user"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func main() {
	// 1. 初始化配置
	global.Config = config.GetConfig()

	// 2. 初始化日志
	logger, err := zap.NewProduction()
	if err != nil {
		panic(fmt.Sprintf("初始化日志失败: %v", err))
	}
	defer logger.Sync()
	global.Log = logger

	// 3. 初始化 MySQL
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		global.Config.Mysql.Username,
		global.Config.Mysql.Password,
		global.Config.Mysql.Host,
		global.Config.Mysql.Port,
		global.Config.Mysql.DBName,
	)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		global.Log.Fatal("初始化MySQL失败", zap.Error(err))
	}
	global.DB = db

	// 4. 自动迁移所有模型
	if err := db.AutoMigrate(
		&user.User{},
		&session.Session{},
		&contract.Contract{},
		&contract.ContractType{},
		&review.ReviewTask{},
		&review.ReviewResult{},
		&riskconfig.RiskPoint{},
		&comparison.ComparisonTask{},
		&prompts.SystemPrompt{},
		&prompts.InstitutionPrompt{},
		&prompts.PersonalPrompt{},
		&modelconfig.ModelConfig{},
		&qa.QAMessage{},
	); err != nil {
		global.Log.Fatal("AutoMigrate失败", zap.Error(err))
	}
	if err := knowledge.AutoMigrate(db); err != nil {
		global.Log.Fatal("知识库表迁移失败", zap.Error(err))
	}
	if err := gateway.AutoMigrate(db); err != nil {
		global.Log.Fatal("网关表迁移失败", zap.Error(err))
	}

	// 5. 初始化 Redis
	redisAddr := fmt.Sprintf("%s:%s", global.Config.Redis.Host, global.Config.Redis.Port)
	global.Redis = redis.NewRedisClient(redisAddr, global.Config.Redis.Password, global.Config.Redis.DB)

	// 6. 初始化 LLM
	ctx := context.Background()
	modelConfigRepo := modelconfig.NewRepo(db)
	modelConfigService := modelconfig.NewService(modelConfigRepo)
	if err := modelConfigService.EnsureDefaultFromConfig(ctx, global.Config.LLMConfig); err != nil {
		global.Log.Fatal("初始化模型配置失败", zap.Error(err))
	}
	llm, err := Initialize.InitLLM(ctx)
	if err != nil {
		global.Log.Fatal("初始化LLM失败", zap.Error(err))
	}
	global.LLM = llm

	// 6.1 初始化大模型网关（统一入口 + 限流/配额/成本/语义缓存）
	gwCfg := gateway.DefaultConfig()
	if global.Config.Gateway != nil {
		gc := global.Config.Gateway
		gwCfg.EnableRateLimit = gc.EnableRateLimit
		if gc.RateLimitPerMin > 0 {
			gwCfg.RateLimitPerMin = gc.RateLimitPerMin
		}
		gwCfg.EnableQuota = gc.EnableQuota
		gwCfg.EnableCostLog = gc.EnableCostLog
		gwCfg.EnableCache = gc.EnableCache
		if gc.CacheThreshold > 0 {
			gwCfg.CacheThreshold = gc.CacheThreshold
		}
		if gc.CacheTTLSeconds > 0 {
			gwCfg.CacheTTL = time.Duration(gc.CacheTTLSeconds) * time.Second
		}
		if gc.CacheMaxEntries > 0 {
			gwCfg.CacheMaxEntries = gc.CacheMaxEntries
		}
		if gc.RequestTimeoutS > 0 {
			gwCfg.RequestTimeoutS = gc.RequestTimeoutS
		}
	}
	// 构建语义缓存所需的 embedder（复用向量检索的 embedding 配置；未配置则缓存自动关闭）
	var gwEmbedder gateway.Embedder
	if global.Config.Vector != nil && global.Config.Vector.Embedding != nil && global.Config.Vector.Embedding.Enabled {
		embCfg := global.Config.Vector.Embedding
		embBase := embCfg.APIBase
		embKey := embCfg.APIKey
		if embBase == "" && global.Config.LLMConfig != nil {
			embBase = global.Config.LLMConfig.APIBase
		}
		if embKey == "" && global.Config.LLMConfig != nil {
			embKey = global.Config.LLMConfig.APIKey
		}
		if embCfg.Model != "" && embBase != "" {
			rawEmbedder := rag.NewOpenAIEmbeddingModel(embCfg.Model, embBase, embKey, time.Duration(embCfg.TimeoutSeconds)*time.Second)
			gwEmbedder = rag.NewCachedEmbedder(rawEmbedder, rag.NewEmbeddingCache())
			if gwCfg.EnableCache {
				global.Log.Info("网关语义缓存已启用", zap.Float64("threshold", gwCfg.CacheThreshold))
			}
		}
	}
	global.Gateway = gateway.NewGateway(db, global.Redis, gwEmbedder, gwCfg)
	if err := global.Gateway.EnsureDefaultRoutes(ctx); err != nil {
		global.Log.Warn("初始化网关默认路由失败", zap.Error(err))
	}
	global.Log.Info("大模型网关初始化完成",
		zap.Bool("rate_limit", gwCfg.EnableRateLimit),
		zap.Bool("quota", gwCfg.EnableQuota),
		zap.Bool("cost_log", gwCfg.EnableCostLog),
		zap.Bool("cache", gwCfg.EnableCache && gwEmbedder != nil))

	// 7. 初始化合同解析智能体
	if err := Initialize.InitContractAgent(ctx); err != nil {
		global.Log.Fatal("初始化合同智能体失败", zap.Error(err))
	}

	// 8. 创建 repos
	userRepo := user.NewUserRepo(db)
	if err := userRepo.EnsureSystemOwner(ctx); err != nil {
		global.Log.Fatal("初始化系统管理员失败", zap.Error(err))
	}
	sessionRepo := session.NewSessionRepo(db)
	contractRepo := contract.NewContractRepo(db)
	reviewRepo := review.NewReviewRepo(db)
	reviewResultRepo := review.NewReviewResultRepo(db)
	riskPointRepo := riskconfig.NewRepo(db)
	comparisonRepo := comparison.NewComparisonRepo(db)
	_ = prompts.NewPromptRepo(db)

	// 创建 services
	userService := user.NewUserService(userRepo, global.Redis)
	contractService := contract.NewContractService(contractRepo, global.Redis)
	sessionService := session.NewSessionService(sessionRepo, contractRepo, db, global.Redis)
	reviewService := review.NewReviewService(reviewRepo, reviewResultRepo, db, global.Redis)
	riskPointService := riskconfig.NewService(riskPointRepo, db)
	comparisonService := comparison.NewComparisonService(comparisonRepo, contractRepo, db, global.Redis)
	knowledgeAPIService := knowledgeapi.NewService(db)

	qaService := qa.NewQAService(db, contractService)

	// 创建 handlers
	userHandler := user.NewUserHandler(userService)
	sessionHandler := session.NewSessionHandler(sessionService)
	contractHandler := contract.NewContractHandler(contractService)
	reviewHandler := review.NewReviewHandler(reviewService, contractService)
	riskPointHandler := riskconfig.NewHandler(riskPointService)
	comparisonHandler := comparison.NewComparisonHandler(comparisonService)
	modelConfigHandler := modelconfig.NewHandler(modelConfigService)
	gatewayHandler := gateway.NewHandler(global.Gateway)
	qaHandler := qa.NewQAHandler(qaService)
	knowledgeAPIHandler := knowledgeapi.NewHandler(knowledgeAPIService)

	// P0 收尾：为存量风险点分块补齐结构化 metadata（幂等，仅更新 metadata IS NULL 的分块）。
	// 必须在预热审阅编排器之前执行，确保关键词/向量索引加载到的是补齐后的 metadata。
	if n, err := riskPointService.BackfillRiskPointMetadata(ctx); err != nil {
		global.Log.Warn("存量风险点 metadata 补齐失败", zap.Error(err))
	} else if n > 0 {
		global.Log.Info("存量风险点 metadata 已补齐", zap.Int("chunks", n))
	}

	// 后台预热审阅编排器：提前加载知识库、构建向量索引，避免首次审阅请求等待 2-5 秒。
	go func() {
		warmCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if _, err := reviewService.InitOrchestrator(warmCtx); err != nil {
			global.Log.Warn("审阅编排器预热失败（首次审阅时将自动初始化）", zap.Error(err))
		} else {
			global.Log.Info("审阅编排器预热完成")
		}
	}()

	// 9. 创建 Hertz 服务器并注册路由
	addr := fmt.Sprintf("%s:%d", global.Config.Server.Host, global.Config.Server.Port)
	h := server.Default(server.WithHostPorts(addr))

	api := h.Group("/api")
	authMW := jwt_auth.AccessJWTAuth()
	adminMW := middleware.RequireSystemRole(middleware.SystemRoleAdmin, middleware.SystemRoleOwner)
	// scopeMW 解析当前账号为统一的 ResourceScope（含数字 UserID、SystemRole 与预留
	// OrganizationID），一次请求解析一次并缓存，供 handler/service 复用，取代各模块
	// 重复的 ResolveUserID。
	scopeMW := middleware.ResolveScope(middleware.NewDBIdentityResolver(
		func(ctx context.Context, account string) (uint64, string, error) {
			u, err := userRepo.GetUserByAccount(ctx, account)
			if err != nil {
				return 0, "", err
			}
			return uint64(u.ID), u.SystemRole, nil
		},
		global.Redis,
	))

	// ============ User 路由（无鉴权） ============
	api.POST("/user/create", userHandler.Register)
	api.POST("/user/login", userHandler.Login)
	api.POST("/user/refresh_token", userHandler.RefreshToken)

	// ============ User 路由（需鉴权） ============
	api.POST("/user/logout", authMW, scopeMW, userHandler.Logout)
	api.GET("/user/me", authMW, scopeMW, userHandler.GetUserInfo)

	// ============ Session 路由 ============
	api.POST("/session/create_session", authMW, scopeMW, sessionHandler.CreateSession)
	api.POST("/session/list_sessions", authMW, scopeMW, sessionHandler.ListSessions)
	api.PUT("/session/title", authMW, scopeMW, sessionHandler.UpdateSessionTitle)
	api.POST("/session/delete_session", authMW, scopeMW, sessionHandler.DeleteSession)
	api.POST("/session/session_history_detail", authMW, scopeMW, sessionHandler.GetSessionHistoryDetail)
	api.GET("/session/:id", authMW, scopeMW, sessionHandler.GetSessionByID)

	// ============ Contract 路由 ============
	api.POST("/contract/upload", authMW, scopeMW, contractHandler.UploadContractFile)
	api.GET("/contract/list", authMW, scopeMW, contractHandler.ListContracts)
	api.PUT("/contract/:id/type", authMW, scopeMW, contractHandler.UpdateContractType)
	api.DELETE("/contract/:id", authMW, scopeMW, contractHandler.DeleteContract)
	api.GET("/contract/download/:id", authMW, scopeMW, contractHandler.DownloadContract)
	api.POST("/contract/save_file", authMW, scopeMW, contractHandler.UploadContractFile)
	api.GET("/contract/check_file", authMW, scopeMW, func(ctx context.Context, c *app.RequestContext) {
		c.JSON(http.StatusOK, map[string]interface{}{
			"code": 200,
			"msg":  "Success",
			"data": map[string]string{"status": "ready"},
		})
	})

	// ============ Contract Type 路由 ============
	api.POST("/contract_type/create", authMW, scopeMW, adminMW, contractHandler.CreateContractType)
	api.GET("/contract_type/detail/:id", authMW, scopeMW, contractHandler.GetContractType)
	api.GET("/contract_type/list", authMW, scopeMW, contractHandler.ListContractTypes)
	api.GET("/contract_type/page", authMW, scopeMW, contractHandler.ListContractTypes)
	api.GET("/contract_type/creators", authMW, scopeMW, contractHandler.ListContractTypeCreators)
	api.PUT("/contract_type/update/:id", authMW, scopeMW, adminMW, contractHandler.UpdateContractTypeName)
	api.DELETE("/contract_type/delete/:id", authMW, scopeMW, adminMW, contractHandler.DeleteContractType)
	api.DELETE("/contract_type/batchdelete", authMW, scopeMW, adminMW, contractHandler.BatchDeleteContractType)

	// ============ Knowledge 路由（长文档入库） ============
	api.POST("/knowledge/document", authMW, scopeMW, adminMW, knowledgeAPIHandler.IngestDocument)

	// ============ Risk Point 路由 ============
	api.POST("/risk_point/create", authMW, scopeMW, adminMW, riskPointHandler.Create)
	api.GET("/risk_point/detail/:id", authMW, scopeMW, riskPointHandler.Detail)
	api.GET("/risk_point/list", authMW, scopeMW, riskPointHandler.List)
	api.GET("/risk_point/page", authMW, scopeMW, riskPointHandler.List)
	api.GET("/risk_point/stats", authMW, scopeMW, riskPointHandler.Stats)
	api.PUT("/risk_point/update/:id", authMW, scopeMW, adminMW, riskPointHandler.Update)
	api.DELETE("/risk_point/delete/:id", authMW, scopeMW, adminMW, riskPointHandler.Delete)
	api.DELETE("/risk_point/batchdelete", authMW, scopeMW, adminMW, riskPointHandler.BatchDelete)

	// ============ Review 路由 ============
	api.POST("/review_task/start_task", authMW, scopeMW, reviewHandler.StartReviewTask)
	api.POST("/review_task/accept_contract_file", authMW, scopeMW, func(ctx context.Context, c *app.RequestContext) {
		c.JSON(http.StatusOK, map[string]interface{}{"code": 200, "msg": "Success"})
	})
	api.GET("/review/task", authMW, scopeMW, reviewHandler.GetReviewTask)
	api.GET("/review/task/:id", authMW, scopeMW, reviewHandler.GetReviewTaskByID)
	api.GET("/review/tasks", authMW, scopeMW, reviewHandler.ListReviewTasks)
	api.GET("/review/results", authMW, scopeMW, reviewHandler.GetReviewResults)
	api.GET("/review/results/session", authMW, scopeMW, reviewHandler.GetReviewResultsBySessionID)
	api.DELETE("/review/task/:id", authMW, scopeMW, reviewHandler.DeleteReviewTask)

	// ============ Comparison 路由 ============
	api.POST("/comparison_task/start", authMW, scopeMW, comparisonHandler.StartComparison)
	api.GET("/comparison/task/:id", authMW, scopeMW, comparisonHandler.GetComparisonTask)
	api.GET("/comparison/task/session", authMW, scopeMW, comparisonHandler.GetComparisonTaskBySession)
	api.GET("/comparison/tasks", authMW, scopeMW, comparisonHandler.ListComparisonTasks)
	api.DELETE("/comparison/task/:id", authMW, scopeMW, comparisonHandler.DeleteComparisonTask)

	// ============ Stub 路由（前端需要但后端未实现） ============
	stubJSON := func(data interface{}) app.HandlerFunc {
		return func(ctx context.Context, c *app.RequestContext) {
			c.JSON(http.StatusOK, map[string]interface{}{"code": 200, "msg": "Success", "data": data})
		}
	}

	api.GET("/signboard/statistics_overview", stubJSON(map[string]interface{}{
		"total_contracts": 0, "total_reviews": 0, "total_comparisons": 0, "total_users": 0,
	}))
	api.GET("/signboard/statistics_revisions", stubJSON([]interface{}{}))
	api.GET("/signboard/departments_usage", stubJSON([]interface{}{}))
	api.GET("/signboard/trends_contracts", stubJSON([]interface{}{}))

	// 模型配置查询——所有用户可查看（用于选择模型）
	api.GET("/model_configs/get_all_models/", authMW, scopeMW, modelConfigHandler.List)
	api.GET("/model_configs/get_default_model", authMW, scopeMW, modelConfigHandler.GetDefault)
	api.GET("/model_configs/provider_options", authMW, scopeMW, modelConfigHandler.ProviderOptions)
	// 模型配置管理——仅管理员可创建/修改
	api.POST("/model_configs/create_model", authMW, scopeMW, adminMW, modelConfigHandler.Create)
	api.PUT("/model_configs/update_model/:id", authMW, scopeMW, adminMW, modelConfigHandler.Update)

	// ============ Gateway 路由（成本看板 / 路由配置 / 配额） ============
	// 个人用量统计——所有用户可查看自己的 token 消耗
	api.GET("/gateway/usage_stats", authMW, scopeMW, gatewayHandler.UsageStats)
	api.GET("/gateway/usage_trend", authMW, scopeMW, gatewayHandler.UsageTrend)
	// 个人配额——所有用户可查看自己的配额
	api.GET("/gateway/quotas", authMW, scopeMW, gatewayHandler.ListQuotas)
	// 管理员路由——路由配置与配额设置
	api.GET("/gateway/routes", authMW, scopeMW, adminMW, gatewayHandler.ListRoutes)
	api.PUT("/gateway/route", authMW, scopeMW, adminMW, gatewayHandler.UpdateRoute)
	api.PUT("/gateway/quota", authMW, scopeMW, adminMW, gatewayHandler.SetQuota)

	api.GET("/prompt_manage/org/", authMW, scopeMW, adminMW, stubJSON(map[string]interface{}{
		"prompt_text": "",
	}))

	// ============ 合同问答 路由（SSE 流式 + 多轮 + 绑定合同） ============
	api.POST("/qa/ask", authMW, scopeMW, qaHandler.Ask)
	api.GET("/qa/messages", authMW, scopeMW, qaHandler.GetMessages)
	api.POST("/qa/clear", authMW, scopeMW, qaHandler.ClearMessages)

	// 10. 启动服务
	global.Log.Info("服务启动", zap.String("addr", addr))
	h.Spin()
}
