package limiter

import (
	"context"
	"github.com/go-redis/redis/v8"
)

type DistributedSentinelV8 struct {
	baseLimiter
	redis redis.UniversalClient
}

func NewDistributedSentinelV8() *DistributedSentinelV8 {
	return &DistributedSentinelV8{}
}

func (d *DistributedSentinelV8) Init(cnf *Config) error {
	//TODO implement me
	d.redis = cnf.Redis
	err := d.constructor(cnf)
	if err != nil {
		return err
	}

	return nil
}

func (d *DistributedSentinelV8) Take(ctx context.Context, uri string, source string) (entry *Entry, err error) {
	//TODO implement me
	group, exists := d.uriMapping[uri]
	if !exists {
		return nil, SourceLimiterNotDefined
	}
	// 判断当前 source 是否已定义限流
	key := d.getKey(uri, source)

	ok, err := d.tryAcquire(ctx, key, group)
	if err != nil {
		if group.ErrController != nil {
			group.ErrController(ctx, key, err)
		}

		return nil, err
	}

	if ok {
		return &Entry{
			tpe:   TypeDistributed,
			entry: nil,
		}, nil
	} else {
		if group.ErrController != nil {
			group.ErrController(ctx, key, err)
		}

		return nil, SourceBlocked
	}
}

// TryAcquire
// @description 尝试获取令牌
func (d *DistributedSentinelV8) tryAcquire(ctx context.Context, key string, rule *uriGroup) (ok bool, err error) {
	//TODO implement me
	args := []interface{}{rule.Burst, rule.PeriodSec, 1}
	rs, err := redisScriptV8.Run(ctx, d.redis, []string{key}, args...).Int()

	return rs == 0, err
}

var redisScriptV8 = redis.NewScript(`
local key = KEYS[1]
-- 最大存储的令牌数
local max_permits = tonumber(ARGV[1])
-- 周期
local period = tonumber(ARGV[2])
-- 请求的令牌数
local required_permits = tonumber(ARGV[3])
-- 如果要求的数据比桶的数量还多直接拒绝
if required_permits > max_permits then
	return 2
end

local data = redis.call('hmget', key, 'next_free_ticket_micros', 'stored_permits')
-- 下次请求可以获取令牌的起始时间
local next_free_ticket_micros = 0
-- 当前存储的令牌数
local stored_permits = 0
if data ~= nil then
	next_free_ticket_micros = tonumber(data[1])
	if next_free_ticket_micros == nil then
		next_free_ticket_micros = 0
	end
	
	stored_permits = tonumber(data[2])
	if stored_permits == nil then
		stored_permits = 0
	end
end
 
-- 当前时间
local time = redis.call('time')
local now_micros = tonumber(time[1]) * 1000000 + tonumber(time[2])
 
-- 添加令牌的时间间隔
local period_micros = period * 1000000
local stable_interval_micros = period_micros / max_permits
local changed = false
local rs = 0
-- 补充令牌
if (now_micros > next_free_ticket_micros) then
    local new_permits = (now_micros - next_free_ticket_micros) / stable_interval_micros
    stored_permits = math.min(max_permits, stored_permits + new_permits)
	next_free_ticket_micros = now_micros
	changed = true
end
local need_wait_time_micros = 0
if stored_permits >= required_permits then
	-- 当前存储的令牌数足够，直接消费
	changed = true
	stored_permits = stored_permits - required_permits
else 
	rs = 1
end

if changed then
	redis.call('hmset', key,'stored_permits', stored_permits, 'next_free_ticket_micros', next_free_ticket_micros)
end
redis.call('expire', key, period + 8)
-- 返回需要等待的时间长度
return rs`)
