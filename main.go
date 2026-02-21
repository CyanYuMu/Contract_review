package main

import (
	"context"
	"contract_review/app/config"
	"contract_review/app/internal/Initialize"
	"contract_review/app/internal/comparison"
	"contract_review/app/internal/contract"
	"contract_review/app/internal/global"
	"contract_review/app/internal/middleware/redis"
	"contract_review/app/internal/review"
	"contract_review/app/internal/session"
	"contract_review/app/internal/user"
	"contract_review/app/pkg/auth"
	"fmt"
	"log"

	"github.com/cloudwego/hertz/pkg/app/server"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func main() {
	var err error
	ctx := context.Background()

	// 1. 加载配置
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	global.Config = cfg

	// 2. 初始化日志
	// TODO: Initialize logger

	// 3. 初始化数据库
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.MySQL.Username,
		cfg.MySQL.Password,
		cfg.MySQL.Host,
		cfg.MySQL.Port,
		cfg.MySQL.DBName,
	)

	global.DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// 自动迁移表结构
	err = global.DB.AutoMigrate(
		&user.User{},
		&contract.Contract{},
		&contract.ContractType{},
		&review.ReviewTask{},
		&review.ReviewResult{},
		&comparison.ComparisonTask{},
		&session.Session{},
	)
	if err != nil {
		log.Printf("Warning: AutoMigrate failed: %v", err)
	}

	// 4. 初始化Redis
	global.Redis = redis.NewRedisClient(&redis.RedisConfig{
		Host:                 cfg.Redis.Host,
		Port:                 cfg.Redis.Port,
		Password:             cfg.Redis.Password,
		DB:                   cfg.Redis.DB,
		SocketConnectTimeout: cfg.Redis.SocketConnectTimeout,
		SocketTimeout:        cfg.Redis.SocketTimeout,
	})

	// 5. 初始化LLM
	global.LLM, err = initialize.InitLLM(ctx)
	if err != nil {
		log.Printf("Warning: Failed to initialize LLM: %v", err)
	}

	// 6. 创建Hertz服务器
	h := server.Default(server.WithHostPorts(fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)))

	// 7. 初始化依赖
	userRepo := user.NewUserRepo(global.DB)
	userService := user.NewUserService(userRepo, global.Redis)
	userHandler := user.NewUserHandler(userService)

	contractRepo := contract.NewContractRepo(global.DB)
	contractService := contract.NewContractService(contractRepo, global.Redis)
	contractHandler := contract.NewContractHandler(contractService)

	reviewRepo := review.NewReviewRepo(global.DB)
	reviewResultRepo := review.NewReviewResultRepo(global.DB)
	reviewService := review.NewReviewService(reviewRepo, reviewResultRepo, nil, global.Redis)
	reviewHandler := review.NewReviewHandler(reviewService, contractService)

	comparisonRepo := comparison.NewComparisonRepo(global.DB)
	comparisonService := comparison.NewComparisonService(comparisonRepo, contractRepo, nil, nil, global.Redis)
	comparisonHandler := comparison.NewComparisonHandler(comparisonService)

	sessionRepo := session.NewSessionRepo(global.DB)
	sessionService := session.NewSessionService(sessionRepo, contractRepo, reviewRepo, reviewResultRepo, comparisonRepo, global.Redis)
	sessionHandler := session.NewSessionHandler(sessionService)

	// 8. 注册路由
	// 用户相关
	h.POST("/api/user/register", userHandler.Register)
	h.POST("/api/user/login", userHandler.Login)
	h.GET("/api/user/info", userHandler.GetUserInfo)
	h.PUT("/api/user/info", userHandler.UpdateUserInfo)

	// 合同相关
	h.POST("/api/contract/upload", contractHandler.UploadContractFile)
	h.GET("/api/contract/list", contractHandler.ListContracts)
	h.PUT("/api/contract/:id/type", contractHandler.UpdateContractType)
	h.DELETE("/api/contract/:id", contractHandler.DeleteContract)
	h.GET("/api/contract/:id/download", contractHandler.DownloadContract)
	h.POST("/api/contract-type", contractHandler.CreateContractType)
	h.GET("/api/contract-type/:id", contractHandler.GetContractType)
	h.GET("/api/contract-type/list", contractHandler.ListContractTypes)
	h.PUT("/api/contract-type/:id", contractHandler.UpdateContractTypeName)
	h.DELETE("/api/contract-type/:id", contractHandler.DeleteContractType)

	// 审阅相关
	h.POST("/api/review/start", reviewHandler.StartReviewTask)
	h.GET("/api/review/task", reviewHandler.GetReviewTask)
	h.GET("/api/review/task/:id", reviewHandler.GetReviewTaskByID)
	h.GET("/api/review/tasks", reviewHandler.ListReviewTasks)
	h.GET("/api/review/results", reviewHandler.GetReviewResults)
	h.GET("/api/review/results/session", reviewHandler.GetReviewResultsBySessionID)
	h.DELETE("/api/review/task/:id", reviewHandler.DeleteReviewTask)

	h.POST("/api/comparison/start", comparisonHandler.StartComparison)
	h.GET("/api/comparison/task/:id", comparisonHandler.GetComparisonTask)
	h.GET("/api/comparison/task/session", comparisonHandler.GetComparisonTaskBySession)
	h.GET("/api/comparison/tasks", comparisonHandler.ListComparisonTasks)
	h.DELETE("/api/comparison/task/:id", comparisonHandler.DeleteComparisonTask)

	// 会话相关
	h.POST("/api/session/create", sessionHandler.CreateSession)
	h.POST("/api/session/list", sessionHandler.ListSessions)
	h.PUT("/api/session/title", sessionHandler.UpdateSessionTitle)
	h.POST("/api/session/delete", sessionHandler.DeleteSession)
	h.POST("/api/session/history", sessionHandler.GetSessionHistoryDetail)
	h.GET("/api/session/:id", sessionHandler.GetSessionByID)

	// 9. 启动服务器
	h.Spin()
}
