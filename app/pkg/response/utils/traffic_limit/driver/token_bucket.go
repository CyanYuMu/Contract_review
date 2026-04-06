package driver

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
	error2 "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/error"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
)

var redisV8Script = redis.NewScript(`
local key = KEYS[1]
-- 最大存储的令牌数
local max_permits = tonumber(ARGV[1])
-- 周期内产生的令牌数
local permits_per_period = tonumber(ARGV[2])
-- 周期
local period = tonumber(ARGV[3])
-- 请求的令牌数
local required_permits = tonumber(ARGV[4])
-- 如果要求的数据比桶的数量还多直接拒绝
if required_permits > max_permits then
	return -2
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
local stable_interval_micros = period_micros / permits_per_period
local changed = false
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
	-- 计算需要多久才能产出需要的令牌数
	need_wait_time_micros = required_permits * stable_interval_micros	
end

if changed then
	redis.call('hmset', key,'stored_permits', stored_permits, 'next_free_ticket_micros', next_free_ticket_micros)
end
redis.call('expire', key, period + 8)
-- 返回需要等待的时间长度
return need_wait_time_micros`)

// TokenBucket
// @Description: 令牌桶
// https://www.freesion.com/article/72551277813/
type TokenBucket struct {
	Conf *TokenBucketConf
}

func (t *TokenBucket) Init() error {
	if t.Conf.Period < 1 {
		su_logger.Debug(nil, "TokenBucket: period is too small", su_logger.E().Uint32("period", t.Conf.Period))
		t.Conf.Period = 1
	}

	if t.Conf.Key == "" {
		return error2.NewSUError(500401, "TokenBucket: key is empty")
	}

	if t.Conf.Capacity < 1 {
		return error2.NewSUError(500402, "TokenBucket: capacity is too small")
	}

	if t.Conf.NumPerPeriod < 1 {
		return error2.NewSUError(500403, "TokenBucket: numPerPeriod is too small")
	}

	if t.Conf.NumPerPeriod > t.Conf.Capacity {
		return error2.NewSUError(500404, "TokenBucket: numPerPeriod is too large, numPerPeriod should low than capacity")
	}

	if t.Conf.Redis == nil {
		return error2.NewSUError(500401, "sticky_window.redis is nil")
	}

	return nil
}

// Acquire
// @description 获取令牌, 当timeout = 0 会一直轮询直到获取到令牌, >0 会在超时范围内一直尝试获取
func (t *TokenBucket) Acquire(timeout time.Duration) (bool, error) {
	var timeTo time.Time
	if timeout > 0 {
		timeTo = time.Now().Add(timeout)
	}

	for timeout == 0 || (time.Now().Before(timeTo)) {
		timeToWait, ok, err := t.TryAcquire()
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		} else {
			//su_logger.Debug(nil, "等待获取令牌桶")
			time.Sleep(timeToWait)
		}
	}

	return false, ErrorTimeout
}

// TryAcquire
// @description 尝试获取令牌
func (t *TokenBucket) TryAcquire() (timeToWait time.Duration, ok bool, err error) {
	args := []interface{}{t.Conf.Capacity, t.Conf.NumPerPeriod, t.Conf.Period, 1}
	timeToWaitN, err := redisV8Script.Run(context.Background(), t.Conf.Redis, []string{t.Conf.Key}, args...).Int()
	if timeToWaitN > 0 {
		return time.Duration(timeToWaitN) * time.Microsecond, false, err
	} else if timeToWaitN == -2 {
		return 0, false, error2.NewSUError(500400, "request ticket is too much")
	} else {
		return 0, true, err
	}
}

func (t *TokenBucket) Wait() error {
	_, err := t.Acquire(0)
	return err
}
