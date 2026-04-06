package jaeger

import (
	"context"
	"fmt"
	"github.com/opentracing/opentracing-go"
	olog "github.com/opentracing/opentracing-go/log"
	log "github.com/sirupsen/logrus"
)

// LogInfo 把日志打进context让jaeger收集 并输出到cli
func LogInfo(ctx context.Context, info string, addon ...interface{}) {
	info = fmt.Sprintf(info, addon...)
	span := opentracing.SpanFromContext(ctx)
	if span != nil {
		span.LogFields(olog.String("msg", info))
	}

	log.Info(info)
}

// LogErr 把日志打进context让jaeger收集 并输出到cli
func LogErr(ctx context.Context, info string, addon ...interface{}) {
	info = fmt.Sprintf(info, addon...)
	span := opentracing.SpanFromContext(ctx)
	if span != nil {
		span.LogFields(olog.String("err", info))
	}
	log.Error(info)
}
