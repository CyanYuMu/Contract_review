package geo

import (
	"context"
	"sync"
)

type Region struct {
	CountryCode string `json:"country_code"` // 国家编码
	Province    string `json:"province"`     // 省份
	City        string `json:"city"`         // 城市
}

type Geo interface {
	Ip2Region(ctx context.Context, ip string) (Region, error)
	SetDBPath(dbPath string)
}

var DefaultGeo Geo = &Maxmind{
	once:   sync.Once{},
	dbPath: "/app/config/ip/geo.mmdb",
}
