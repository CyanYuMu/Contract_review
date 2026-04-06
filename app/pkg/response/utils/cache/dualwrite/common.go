package dualwrite

import (
	"fmt"
	"runtime"
	"strings"
	"time"
)

// 超时配置常量
const (
	// SingleCmdTimeout 单个命令双写超时时间
	SingleCmdTimeout = 5 * time.Second
	// PipelineCmdTimeout Pipeline 批量双写超时时间
	PipelineCmdTimeout = 10 * time.Second
	// SampleRate 抽样日志频率（千分之一）
	// 正式环境需要改成1000，测试环境暂时改成1
	SampleRate = 1000
)

// multiKeyCmds 在集群模式下需要拆分的多key命令
// 这些命令语法为：CMD key1 key2 ...，在Cluster模式下多key必须在同一slot，否则报CROSSSLOT
var multiKeyCmds = map[string]bool{
	"DEL": true, "UNLINK": true,
}

// IsMultiKeyCmd 判断是否为需要按key拆分执行的多key命令
func IsMultiKeyCmd(cmdName string) bool {
	return multiKeyCmds[strings.ToUpper(cmdName)]
}

// 幂等命令白名单（只有这些命令在增量同步期间才双写）
var idempotentCommands = map[string]bool{
	"SET": true, "SETEX": true, "PSETEX": true, "SETNX": true,
	"HSET": true, "HMSET": true, "HSETNX": true,
	"SADD": true,
	"ZADD": true,
	"DEL":  true, "UNLINK": true,
	"HDEL": true,
	"SREM": true,
	"ZREM": true, "ZREMRANGEBYRANK": true, "ZREMRANGEBYSCORE": true,
	"EXPIRE": true, "EXPIREAT": true, "PEXPIRE": true, "PEXPIREAT": true,
	"PERSIST": true,
	"RENAME":  true, "RENAMENX": true,
	"SCRIPT": true, // SCRIPT LOAD 是幂等的，确保新库也有脚本缓存
}

// 读命令列表
var readCommands = map[string]bool{
	"GET": true, "MGET": true, "GETRANGE": true, "STRLEN": true,
	"HGET": true, "HMGET": true, "HGETALL": true, "HKEYS": true, "HVALS": true, "HLEN": true, "HEXISTS": true, "HSCAN": true,
	"LRANGE": true, "LINDEX": true, "LLEN": true,
	"SMEMBERS": true, "SISMEMBER": true, "SCARD": true, "SRANDMEMBER": true, "SSCAN": true, "SMISMEMBER": true,
	"ZRANGE": true, "ZREVRANGE": true, "ZRANGEBYSCORE": true, "ZREVRANGEBYSCORE": true,
	"ZSCORE": true, "ZRANK": true, "ZREVRANK": true, "ZCARD": true, "ZCOUNT": true, "ZSCAN": true,
	"EXISTS": true, "TYPE": true, "TTL": true, "PTTL": true, "KEYS": true, "SCAN": true,
	"GETBIT": true, "BITCOUNT": true, "BITPOS": true,
	"PFCOUNT": true,
	"INFO":    true, "DBSIZE": true, "DEBUG": true, "TIME": true,
	// Stream 读命令--！！XREADGROUP 会更新消费位置，如果不双写则会丢失位置信息
	"XREAD": true, "XREADGROUP": true, "XRANGE": true, "XREVRANGE": true, "XLEN": true,
	"XPENDING": true, "XINFO": true,
}

// IsReadCommand 判断是否为读命令
func IsReadCommand(cmdName string) bool {
	return readCommands[strings.ToUpper(cmdName)]
}

// IsIdempotentCommand 判断是否为幂等命令
func IsIdempotentCommand(cmdName string) bool {
	return idempotentCommands[strings.ToUpper(cmdName)]
}

// ShouldWriteToNew 判断是否需要写入新库（公共逻辑）
func ShouldWriteToNew(cmdName string, config *Config) bool {
	// 读命令不需要双写
	if IsReadCommand(cmdName) {
		return false
	}

	// 增量同步运行中
	if config.SyncRunning {
		// 只有幂等命令才双写
		return IsIdempotentCommand(cmdName)
	}

	// 增量同步已停止，所有写命令都双写
	return true
}

// LogSampleWrite 单个命令抽样日志
func LogSampleWrite(cmdName string, args []interface{}, config *Config) {
	argsStr := formatArgs(args, 200) // 限制参数长度
	fmt.Printf("[DualWrite] sample log: cmd=%s, args=%s, syncRunning=%v, readFrom=%s\n",
		cmdName, argsStr, config.SyncRunning, config.ReadFrom)
}

// LogSamplePipeline Pipeline 抽样日志
func LogSamplePipeline(cmds []interface{}, config *Config) {
	cmdInfo := formatCmdList(cmds, 10) // 最多显示10个命令
	fmt.Printf("[DualWrite] sample log pipeline: cmdCount=%d, cmds=%s, syncRunning=%v, readFrom=%s\n",
		len(cmds), cmdInfo, config.SyncRunning, config.ReadFrom)
}

// LogWriteError 单个命令写入失败日志
func LogWriteError(cmdName string, args []interface{}, err error) {
	argsStr := formatArgs(args, 200) // 限制参数长度
	fmt.Printf("[DualWrite] write to new redis failed: cmd=%s, args=%s, err=%v\n", cmdName, argsStr, err)
}

// LogPipelineError Pipeline 写入失败日志
func LogPipelineError(cmds []interface{}, err error) {
	cmdInfo := formatCmdList(cmds, 10) // 最多显示10个命令
	fmt.Printf("[DualWrite] pipeline write to new redis failed: cmdCount=%d, cmds=%s, err=%v\n", len(cmds), cmdInfo, err)
}

// formatCmdList 格式化命令列表（用于日志）
func formatCmdList(cmds []interface{}, maxCount int) string {
	if len(cmds) == 0 {
		return "[]"
	}

	var cmdNames []string
	count := len(cmds)
	if count > maxCount {
		count = maxCount
	}

	for i := 0; i < count; i++ {
		if cmd, ok := cmds[i].(interface{ Name() string }); ok {
			cmdNames = append(cmdNames, cmd.Name())
		} else if cmd, ok := cmds[i].(interface{ Args() []interface{} }); ok {
			args := cmd.Args()
			if len(args) > 0 {
				cmdNames = append(cmdNames, fmt.Sprintf("%v", args[0]))
			}
		}
	}

	result := "[" + strings.Join(cmdNames, ",") + "]"
	if len(cmds) > maxCount {
		result += fmt.Sprintf("...(%d more)", len(cmds)-maxCount)
	}
	return result
}

// formatArgs 格式化参数（限制长度，避免日志过长）
func formatArgs(args []interface{}, maxLen int) string {
	str := fmt.Sprintf("%v", args)
	if len(str) > maxLen {
		return str[:maxLen] + "...(truncated)"
	}
	return str
}

// LogPanicRecovered panic 恢复日志
func LogPanicRecovered(source string, r interface{}) {
	// 获取堆栈信息（限制前 10 层）
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	stackTrace := string(buf[:n])

	// 只保留前几层堆栈，避免日志过长
	lines := strings.Split(stackTrace, "\n")
	if len(lines) > 20 {
		lines = lines[:20]
		stackTrace = strings.Join(lines, "\n") + "\n...(truncated)"
	} else {
		stackTrace = strings.Join(lines, "\n")
	}

	fmt.Printf("[DualWrite] panic recovered in %s: %v\nStack trace:\n%s\n", source, r, stackTrace)
}

// SafeGo 安全执行 goroutine（带 panic 恢复）
func SafeGo(source string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				LogPanicRecovered(source, r)
			}
		}()
		fn()
	}()
}
