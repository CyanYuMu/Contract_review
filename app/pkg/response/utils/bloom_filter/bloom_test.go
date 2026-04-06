package bloom_filter

import (
	"context"
	"fmt"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/bloom_filter/driver"
	"testing"
	"time"
)

func TestMAdd(t *testing.T) {
	bf, _ := NewBitsetV8(&driver.BitsetV8Conf{
		P:   0.001,
		Cap: 1000,
		//Redis: boot.RedisInst.GetInst().(redis.UniversalClient),
	})
	ctx := context.Background()
	bf.MAdd(ctx, "test", []string{"test", "test1", "test2"}, time.Minute)
	rs, err := bf.MExists(ctx, "test", []string{"test", "test1", "aaa", "bbb"})
	if err != nil {
		t.Error(err)
		return
	}
	//4246724928
	//2147483648
	//
	//info, err := bf.Info()

	fmt.Println(rs, err)
}

func TestGoBloom(t *testing.T) {
	bf, _ := NewGoBloom(&driver.GoBloomFilterConf{
		P:   0.001,
		Cap: 1000,
	})

	ctx := context.Background()

	add, err2 := bf.MAdd(ctx, "test", []string{"aaa", "bbb", "ccc"}, time.Minute)
	fmt.Println(add, err2)

	exists, err := bf.MExists(ctx, "test", []string{"aaa", "bbb", "ccc", "ddd", "eee"})
	fmt.Println(exists, err)
}
