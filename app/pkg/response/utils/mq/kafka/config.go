package kafka

import (
	"path/filepath"
	"strings"

	"github.com/confluentinc/confluent-kafka-go/kafka"
)

type Config struct {
	// 证书配置
	Cert *Cert
	// broker地址
	Brokers []string
}

func (c *Config) ToConfigMap() *kafka.ConfigMap {
	conf := &kafka.ConfigMap{
		"bootstrap.servers": strings.Join(c.Brokers, ","),
	}

	if c.Cert != nil {
		// 1. 优先使用 PEM 格式的证书内容（字符串形式）
		if c.Cert.CACertPEM != "" {
			conf.SetKey("ssl.ca.pem", c.Cert.CACertPEM)
		} else if c.Cert.CACert != "" {
			// 2. 根据文件后缀自动识别格式
			if c.Cert.isPEMFormat(c.Cert.CACert) {
				conf.SetKey("ssl.ca.pem", c.Cert.CACert)
			} else {
				conf.SetKey("ssl.ca.location", c.Cert.CACert)
			}
		} else if c.Cert.CertFile != "" {
			// 3. 兼容旧版本配置
			conf.SetKey("ssl.ca.location", c.Cert.CertFile)
		}

		// 客户端证书配置
		if c.Cert.ClientCertPEM != "" {
			conf.SetKey("ssl.certificate.pem", c.Cert.ClientCertPEM)
		} else if c.Cert.ClientCert != "" {
			if c.Cert.isPEMFormat(c.Cert.ClientCert) {
				conf.SetKey("ssl.certificate.pem", c.Cert.ClientCert)
			} else {
				conf.SetKey("ssl.certificate.location", c.Cert.ClientCert)
			}
		}

		// 客户端私钥配置
		if c.Cert.ClientKeyPEM != "" {
			conf.SetKey("ssl.key.pem", c.Cert.ClientKeyPEM)
		} else if c.Cert.ClientKey != "" {
			if c.Cert.isPEMFormat(c.Cert.ClientKey) {
				conf.SetKey("ssl.key.pem", c.Cert.ClientKey)
			} else {
				conf.SetKey("ssl.key.location", c.Cert.ClientKey)
			}
		}

		// 私钥密码
		if c.Cert.KeyPassword != "" {
			conf.SetKey("ssl.key.password", c.Cert.KeyPassword)
		}

		// SASL 认证信息
		if c.Cert.Username != "" {
			conf.SetKey("sasl.username", c.Cert.Username)
		}
		if c.Cert.Password != "" {
			conf.SetKey("sasl.password", c.Cert.Password)
		}

		// SSL 配置
		conf.SetKey("ssl.endpoint.identification.algorithm", c.Cert.GetAlgorithm())
		conf.SetKey("security.protocol", c.Cert.GetProtocol())
		conf.SetKey("sasl.mechanism", c.Cert.GetMechanism())

	}

	return conf
}

type Cert struct {
	// === 简化的证书配置 (自动识别格式) ===
	// * CA证书 (支持文件路径或PEM内容)
	CACert string
	// * 客户端证书 (支持文件路径或PEM内容)
	ClientCert string
	// * 客户端私钥 (支持文件路径或PEM内容)
	ClientKey string

	// === PEM 格式证书内容配置 (字符串形式，优先级最高) ===
	// * CA 证书 PEM 内容
	CACertPEM string
	// * 客户端证书 PEM 内容
	ClientCertPEM string
	// * 客户端私钥 PEM 内容
	ClientKeyPEM string

	// === 兼容性配置 (保持向后兼容) ===
	// * 旧版本证书文件配置
	CertFile string

	// === 认证信息 ===
	// * 私钥密码
	KeyPassword string
	// * SASL 用户名
	Username string
	// * SASL 密码
	Password string

	// === SSL 配置选项 ===
	// ? 证书验证算法, default none
	Algorithm string
	// ? SASL 机制, default PLAIN
	Mechanism string
	// ? 安全协议, default SASL_SSL
	Protocol string
}

// isPEMFormat 判断是否为PEM格式内容或PEM文件
func (c *Cert) isPEMFormat(content string) bool {
	// 1. 如果包含PEM标识符，则为PEM内容
	if strings.Contains(content, "-----BEGIN") && strings.Contains(content, "-----END") {
		return true
	}

	// 2. 如果是文件路径，根据扩展名判断
	ext := strings.ToLower(filepath.Ext(content))
	switch ext {
	case ".pem":
		return false // 文件路径，使用location配置
	case ".crt", ".cer", ".der", ".key":
		return false // 文件路径，使用location配置
	default:
		// 3. 如果没有扩展名但看起来像路径，默认为文件路径
		if strings.Contains(content, "/") || strings.Contains(content, "\\") {
			return false
		}
		// 4. 其他情况（可能是PEM内容但没有标识符），默认为文件路径
		return false
	}
}

func (c *Cert) GetAlgorithm() string {
	if c.Algorithm == "" {
		return "none"
	}
	return c.Algorithm
}

func (c *Cert) GetProtocol() string {
	if c.Protocol == "" {
		return "SASL_SSL"
	}
	return c.Protocol
}

func (c *Cert) GetMechanism() string {
	if c.Mechanism == "" {
		return "PLAIN"
	}
	return c.Mechanism
}
