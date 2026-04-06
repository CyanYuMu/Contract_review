package cache

import (
	"fmt"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	m.Run()
}

type User struct {
	Name          string    `json:"name"`
	Age           int       `json:"age"`
	IsMale        bool      `json:"is_male"`
	LastLoginTime time.Time `json:"last_login_time"`
}

type User2 struct {
	A string `json:"a"`
	B string `json:"b"`
	C string `json:"c"`
	D string `json:"d"`
}

// TestPackString
// @description 数据源是结构体并以string形式缓存
func TestPackString(t *testing.T) {
	redisCache := Get(TypeRedis)
	rs, err := Pack(redisCache).StoreType(StoreTypeString).GetSet("test", time.Second*10, func() (interface{}, error) {
		return User{
			Name:          "bee",
			Age:           10,
			LastLoginTime: time.Now(),
		}, nil
	})
	if err != nil {
		t.Error(err)
		return
	}

	var u User
	// 结果赋值
	err = rs.Unmarshal(&u)
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Printf("%+v\n", u)
}

func TestPackHash(t *testing.T) {
	redisCache := Get(TypeRedis)
	rs, err := Pack(redisCache).StoreType(StoreTypeHash).GetSet("test", time.Second*10, func() (interface{}, error) {
		return User{
			Name:          "bee",
			Age:           10,
			IsMale:        true,
			LastLoginTime: time.Now(),
		}, nil
	})
	if err != nil {
		t.Error(err)
		return
	}

	var u User
	// 结果赋值
	err = rs.Unmarshal(&u)
	if err != nil {
		t.Error(err)
		return
	}

	fmt.Printf("%+v\n", u)
}

// TestPackHashPointer
// @description 数据源为指针类型 并以 Hash 形式缓存
func TestPackHashPointer(t *testing.T) {
	redisCache := Get(TypeRedis)
	rs, err := Pack(redisCache).StoreType(StoreTypeHash).GetSet("test", time.Second*10, func() (interface{}, error) {
		return &User{
			Name:          "bee",
			Age:           10,
			IsMale:        true,
			LastLoginTime: time.Now(),
		}, nil
	})
	if err != nil {
		t.Error(err)
		return
	}

	var u User
	err = rs.Unmarshal(&u)
	if err != nil {
		t.Error(err)
		return
	}

	fmt.Printf("%+v\n", u)
}

// TestMapStringInterface
// @description 数据源为map[string]interface{} 并以 hash形式缓存
func TestMapStringInterface(t *testing.T) {
	redisCache := Get(TypeRedis)
	rs, err := Pack(redisCache).StoreType(StoreTypeHash).GetSet("test", time.Second*10, func() (interface{}, error) {
		return map[string]interface{}{
			"name":    "bee",
			"age":     10,
			"is_male": true,
		}, nil
	})
	if err != nil {
		t.Error(err)
		return
	}

	var u User
	err = rs.Unmarshal(&u)
	if err != nil {
		t.Error(err)
		return
	}

	fmt.Printf("%+v\n", u)
}

// TestMapStringString
// @description 数据源为map[string]string 并以 hash 形式存储
func TestMapStringString(t *testing.T) {
	redisCache := Get(TypeRedis)
	rs, err := Pack(redisCache).StoreType(StoreTypeHash).GetSet("test", time.Second*10, func() (interface{}, error) {
		return map[string]string{
			"a": "bee",
			"b": "bee1",
			"c": "bee2",
			"d": "bee3",
		}, nil
	})
	if err != nil {
		t.Error(err)
		return
	}

	var u User2
	err = rs.Unmarshal(&u)
	if err != nil {
		t.Error(err)
		return
	}

	fmt.Printf("%+v\n", u)
}

// TestEmptyMap
// @description 空值场景
func TestEmptyMap(t *testing.T) {
	redisCache := Get(TypeRedis)
	rs, err := Pack(redisCache).StoreType(StoreTypeHash).GetSet("test", time.Second*10, func() (interface{}, error) {
		return map[string]string{}, nil
	})
	if err != nil {
		t.Error(err)
		return
	}

	var u User2
	err = rs.Unmarshal(&u)
	if err != nil {
		if err == ErrorNil {
			fmt.Println("empty data")
		} else {
			t.Error(err)
			return
		}
	}

	fmt.Printf("%+v\n", u)
}

// TestPackHashEmpty
// @description nil值场景, 并启用赋空值操作
func TestPackHashEmpty(t *testing.T) {
	redisCache := Get(TypeRedis)
	rs, err := Pack(redisCache).Empty(time.Second*20).StoreType(StoreTypeHash).GetSet("test", time.Second*10, func() (interface{}, error) {
		return nil, nil
	})
	if err != nil {
		t.Error(err)
		return
	}

	var u User
	err = rs.Unmarshal(&u)
	if err != nil {
		if err != ErrorNil {
			t.Error(err)
			return
		} else {
			t.Logf("nil data\n")
		}
	}

	fmt.Printf("%+v\n", u)
}

// TestGoCache
// @description GoCache 方法测试
func TestGoCache(t *testing.T) {
	goCache := Get(TypeGoCache)
	k := "test"
	err := goCache.HMSet(k, map[string]interface{}{
		"name": "bee",
		"age":  20,
	})

	if err != nil {
		t.Error(err)
		return
	}

	data, err := goCache.HGetAll(k)
	if err != nil {
		t.Error(err)
		return
	}

	t.Logf("%+v\n", data)

	err = goCache.HDel(k, []string{"name"})
	if err != nil {
		t.Error(err)
		return
	}

	data, err = goCache.HGetAll(k)
	if err != nil {
		t.Error(err)
		return
	}
	t.Logf("%+v\n", data)
}
