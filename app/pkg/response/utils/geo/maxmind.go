package geo

import (
	"context"
	"net"
	"sync"

	"github.com/oschwald/geoip2-golang"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
)

type Maxmind struct {
	dbPath  string
	db      *geoip2.Reader
	once    sync.Once
	initErr error
}

func NewMaxmind(dbPath string) *Maxmind {
	return &Maxmind{dbPath: dbPath}
}

func (m *Maxmind) getDB() *geoip2.Reader {
	if m.db != nil {
		return m.db
	}
	m.once.Do(func() {
		db, err := geoip2.Open(m.dbPath)
		if err != nil {
			m.initErr = err
			su_logger.Error(context.Background(), err, "maxmind geoip2 open failed", su_logger.E().String("dbPath", m.dbPath))
			return
		}
		m.db = db
	})
	return m.db
}

func (m *Maxmind) Ip2Region(ctx context.Context, ip string) (Region, error) {
	db := m.getDB()
	if m.initErr != nil {
		su_logger.Error(ctx, m.initErr, "maxmind geoip2 init failed")
		return Region{}, m.initErr
	}
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		err := su_logger.E().String("ip", ip)
		su_logger.Error(ctx, nil, "invalid ip format", err)
		return Region{}, nil
	}
	city, err := db.City(parsedIP)
	if err != nil {
		su_logger.Error(ctx, err, "maxmind geoip2 query failed", su_logger.E().String("ip", ip))
		return Region{}, err
	}
	region := Region{
		CountryCode: city.Country.IsoCode,
		Province:    "",
		City:        "",
	}
	if len(city.Subdivisions) > 0 {
		region.Province = city.Subdivisions[0].Names["zh-CN"]
	}
	if city.City.Names != nil {
		region.City = city.City.Names["zh-CN"]
	}
	return region, nil
}

func (m *Maxmind) SetDBPath(dbPath string) {
	m.dbPath = dbPath
}
