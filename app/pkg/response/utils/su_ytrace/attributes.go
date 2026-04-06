package su_ytrace

import (
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
)

// 通用属性键定义
const (
	// 数据库相关
	AttrDBSystem           = "db.system"
	AttrDBName             = "db.name"
	AttrDBOperation        = "db.operation"
	AttrDBStatement        = "db.statement"
	AttrDBTable            = "db.table"
	AttrDBConnectionString = "db.connection_string"

	// Redis 相关
	AttrRedisCommand     = "redis.command"
	AttrRedisPipelineLen = "redis.pipeline.length"
	AttrRedisKey         = "redis.key"

	// HTTP 相关
	AttrHTTPMethod     = "http.method"
	AttrHTTPURL        = "http.url"
	AttrHTTPStatusCode = "http.status_code"
	AttrHTTPHost       = "http.host"
	AttrHTTPScheme     = "http.scheme"
	AttrHTTPTarget     = "http.target"

	// 消息队列相关
	AttrMessagingSystem    = "messaging.system"
	AttrMessagingOperation = "messaging.operation"
	AttrMessagingTopic     = "messaging.destination"
	AttrMessagingMessageID = "messaging.message_id"
)

// DBAttributes 数据库通用属性
func DBAttributes(system, operation, table string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String(AttrDBSystem, system),
		attribute.String(AttrDBOperation, operation),
		attribute.String(AttrDBTable, table),
	}
}

// RedisAttributes Redis 通用属性
func RedisAttributes(command string, addr string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String(AttrDBSystem, "redis"),
		attribute.String(AttrRedisCommand, command),
		attribute.String(AttrDBConnectionString, addr),
	}
}

// HTTPClientAttributes HTTP 客户端属性
func HTTPClientAttributes(method, url string, statusCode int) []attribute.KeyValue {
	return []attribute.KeyValue{
		semconv.HTTPMethod(method),
		semconv.HTTPURL(url),
		semconv.HTTPStatusCode(statusCode),
	}
}

// HTTPServerAttributes HTTP 服务端属性
func HTTPServerAttributes(method, target, scheme, host string, statusCode int) []attribute.KeyValue {
	return []attribute.KeyValue{
		semconv.HTTPMethod(method),
		semconv.HTTPTarget(target),
		semconv.HTTPScheme(scheme),
		semconv.NetHostName(host),
		semconv.HTTPStatusCode(statusCode),
	}
}

// MessagingAttributes 消息队列通用属性
func MessagingAttributes(system, operation, topic, messageID string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String(AttrMessagingSystem, system),
		attribute.String(AttrMessagingOperation, operation),
		attribute.String(AttrMessagingTopic, topic),
		attribute.String(AttrMessagingMessageID, messageID),
	}
}
