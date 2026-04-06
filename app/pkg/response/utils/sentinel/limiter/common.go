package limiter

import (
	"context"
	"errors"
	"github.com/alibaba/sentinel-golang/core/base"
	"github.com/go-redis/redis/v8"
	"github.com/spf13/cast"
	"regexp"
	"strings"
	"sync"
)

var SourceLimiterNotDefined = errors.New("source limiter not defined")
var SourceUndefined = errors.New("source undefined")
var SourceBlocked = errors.New("flow reject check blocked")

const ControllerTypeLog = "log"
const ControllerTypeAlert = "alert"

const SourceKeyUri = "$uri"
const SourceKeyAppKey = "$app_key"
const SourceKeyIp = "$ip"

type Type string

const (
	// 分布式限流, 采用 令牌桶 算法进行限流, 对精度控制较高, 依赖redis
	TypeDistributed   Type = "distributed"
	TypeDistributedV8 Type = "distributed_v8"
	// 单机限流, 采用 滑动时间窗口 算法进行限流, 高性能, 推荐
	TypeLocal Type = "local"
	TypeAli        = "ali"
)

type Entry struct {
	tpe   Type
	entry interface{}
}

func (e *Entry) Exit() {
	switch e.tpe {
	case TypeAli:
		e.entry.(*base.SentinelEntry).Exit()
	case TypeDistributed:
	case TypeLocal:

	}
}

type uri struct {
	group *uriGroup
	Tag   string
}

type LimiterController string

const (
	//  日志
	ControllerLogger LimiterController = "log"
	// 机器人告警
	ControllerAlert LimiterController = "alert"
)

type baseLimiter struct {
	subnetMask        []int64
	controllerMapping map[LimiterController]Controller
	uriGroupMapping   map[string]*uriGroup
	uriMapping        map[string]*uriGroup
	lock              sync.Mutex
}

type Rule struct {
	// 最大请求数  1/s  1/m  1/h
	//Burst string
	// 缺省 错误处理
	ErrController ErrController
	// 指定获取资源的key, 默认获取当前的uri
	//SourceKey string
	// @description 当触发限流时如何处理
	//Controller LimiterController
	Groups []*Group
}

type GetSource func() string
type ErrController func(ctx context.Context, source string, err error)

type Group struct {
	Name string
	// 周期内最大令牌数
	Burst         string
	ErrController ErrController
	Uris          []string
}

type uriGroup struct {
	Name string
	// 周期内最大令牌数
	Burst float64
	// 周期
	PeriodSec float64
	//SourceKey     string
	ErrController ErrController
	//Controller LimiterController
}

func (b *baseLimiter) getKey(domain, source string) string {
	return domain + "#" + source
}

var bustPattern = regexp.MustCompile(`^(\d+)/(\d+)?([hms])$`)

func parseBurst(b string) (burst float64, periodSec float64, err error) {
	match := bustPattern.FindAllStringSubmatch(strings.ToLower(b), 3)
	if len(match) == 0 || len(match[0]) != 4 {
		err = errors.New("param `bust` is invalid: " + b + " format: 数量/[数量][hms]")
		return
	}

	burst = cast.ToFloat64(match[0][1])
	switch match[0][3] {
	case "h":
		periodSec = 3600
	case "m":
		periodSec = 60
	default:
		periodSec = 1
	}
	if match[0][2] != "" {
		periodSec *= cast.ToFloat64(match[0][2])
	}

	return
}

func (b *baseLimiter) constructor(conf *Config) error {
	// 初始化白名单
	b.subnetMask = make([]int64, 0, len(conf.Trust.SubnetMask))
	for _, mask := range conf.Trust.SubnetMask {
		if mask == "" {
			continue
		}
		//i := geoip.Ipv4ToInt(mask)
		//b.subnetMask = append(b.subnetMask, i)
	}

	//b.controllerMapping = make(map[LimiterController]Controller, 2)
	b.uriMapping = make(map[string]*uriGroup, len(conf.Rule.Groups))
	b.uriGroupMapping = make(map[string]*uriGroup, len(conf.Rule.Groups))

	// 初始化sourceMapping
	for _, g := range conf.Rule.Groups {
		//if conf.Rules[i].Controller != ControllerTypeAlert {
		//	conf.Rules[i].Controller = ControllerTypeLog
		//}
		//if conf.Rules[i].SourceKey == "" {
		//	conf.Rules[i].SourceKey = SourceKeyUri
		//}
		//if _, exists := b.controllerMapping[conf.Rules[i].Controller]; !exists {
		//	b.controllerMapping[conf.Rules[i].Controller] = NewController(conf.Rules[i].Controller)
		//}
		if g.ErrController == nil {
			g.ErrController = conf.Rule.ErrController
		}

		if _, ok := b.uriGroupMapping[g.Name]; !ok {
			bust, sec, err := parseBurst(g.Burst)
			if err != nil {
				return err
			}
			b.uriGroupMapping[g.Name] = &uriGroup{
				Name:          g.Name,
				Burst:         bust,
				PeriodSec:     sec,
				ErrController: g.ErrController,
			}
		}

		for _, curUri := range g.Uris {
			b.uriMapping[curUri] = b.uriGroupMapping[g.Name]
		}
	}

	return nil
}

func (b *baseLimiter) IsTrustedIp(ip string) bool {
	if len(b.subnetMask) == 0 {
		return false
	}
	//intIp := geoip.Ipv4ToInt(ip)
	//for i, _ := range b.subnetMask {
	//	if (intIp & b.subnetMask[i]) == b.subnetMask[i] {
	//		return true
	//	}
	//}

	return false
}

// config

type RouteSource struct {
	// 资源的参数值
	Source string
	// 对应的路由
	Route string
}

type Trust struct {
	// 子网掩码
	SubnetMask []string
}

type Config struct {
	Trust Trust
	Rule  *Rule
	// 当 Category = TypeDistributed 时需要
	// 与redis二选一
	Redis redis.UniversalClient
	// 配置名
	Name string
	// distributed => 分布式    local => 单机
	//Category Type
	// 指定如何获取数据
	//Taker func(input interface{}, key string) string
}
