package stack

import (
	"fmt"
	"runtime"
	"strings"
)

type Option struct {
	Skip      int    // 跳过多少层
	Levels    int    // 要获取的堆栈层级数
	Size      int    // 每一层堆栈信息的大小
	Separator string // 自定义分隔符
}

func (o *Option) Apply(opt *Option) *Option {
	if opt == nil {
		return o
	}

	if opt.Levels > 0 {
		o.Levels = opt.Levels
	}
	if opt.Size > 0 {
		o.Size = opt.Size
	}
	if opt.Separator != "" {
		o.Separator = opt.Separator
	}

	if opt.Skip > 0 {
		o.Skip = opt.Skip
	}

	return o
}

var dft = &Option{
	Levels:    10,
	Size:      1024,
	Separator: "\n", // 默认为换行符
	Skip:      0,
}

/*Get
* @Description: 获取堆栈信息
* @param opt 可选项, 默认10层调用链
* @return string
 */
func Get(opt *Option) string {
	options := dft.Apply(opt)
	var stackInfo strings.Builder

	pc := make([]uintptr, options.Levels)
	n := runtime.Callers(0, pc)
	frames := runtime.CallersFrames(pc[:n])
	for {
		frame, more := frames.Next()
		// 截取文件路径，只保留最后三层
		filePathParts := strings.Split(frame.File, "/")
		if len(filePathParts) > 3 {
			frame.File = strings.Join(filePathParts[len(filePathParts)-3:], "/")
		}
		stackInfo.WriteString(fmt.Sprintf("%s:%d %s", frame.File, frame.Line, frame.Function))
		if !more || stackInfo.Len() >= options.Size {
			break
		}
		stackInfo.WriteString(options.Separator)
	}

	return stackInfo.String()
}
