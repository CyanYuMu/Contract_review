package breaker

import (
	"context"
	"fmt"
	"time"

	"github.com/afex/hystrix-go/hystrix"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
)

type GoBreaker struct {
	name string
}

type Logger struct {
}

func (l Logger) Printf(format string, items ...interface{}) {
	su_logger.Info(context.Background(), fmt.Sprintf(format, items...))
}

var (
	logInst = Logger{}
)

func init() {
	hystrix.SetLogger(logInst)
}

type Conf struct {
	// 单次操作的超时时间, 超过设定设定时间直接返回错误
	Timeout time.Duration
	// 相同命令(name)在同一时间的最大并发数
	MaxConcurrentRequests int
	// 触发熔断器检测的最小处理数量
	RequestVolumeThreshold int
	// 错误率阈值, 达到阈值, 启动熔断, n%
	ErrorPercentThreshold int
	// 当熔断开始后, 尝试恢复正常的探测周期
	// 放开对接口的限制（断路器状态 HALF-OPEN），然后尝试使用 1 个请求去调用接口，如果调用成功，则恢复正常（断路器状态 CLOSED），如果调用失败或出现超时等待，就需要再重新等待circuitGoBreakerSleepWindowInMilliseconds 的时间
	SleepWindow time.Duration
}

func NewGoBreaker(name string, conf Conf) Breaker {
	hystrix.ConfigureCommand(name, hystrix.CommandConfig{
		Timeout:                int(conf.Timeout.Milliseconds()),
		MaxConcurrentRequests:  conf.MaxConcurrentRequests,
		RequestVolumeThreshold: conf.RequestVolumeThreshold,
		SleepWindow:            int(conf.SleepWindow.Milliseconds()),
		ErrorPercentThreshold:  conf.ErrorPercentThreshold,
	})

	return &GoBreaker{
		name: name,
	}
}

func (b *GoBreaker) Do(ctx context.Context, run func(ctx context.Context) error, fallback func(context.Context, error) error) error {
	return hystrix.DoC(ctx, b.name, run, fallback)
}
