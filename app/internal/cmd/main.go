package main

import (
	"context"
	"fmt"
	"net/http"

	"contract_review/app/config"
	"contract_review/app/internal/Initialize"
	"contract_review/app/internal/comparison"
	"contract_review/app/internal/contract"
	"contract_review/app/internal/global"
	"contract_review/app/internal/knowledge"
	"contract_review/app/internal/middleware/jwt_auth"
	"contract_review/app/internal/middleware/redis"
	"contract_review/app/internal/prompts"
	"contract_review/app/internal/review"
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
		&comparison.ComparisonTask{},
		&prompts.SystemPrompt{},
		&prompts.InstitutionPrompt{},
		&prompts.PersonalPrompt{},
	); err != nil {
		global.Log.Fatal("AutoMigrate失败", zap.Error(err))
	}
	if err := knowledge.AutoMigrate(db); err != nil {
		global.Log.Fatal("知识库表迁移失败", zap.Error(err))
	}

	// 5. 初始化 Redis
	redisAddr := fmt.Sprintf("%s:%s", global.Config.Redis.Host, global.Config.Redis.Port)
	global.Redis = redis.NewRedisClient(redisAddr, global.Config.Redis.Password, global.Config.Redis.DB)

	// 6. 初始化 LLM
	ctx := context.Background()
	llm, err := Initialize.InitLLM(ctx)
	if err != nil {
		global.Log.Fatal("初始化LLM失败", zap.Error(err))
	}
	global.LLM = llm

	// 7. 初始化合同解析智能体
	if err := Initialize.InitContractAgent(ctx); err != nil {
		global.Log.Fatal("初始化合同智能体失败", zap.Error(err))
	}

	// 8. 创建 repos
	userRepo := user.NewUserRepo(db)
	sessionRepo := session.NewSessionRepo(db)
	contractRepo := contract.NewContractRepo(db)
	reviewRepo := review.NewReviewRepo(db)
	reviewResultRepo := review.NewReviewResultRepo(db)
	comparisonRepo := comparison.NewComparisonRepo(db)
	_ = prompts.NewPromptRepo(db)

	// 创建 services
	userService := user.NewUserService(userRepo, global.Redis)
	contractService := contract.NewContractService(contractRepo, global.Redis)
	sessionService := session.NewSessionService(sessionRepo, contractRepo, db, global.Redis)
	reviewService := review.NewReviewService(reviewRepo, reviewResultRepo, db, global.Redis)
	comparisonService := comparison.NewComparisonService(comparisonRepo, contractRepo, db, global.Redis)

	// 创建 handlers
	userHandler := user.NewUserHandler(userService)
	sessionHandler := session.NewSessionHandler(sessionService)
	contractHandler := contract.NewContractHandler(contractService)
	reviewHandler := review.NewReviewHandler(reviewService, contractService)
	comparisonHandler := comparison.NewComparisonHandler(comparisonService)

	// 9. 创建 Hertz 服务器并注册路由
	addr := fmt.Sprintf("%s:%d", global.Config.Server.Host, global.Config.Server.Port)
	h := server.Default(server.WithHostPorts(addr))

	// 静态文件
	h.Static("/api/static", "./uploads")

	api := h.Group("/api")
	authMW := jwt_auth.AccessJWTAuth()

	// ============ User 路由（无鉴权） ============
	api.POST("/user/create", userHandler.Register)
	api.POST("/user/login", userHandler.Login)
	api.POST("/user/refresh_token", userHandler.RefreshToken)

	// ============ User 路由（需鉴权） ============
	api.POST("/user/logout", authMW, userHandler.Logout)
	api.GET("/user/me", authMW, userHandler.GetUserInfo)

	// ============ Session 路由 ============
	api.POST("/session/create_session", authMW, sessionHandler.CreateSession)
	api.POST("/session/list_sessions", authMW, sessionHandler.ListSessions)
	api.PUT("/session/title", authMW, sessionHandler.UpdateSessionTitle)
	api.POST("/session/delete_session", authMW, sessionHandler.DeleteSession)
	api.POST("/session/session_history_detail", authMW, sessionHandler.GetSessionHistoryDetail)
	api.GET("/session/:id", authMW, sessionHandler.GetSessionByID)

	// ============ Contract 路由 ============
	api.POST("/contract/upload", authMW, contractHandler.UploadContractFile)
	api.GET("/contract/list", authMW, contractHandler.ListContracts)
	api.PUT("/contract/:id/type", authMW, contractHandler.UpdateContractType)
	api.DELETE("/contract/:id", authMW, contractHandler.DeleteContract)
	api.GET("/contract/download/:id", authMW, contractHandler.DownloadContract)
	api.POST("/contract/save_file", authMW, contractHandler.UploadContractFile)
	api.GET("/contract/check_file", authMW, func(ctx context.Context, c *app.RequestContext) {
		c.JSON(http.StatusOK, map[string]interface{}{
			"code": 200,
			"msg":  "Success",
			"data": map[string]string{"status": "ready"},
		})
	})

	// ============ Contract Type 路由 ============
	api.POST("/contract_type/create", authMW, contractHandler.CreateContractType)
	api.GET("/contract_type/detail/:id", authMW, contractHandler.GetContractType)
	api.GET("/contract_type/list", authMW, contractHandler.ListContractTypes)
	api.GET("/contract_type/page", authMW, contractHandler.ListContractTypes)
	api.PUT("/contract_type/update/:id", authMW, contractHandler.UpdateContractTypeName)
	api.DELETE("/contract_type/delete/:id", authMW, contractHandler.DeleteContractType)

	// ============ Review 路由 ============
	api.POST("/review_task/start_task", authMW, reviewHandler.StartReviewTask)
	api.POST("/review_task/accept_contract_file", authMW, func(ctx context.Context, c *app.RequestContext) {
		c.JSON(http.StatusOK, map[string]interface{}{"code": 200, "msg": "Success"})
	})
	api.GET("/review/task", authMW, reviewHandler.GetReviewTask)
	api.GET("/review/task/:id", authMW, reviewHandler.GetReviewTaskByID)
	api.GET("/review/tasks", authMW, reviewHandler.ListReviewTasks)
	api.GET("/review/results", authMW, reviewHandler.GetReviewResults)
	api.GET("/review/results/session", authMW, reviewHandler.GetReviewResultsBySessionID)
	api.DELETE("/review/task/:id", authMW, reviewHandler.DeleteReviewTask)

	// ============ Comparison 路由 ============
	api.POST("/comparison_task/start", authMW, comparisonHandler.StartComparison)
	api.GET("/comparison/task/:id", authMW, comparisonHandler.GetComparisonTask)
	api.GET("/comparison/task/session", authMW, comparisonHandler.GetComparisonTaskBySession)
	api.GET("/comparison/tasks", authMW, comparisonHandler.ListComparisonTasks)
	api.DELETE("/comparison/task/:id", authMW, comparisonHandler.DeleteComparisonTask)

	// ============ Stub 路由（前端需要但后端未实现） ============
	stubOK := func(ctx context.Context, c *app.RequestContext) {
		c.JSON(http.StatusOK, map[string]interface{}{"code": 200, "msg": "Success"})
	}
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

	api.GET("/model_configs/get_all_models/", stubJSON([]interface{}{}))
	api.GET("/model_configs/get_default_model", stubJSON(map[string]interface{}{
		"model_name": global.Config.LLMConfig.Model,
	}))
	api.POST("/model_configs/create_model", stubOK)
	api.PUT("/model_configs/update_model/:id", stubOK)

	api.GET("/prompt_manage/org/", stubJSON(map[string]interface{}{
		"prompt_text": "",
	}))

	api.POST("/chat", func(ctx context.Context, c *app.RequestContext) {
		c.JSON(http.StatusOK, map[string]interface{}{
			"code": 200,
			"msg":  "Success",
			"data": map[string]interface{}{
				"reply": "聊天功能暂未开放",
			},
		})
	})

	// 10. 启动服务
	global.Log.Info("服务启动", zap.String("addr", addr))
	h.Spin()
}
