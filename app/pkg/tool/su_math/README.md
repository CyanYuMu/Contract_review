# Su_math数学工具包

## 概述

`su_math`包提供了一组简单实用的数学工具函数，包括泛型支持的最大最小值获取、默认值处理、三元表达式替代以及随机数生成等功能。这些工具函数可以帮助开发者简化代码，提高开发效率。

## 主要功能

### 1. 数值比较与处理 (number.go)

提供了泛型支持的数值操作函数：

```go
// 获取最大值（支持所有可比较类型）
func GetMax[T constraints.Ordered](i ...T) T

// 获取最小值（支持所有可比较类型）
func GetMin[T constraints.Ordered](i ...T) T

// 获取值或默认值（如果为0则返回默认值）
func GetWithDefault[T Number](i T, dft T) T

// 三元表达式替代（支持所有可比较类型）
func Ternary[T constraints.Ordered](ok bool, trueA, falseB T) T
```

### 2. 随机数生成 (rand.go)

提供了多种类型随机数生成函数：

```go
// 生成指定范围内的随机整数
func RandInt(min, max int) int

// 生成指定范围内的随机int64整数
func RandInt64(min, max int) int64

// 生成指定范围内的随机float32浮点数
func RandFloat32(min, max float32) float32

// 生成指定范围内的随机float64浮点数
func RandFloat64(min, max float64) float64
```

## 使用示例

### 获取最大最小值

```go
import (
    "fmt"
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/su_math"
)

func main() {
    // 获取整数最大值
    max := su_math.GetMax(1, 5, 3, 9, 2)
    fmt.Println("最大值:", max) // 输出: 最大值: 9
    
    // 获取浮点数最小值
    min := su_math.GetMin(3.14, 2.71, 1.41, 1.73)
    fmt.Println("最小值:", min) // 输出: 最小值: 1.41
    
    // 获取字符串最大值（按字典序）
    maxStr := su_math.GetMax("apple", "banana", "orange")
    fmt.Println("字典序最大:", maxStr) // 输出: 字典序最大: orange
}
```

### 默认值处理

```go
import (
    "fmt"
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/su_math"
)

func main() {
    // 如果值为0，则使用默认值
    value1 := su_math.GetWithDefault(0, 10)
    fmt.Println("值1:", value1) // 输出: 值1: 10
    
    // 如果值不为0，则使用原值
    value2 := su_math.GetWithDefault(5, 10)
    fmt.Println("值2:", value2) // 输出: 值2: 5
}
```

### 三元表达式

```go
import (
    "fmt"
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/su_math"
)

func main() {
    // 根据条件返回不同的值
    age := 20
    status := su_math.Ternary(age >= 18, "成年", "未成年")
    fmt.Println("状态:", status) // 输出: 状态: 成年
    
    // 用于数值计算
    discount := su_math.Ternary(age < 12, 0.5, 1.0)
    price := 100.0 * discount
    fmt.Println("价格:", price) // 输出: 价格: 100
}
```

### 随机数生成

```go
import (
    "fmt"
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/su_math"
)

func main() {
    // 生成1到100之间的随机整数
    randInt := su_math.RandInt(1, 100)
    fmt.Println("随机整数:", randInt)
    
    // 生成0.0到1.0之间的随机浮点数
    randFloat := su_math.RandFloat64(0.0, 1.0)
    fmt.Println("随机浮点数:", randFloat)
}
```

## 特性说明

### 泛型支持

本包充分利用了Go的泛型特性，使得同一个函数可以处理多种数据类型：

- 所有的比较函数支持`constraints.Ordered`类型，包括整数、浮点数和字符串
- `Number`接口定义了可用于数值计算的类型，包括整数和浮点数

### 随机数生成

随机数生成函数每次调用都会使用当前时间作为种子，确保生成的随机数在不同调用间具有良好的随机性。

## 注意事项

1. **GetMax和GetMin函数**：这些函数要求至少传入一个参数，否则会导致运行时错误

2. **GetWithDefault函数**：只有当第一个参数为0时才会返回默认值，对于非0值总是返回原值

3. **随机数生成**：
   - 每次调用随机函数都会重新创建随机源，适合偶尔调用的场景
   - 如果需要频繁生成随机数，建议创建一个全局随机源以提高性能
   - `RandInt`和`RandInt64`的范围是包含`min`和`max`的闭区间
   - `RandFloat32`和`RandFloat64`的范围是包含`min`但不包含`max`的左闭右开区间
