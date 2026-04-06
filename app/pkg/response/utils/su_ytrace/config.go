package su_ytrace

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// ExporterType 定义导出器类型
type ExporterType string

const (
	ExporterJaeger ExporterType = "jaeger"
	ExporterOTLP   ExporterType = "otlp"
	ExporterStdout ExporterType = "stdout"
)

// Config 追踪配置
type Config struct {
	Enabled     bool           `yaml:"enabled"`
	ServiceName string         `yaml:"service_name"`
	Exporter    ExporterType   `yaml:"exporter"`
	Jaeger      JaegerConfig   `yaml:"jaeger"`
	OTLP        OTLPConfig     `yaml:"otlp"`
	Sampling    SamplingConfig `yaml:"sampling"`
	Debug       bool           `yaml:"debug"` // 启用调试日志，打印导出统计信息
}

// JaegerConfig Jaeger 导出器配置
type JaegerConfig struct {
	Endpoint string `yaml:"endpoint"`
}

// OTLPConfig OTLP 导出器配置
type OTLPConfig struct {
	Endpoint string        `yaml:"endpoint"`
	Insecure bool          `yaml:"insecure"`
	Timeout  time.Duration `yaml:"timeout"`
}

// SamplingConfig 采样配置
type SamplingConfig struct {
	Ratio float64 `yaml:"ratio"` // 采样率 0.0-1.0
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Enabled:     true,
		ServiceName: "unknown-service",
		Exporter:    ExporterStdout,
		Jaeger: JaegerConfig{
			Endpoint: "http://localhost:14268/api/traces",
		},
		OTLP: OTLPConfig{
			Endpoint: "localhost:4317",
			Insecure: true,
			Timeout:  10 * time.Second,
		},
		Sampling: SamplingConfig{
			Ratio: 1.0,
		},
	}
}

// LoadConfig 从文件加载配置
func LoadConfig(path string) (*Config, error) {
	config := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		// 如果配置文件不存在，返回默认配置
		if os.IsNotExist(err) {
			return config, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return config, nil
}

// Validate 验证配置
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}

	if c.ServiceName == "" {
		return fmt.Errorf("service_name is required")
	}

	switch c.Exporter {
	case ExporterJaeger:
		if c.Jaeger.Endpoint == "" {
			return fmt.Errorf("jaeger.endpoint is required")
		}
	case ExporterOTLP:
		if c.OTLP.Endpoint == "" {
			return fmt.Errorf("otlp.endpoint is required")
		}
	case ExporterStdout:
		// stdout 不需要额外配置
	default:
		return fmt.Errorf("unsupported exporter: %s", c.Exporter)
	}

	if c.Sampling.Ratio < 0 || c.Sampling.Ratio > 1 {
		return fmt.Errorf("sampling.ratio must be between 0 and 1")
	}

	return nil
}
