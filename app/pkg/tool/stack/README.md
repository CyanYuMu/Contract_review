# Stack工具包使用手册

## 概述

`stack`包提供了获取程序运行时堆栈信息的工具函数，可以帮助开发者在调试过程中追踪代码执行路径，对定位问题和记录日志特别有用。

## 功能特点

- **简单易用**：只需一个函数调用即可获取完整堆栈信息
- **高度可定制**：支持自定义堆栈层级数、信息大小和格式
- **优化显示**：自动简化文件路径，只保留关键部分
- **灵活配置**：通过Option结构体可以灵活控制输出格式

## 核心API

```go
// 获取堆栈信息的函数
func Get(opt *Option) string

// 配置选项
type Option struct {
    Skip      int    // 跳过多少层堆栈
    Levels    int    // 要获取的堆栈层级数（默认10层）
    Size      int    // 每一层堆栈信息的大小上限（默认1024字节）
    Separator string // 自定义分隔符（默认为换行符"\n"）
}
```

## 使用示例

### 基本用法

```go
import (
    "fmt"
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/stack"
)

func main() {
    // 使用默认配置获取堆栈信息
    stackInfo := stack.Get(nil)
    fmt.Println("当前堆栈信息:")
    fmt.Println(stackInfo)
}
```

### 自定义选项

```go
import (
    "fmt"
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/stack"
)

func main() {
    // 自定义堆栈信息选项
    opt := &stack.Option{
        Levels:    5,         // 只获取5层堆栈
        Size:      2048,      // 允许更大的信息量
        Separator: " -> ",    // 使用箭头作为分隔符
        Skip:      1,         // 跳过最顶层调用
    }
    
    stackInfo := stack.Get(opt)
    fmt.Println("自定义堆栈信息:")
    fmt.Println(stackInfo)
}
```

### 在日志记录中使用

```go
import (
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/stack"
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
)

func someFunction() error {
    // 发生错误时
    err := someOperation()
    if err != nil {
        // 获取堆栈信息并记录到日志
        stackInfo := stack.Get(nil)
        su_logger.Error(ctx, err, "操作失败", 
            su_logger.E().String("stack_trace", stackInfo))
        return err
    }
    return nil
}
```

## 工作原理

`stack.Get`函数通过以下步骤获取堆栈信息:

1. 使用`runtime.Callers`获取程序计数器（PC）值
2. 将PC值转换为堆栈帧信息
3. 对每个堆栈帧处理文件路径（只保留最后三层路径）
4. 将文件名、行号和函数名组合成可读格式
5. 使用分隔符连接各个堆栈帧信息

## 注意事项

1. **性能考虑**：堆栈获取会有一定的性能开销，不建议在高频调用的代码路径中使用

2. **默认值**：
   - 默认获取10层堆栈信息
   - 默认每层信息大小上限为1024字节
   - 默认使用换行符作为分隔符

3. **路径简化**：文件路径会自动简化，只保留最后三层目录，使输出更加清晰

4. **Skip参数**：如果您在封装`stack.Get`函数，可能需要增加`Skip`值，以跳过包装函数自身的堆栈信息
