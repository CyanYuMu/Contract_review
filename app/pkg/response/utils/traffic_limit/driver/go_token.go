package driver

import (
	"time"

	error2 "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/error"
)

type TokenParam struct {
	// 桶大小
	Burst        int
	RateInSecond float64
	Key          string
}

type TokenLimiter struct {
	rate      float64
	burst     int
	tokens    int
	lastToken time.Time
	key       string
}

func NewTokenLimiter(param TokenParam) *TokenLimiter {
	return &TokenLimiter{
		rate:      param.RateInSecond,
		burst:     param.Burst,
		tokens:    param.Burst,
		lastToken: time.Now(),
		key:       param.Key,
	}
}

func (tb *TokenLimiter) Init() error {
	if tb.burst < 1 {
		return error2.NewSUError(500402, "TokenLimiter: capacity is too small")
	}
	if tb.rate < 1 {
		return error2.NewSUError(500403, "TokenLimiter: rate is too small")
	}
	if tb.key == "" {
		return error2.NewSUError(500401, "TokenLimiter: key is empty")
	}
	return nil
}

func (tb *TokenLimiter) Acquire(timeout time.Duration) (bool, error) {
	deadline := time.Now().Add(timeout)
	for {
		if tb.tryAcquire() {
			return true, nil
		}
		if timeout > 0 && time.Now().After(deadline) {
			return false, error2.NewSUError(500405, "TokenLimiter: acquire timeout")
		}
		time.Sleep(time.Millisecond)
	}
}

func (tb *TokenLimiter) TryAcquire() (time.Duration, bool, error) {
	if tb.tryAcquire() {
		return 0, true, nil
	}
	return time.Second / time.Duration(tb.rate), false, nil
}

func (tb *TokenLimiter) Wait() error {
	_, err := tb.Acquire(0)
	return err
}

func (tb *TokenLimiter) tryAcquire() bool {
	now := time.Now()
	elapsed := now.Sub(tb.lastToken)

	newTokens := int(float64(elapsed) / (1e9 / tb.rate))
	if newTokens > 0 {
		tb.lastToken = now
	}
	tb.tokens += newTokens
	if tb.tokens > tb.burst {
		tb.tokens = tb.burst
	}

	if tb.tokens > 0 {
		tb.tokens--
		return true
	}
	return false
}
