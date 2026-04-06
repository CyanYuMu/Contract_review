package breaker

import "context"

type Breaker interface {
	// run 是实际执行的函数，fallback 是熔断后的降级函数
	Do(ctx context.Context, run func(ctx context.Context) error, fallback func(context.Context, error) error) error
}
