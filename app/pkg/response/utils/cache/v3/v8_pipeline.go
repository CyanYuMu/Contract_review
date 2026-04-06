package cache

import (
	"context"
	"time"

	redis "github.com/go-redis/redis/v8"
)

// V8Pipeline Redis v8 版本的 Pipeline 实现
type V8Pipeline struct {
	pipeline redis.Pipeliner
}

// NewV8Pipeline 创建新的 V8Pipeline 实例
func NewV8Pipeline(p redis.Pipeliner) *V8Pipeline {
	return &V8Pipeline{pipeline: p}
}

// Get 实现 Get 命令
func (p *V8Pipeline) Get(ctx context.Context, key string) StringCmd {
	return &stringCmdWrapper{cmd: p.pipeline.Get(ctx, key)}
}

// Set 实现 Set 命令
func (p *V8Pipeline) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) Cmder {
	return p.pipeline.Set(ctx, key, value, expiration)
}

// TTL 实现 TTL 命令
func (p *V8Pipeline) TTL(ctx context.Context, key string) TimeCmd {
	return &timeCmdWrapper{cmd: p.pipeline.TTL(ctx, key)}
}

// Del 实现 Del 命令
func (p *V8Pipeline) Del(ctx context.Context, key string) Cmder {
	return p.pipeline.Del(ctx, key)
}

// Exists 实现 Exists 命令
func (p *V8Pipeline) Exists(ctx context.Context, keys ...string) IntCmd {
	cmd := p.pipeline.Exists(ctx, keys...)
	return &intCmdWrapper{cmd: cmd}
}

// ZAdd 实现 ZAdd 命令
func (p *V8Pipeline) ZAdd(ctx context.Context, key string, members ...Z) Cmder {
	v8Members := make([]*redis.Z, len(members))
	for i, m := range members {
		v8Members[i] = &redis.Z{
			Score:  m.Score,
			Member: m.Member,
		}
	}
	return p.pipeline.ZAdd(ctx, key, v8Members...)
}

// ZCard 实现 ZCard 命令
func (p *V8Pipeline) ZCard(ctx context.Context, key string) IntCmd {
	return &intCmdWrapper{cmd: p.pipeline.ZCard(ctx, key)}
}

// ZRem 实现 ZRem 命令
func (p *V8Pipeline) ZRem(ctx context.Context, key string, members ...interface{}) Cmder {
	return p.pipeline.ZRem(ctx, key, members...)
}

// ZRange 实现 ZRange 命令
func (p *V8Pipeline) ZRange(ctx context.Context, key string, start, stop int64) StringSliceCmd {
	return p.pipeline.ZRange(ctx, key, start, stop)
}

// Expire 实现 Expire 命令
func (p *V8Pipeline) Expire(ctx context.Context, key string, expiration time.Duration) Cmder {
	return p.pipeline.Expire(ctx, key, expiration)
}

// HGetAll 实现 HGetAll 命令
func (p *V8Pipeline) HGetAll(ctx context.Context, key string) StringStringMapCmd {
	return &stringStringMapCmdWrapper{cmd: p.pipeline.HGetAll(ctx, key)}
}

// HMSet 实现 HMSet 命令
func (p *V8Pipeline) HMSet(ctx context.Context, key string, values ...interface{}) Cmder {
	return p.pipeline.HMSet(ctx, key, values...)
}

// HSet 实现 HSet 命令
func (p *V8Pipeline) HSet(ctx context.Context, key string, values ...interface{}) Cmder {
	return p.pipeline.HSet(ctx, key, values...)
}

// HMGet 实现 HMGet 命令
func (p *V8Pipeline) HMGet(ctx context.Context, key string, fields ...string) StringSliceCmd {
	return &sliceCmdWrapper{cmd: p.pipeline.HMGet(ctx, key, fields...)}
}

// HDel 实现 HDel 命令
func (p *V8Pipeline) HDel(ctx context.Context, key string, fields ...string) Cmder {
	return p.pipeline.HDel(ctx, key, fields...)
}

// LPush 实现 LPush 命令
func (p *V8Pipeline) LPush(ctx context.Context, key string, values ...interface{}) Cmder {
	return p.pipeline.LPush(ctx, key, values...)
}

// LPop 实现 LPop 命令
func (p *V8Pipeline) LPop(ctx context.Context, key string) StringCmd {
	return &stringCmdWrapper{cmd: p.pipeline.LPop(ctx, key)}
}

// LPopCount 实现 LPopCount 命令
func (p *V8Pipeline) LPopCount(ctx context.Context, key string, count int) StringSliceCmd {
	return &stringSliceCmdWrapper{cmd: p.pipeline.LPopCount(ctx, key, count)}
}

// RPush 实现 RPush 命令
func (p *V8Pipeline) RPush(ctx context.Context, key string, values ...interface{}) Cmder {
	return p.pipeline.RPush(ctx, key, values...)
}

// LLen 实现 LLen 命令
func (p *V8Pipeline) LLen(ctx context.Context, key string) IntCmd {
	return &intCmdWrapper{cmd: p.pipeline.LLen(ctx, key)}
}

// RPop 实现 RPop 命令
func (p *V8Pipeline) RPop(ctx context.Context, key string) StringCmd {
	return &stringCmdWrapper{cmd: p.pipeline.RPop(ctx, key)}
}

// RPopCount 实现 RPopCount 命令
func (p *V8Pipeline) RPopCount(ctx context.Context, key string, count int) StringSliceCmd {
	return &stringSliceCmdWrapper{cmd: p.pipeline.RPopCount(ctx, key, count)}
}

// SAdd 实现 SAdd 命令
func (p *V8Pipeline) SAdd(ctx context.Context, key string, members ...interface{}) Cmder {
	return p.pipeline.SAdd(ctx, key, members...)
}

// SRem 实现 SRem 命令
func (p *V8Pipeline) SRem(ctx context.Context, key string, members ...interface{}) Cmder {
	return p.pipeline.SRem(ctx, key, members...)
}

// SCard 实现 SCard 命令
func (p *V8Pipeline) SCard(ctx context.Context, key string) IntCmd {
	return &intCmdWrapper{cmd: p.pipeline.SCard(ctx, key)}
}

// Exec 实现 Exec 命令
func (p *V8Pipeline) Exec(ctx context.Context) (cmders []Cmder, err error) {
	cmds, err := p.pipeline.Exec(ctx)

	results := make([]Cmder, len(cmds))
	for i, cmd := range cmds {
		switch cmd := cmd.(type) {
		case *redis.StringCmd:
			results[i] = &stringCmdWrapper{cmd: cmd}
		case *redis.IntCmd:
			results[i] = &intCmdWrapper{cmd: cmd}
		case *redis.DurationCmd:
			results[i] = &timeCmdWrapper{cmd: cmd}
		case *redis.StringStringMapCmd:
			results[i] = &stringStringMapCmdWrapper{cmd: cmd}
		case *redis.StringSliceCmd:
			results[i] = &stringSliceCmdWrapper{cmd: cmd}
		default:
			results[i] = &cmderWrapper{cmd: cmd}
		}
	}
	return results, err
}
