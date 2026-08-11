package gateway

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// GetRoute 按 feature 查询模型路由
func (r *Repo) GetRoute(ctx context.Context, feature string) (*LLMRoute, error) {
	var route LLMRoute
	err := r.db.WithContext(ctx).Where("feature = ?", feature).First(&route).Error
	if err != nil {
		return nil, err
	}
	return &route, nil
}

// UpsertRoute 创建或更新路由
func (r *Repo) UpsertRoute(ctx context.Context, route *LLMRoute) error {
	return r.db.WithContext(ctx).Save(route).Error
}

// ListRoutes 所有路由
func (r *Repo) ListRoutes(ctx context.Context) ([]LLMRoute, error) {
	var routes []LLMRoute
	err := r.db.WithContext(ctx).Order("feature").Find(&routes).Error
	return routes, err
}

// GetQuotas 查询配额（user 维度 + 通配 feature=NULL）
func (r *Repo) GetQuotas(ctx context.Context, userID uint64) ([]LLMQuota, error) {
	var quotas []LLMQuota
	err := r.db.WithContext(ctx).
		Where("subject_type = ? AND subject_id = ? AND (feature IS NULL OR feature = ?)", "user", userID, "").
		Find(&quotas).Error
	return quotas, err
}

// LogUsage 写入一条用量日志（失败仅返回 error，调用方自行兜底）
func (r *Repo) LogUsage(ctx context.Context, log *LLMUsageLog) error {
	return r.db.WithContext(ctx).Create(log).Error
}

// UsageStat 用量统计行
type UsageStat struct {
	Feature          string  `json:"feature"`
	ModelName        string  `json:"model_name"`
	CallCount        int64   `json:"call_count"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	Cost             float64 `json:"cost"`
	CacheHitCount    int64   `json:"cache_hit_count"`
}

// UsageStatsByFeature 按功能模块聚合用量
func (r *Repo) UsageStatsByFeature(ctx context.Context, start, end time.Time) ([]UsageStat, error) {
	var stats []UsageStat
	err := r.db.WithContext(ctx).Table("llm_usage_logs").
		Select("feature, model_name, COUNT(*) as call_count, "+
			"COALESCE(SUM(prompt_tokens),0) as prompt_tokens, "+
			"COALESCE(SUM(completion_tokens),0) as completion_tokens, "+
			"COALESCE(SUM(total_tokens),0) as total_tokens, "+
			"COALESCE(SUM(cost),0) as cost, "+
			"COALESCE(SUM(CASE WHEN cache_hit=1 THEN 1 ELSE 0 END),0) as cache_hit_count").
		Where("created_at BETWEEN ? AND ?", start, end).
		Group("feature, model_name").
		Order("total_tokens DESC").
		Scan(&stats).Error
	return stats, err
}

// UsageStatsByUser 按用户聚合用量
func (r *Repo) UsageStatsByUser(ctx context.Context, start, end time.Time, limit int) ([]UsageStat, error) {
	if limit <= 0 {
		limit = 50
	}
	var stats []UsageStat
	err := r.db.WithContext(ctx).Table("llm_usage_logs").
		Select("CAST(user_id AS CHAR) as feature, '' as model_name, COUNT(*) as call_count, "+
			"COALESCE(SUM(prompt_tokens),0) as prompt_tokens, "+
			"COALESCE(SUM(completion_tokens),0) as completion_tokens, "+
			"COALESCE(SUM(total_tokens),0) as total_tokens, "+
			"COALESCE(SUM(cost),0) as cost, "+
			"COALESCE(SUM(CASE WHEN cache_hit=1 THEN 1 ELSE 0 END),0) as cache_hit_count").
		Where("created_at BETWEEN ? AND ?", start, end).
		Group("user_id").
		Order("total_tokens DESC").
		Limit(limit).
		Scan(&stats).Error
	return stats, err
}

// DailyUsageTrend 每日用量趋势
type DailyUsageTrend struct {
	Date        string  `json:"date"`
	TotalTokens int64   `json:"total_tokens"`
	Cost        float64 `json:"cost"`
	CallCount   int64   `json:"call_count"`
}

// UsageTrend 按日聚合趋势
func (r *Repo) UsageTrend(ctx context.Context, start, end time.Time) ([]DailyUsageTrend, error) {
	var trend []DailyUsageTrend
	err := r.db.WithContext(ctx).Table("llm_usage_logs").
		Select("DATE(created_at) as date, COALESCE(SUM(total_tokens),0) as total_tokens, "+
			"COALESCE(SUM(cost),0) as cost, COUNT(*) as call_count").
		Where("created_at BETWEEN ? AND ?", start, end).
		Group("DATE(created_at)").
		Order("date").
		Scan(&trend).Error
	return trend, err
}
