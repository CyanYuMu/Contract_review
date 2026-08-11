package gateway

import (
	"context"
	"time"

	"github.com/cloudwego/eino/schema"
	"gorm.io/gorm"
)

// 业务功能维度（同时作为模型路由 purpose 与成本统计维度）
const (
	FeatureReview    = "review"
	FeatureQA        = "qa"
	FeatureCompare   = "comparison"
	FeatureChat      = "chat"
	FeatureDefault   = "default"
	FeatureEmbedding = "embedding"
)

// LLMRoute 模型路由：feature -> model_config_id
// 业务代码只按 feature 调用，换模型只改本表，不动应用代码。
type LLMRoute struct {
	Feature      string    `gorm:"primaryKey;type:varchar(32);comment:功能维度" json:"feature"`
	ModelConfigID uint      `gorm:"not null;comment:指向 model_configs.id" json:"model_config_id"`
	Params       string    `gorm:"type:json;comment:temperature/max_tokens 等 JSON 参数" json:"params"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (LLMRoute) TableName() string { return "llm_routes" }

// LLMUsageLog 成本日志（用户 + 功能模块双维度）
type LLMUsageLog struct {
	ID               uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID           uint64    `gorm:"index;comment:用户ID" json:"user_id"`
	Account          string    `gorm:"type:varchar(64);index;comment:用户账号" json:"account"`
	Feature          string    `gorm:"type:varchar(32);not null;index;comment:功能维度 review/qa/comparison/chat" json:"feature"`
	ModelName        string    `gorm:"type:varchar(128)" json:"model_name"`
	Provider         string    `gorm:"type:varchar(64)" json:"provider"`
	PromptTokens     int       `gorm:"default:0" json:"prompt_tokens"`
	CompletionTokens int       `gorm:"default:0" json:"completion_tokens"`
	TotalTokens      int       `gorm:"default:0" json:"total_tokens"`
	Cost             float64   `gorm:"type:decimal(12,6);default:0" json:"cost"`
	LatencyMs        int       `gorm:"default:0" json:"latency_ms"`
	CacheHit         bool      `gorm:"default:false" json:"cache_hit"`
	Status           string    `gorm:"type:varchar(16);default:ok" json:"status"`
	Error            string    `gorm:"type:varchar(512)" json:"error,omitempty"`
	CreatedAt        time.Time `gorm:"index;autoCreateTime" json:"created_at"`
}

func (LLMUsageLog) TableName() string { return "llm_usage_logs" }

// LLMQuota 配额配置
type LLMQuota struct {
	ID                 uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	SubjectType        string    `gorm:"type:varchar(16);not null;comment:user/role" json:"subject_type"`
	SubjectID         uint64    `gorm:"not null;comment:用户ID或角色ID" json:"subject_id"`
	Feature           string    `gorm:"type:varchar(32);comment:NULL 表示该用户全部功能" json:"feature"`
	DailyTokenLimit   int       `gorm:"default:0;comment:每日 token 上限，0=不限" json:"daily_token_limit"`
	MonthlyTokenLimit int       `gorm:"default:0;comment:每月 token 上限，0=不限" json:"monthly_token_limit"`
	UpdatedAt         time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (LLMQuota) TableName() string { return "llm_quotas" }

// Usage token 用量
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// ChatOptions 调用参数
type ChatOptions struct {
	Temperature float64
	MaxTokens   int
	TopP        float64
}

// GatewayRequest 网关统一请求
type GatewayRequest struct {
	Feature  string
	UserID   uint64
	Account  string
	Messages []*schema.Message
	Stream   bool
	Options  *ChatOptions
}

// GatewayResponse 网关统一响应
type GatewayResponse struct {
	Message   *schema.Message
	Usage     Usage
	CacheHit  bool
	ModelName string
	Provider  string
}

// ============ caller context ============

type callerKey struct{}

// Caller 携带调用方信息（用户 + 功能），用于成本归因与限流配额。
type Caller struct {
	UserID  uint64
	Account string
	Feature string
}

// WithCaller 将调用方信息注入 context
func WithCaller(ctx context.Context, userID uint64, account, feature string) context.Context {
	if feature == "" {
		feature = FeatureDefault
	}
	return context.WithValue(ctx, callerKey{}, Caller{UserID: userID, Account: account, Feature: feature})
}

func callerFromCtx(ctx context.Context) Caller {
	if v, ok := ctx.Value(callerKey{}).(Caller); ok {
		return v
	}
	return Caller{Feature: FeatureDefault}
}

// AutoMigrate 网关表迁移
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&LLMRoute{}, &LLMUsageLog{}, &LLMQuota{})
}

// Embedder 语义缓存所需的向量化能力。
// rag.OpenAIEmbeddingModel / rag.CachedEmbedder 等均实现该接口，故网关无需直接依赖 rag 包（避免循环引用）。
type Embedder interface {
	Embed(text string) ([]float32, error)
}
