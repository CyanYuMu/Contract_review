package cache

import (
	"context"
	"time"

	redis "github.com/redis/go-redis/v9"
)

// V9Pipeline Redis v9 版本的 Pipeline 实现
type V9Pipeline struct {
	pipeline redis.Pipeliner
}

// NewV9Pipeline 创建新的 V9Pipeline 实例
func NewV9Pipeline(p redis.Pipeliner) *V9Pipeline {
	return &V9Pipeline{pipeline: p}
}

// Get 实现 Get 命令
func (p *V9Pipeline) Get(ctx context.Context, key string) StringCmd {
	return &stringCmdWrapper{cmd: p.pipeline.Get(ctx, key)}
}

// Set 实现 Set 命令
func (p *V9Pipeline) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) Cmder {
	return p.pipeline.Set(ctx, key, value, expiration)
}

// TTL 实现 TTL 命令
func (p *V9Pipeline) TTL(ctx context.Context, key string) TimeCmd {
	return &timeCmdWrapper{cmd: p.pipeline.TTL(ctx, key)}
}

// Del 实现 Del 命令
func (p *V9Pipeline) Del(ctx context.Context, key string) Cmder {
	return p.pipeline.Del(ctx, key)
}

// Exists 实现 Exists 命令
func (p *V9Pipeline) Exists(ctx context.Context, keys ...string) IntCmd {
	cmd := p.pipeline.Exists(ctx, keys...)
	return &intCmdWrapper{cmd: cmd}
}

// ZAdd 实现 ZAdd 命令
func (p *V9Pipeline) ZAdd(ctx context.Context, key string, members ...Z) Cmder {
	v9Members := make([]redis.Z, len(members))
	for i, m := range members {
		v9Members[i] = redis.Z{
			Score:  m.Score,
			Member: m.Member,
		}
	}
	return p.pipeline.ZAdd(ctx, key, v9Members...)
}

// ZCard 实现 ZCard 命令
func (p *V9Pipeline) ZCard(ctx context.Context, key string) IntCmd {
	return &intCmdWrapper{cmd: p.pipeline.ZCard(ctx, key)}
}

// ZRem 实现 ZRem 命令
func (p *V9Pipeline) ZRem(ctx context.Context, key string, members ...interface{}) Cmder {
	return p.pipeline.ZRem(ctx, key, members...)
}

// ZRange 实现 ZRange 命令
func (p *V9Pipeline) ZRange(ctx context.Context, key string, start, stop int64) StringSliceCmd {
	return p.pipeline.ZRange(ctx, key, start, stop)
}

// Expire 实现 Expire 命令
func (p *V9Pipeline) Expire(ctx context.Context, key string, expiration time.Duration) Cmder {
	return p.pipeline.Expire(ctx, key, expiration)
}

// HGetAll 实现 HGetAll 命令
func (p *V9Pipeline) HGetAll(ctx context.Context, key string) StringStringMapCmd {
	return &stringStringMapCmdWrapper{cmd: p.pipeline.HGetAll(ctx, key)}
}

// HMSet 实现 HMSet 命令
func (p *V9Pipeline) HMSet(ctx context.Context, key string, values ...interface{}) Cmder {
	return p.pipeline.HMSet(ctx, key, values...)
}

// HSet 实现 HSet 命令
func (p *V9Pipeline) HSet(ctx context.Context, key string, values ...interface{}) Cmder {
	return p.pipeline.HSet(ctx, key, values...)
}

// HMGet 实现 HMGet 命令
func (p *V9Pipeline) HMGet(ctx context.Context, key string, fields ...string) StringSliceCmd {
	return &sliceCmdWrapper{cmd: p.pipeline.HMGet(ctx, key, fields...)}
}

// HDel 实现 HDel 命令
func (p *V9Pipeline) HDel(ctx context.Context, key string, fields ...string) Cmder {
	return p.pipeline.HDel(ctx, key, fields...)
}

// LPush 实现 LPush 命令
func (p *V9Pipeline) LPush(ctx context.Context, key string, values ...interface{}) Cmder {
	return p.pipeline.LPush(ctx, key, values...)
}

// LPopCount 实现 LPopCount 命令
func (p *V9Pipeline) LPopCount(ctx context.Context, key string, count int) StringSliceCmd {
	return &stringSliceCmdWrapper{cmd: p.pipeline.LPopCount(ctx, key, count)}
}

// LPop 实现 LPop 命令
func (p *V9Pipeline) LPop(ctx context.Context, key string) StringCmd {
	return &stringCmdWrapper{cmd: p.pipeline.LPop(ctx, key)}
}

// LLen 实现 LLen 命令
func (p *V9Pipeline) LLen(ctx context.Context, key string) IntCmd {
	return &intCmdWrapper{cmd: p.pipeline.LLen(ctx, key)}
}

// RPush 实现 RPush 命令
func (p *V9Pipeline) RPush(ctx context.Context, key string, values ...interface{}) Cmder {
	return p.pipeline.RPush(ctx, key, values...)
}

// RPopCount 实现 RPopCount 命令
func (p *V9Pipeline) RPopCount(ctx context.Context, key string, count int) StringSliceCmd {
	return &stringSliceCmdWrapper{cmd: p.pipeline.RPopCount(ctx, key, count)}
}

// RPop 实现 RPop 命令
func (p *V9Pipeline) RPop(ctx context.Context, key string) StringCmd {
	return &stringCmdWrapper{cmd: p.pipeline.RPop(ctx, key)}
}

// SAdd 实现 SAdd 命令
func (p *V9Pipeline) SAdd(ctx context.Context, key string, members ...interface{}) Cmder {
	return p.pipeline.SAdd(ctx, key, members...)
}

// SRem 实现 SRem 命令
func (p *V9Pipeline) SRem(ctx context.Context, key string, members ...interface{}) Cmder {
	return p.pipeline.SRem(ctx, key, members...)
}

// SCard 实现 SCard 命令
func (p *V9Pipeline) SCard(ctx context.Context, key string) IntCmd {
	return &intCmdWrapper{cmd: p.pipeline.SCard(ctx, key)}
}

// Exec 实现 Exec 命令
func (p *V9Pipeline) Exec(ctx context.Context) (result []Cmder, err error) {
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
		case *redis.MapStringStringCmd:
			results[i] = &stringStringMapCmdWrapper{cmd: cmd}
		case *redis.SliceCmd:
			results[i] = &sliceCmdWrapper{cmd: cmd}
		case *redis.StringSliceCmd:
			results[i] = &stringSliceCmdWrapper{cmd: cmd}
		default:
			results[i] = &cmderWrapper{cmd: cmd}
		}
	}
	return results, err
}
