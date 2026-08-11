package gateway

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// ErrRateLimited 触发限流
var ErrRateLimited = errors.New("请求过于频繁，已触发限流")

// checkRateLimit 固定窗口限流：每用户+功能 每分钟 N 次
func (g *Gateway) checkRateLimit(ctx context.Context, userID uint64, feature string) error {
	if g.redis == nil {
		return nil
	}
	client := g.redis.Client()
	if client == nil {
		return nil
	}
	now := time.Now()
	windowKey := fmt.Sprintf("gw:rl:%d:%s:%d", userID, feature, now.Unix()/60)
	n, err := client.Incr(ctx, windowKey).Result()
	if err != nil {
		zap.L().Warn("限流计数失败，放行", zap.Error(err))
		return nil // 限流中间件失败不阻断主链路
	}
	if n == 1 {
		_ = client.Expire(ctx, windowKey, 70*time.Second).Err()
	}
	if n > int64(g.config.RateLimitPerMin) {
		return ErrRateLimited
	}
	return nil
}
