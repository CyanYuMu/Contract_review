package gateway

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// ErrQuotaExceeded 配额耗尽
var ErrQuotaExceeded = errors.New("token 配额已用尽，请联系管理员")

// checkQuota 调用前检查当日/当月 token 配额是否超限
func (g *Gateway) checkQuota(ctx context.Context, userID uint64, feature string) error {
	if g.redis == nil || userID == 0 {
		return nil
	}
	quotas, err := g.repo.GetQuotas(ctx, userID)
	if err != nil {
		zap.L().Warn("查询配额配置失败，放行", zap.Error(err))
		return nil
	}
	if len(quotas) == 0 {
		return nil // 未配置配额 = 不限
	}

	now := time.Now()
	dateKey := now.Format("20060102")
	monthKey := now.Format("200601")
	client := g.redis.Client()
	if client == nil {
		return nil
	}

	for _, q := range quotas {
		// 只校验 feature 匹配的（精确）或通配（feature 为空）的配额
		if q.Feature != "" && q.Feature != feature {
			continue
		}
		if q.DailyTokenLimit > 0 {
			used, _ := client.Get(ctx, dailyQuotaKey(userID, q.Feature, dateKey)).Int64()
			if used >= int64(q.DailyTokenLimit) {
				return ErrQuotaExceeded
			}
		}
		if q.MonthlyTokenLimit > 0 {
			used, _ := client.Get(ctx, monthlyQuotaKey(userID, q.Feature, monthKey)).Int64()
			if used >= int64(q.MonthlyTokenLimit) {
				return ErrQuotaExceeded
			}
		}
	}
	return nil
}

// deductQuota 调用后按实际 token 数扣减配额计数
func (g *Gateway) deductQuota(ctx context.Context, userID uint64, feature string, tokens int) {
	if g.redis == nil || userID == 0 || tokens <= 0 {
		return
	}
	client := g.redis.Client()
	if client == nil {
		return
	}
	now := time.Now()
	dateKey := now.Format("20060102")
	monthKey := now.Format("200601")
	// 同时累加 feature 维度与通配维度的计数
	keys := []string{
		dailyQuotaKey(userID, feature, dateKey),
		dailyQuotaKey(userID, "", dateKey),
		monthlyQuotaKey(userID, feature, monthKey),
		monthlyQuotaKey(userID, "", monthKey),
	}
	for _, k := range keys {
		if err := client.IncrBy(ctx, k, int64(tokens)).Err(); err != nil {
			zap.L().Warn("配额扣减失败", zap.String("key", k), zap.Error(err))
			continue
		}
	}
	// 设置过期，避免 key 堆积
	_ = client.Expire(ctx, dailyQuotaKey(userID, feature, dateKey), 48*time.Hour).Err()
	_ = client.Expire(ctx, dailyQuotaKey(userID, "", dateKey), 48*time.Hour).Err()
	_ = client.Expire(ctx, monthlyQuotaKey(userID, feature, monthKey), 35*24*time.Hour).Err()
	_ = client.Expire(ctx, monthlyQuotaKey(userID, "", monthKey), 35*24*time.Hour).Err()
}

func dailyQuotaKey(userID uint64, feature, date string) string {
	return fmt.Sprintf("gw:quota:daily:%d:%s:%s", userID, feature, date)
}

func monthlyQuotaKey(userID uint64, feature, month string) string {
	return fmt.Sprintf("gw:quota:monthly:%d:%s:%s", userID, feature, month)
}
