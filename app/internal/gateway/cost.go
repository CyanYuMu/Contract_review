package gateway

import (
	"context"
	"time"

	"contract_review/app/internal/modelconfig"

	"go.uber.org/zap"
)

// logCostSafely 异步记录成本日志，绝不阻断主调用链。
// 采用独立 context（带超时）避免请求结束后 ctx 被取消导致写入丢失。
func (g *Gateway) logCostSafely(ctx context.Context, req GatewayRequest, cfg *modelconfig.ModelConfig, usage Usage, latencyMs int, status string, start time.Time) {
	if !g.config.EnableCostLog || g.db == nil {
		return
	}
	// 限流/配额拦截等无实际 usage 时，cost 记为 0
	cost := 0.0
	if usage.TotalTokens > 0 {
		cost = estimateCost(cfg.ProviderType, cfg.ModelName, usage)
	}
	errMsg := ""
	if len(status) > 6 && status[:6] == "error:" {
		errMsg = status[7:]
		status = "error"
	}
	log := &LLMUsageLog{
		UserID:           req.UserID,
		Account:          req.Account,
		Feature:          req.Feature,
		ModelName:        cfg.ModelName,
		Provider:         cfg.ProviderType,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
		Cost:             cost,
		LatencyMs:        latencyMs,
		CacheHit:         status == "cache_hit",
		Status:           status,
		Error:            truncate(errMsg, 500),
	}

	go func(l *LLMUsageLog) {
		defer func() {
			if r := recover(); r != nil {
				zap.L().Warn("成本日志写入 panic", zap.Any("recover", r))
			}
		}()
		writeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := g.repo.LogUsage(writeCtx, l); err != nil {
			zap.L().Warn("成本日志写入失败", zap.Error(err))
		}
	}(log)
}
