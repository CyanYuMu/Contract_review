package limiter

import (
	"context"
	redis "github.com/redis/go-redis/v9"
)

type DistributedSentinelV9 struct {
	baseLimiter
	redis redis.UniversalClient
}

func NewDistributedSentinelV9() *DistributedSentinelV9 {
	return &DistributedSentinelV9{}
}

// ConfigV9 v9 版本的配置
type ConfigV9 struct {
	Trust Trust
	Rule  *Rule
	Redis redis.UniversalClient
	Name  string
}

func (d *DistributedSentinelV9) Init(cnf *ConfigV9) error {
	d.redis = cnf.Redis
	err := d.constructor(&Config{
		Trust: cnf.Trust,
		Rule:  cnf.Rule,
		Name:  cnf.Name,
	})
	if err != nil {
		return err
	}
	return nil
}

func (d *DistributedSentinelV9) Take(ctx context.Context, uri string, source string) (entry *Entry, err error) {
	group, exists := d.uriMapping[uri]
	if !exists {
		return nil, SourceLimiterNotDefined
	}
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

func (d *DistributedSentinelV9) tryAcquire(ctx context.Context, key string, rule *uriGroup) (ok bool, err error) {
	args := []interface{}{rule.Burst, rule.PeriodSec, 1}
	rs, err := redisScriptV9.Run(ctx, d.redis, []string{key}, args...).Int()
	return rs == 0, err
}

var redisScriptV9 = redis.NewScript(`
local key = KEYS[1]
local max_permits = tonumber(ARGV[1])
local period = tonumber(ARGV[2])
local required_permits = tonumber(ARGV[3])
if required_permits > max_permits then
	return 2
end

local data = redis.call('hmget', key, 'next_free_ticket_micros', 'stored_permits')
local next_free_ticket_micros = 0
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
 
local time = redis.call('time')
local now_micros = tonumber(time[1]) * 1000000 + tonumber(time[2])
 
local period_micros = period * 1000000
local stable_interval_micros = period_micros / max_permits
local changed = false
local rs = 0
if (now_micros > next_free_ticket_micros) then
    local new_permits = (now_micros - next_free_ticket_micros) / stable_interval_micros
    stored_permits = math.min(max_permits, stored_permits + new_permits)
	next_free_ticket_micros = now_micros
	changed = true
end
local need_wait_time_micros = 0
if stored_permits >= required_permits then
	changed = true
	stored_permits = stored_permits - required_permits
else 
	rs = 1
end

if changed then
	redis.call('hmset', key,'stored_permits', stored_permits, 'next_free_ticket_micros', next_free_ticket_micros)
end
redis.call('expire', key, period + 8)
return rs`)
