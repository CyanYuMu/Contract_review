package config_center

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/nacos-group/nacos-sdk-go/clients"
	"github.com/nacos-group/nacos-sdk-go/clients/config_client"
	"github.com/nacos-group/nacos-sdk-go/common/constant"
	"github.com/nacos-group/nacos-sdk-go/vo"
	"github.com/spf13/cast"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
)

type NacosConfig struct {
	// * 命名空间
	Namespace string
	// * 用户名
	UserName string
	// * 密码
	Password string
	// * 服务器列表
	Servers []string
	// ? 组名, default: DEFAULT_GROUP
	// GroupName string `yaml:"group_name"`
	// ? 超时时间
	TimeoutMs uint64
}

// NacosConfigCenter implements ConfigCenter interface for Nacos
type NacosConfigCenter struct {
	clientConfig  constant.ClientConfig
	ServerConfigs []constant.ServerConfig
	client        config_client.IConfigClient
}

func (n *NacosConfigCenter) Name() string {
	return "nacos center"
}

// ConvertNacosConfig converts BaseConfig's Nacos configuration into Nacos client and server configs
func (bc *NacosConfig) ConvertNacosConfig() ([]constant.ServerConfig, *constant.ClientConfig, error) {
	// Convert server addresses to ServerConfig
	serverConfigs := make([]constant.ServerConfig, 0, len(bc.Servers))
	for _, server := range bc.Servers {
		// services := strings.Split(server, ":")
		// if len(services) != 2 {
		// 	return nil, nil, fmt.Errorf("invalid nacos server format: %s", server)
		// }
		parseData, err := url.Parse(server)
		if err != nil {
			return nil, nil, err
		}
		port := parseData.Port()
		if port == "" {
			if parseData.Scheme == "https" {
				port = "443"
			} else {
				port = "80"
			}
		}
		// 处理contextPath，确保只使用基础路径前缀
		var contextPath string
		if parseData.Path != "" {
			// 如果路径以/nacos开头，只取/nacos作为contextPath
			if strings.HasPrefix(parseData.Path, "/nacos") {
				contextPath = "/nacos"
			} else {
				// 否则使用第一级路径作为contextPath
				pathParts := strings.SplitN(parseData.Path, "/", 3)
				if len(pathParts) >= 2 {
					contextPath = "/" + pathParts[1]
				} else {
					contextPath = parseData.Path
				}
			}
		} else {
			contextPath = "/nacos"
		}

		// 从Host中提取域名部分，不包含端口
		hostOnly := parseData.Hostname()

		su_logger.Debug(context.Background(), "配置Nacos服务器",
			su_logger.E().
				String("host", hostOnly).
				String("port", port).
				String("contextPath", contextPath).
				String("scheme", parseData.Scheme))

		serverConfigs = append(serverConfigs, constant.ServerConfig{
			IpAddr:      hostOnly,            // 只使用主机名部分，例如 nacos.ops.seaart.dev
			Port:        cast.ToUint64(port), // 端口部分
			ContextPath: contextPath,
			Scheme:      parseData.Scheme,
		})
	}

	// Create ClientConfig
	clientConfig := &constant.ClientConfig{
		NotLoadCacheAtStart: true,
		NamespaceId:         bc.Namespace,
		TimeoutMs:           bc.TimeoutMs,
		Username:            bc.UserName,
		Password:            bc.Password,
		AppendToStdout:      true,
		CacheDir:            "/tmp",
		CustomLogger:        &NacosLogger{},
	}

	return serverConfigs, clientConfig, nil
}

// NacosLogger 实现了 nacos-sdk-go 的 logger.Logger 接口，使用 su_logger 作为底层日志实现
type NacosLogger struct{}

// Info 实现 logger.Logger 接口的 Info 方法
func (l *NacosLogger) Info(args ...interface{}) {
	su_logger.Info(context.Background(), fmt.Sprint(args...))
}

// Warn 实现 logger.Logger 接口的 Warn 方法
func (l *NacosLogger) Warn(args ...interface{}) {
	su_logger.Warn(context.Background(), fmt.Sprint(args...))
}

// Error 实现 logger.Logger 接口的 Error 方法
func (l *NacosLogger) Error(args ...interface{}) {
	su_logger.Error(context.Background(), nil, fmt.Sprint(args...))
}

// Debug 实现 logger.Logger 接口的 Debug 方法
func (l *NacosLogger) Debug(args ...interface{}) {
	su_logger.Debug(context.Background(), fmt.Sprint(args...))
}

// Infof 实现 logger.Logger 接口的 Infof 方法
func (l *NacosLogger) Infof(format string, args ...interface{}) {
	su_logger.Info(context.Background(), fmt.Sprintf(format, args...))
}

// Warnf 实现 logger.Logger 接口的 Warnf 方法
func (l *NacosLogger) Warnf(format string, args ...interface{}) {
	su_logger.Warn(context.Background(), fmt.Sprintf(format, args...))
}

// Errorf 实现 logger.Logger 接口的 Errorf 方法
func (l *NacosLogger) Errorf(format string, args ...interface{}) {
	su_logger.Error(context.Background(), nil, fmt.Sprintf(format, args...))
}

// Debugf 实现 logger.Logger 接口的 Debugf 方法
func (l *NacosLogger) Debugf(format string, args ...interface{}) {
	su_logger.Debug(context.Background(), fmt.Sprintf(format, args...))
}

// NewNacosConfigCenter creates a new NacosConfigCenter instance
func NewNacosConfigCenter(config *NacosConfig) (*NacosConfigCenter, error) {
	serverConfigs, clientConfig, err := config.ConvertNacosConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to convert nacos config: %w", err)
	}
	// create nacos instance client
	client, err := clients.CreateConfigClient(map[string]interface{}{
		"serverConfigs": serverConfigs,
		"clientConfig":  *clientConfig,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create nacos config client: %w", err)
	}

	return &NacosConfigCenter{
		clientConfig:  *clientConfig,
		ServerConfigs: serverConfigs,
		client:        client,
	}, nil
}

// Load 加载配置并组装成合法的YAML格式
func (n *NacosConfigCenter) Load(ctx context.Context, keys ...string) ([]byte, error) {
	if len(keys) == 0 {
		return nil, fmt.Errorf("config keys is empty")
	}

	group := "DEFAULT_GROUP"

	// 用于存储每个key对应的配置内容
	results := make(map[string]string, len(keys))

	// 顺序获取配置
	for _, key := range keys {
		// 从Nacos获取配置
		content, err := n.client.GetConfig(vo.ConfigParam{
			DataId: key,
			Group:  group,
		})
		// if key == "redisTranslate" {
		// 	fmt.Println("--------------------------------")
		// 	fmt.Println("content: ", content, err)
		// 	fmt.Println("--------------------------------")
		// }

		if err != nil {
			return nil, fmt.Errorf("get config from nacos failed, key: %s, group: %s: %w", key, group, err)
		}

		// 保存结果
		results[key] = content
	}

	// 按原始keys的顺序组装YAML
	var sb strings.Builder

	for _, key := range keys {
		content, exists := results[key]
		if !exists || content == "" {
			continue
		}

		// 添加模块顶级键
		sb.WriteString(fmt.Sprintf("%s:\n", key))

		// 处理内容，对每一行添加两个缩进
		lines := strings.Split(content, "\n")
		for _, line := range lines {
			// 跳过空行
			if strings.TrimSpace(line) == "" {
				sb.WriteString("\n")
				continue
			}

			// 为每行添加两个缩进（两个空格）
			sb.WriteString(fmt.Sprintf("  %s\n", line))
		}

		// 在模块之间添加空行，增强可读性
		sb.WriteString("\n")
	}

	result := sb.String()
	if result == "" {
		return nil, fmt.Errorf("no valid configuration found for keys: %v in group: %s", keys, group)
	}

	return []byte(result), nil
}

func IsNotExists(err error) bool {
	if err == nil {
		return false
	}
	errS := err.Error()
	return strings.Contains(errS, "read config") || strings.Contains(errS, "404") || strings.Contains(errS, "no valid")
}

func (n *NacosConfigCenter) Save(ctx context.Context, key string, data string, opt *SaveOption) error {
	if opt == nil {
		opt = &SaveOption{}
	}
	if opt.SetNX {
		_, err := n.Load(ctx, key)
		// read config from both server and cache fail
		// 不存在的错误，不认为是错误
		if err != nil {
			if !IsNotExists(err) {
				return fmt.Errorf("failed to check if config exists: %w", err)
			}
			err = nil
		}
	}
	_, err := n.client.PublishConfig(vo.ConfigParam{
		DataId:  key,
		Group:   opt.GetGroup(),
		Content: data,
		Type:    vo.ConfigType(opt.GetDataType()),
	})
	if err != nil {
		return fmt.Errorf("failed to save config to nacos: %w", err)
	}
	return nil
}
