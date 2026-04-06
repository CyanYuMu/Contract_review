package nsq

import (
	"time"
)

// Config NSQ配置
type Config struct {
	// NSQLookupd 地址列表
	LookupdAddresses []string
	// NSQd 地址列表 (直连模式)
	NSQdAddresses []string
	// 认证配置
	Auth *Auth
	// TLS配置
	TLS *TLSConfig
	// 连接配置
	Connection *ConnectionConfig
}

// Auth 认证配置
type Auth struct {
	// 认证密钥
	Secret string
}

// TLSConfig TLS配置
type TLSConfig struct {
	// 是否启用TLS
	Enabled bool
	// 是否跳过证书验证
	InsecureSkipVerify bool
	// 根证书文件路径
	RootCAFile string
	// 客户端证书文件路径
	CertFile string
	// 客户端私钥文件路径
	KeyFile string
}

// ConnectionConfig 连接配置
type ConnectionConfig struct {
	// 连接超时时间
	DialTimeout time.Duration
	// 读取超时时间
	ReadTimeout time.Duration
	// 写入超时时间
	WriteTimeout time.Duration
	// 心跳间隔
	HeartbeatInterval time.Duration
	// 消息超时时间
	MsgTimeout time.Duration
	// 最大重连间隔
	MaxRequeueDelay time.Duration
	// 默认重连延迟
	DefaultRequeueDelay time.Duration
	// 最大尝试次数
	MaxAttempts uint16
	// 低水位线
	LowRdyIdleTimeout time.Duration
	// 高水位线
	RDYRedistributeInterval time.Duration
	// 客户端ID
	ClientID string
	// 主机名
	Hostname string
	// 用户代理
	UserAgent string
}

// GetDefaultConnectionConfig 获取默认连接配置
func GetDefaultConnectionConfig() *ConnectionConfig {
	return &ConnectionConfig{
		DialTimeout:             time.Second,
		ReadTimeout:             60 * time.Second,
		WriteTimeout:            time.Second,
		HeartbeatInterval:       30 * time.Second,
		MsgTimeout:              60 * time.Second,
		MaxRequeueDelay:         15 * time.Minute,
		DefaultRequeueDelay:     90 * time.Second,
		MaxAttempts:             5,
		LowRdyIdleTimeout:       10 * time.Second,
		RDYRedistributeInterval: 5 * time.Second,
		ClientID:                "",
		Hostname:                "",
		UserAgent:               "seago-nsq-client/1.0",
	}
}
