package cache

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

// 定义通用的命令结果接口
type Cmder interface {
	Err() error
	String() string
}

// StringCmd 接口定义字符串命令的结果
type StringCmd interface {
	Cmder
	Result() (string, error)
}

// StringStringMapCmd 接口定义哈希命令的结果
type StringStringMapCmd interface {
	Cmder
	Result() (map[string]string, error)
}

// StringSliceCmd 接口定义切片命令的结果
type StringSliceCmd interface {
	Cmder
	Result() ([]string, error)
}

type IntCmd interface {
	Cmder
	Result() (int64, error)
}

type TimeCmd interface {
	Cmder
	Result() (time.Duration, error)
}

type BoolCmd interface {
	Cmder
	Result() (bool, error)
}

// RedisPipeline 定义通用的Pipeline接口
type Pipeline interface {
	Get(ctx context.Context, key string) StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) Cmder
	TTL(ctx context.Context, key string) TimeCmd
	Del(ctx context.Context, key string) Cmder
	Exists(ctx context.Context, keys ...string) IntCmd
	ZAdd(ctx context.Context, key string, members ...Z) Cmder
	ZCard(ctx context.Context, key string) IntCmd
	ZRem(ctx context.Context, key string, members ...interface{}) Cmder
	ZRange(ctx context.Context, key string, start, stop int64) StringSliceCmd
	Expire(ctx context.Context, key string, expiration time.Duration) Cmder
	HGetAll(ctx context.Context, key string) StringStringMapCmd
	HMSet(ctx context.Context, key string, values ...interface{}) Cmder
	HSet(ctx context.Context, key string, values ...interface{}) Cmder
	HMGet(ctx context.Context, key string, fields ...string) StringSliceCmd
	HDel(ctx context.Context, key string, fields ...string) Cmder
	LPush(ctx context.Context, key string, values ...interface{}) Cmder
	LPop(ctx context.Context, key string) StringCmd
	LLen(ctx context.Context, key string) IntCmd
	LPopCount(ctx context.Context, key string, count int) StringSliceCmd
	RPopCount(ctx context.Context, key string, count int) StringSliceCmd
	RPush(ctx context.Context, key string, values ...interface{}) Cmder
	RPop(ctx context.Context, key string) StringCmd
	SAdd(ctx context.Context, key string, members ...interface{}) Cmder
	SRem(ctx context.Context, key string, members ...interface{}) Cmder
	SCard(ctx context.Context, key string) IntCmd
	Exec(ctx context.Context) ([]Cmder, error)
}

// cmdWrapper 基础命令包装器
type cmdWrapper struct {
	err error
}

func (c *cmdWrapper) Err() error {
	return c.err
}

func (c *cmdWrapper) String() string {
	return ""
}

// boolCmdWrapper bool 命令包装器
type boolCmdWrapper struct {
	cmd interface {
		Result() (int64, error)
	}
}

func (w *boolCmdWrapper) Err() error {
	_, err := w.cmd.Result()
	return err
}

func (w *boolCmdWrapper) String() string {
	v, _ := w.Result()
	return strconv.FormatBool(v)
}

func (w *boolCmdWrapper) Result() (bool, error) {
	n, err := w.cmd.Result()
	return n > 0, err
}

// stringSliceCmdWrapper 字符串切片命令包装器
type stringSliceCmdWrapper struct {
	cmd interface {
		Err() error
		String() string
		Result() ([]string, error)
	}
}

func (w *stringSliceCmdWrapper) Err() error {
	return w.cmd.Err()
}

func (w *stringSliceCmdWrapper) String() string {
	return w.cmd.String()
}

func (w *stringSliceCmdWrapper) Result() ([]string, error) {
	return w.cmd.Result()
}

// cmderWrapper Cmder 包装器
type cmderWrapper struct {
	cmd interface {
		Err() error
		String() string
	}
}

func (w *cmderWrapper) Err() error {
	return w.cmd.Err()
}

func (w *cmderWrapper) String() string {
	return w.cmd.String()
}

// stringCmdWrapper 字符串命令包装器
type stringCmdWrapper struct {
	cmd interface {
		Err() error
		String() string
		Result() (string, error)
	}
}

func (w *stringCmdWrapper) Err() error              { return w.cmd.Err() }
func (w *stringCmdWrapper) String() string          { return w.cmd.String() }
func (w *stringCmdWrapper) Result() (string, error) { return w.cmd.Result() }

// intCmdWrapper 整数命令包装器
type intCmdWrapper struct {
	cmd interface {
		Err() error
		String() string
		Result() (int64, error)
	}
}

func (w *intCmdWrapper) Err() error             { return w.cmd.Err() }
func (w *intCmdWrapper) String() string         { return w.cmd.String() }
func (w *intCmdWrapper) Result() (int64, error) { return w.cmd.Result() }

// timeCmdWrapper 时间命令包装器
type timeCmdWrapper struct {
	cmd interface {
		Err() error
		String() string
		Result() (time.Duration, error)
	}
}

func (w *timeCmdWrapper) Err() error                     { return w.cmd.Err() }
func (w *timeCmdWrapper) String() string                 { return w.cmd.String() }
func (w *timeCmdWrapper) Result() (time.Duration, error) { return w.cmd.Result() }

// stringStringMapCmdWrapper 哈希命令包装器
type stringStringMapCmdWrapper struct {
	cmd interface {
		Err() error
		String() string
		Result() (map[string]string, error)
	}
}

func (w *stringStringMapCmdWrapper) Err() error                         { return w.cmd.Err() }
func (w *stringStringMapCmdWrapper) String() string                     { return w.cmd.String() }
func (w *stringStringMapCmdWrapper) Result() (map[string]string, error) { return w.cmd.Result() }

// sliceCmdWrapper 用于处理 HMGet 的返回值
type sliceCmdWrapper struct {
	cmd interface {
		Err() error
		String() string
		Result() ([]interface{}, error)
	}
}

func (w *sliceCmdWrapper) Err() error     { return w.cmd.Err() }
func (w *sliceCmdWrapper) String() string { return w.cmd.String() }
func (w *sliceCmdWrapper) Result() ([]string, error) {
	vals, err := w.cmd.Result()
	if err != nil {
		return nil, err
	}

	results := make([]string, len(vals))
	for i, val := range vals {
		if val == nil {
			results[i] = ""
			continue
		}
		results[i] = fmt.Sprint(val)
	}
	return results, nil
}
