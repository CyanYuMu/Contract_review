package driver

import (
	"context"
	"time"

	redis "github.com/redis/go-redis/v9"
	error2 "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/error"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
)

var redisV9Script = redis.NewScript(`
local key = KEYS[1]
local max_permits = tonumber(ARGV[1])
local permits_per_period = tonumber(ARGV[2])
local period = tonumber(ARGV[3])
local required_permits = tonumber(ARGV[4])
if required_permits > max_permits then
	return -2
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
local stable_interval_micros = period_micros / permits_per_period
local changed = false
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
	need_wait_time_micros = required_permits * stable_interval_micros	
end

if changed then
	redis.call('hmset', key,'stored_permits', stored_permits, 'next_free_ticket_micros', next_free_ticket_micros)
end
redis.call('expire', key, period + 8)
return need_wait_time_micros`)

// TokenBucketV9 令牌桶（v9 版本）
type TokenBucketV9 struct {
	Conf *TokenBucketConfV9
}

func (t *TokenBucketV9) Init() error {
	if t.Conf.Period < 1 {
		su_logger.Debug(nil, "TokenBucketV9: period is too small", su_logger.E().Uint32("period", t.Conf.Period))
		t.Conf.Period = 1
	}
	if t.Conf.Key == "" {
		return error2.NewSUError(500401, "TokenBucketV9: key is empty")
	}
	if t.Conf.Capacity < 1 {
		return error2.NewSUError(500402, "TokenBucketV9: capacity is too small")
	}
	if t.Conf.NumPerPeriod < 1 {
		return error2.NewSUError(500403, "TokenBucketV9: numPerPeriod is too small")
	}
	if t.Conf.NumPerPeriod > t.Conf.Capacity {
		return error2.NewSUError(500404, "TokenBucketV9: numPerPeriod is too large")
	}
	if t.Conf.Redis == nil {
		return error2.NewSUError(500401, "TokenBucketV9: redis is nil")
	}
	return nil
}

func (t *TokenBucketV9) Acquire(timeout time.Duration) (bool, error) {
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
			time.Sleep(timeToWait)
		}
	}
	return false, ErrorTimeout
}

func (t *TokenBucketV9) TryAcquire() (timeToWait time.Duration, ok bool, err error) {
	args := []interface{}{t.Conf.Capacity, t.Conf.NumPerPeriod, t.Conf.Period, 1}
	timeToWaitN, err := redisV9Script.Run(context.Background(), t.Conf.Redis, []string{t.Conf.Key}, args...).Int()
	if timeToWaitN > 0 {
		return time.Duration(timeToWaitN) * time.Microsecond, false, err
	} else if timeToWaitN == -2 {
		return 0, false, error2.NewSUError(500400, "request ticket is too much")
	} else {
		return 0, true, err
	}
}

func (t *TokenBucketV9) Wait() error {
	_, err := t.Acquire(0)
	return err
}
