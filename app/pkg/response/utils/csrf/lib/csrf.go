package lib

import (
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/su_id"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/cache"
	"time"
)

type Csrf struct {
	cnf *Config
}

type Config struct {
	// * cache
	Cache cache.Cache
	// ? 过期时间, 单位秒, 默认 1200
	TtlSecond int
	// ? token 生成器, 默认 id.UUID()
	TokenGenerator func() string
}

func (c *Csrf) New(cfg *Config) *Csrf {
	if cfg.TtlSecond <= 0 {
		cfg.TtlSecond = 1200
	}

	if cfg.TokenGenerator == nil {
		cfg.TokenGenerator = su_id.GetWithUUID
	}

	return &Csrf{cnf: cfg}
}

func (c *Csrf) GetToken(key string) (token string, err error) {
	token = c.cnf.TokenGenerator()
	_, err = c.cnf.Cache.Set(key, token, time.Duration(c.cnf.TtlSecond)*time.Second)

	return
}

func (c *Csrf) Verify(key string, token string) (ok bool, err error) {
	cacheToken, err := c.cnf.Cache.Get(key)
	if err != nil {
		return
	}
	ok = cacheToken == token
	if ok {
		// 校验通过, 回收token
		_, _ = c.cnf.Cache.Del(key)
	}

	return
}
