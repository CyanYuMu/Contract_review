package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/panjf2000/ants/v2"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
)

func Recover(ctx context.Context, module string, data interface{}) {
	if err := recover(); err != nil {
		su_logger.Panic(ctx, module, su_logger.E().Interface("err", err).Interface("data", data))
	}
}

func Go(ctx context.Context, tag string, f func()) {
	go func(ctx context.Context) {
		defer func() {
			if err := recover(); err != nil {
				su_logger.Panic(ctx, tag, su_logger.E().Interface("err", err))
			}
		}()
		f()
	}(ctx)
}

func GoWithNotify(ctx context.Context, tag string, f func()) {
	go func(ctx context.Context) {
		defer func() {
			if err := recover(); err != nil {
				su_logger.PanicWithNotify(ctx, tag, su_logger.E().Interface("err", err))
			}

			f()
		}()
	}(ctx)
}

type PoolConfig struct {
	// * 协程池大小
	Size int
	// * 处理句柄
	Handler func(interface{})
	// * 当任务队列满时, 是否阻塞, 默认 false
	Blocking bool
	// ? 最大阻塞的任务量, 当 Nonblocking 设置为true, 此参数无效
	MaxBlockingTasks int
	// ? 过期时间, 默认1h
	ExpiryDuration time.Duration
	// ? 是否预分配
	PreAlloc bool

	// ? panic 处理句柄
	PanicHandler func(interface{})
	// ? 自定义日志打印
	logger ants.Logger
}

func dftPanicHandler(module string) (f func(interface{})) {
	return func(i interface{}) {
		su_logger.Panic(nil, module, su_logger.E().Interface("msg", i))
	}
}

type poolLogger struct {
	ctx    context.Context
	module string
}

func (l poolLogger) Printf(format string, args ...interface{}) {
	su_logger.Info(l.ctx, fmt.Sprintf(format, args...), su_logger.E().String("module", l.module))
}

func Pool(ctx context.Context, module string, cfg PoolConfig) *ants.PoolWithFunc {
	opt := ants.Options{
		ExpiryDuration:   cfg.ExpiryDuration,
		PreAlloc:         cfg.PreAlloc,
		MaxBlockingTasks: cfg.MaxBlockingTasks,
		Nonblocking:      !cfg.Blocking,
		PanicHandler:     nil,
		Logger:           nil,
	}

	if cfg.ExpiryDuration <= 0 {
		cfg.ExpiryDuration = time.Hour
	}

	if cfg.MaxBlockingTasks <= 0 {
		cfg.MaxBlockingTasks = 10000
	}

	opt.ExpiryDuration = cfg.ExpiryDuration

	if cfg.PanicHandler == nil {
		cfg.PanicHandler = dftPanicHandler(module)
	}
	opt.PanicHandler = cfg.PanicHandler

	if cfg.logger == nil {
		cfg.logger = poolLogger{ctx: ctx, module: module}
	}
	opt.Logger = cfg.logger

	options := ants.WithOptions(opt)
	pool, err := ants.NewPoolWithFunc(cfg.Size, cfg.Handler, options)
	if err != nil {
		panic(err)
	}

	return pool
}
