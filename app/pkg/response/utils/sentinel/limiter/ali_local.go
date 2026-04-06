package limiter

import (
	"context"
	goSentinel "github.com/alibaba/sentinel-golang/api"
	"github.com/alibaba/sentinel-golang/core/base"
	goSentinelConfig "github.com/alibaba/sentinel-golang/core/config"
	"github.com/alibaba/sentinel-golang/core/flow"
	"github.com/alibaba/sentinel-golang/logging"
)

type AliLocalLimiter struct {
	baseLimiter
}

func NewAliLocalLimiter() *AliLocalLimiter {
	return &AliLocalLimiter{}
}

type DftLog struct {
}

func (d *DftLog) Debug(msg string, keysAndValues ...interface{}) {

}
func (d *DftLog) DebugEnabled() bool {
	return false
}

func (d *DftLog) Info(msg string, keysAndValues ...interface{}) {

}

func (d *DftLog) InfoEnabled() bool {
	return false
}

func (d *DftLog) Warn(msg string, keysAndValues ...interface{}) {

}

func (d *DftLog) WarnEnabled() bool {
	return false
}

func (d *DftLog) Error(err error, msg string, keysAndValues ...interface{}) {
}

func (d *DftLog) ErrorEnabled() bool {
	return false
}

func (l *AliLocalLimiter) Init(cnf *Config) error {
	//TODO implement me
	c := &goSentinelConfig.Entity{
		Version: "1.0.0",
		Sentinel: goSentinelConfig.SentinelConfig{
			App: struct {
				Name string
				Type int32
			}{
				Name: cnf.Name,
				Type: 1,
			},
			Exporter: goSentinelConfig.ExporterConfig{},
			Log: goSentinelConfig.LogConfig{
				Logger: &DftLog{},
				Dir:    "/tmp",
				UsePid: false,
				Metric: goSentinelConfig.MetricLogConfig{
					SingleFileMaxSize: 2 << 32,
					MaxFileCount:      1,
					FlushIntervalSec:  0,
				},
			},
			Stat: goSentinelConfig.StatConfig{
				GlobalStatisticSampleCountTotal: 20,
				GlobalStatisticIntervalMsTotal:  10000,
				MetricStatisticSampleCount:      2,
				MetricStatisticIntervalMs:       1000,
				System: goSentinelConfig.SystemStatConfig{
					CollectIntervalMs:       1000,
					CollectLoadIntervalMs:   1000,
					CollectCpuIntervalMs:    1000,
					CollectMemoryIntervalMs: 150,
				},
			},
			UseCacheTime: true,
		},
	}
	logging.ResetGlobalLoggerLevel(logging.ErrorLevel)
	err := goSentinel.InitWithConfig(c)
	if err != nil {
		return err
	}
	err = l.constructor(cnf)

	return err
}

func (l *AliLocalLimiter) Take(ctx context.Context, uri string, source string) (entry *Entry, err error) {
	group, exists := l.uriMapping[uri]
	if !exists {
		return nil, SourceLimiterNotDefined
	}
	// 判断当前 source 是否已定义限流
	key := l.getKey(uri, source)

	if rule := flow.GetRulesOfResource(key); len(rule) == 0 {
		l.lock.Lock()
		defer l.lock.Unlock()
		if rule = flow.GetRulesOfResource(key); len(rule) == 0 {
			// 创建限流
			// 阿里的仅支持秒级别的限流, 所以需要将配置的限流转换为秒
			burst := group.Burst / group.PeriodSec
			_, err = flow.LoadRulesOfResource(key, []*flow.Rule{
				{
					Resource:               key,
					Threshold:              burst,
					TokenCalculateStrategy: flow.Direct,
					ControlBehavior:        flow.Reject,
				},
			})
			if err != nil {
				return
			}
		}
	}
	//TODO implement me
	sentinelEntry, blockError := goSentinel.Entry(key, goSentinel.WithTrafficType(base.Inbound))
	if blockError != nil {
		if group.ErrController != nil {
			group.ErrController(ctx, key, blockError)
		}

		return nil, blockError
	} else {
		return &Entry{
			tpe:   TypeAli,
			entry: sentinelEntry,
		}, nil
	}
}
