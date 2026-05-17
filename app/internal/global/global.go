package global

import (
	"contract_review/app/config"
	"contract_review/app/internal/middleware/redis"

	fmodel "github.com/cloudwego/eino/components/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var (
	Config *config.Config
	Log    *zap.Logger
	DB     *gorm.DB
	Redis  *redis.RedisClient
	LLM    fmodel.BaseChatModel // LLM客户端（可选，agent包内部也有独立的LLM）
)
