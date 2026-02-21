package global

import (
	"contract_review/app/config"
	"contract_review/app/internal/middleware/redis"

	"github.com/cloudwego/eino-ext/components/model/arkbot"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var (
	Config *config.Config
	Log    *zap.Logger
	DB     *gorm.DB
	Redis  *redis.RedisClient
	LLM    *arkbot.ChatModel // LLM客户端（可选，agent包内部也有独立的LLM）
)
