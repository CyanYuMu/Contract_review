package driver

import (
	"context"
	"time"

	"github.com/bits-and-blooms/bloom/v3"
)

type GoBloomFilter struct {
	conf *GoBloomFilterConf
	inst *bloom.BloomFilter
}

type GoBloomFilterConf struct {
	// 容错率
	P float64
	// 容量
	Cap uint
}

func NewGoBloomFilter(conf *GoBloomFilterConf) (*GoBloomFilter, error) {
	b := &GoBloomFilter{
		conf: conf,
	}
	_ = b.init()
	b.inst = bloom.NewWithEstimates(conf.Cap, conf.P)

	return b, nil
}

func (g *GoBloomFilter) init() error {
	g.conf.Cap = getCap(g.conf.Cap)
	g.conf.P = getErrorRatio(g.conf.P)

	return nil
}

func (g *GoBloomFilter) Exists(ctx context.Context, key string, item string) (exists bool, err error) {
	exists = g.inst.TestString(item)

	return
}

func (g *GoBloomFilter) MExists(ctx context.Context, key string, items []string) (statusList []bool, err error) {
	//TODO implement me
	statusList = make([]bool, len(items))
	for i, _ := range items {
		statusList[i] = g.inst.TestString(items[i])
	}

	return
}

func (g *GoBloomFilter) Add(ctx context.Context, key string, item string, ttl time.Duration) (ok bool, err error) {
	ok = g.inst.TestOrAddString(item)

	return
}

func (g *GoBloomFilter) MAdd(ctx context.Context, key string, items []string, ttl time.Duration) (statusList []bool, err error) {
	//TODO implement me
	statusList = make([]bool, len(items))
	for i, _ := range items {
		statusList[i] = g.inst.TestOrAddString(items[i])
	}

	return
}

func (g *GoBloomFilter) Insert(ctx context.Context, key string, item string, ttl time.Duration) (ok bool, err error) {
	ok = g.inst.TestOrAddString(item)

	return
}

func (g *GoBloomFilter) MInsert(ctx context.Context, key string, items []string, ttl time.Duration) (statusList []bool, err error) {
	//TODO implement me
	statusList = make([]bool, len(items))
	for i, _ := range items {
		statusList[i] = g.inst.TestOrAddString(items[i])
	}

	return
}

func (g *GoBloomFilter) Info(ctx context.Context, key string) (info map[string]interface{}, err error) {
	//TODO implement me
	info = map[string]interface{}{
		"hash_count": g.inst.K(),
		"cap":        g.inst.Cap(),
	}

	return
}

func (g *GoBloomFilter) Clear(ctx context.Context, key string) (err error) {
	// 重新初始化布隆过滤器实例
	g.inst = bloom.NewWithEstimates(g.conf.Cap, g.conf.P)
	return nil
}
