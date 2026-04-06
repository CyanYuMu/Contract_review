package db_scanner

import (
	"context"
)

type Limiter interface {
	Wait(ctx context.Context) error
}

type Scanner interface {
	Pool(opt *PoolOption) *Base
	StopWhenErr(b bool) *Base
	PanicHandler(fn func(ctx context.Context)) *Base
	Counter(fn Counter) *Base
	ErrHandler(fn ErrHandler) *Base
	StashSize(n int) *Base
	Limiter(lmt Limiter) *Base
	Filter(fn Filter) *Base
	BeforeStartHook(fn func(ctx context.Context)) *Base
	FinishHook(fn func(ctx context.Context)) *Base
	//Scan(ctx context.Context, coll string, opt *ScanOption) (buff chan map[string]interface{})
	Run(ctx context.Context, handler Handler)
}
