package bloom_filter

import (
	"context"
	"time"

	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/bloom_filter/driver"
)

type BloomInterface interface {
	// 判断key是否存在
	Exists(ctx context.Context, key string, item string) (exists bool, err error)
	// 批量判断key是否存在, 返回值通知索引下标关联key是否存在
	MExists(ctx context.Context, key string, items []string) (statusList []bool, err error)
	/*Add
	 * @Description: Add
	 * @param key
	 * @param item
	 * @param ttl 仅在key不存在的时候, 才会设置过期时间
	 * @return ok
	 * @return err
	 */
	Add(ctx context.Context, key, item string, ttl time.Duration) (ok bool, err error)
	/*MAdd
	 * @Description: 批量添加key
	 * @param key
	 * @param items
	 * @param ttl 仅在key不存在的时候, 才会设置过期时间
	 * @return statusList
	 * @return err
	 */
	MAdd(ctx context.Context, key string, items []string, ttl time.Duration) (statusList []bool, err error)
	// 查看当前布隆的状态, 比如, 容量, key 数量, ...
	Info(ctx context.Context, key string) (info map[string]interface{}, err error)
	/*Insert
	 * @Description: 插入
	 * @param key
	 * @param item
	 * @param ttl key的过期时间, 与add不同的是insert方法会主动延长key的过期时间
	 * @return ok
	 * @return err
	 */
	Insert(ctx context.Context, key string, item string, ttl time.Duration) (ok bool, err error)
	/*MInsert
	 * @Description: 批量插入
	 * @param key
	 * @param items
	 * @param tt key的过期时间, 与add不同的是insert方法会主动延长key的过期时间
	 * @return statusList
	 * @return err
	 */
	MInsert(ctx context.Context, key string, items []string, ttl time.Duration) (statusList []bool, err error)
	// 清空指定布隆
	Clear(ctx context.Context, key string) (err error)
}

//func NewRedisBloom(conf *config.RedisBloomFilterConf) (BloomInterface, error) {
//	return driver.NewRedisBloom(conf)
//}

func NewGoBloom(conf *driver.GoBloomFilterConf) (BloomInterface, error) {
	return driver.NewGoBloomFilter(conf)
}

func NewBitsetV8(conf *driver.BitsetV8Conf) (BloomInterface, error) {
	return driver.NewBitsetV8(conf)
}

func NewBitsetV9(conf *driver.BitsetV9Conf) (BloomInterface, error) {
	return driver.NewBitsetV9(conf)
}

func NewBitset(conf *driver.BitsetV8Conf) (BloomInterface, error) {
	return driver.NewBitsetV8(conf)
}

func NewBitsetV9Sliding(conf *driver.BitsetV9SlidingConf) (BloomInterface, error) {
	return driver.NewBitsetV9Sliding(conf)
}

func NewBitsetV8Sliding(conf *driver.BitsetV8SlidingConf) (BloomInterface, error) {
	return driver.NewBitsetV8Sliding(conf)
}

func NewGoBloomSliding(conf *driver.GoBloomSlidingConf) (BloomInterface, error) {
	return driver.NewGoBloomSliding(conf)
}

// 特点:
//   - 读取优先本地，高性能
//   - 通过 Syncer 实现节点间实时同步
//   - Buffer 批量写入 Redis，减少网络IO
func NewBitsetL2(conf *driver.BitsetL2Conf) (BloomInterface, error) {
	return driver.NewBitsetL2(conf)
}
