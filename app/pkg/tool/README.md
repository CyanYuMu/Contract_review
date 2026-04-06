# Mapping工具包使用手册

## 概述

`mapping`包提供了两类主要功能：
1. 将Map数据转换为结构体的映射工具
2. 一个安全的Map实现(SafeMap)，可避免Go语言原生map在高并发删除操作时可能出现的内存泄漏问题

## 功能模块

### 1. Map到结构体的转换 (mapping.go)

提供将`map[string]interface{}`和`map[string]string`转换为结构体的工具函数：

```go
// 将map[string]interface{}转换为结构体
func StringInterface2Struct(mapData map[string]interface{}, data interface{}) error

// 将map[string]string转换为结构体
func StringString2Struct(mapData map[string]string, data interface{}) error
```

### 2. 安全的Map实现 (safemap.go)

提供一个线程安全且避免内存泄漏的Map实现：

```go
// 创建一个新的SafeMap
func NewSafeMap() *SafeMap

// SafeMap提供的方法
func (m *SafeMap) Del(key interface{})                           // 删除键值对
func (m *SafeMap) Get(key interface{}) (interface{}, bool)       // 获取值
func (m *SafeMap) Set(key, value interface{})                    // 设置键值对
func (m *SafeMap) Size() int                                     // 获取大小
```

## 使用示例

### Map到结构体转换

```go
import (
    "fmt"
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/mapping"
)

// 定义一个结构体
type User struct {
    Name  string `json:"name"`
    Age   int    `json:"age"`
    Email string `json:"email"`
    Admin bool   `json:"is_admin"`
}

func main() {
    // 从map[string]interface{}转换为结构体
    mapData := map[string]interface{}{
        "name":     "张三",
        "age":      30,
        "email":    "zhangsan@example.com",
        "is_admin": true,
    }
    
    user := User{}
    err := mapping.StringInterface2Struct(mapData, &user)
    if err != nil {
        fmt.Println("转换失败:", err)
        return
    }
    fmt.Printf("用户信息: %+v\n", user)
    
    // 从map[string]string转换为结构体
    strMapData := map[string]string{
        "name":     "李四",
        "age":      "25",
        "email":    "lisi@example.com",
        "is_admin": "false",
    }
    
    user2 := User{}
    err = mapping.StringString2Struct(strMapData, &user2)
    if err != nil {
        fmt.Println("转换失败:", err)
        return
    }
    fmt.Printf("用户信息: %+v\n", user2)
}
```

### SafeMap使用

```go
import (
    "fmt"
    "sync"
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/mapping"
)

func main() {
    // 创建SafeMap
    safeMap := mapping.NewSafeMap()
    
    // 设置值
    safeMap.Set("name", "张三")
    safeMap.Set("age", 30)
    
    // 获取值
    name, ok := safeMap.Get("name")
    if ok {
        fmt.Println("姓名:", name)
    }
    
    // 获取Map大小
    size := safeMap.Size()
    fmt.Println("Map大小:", size)
    
    // 删除键
    safeMap.Del("age")
    
    // 并发安全操作
    var wg sync.WaitGroup
    for i := 0; i < 1000; i++ {
        wg.Add(1)
        go func(i int) {
            defer wg.Done()
            key := fmt.Sprintf("key_%d", i)
            safeMap.Set(key, i)
        }(i)
    }
    wg.Wait()
    
    fmt.Println("添加1000个元素后的大小:", safeMap.Size())
}
```

## 工作原理

### 1. Map到结构体转换

这两个函数通过以下步骤工作：

1. 使用反射获取目标结构体的类型信息
2. 遍历结构体的每个字段，查找与Map中键匹配的字段
   - 优先使用`json`标签进行匹配
   - 如果没有`json`标签，则将驼峰命名的字段名转换为蛇形命名进行匹配
3. 根据字段类型处理值的转换
4. 构建一个JSON字符串，然后反序列化到目标结构体

### 2. SafeMap的设计

SafeMap使用两个内部map (`dirtyOld`和`dirtyNew`)和一个读写锁来实现：

1. 使用读写锁确保并发安全
2. 通过两个内部map来解决Go原生map在频繁删除操作下的内存泄漏问题
3. 当删除操作达到阈值时，会创建新的map并迁移数据，释放旧map占用的内存

## 注意事项

1. **字段映射规则**：
   - 优先使用`json`标签进行匹配
   - 无标签时使用字段名的蛇形命名形式(如`UserName`对应`user_name`)

2. **类型转换**：
   - 数值型字段会被正确转换，如字符串"123"会转为整数123
   - 布尔型字段接受"true"、"1"为true，其他值为false

3. **SafeMap的性能考虑**：
   - SafeMap适用于有大量删除操作的场景
   - 对于只有少量删除操作的简单场景，使用`sync.Map`可能更合适 