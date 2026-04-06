package config_center

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// Unmarshal 将YAML配置解析到结构体中，支持大小驼峰不敏感匹配
func Unmarshal(in []byte, out interface{}) error {
	yaml.Unmarshal(in, out)

	// 创建一个新的Viper实例
	v := viper.New()

	// 配置Viper实例
	v.SetConfigType("yaml") // 指定配置类型为YAML
	// v.SetKeysCaseSensitive(false) // 设置键名不区分大小写

	// 从字节流读取配置
	if err := v.ReadConfig(bytes.NewBuffer(in)); err != nil {
		return fmt.Errorf("读取YAML配置失败: %w", err)
	}

	// 将配置映射到输出结构体
	if err := v.Unmarshal(out); err != nil {
		return fmt.Errorf("解析配置到结构体失败: %w", err)
	}

	return nil
}

// Marshal 将数据结构序列化为YAML格式
func Marshal(data interface{}) ([]byte, error) {
	// 检查参数是否为nil
	if data == nil {
		return nil, errors.New("failed to marshal config: data is nil")
	}

	return yaml.Marshal(data)
}
