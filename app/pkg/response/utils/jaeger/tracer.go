package jaeger

import (
	"github.com/grpc-ecosystem/grpc-opentracing/go/otgrpc"
	"github.com/jinzhu/copier"
	"github.com/opentracing/opentracing-go"
	log "github.com/sirupsen/logrus"
	"github.com/uber/jaeger-client-go/config"
	"google.golang.org/grpc"
)

type Config struct {
	Sampler  JaegerSampler
	Reporter JaegerReporter
}

type JaegerSampler struct {
	Type  string
	Param float64
}
type JaegerReporter struct {
	AgentAddr string
}

type Jaeger struct {
	tc opentracing.Tracer
}

// InitTracer serviceName 服务名称， jaegerConfig jaeger初始化配置，传入值结构请与 Config 一致
func (t *Jaeger) InitTracer(serviceName string, jaegerConfig interface{}) {
	var cf Config
	err := copier.Copy(&cf, jaegerConfig)
	if err != nil {
		panic(err)
	}
	cfg := config.Configuration{
		Sampler: &config.SamplerConfig{
			Type:  cf.Sampler.Type,
			Param: cf.Sampler.Param,
		},
		Reporter: &config.ReporterConfig{
			LogSpans:           true,
			LocalAgentHostPort: cf.Reporter.AgentAddr,
		},
		ServiceName: serviceName,
	}
	// 设置服务名称
	// 创建tracer
	tracer, _, err := cfg.NewTracer()
	if err != nil {
		log.WithFields(log.Fields{"error": err.Error()}).Errorf("NewTracer失败")
	}
	t.tc = tracer
}

func (t *Jaeger) checkInit() {
	if t.tc == nil {
		panic("请先配置tracer")
	}
}

// GetUnaryInterceptor 获取grpc client 拦截器配置
func (t *Jaeger) GetUnaryInterceptor() grpc.UnaryClientInterceptor {
	t.checkInit()
	return otgrpc.OpenTracingClientInterceptor(t.tc, otgrpc.LogPayloads())
}

// GetStreamClientInterceptor 获取grpc client 拦截器配置
func (t *Jaeger) GetStreamClientInterceptor() grpc.StreamClientInterceptor {
	t.checkInit()
	return otgrpc.OpenTracingStreamClientInterceptor(t.tc, otgrpc.LogPayloads())
}

// GetServerUnaryInterceptor 获取grpc server 一元拦截器配置
func (t *Jaeger) GetServerUnaryInterceptor() grpc.UnaryServerInterceptor {
	t.checkInit()
	return otgrpc.OpenTracingServerInterceptor(t.tc)
}

// GetServerStreamInterceptor 获取grpc server 流拦截器配置
func (t *Jaeger) GetServerStreamInterceptor() grpc.StreamServerInterceptor {
	t.checkInit()
	return otgrpc.OpenTracingStreamServerInterceptor(t.tc)
}
