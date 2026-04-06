package service_resolver

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-redis/redis"
	jsoniter "github.com/json-iterator/go"
	"github.com/spf13/cast"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
	"strings"
	"sync"
	"time"
)

var serviceScript = redis.NewScript(`
local key = KEYS[1]
local field = ARGV[1]
local value = ARGV[2]
local ttl = tonumber(ARGV[3])
local data = redis.call('hset', key, field, value)
redis.call('expire', key, ttl)
-- 延长过去时间
return redis.call('hgetall', key)`)

const globalServiceDiscoveryTopic = "mid_global_service_discovery"
const keepaliveIntervalSec int64 = 10
const maxInterval int64 = keepaliveIntervalSec * 2

const (
	ActionOnline  = "on"
	ActionOffline = "off"
)

type GlobalServiceDiscovery struct {
	redis redis.UniversalClient
	msg   chan Event
}

func (g *GlobalServiceDiscovery) Send(msg Event) error {
	msgStr, _ := jsoniter.MarshalToString(msg)
	return g.redis.Publish(globalServiceDiscoveryTopic, msgStr).Err()
}

var gsd *GlobalServiceDiscovery
var gsdOnce sync.Once

func initGSD(redis redis.UniversalClient) error {
	if gsd == nil {
		gsdOnce.Do(func() {
			gsd = &GlobalServiceDiscovery{
				redis: redis,
				msg:   make(chan Event, 16),
			}
			//TODO implement me
			go func() {
				sub := redis.Subscribe(globalServiceDiscoveryTopic)
				for {
					select {
					case msgStr := <-sub.Channel():
						var msg Event
						err := jsoniter.UnmarshalFromString(msgStr.Payload, &msg)
						if err == nil {
							gsd.msg <- msg
						} else {
							su_logger.Error(nil, err, "global service discovery", su_logger.E().String("payload", msgStr.Payload))
						}
					default:
						time.Sleep(time.Millisecond * 100)
					}
				}
			}()
		})
	}

	return nil
}

type ServiceResolver struct {
	serviceName  string
	redis        redis.UniversalClient
	resolverData map[string]*Addr
	lock         sync.Mutex
	cacheKey     string
	addr         *Addr
}

func New() *ServiceResolver {
	return &ServiceResolver{
		resolverData: make(map[string]*Addr, 1),
	}
}

type ServiceResolverConf struct {
	Redis       redis.UniversalClient
	ServiceName string
	Ip          string
	Port        int
}

type Event struct {
	Service string `json:"service"`
	Action  string `json:"action"`
	Ip      string `json:"ip"`
	Port    int    `json:"port"`
	Time    int64  `json:"time"`
}

func getServiceCacheKey(srv string) string {
	return "service_disc_" + srv
}

func getAddrKey(addr *Addr) string {
	return fmt.Sprintf("%s:%d", addr.Ip, addr.Port)
}

func (s *ServiceResolver) updateResolver(addr *Addr, action string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	key := getAddrKey(addr)
	if action == ActionOnline {
		if v, exists := s.resolverData[key]; exists {
			v.LastUpdateSec = addr.LastUpdateSec
		} else {
			s.resolverData[key] = addr
		}
	} else {
		delete(s.resolverData, key)
	}
}

func (s *ServiceResolver) keepAlive() {
	defer func() {
		if r := recover(); r != nil {
			su_logger.Error(nil, fmt.Errorf("keepAlive panic: %v", r), "global service discovery")
		}
	}()
	f, v := encode(s.addr)
	su_logger.Debug(nil, "keep alive", su_logger.E().String("service", s.serviceName).String("key", s.cacheKey).String("field", f).String("value", v))
	rs, err2 := serviceScript.Run(s.redis, []string{s.cacheKey}, f, v, maxInterval).Result()
	if err2 != nil {
		su_logger.Error(nil, err2, "global service discovery", su_logger.E().String("service", s.serviceName))
	} else {
		items := rs.([]interface{})
		for i := 0; i < len(items); i += 2 {
			addr, err := decode(items[i].(string), items[i+1].(string))
			if err != nil {
				continue
			}
			s.updateResolver(&addr, ActionOnline)
		}
	}
}

func (s *ServiceResolver) Init(ctx context.Context, cnf ServiceResolverConf) error {
	if cnf.Redis == nil {
		return errors.New("redis is nil")
	}
	if cnf.Port < 1 {
		return errors.New("port is required")
	}
	s.addr = &Addr{
		Ip:   cnf.ServiceName,
		Port: cnf.Port,
	}

	s.redis = cnf.Redis
	err := initGSD(cnf.Redis)
	if err != nil {
		return err
	}
	s.serviceName = cnf.ServiceName
	s.cacheKey = getServiceCacheKey(s.serviceName)

	// 报活以及感知服务上下线
	var nextTime time.Time = time.Now().Add(time.Duration(keepaliveIntervalSec) * time.Second)
	s.keepAlive()
	err = s.Register(s.addr)
	if err != nil {
		return err
	}
	go func() {
		for {
			select {
			case entry := <-gsd.msg:
				if entry.Service == s.serviceName {
					su_logger.Debug(nil, entry.Service, su_logger.E().String("action", entry.Action).String("ip", entry.Ip).Int("port", entry.Port))

					addr := &Addr{
						Ip:            entry.Ip,
						Port:          entry.Port,
						LastUpdateSec: entry.Time,
					}
					s.updateResolver(addr, entry.Action)
				}
			case <-ctx.Done():
				_ = s.DeRegister(s.addr)
				return
			default:
				if time.Now().After(nextTime) {
					nextTime = time.Now().Add(time.Duration(keepaliveIntervalSec) * time.Second)
					s.keepAlive()
				} else {
					time.Sleep(time.Millisecond * 100)
				}
			}
		}
	}()

	// 获取上游列表
	result, err := s.redis.HGetAll(s.cacheKey).Result()
	if err != nil {
		return err
	}
	for k, v := range result {
		a, err1 := decode(k, v)
		if err1 != nil {
			continue
		}
		s.updateResolver(&a, ActionOnline)
	}

	return nil
}

func (s *ServiceResolver) Resolver() (list []*Addr) {
	timeFrom := time.Now().Unix() - maxInterval
	list = make([]*Addr, 0, len(s.resolverData))
	for k, _ := range s.resolverData {
		if s.resolverData[k].LastUpdateSec <= timeFrom {
			s.redis.HDel(s.cacheKey, getAddrKey(s.resolverData[k]))
			s.updateResolver(s.resolverData[k], ActionOffline)
			continue
		}
		list = append(list, s.resolverData[k])
	}

	return list
}

func encode(a *Addr) (field string, val string) {
	ts := time.Now().Unix()
	return fmt.Sprintf("%s:%d", a.Ip, a.Port), cast.ToString(ts)
}

func decode(field string, v string) (a Addr, err error) {
	a.LastUpdateSec = cast.ToInt64(v)
	items := strings.SplitN(field, ":", 2)
	if len(items) != 2 {
		err = errors.New("invalid field")
		return
	}
	a.Ip = items[0]
	a.Port = cast.ToInt(items[1])

	return
}

func (s *ServiceResolver) Register(addr *Addr) error {
	return gsd.Send(Event{
		Service: s.serviceName,
		Action:  ActionOnline,
		Ip:      addr.Ip,
		Port:    addr.Port,
		Time:    time.Now().Unix(),
	})
}

func (s *ServiceResolver) DeRegister(addr *Addr) error {
	// 消息通知
	return gsd.Send(Event{
		Service: s.serviceName,
		Action:  ActionOffline,
		Ip:      addr.Ip,
		Port:    addr.Port,
		Time:    time.Now().Unix(),
	})
}

type Addr struct {
	Ip            string
	Port          int
	LastUpdateSec int64
}
