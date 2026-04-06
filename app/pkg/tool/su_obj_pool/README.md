# Su_obj_pool对象池工具包

## 概述

`su_obj_pool`包提供了两种高性能的对象池实现：
1. **限制大小的对象池(Sized)** - 限制了对象数量的通用对象池
2. **循环队列(Circular)** - 固定大小的循环队列实现

这两种池都是线程安全的，适用于高并发环境，能有效减少频繁创建和销毁对象带来的性能开销。

## 主要功能

### 1. 限制大小的对象池 (sized_pool.go)

提供一个控制最大数量的对象池，适合需要限制资源使用的场景：

```go
// 创建一个指定大小的对象池
func NewSizedPool(size int, newFunc func() interface{}) *Sized

// 获取一个对象，如果池为空则创建新对象（如果未到达大小限制）
func (s *Sized) Get() interface{}

// 放回一个对象到池中，如果池已满则丢弃
func (s *Sized) Put(item interface{})

// 尝试获取资源并执行自定义操作，操作完成后自动归还资源
func (s *Sized) Try(fn func(i interface{}) error) error
```

### 2. 循环队列 (circular.go)

提供一个固定大小、线程安全的循环队列实现：

```go
// 创建一个指定容量的循环队列
func NewCircularQueue(capacity int32) *Circular

// 入队一个元素，如果队列已满则返回错误
func (q *Circular) Enqueue(item interface{}) error

// 出队一个元素，如果队列为空则返回错误
func (q *Circular) Dequeue() (interface{}, error)

// 检查队列是否为空
func (q *Circular) IsEmpty() bool

// 检查队列是否已满
func (q *Circular) IsFull() bool
```

## 使用示例

### 限制大小的对象池

```go
import (
    "fmt"
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/su_obj_pool"
)

// 定义一个昂贵的对象
type ExpensiveObject struct {
    data []byte
}

func main() {
    // 创建一个最多包含10个对象的对象池
    pool := su_obj_pool.NewSizedPool(10, func() interface{} {
        // 创建对象的函数
        return &ExpensiveObject{
            data: make([]byte, 1024*1024), // 1MB的数据
        }
    })
    
    // 获取一个对象
    obj := pool.Get().(*ExpensiveObject)
    
    // 使用对象...
    
    // 使用完毕，归还对象
    pool.Put(obj)
    
    // 使用Try方法简化获取、使用和归还流程
    err := pool.Try(func(i interface{}) error {
        obj := i.(*ExpensiveObject)
        // 使用对象...
        return nil // 返回nil表示成功，对象会被自动归还
    })
    
    if err == su_obj_pool.ErrNoObject {
        fmt.Println("无法获取对象")
    }
}
```

### 循环队列

```go
import (
    "fmt"
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/su_obj_pool"
)

func main() {
    // 创建一个容量为100的循环队列
    queue := su_obj_pool.NewCircularQueue(100)
    
    // 入队
    err := queue.Enqueue("item1")
    if err != nil {
        fmt.Println("队列已满:", err)
    }
    
    // 判断队列是否为空
    if !queue.IsEmpty() {
        // 出队
        item, err := queue.Dequeue()
        if err != nil {
            fmt.Println("队列为空:", err)
        } else {
            fmt.Println("获取到元素:", item)
        }
    }
    
    // 判断队列是否已满
    if queue.IsFull() {
        fmt.Println("队列已满")
    }
}
```

## 工作原理

### 限制大小的对象池

`Sized`对象池基于Go标准库的`sync.Pool`实现，但增加了对象数量的限制：

1. 使用原子操作(`atomic`)确保线程安全
2. 维护当前池中对象数量的计数器
3. 当池中对象数量达到限制时，不再接受新的对象
4. `Try`方法提供了获取、使用和归还对象的便捷方式

### 循环队列

`Circular`循环队列使用一个固定大小的数组和两个指针(head和tail)实现：

1. 使用原子操作确保线程安全，无需加锁
2. 使用比较并交换(CAS)操作实现无锁并发访问
3. 当`tail+1`等于`head`时队列满
4. 当`head`等于`tail`时队列空
5. 出队时移除对象引用，辅助GC回收内存

## 性能特点

1. **无锁设计**：两种池实现都使用原子操作而非互斥锁，在高并发场景下有更好的性能
2. **内存优化**：通过复用对象减少GC压力，降低内存分配和回收开销
3. **资源控制**：提供对象数量限制，防止资源耗尽
4. **自动清理**：出队操作会清除对象引用，帮助垃圾回收

## 注意事项

1. **类型安全**：从池中获取对象后需要进行类型断言，使用前请确保类型正确

2. **对象重置**：从对象池获取对象后，可能需要重置对象状态，因为对象可能是重用的

3. **资源释放**：使用完对象后务必调用`Put`方法归还，否则会导致资源泄露
   
4. **循环队列容量**：创建循环队列时需指定容量，且容量是固定的，无法动态调整

5. **错误处理**：循环队列的`Enqueue`和`Dequeue`方法会返回错误，需要妥善处理
   - `ErrQueueFull`: 队列已满，无法入队
   - `ErrQueueEmpty`: 队列为空，无法出队
   - `ErrNoObject`: 对象池中无可用对象
