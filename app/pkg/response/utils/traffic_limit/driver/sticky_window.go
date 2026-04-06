package driver

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
	error2 "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/error"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
)

var stickyWindowScriptV8 = redis.NewScript(`
local key = KEYS[1]
-- 周期内产生的令牌数
local permits_per_period = tonumber(ARGV[1])
-- 周期
local period = tonumber(ARGV[2]) * 1000000
-- 请求的令牌数
local required_permits = tonumber(ARGV[3])

-- 如果要求的数据比桶的数量还多直接拒绝
if required_permits > permits_per_period then
	return -2
end

-- 下次请求可以获取令牌的起始时间
local next_free_ticket_micros = tonumber(redis.call('hget', key, 'next_free_ticket_micros') or 0)

local time = redis.call('time')
local now_micros = tonumber(time[1]) * 1000000 + tonumber(time[2])
-- 当前存储的令牌数
local stored_permits = tonumber(redis.call('hget', key, 'stored_permits') or 0)
-- 重新填充
local new_permits = 0
if (now_micros > next_free_ticket_micros) then
    stored_permits = permits_per_period
    next_free_ticket_micros = now_micros + period
end

local need_wait_time_micros = 0
 
if (stored_permits < required_permits) then
	-- 等待下一个周期再来拿令牌
	need_wait_time_micros = period
	redis.call('hset', key, 'next_free_ticket_micros', next_free_ticket_micros)
else 
	-- 当前存储的令牌数足够，直接消费
	stored_permits = stored_permits - required_permits
	redis.call('hmset', key, 'stored_permits', stored_permits, 'next_free_ticket_micros', next_free_ticket_micros)
end

-- 不开启事务 
-- redis.replicate_commands()
redis.call('expire', key, period + 8)
-- 返回需要等待的时间长度
return need_wait_time_micros`)

// StickyWindow
// @Description: 固定窗口
type StickyWindow struct {
	Conf *StickyWindowConf
}

func (s *StickyWindow) Init() error {
	if s.Conf.NumPerPeriod < 1 {
		return error2.NewSUError(500401, "sticky_window.num_per_period must be greater than 0")
	}

	if s.Conf.Period < 1 {
		su_logger.Debug(nil, "sticky_window.period is 0, auto fill by 1")
		s.Conf.Period = 1
	}

	if s.Conf.Key == "" {
		return error2.NewSUError(500401, "sticky_window.key is empty")
	}

	if s.Conf.Redis == nil {
		return error2.NewSUError(500401, "sticky_window.redis is nil")
	}

	return nil
}

// Acquire
// @description 获取令牌, 当timeout = 0 会一直轮询直到获取到令牌, >0 会在超时范围内一直尝试获取
func (s *StickyWindow) Acquire(timeout time.Duration) (bool, error) {
	var timeTo time.Time
	if timeout > 0 {
		timeTo = time.Now().Add(timeout)
	}

	for timeout == 0 || time.Now().Before(timeTo) {
		timeToWait, ok, err := s.TryAcquire()
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
func (s *StickyWindow) TryAcquire() (timeToWait time.Duration, ok bool, err error) {
	//TODO implement me
	args := []interface{}{s.Conf.NumPerPeriod, s.Conf.Period, 1}
	timeToWaitN, err := stickyWindowScriptV8.Run(context.Background(), s.Conf.Redis, []string{s.Conf.Key}, args...).Int()

	if timeToWaitN > 0 {
		return time.Duration(timeToWaitN) * time.Microsecond, false, err
	} else if timeToWaitN == -2 {
		return 0, false, error2.NewSUError(500400, "request ticket is too much")
	} else {
		return 0, true, err
	}
}

func (s *StickyWindow) Wait() error {
	_, err := s.Acquire(0)
	return err
}
