package config

import (
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"os"
	"sync"
)

type Server struct {
	Host    string `mapstructure:"host"`
	Port    int    `mapstructure:"port"`
	Timeout int    `mapstructure:"timeout"`
	Workers int    `mapstructure:"workers"`
}

type LLMConfig struct {
	APIKey  string `mapstructure:"api_key"`
	APIBase string `mapstructure:"api_url"`
	Model   string `mapstructure:"model"`
}

type EmbeddingConfig struct {
	Enabled        bool   `mapstructure:"enabled"`
	APIKey         string `mapstructure:"api_key"`
	APIBase        string `mapstructure:"api_url"`
	Model          string `mapstructure:"model"`
	Dimension      int    `mapstructure:"dimension"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds"`
}

type MilvusConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	Address    string `mapstructure:"address"`
	Username   string `mapstructure:"username"`
	Password   string `mapstructure:"password"`
	DBName     string `mapstructure:"db_name"`
	Collection string `mapstructure:"collection"`
	Dimension  int    `mapstructure:"dimension"`
	MetricType string `mapstructure:"metric_type"`
	UseTLS     bool   `mapstructure:"use_tls"`
}

type RerankConfig struct {
	Enabled        bool    `mapstructure:"enabled"`
	APIKey         string  `mapstructure:"api_key"`
	APIBase        string  `mapstructure:"api_url"`
	Model          string  `mapstructure:"model"`
	TopK           int     `mapstructure:"top_k"`
	Threshold      float64 `mapstructure:"threshold"`
	TimeoutSeconds int     `mapstructure:"timeout_seconds"`
}

type VectorConfig struct {
	Enabled    bool             `mapstructure:"enabled"`
	Embedding  *EmbeddingConfig `mapstructure:"embedding"`
	Milvus     *MilvusConfig    `mapstructure:"milvus"`
	Rerank     *RerankConfig    `mapstructure:"rerank"`
	EnableBM25 bool             `mapstructure:"enable_bm25"`
}

type OSS struct {
	Endpoint   string `mapstructure:"endpoint"`
	AccessKey  string `mapstructure:"access_key"`
	SecretKey  string `mapstructure:"secret_key"`
	BucketName string `mapstructure:"bucket_name"`
}

type Mysql struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"db_name"`
	PoolSize int    `mapstructure:"pool_size"`
	Timeout  int    `mapstructure:"pool_timeout"`
}

type Redis struct {
	Host                 string `mapstructure:"host"`
	Port                 string `mapstructure:"port"`
	DB                   int    `mapstructure:"db"`
	Password             string `mapstructure:"password"`
	SocketConnectTimeout int    `mapstructure:"socket_connect_timeout"`
	SocketTimeout        int    `mapstructure:"socket_timeout"`
}

type JWTConfig struct {
	SecretKey              string `mapstructure:"secret_key"`
	RefreshSecretKey       string `mapstructure:"refresh_secret_key"`
	AccessTokenExpireMin   int    `mapstructure:"access_token_expire_minutes"`
	RefreshTokenExpireDays int    `mapstructure:"refresh_token_expire_days"`
}

type CASConfig struct {
	//// CAS认证中心地址（重邮统一身份认证地址）
	CasServerURL string `mapstructure:"cas_server_url"`
	// 你的服务回调地址（已在认证中心绑定）
	ServiceURL string `mapstructure:"service_url"`
	// Session加密密钥（生产环境替换为随机强密钥）
	SessionSecret string `mapstructure:"session_secret"`
	//Cookie名称
	SessionCookieName string `mapstructure:"session_cookie_name"`
}

type Config struct {
	Server    *Server       `mapstructure:"server"`
	LLMConfig *LLMConfig    `mapstructure:"llm_config"`
	Mysql     *Mysql        `mapstructure:"mysql"`
	Redis     *Redis        `mapstructure:"redis"`
	CasConfig *CASConfig    `mapstructure:"cas_config"`
	JWT       *JWTConfig    `mapstructure:"jwt"`
	OSS       *OSS          `mapstructure:"oss"`
	Vector    *VectorConfig `mapstructure:"vector"`
}

var (
	configOnce sync.Once
	config     *Config
)

func GetConfig() *Config {
	configOnce.Do(func() {
		debug := os.Getenv("DEBUG")

		configFile := "./config/conf-dev.yaml"
		if debug != "true" {
			configFile = "./config/conf-pro.yaml"
		}

		v := viper.New()
		v.SetConfigFile(configFile)
		v.SetConfigType("yaml")

		if err := v.ReadInConfig(); err != nil {
			zap.S().Fatalf("读取配置文件失败: %v", err)
		}

		config = &Config{}
		if err := v.Unmarshal(config); err != nil {
			zap.S().Fatalf("解析配置文件失败: %v", err)
		}
	})

	return config
}
