package config_center

import (
	"context"
	"errors"
)

type ConfigCenter interface {
	Name() string
	Load(ctx context.Context, keys ...string) ([]byte, error)
	Save(ctx context.Context, key string, data string, opt *SaveOption) error
}

type SaveOption struct {
	Group string
	// 数据类型, 默认yaml
	DataType string
	// 不存在的时候才保存
	SetNX bool
}

func (s *SaveOption) GetGroup() string {
	if s == nil || s.Group == "" {
		return "DEFAULT_GROUP"
	}
	return s.Group
}

func (s *SaveOption) GetDataType() string {
	if s == nil || s.DataType == "" {
		return "yaml"
	}
	return s.DataType
}

var (
	ErrConfigExists = errors.New("config already exists")
)
