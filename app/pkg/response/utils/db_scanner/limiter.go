package db_scanner

import (
	"context"
	"time"
)

type LimiterOpt struct {
	// 桶大小
	Burst int
}

type TokenLimiter struct {
	rate      float64
	burst     int
	tokens    int
	lastToken time.Time
}

func NewTokenLimiter(rate float64, burst int) *TokenLimiter {
	return &TokenLimiter{
		rate:      rate,
		burst:     burst,
		tokens:    burst,
		lastToken: time.Now(),
	}
}

// func (tb *TokenLimiter) refillTokens() {
// 	for {
// 		time.Sleep(time.Second / time.Duration(tb.rate))
// 		tb.tokens = tb.burst
// 		tb.lastToken = time.Now()
// 	}
// }

func (tb *TokenLimiter) Allow() bool {
	now := time.Now()
	elapsed := now.Sub(tb.lastToken)

	nT := int(float64(elapsed) / (1e9 / tb.rate))
	if nT > 0 {
		tb.lastToken = now
	}
	tb.tokens += nT
	if tb.tokens > tb.burst {
		tb.tokens = tb.burst
	}

	if tb.tokens > 0 {
		tb.tokens--
		return true
	}
	return false
}

func (tb *TokenLimiter) Wait(ctx context.Context) error {
	for !tb.Allow() {
		time.Sleep(time.Millisecond)
	}

	return nil
}

func NewRateLimiter(limitPerSecond int, opt *LimiterOpt) *TokenLimiter {
	burst := limitPerSecond
	if opt != nil {
		burst = opt.Burst
	}
	limiter := NewTokenLimiter(float64(limitPerSecond), burst)
	return limiter
}
