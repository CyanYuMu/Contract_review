package dualwrite

import (
	"context"
	"errors"
	"math/rand"
	"net"

	"github.com/redis/go-redis/v9"
)

// HookV9 双写 Hook（v9 版本）
// 实现 redis.Hook 接口（中间件模式），用于拦截 Redis 命令并实现双写
type HookV9 struct {
	newCli redis.UniversalClient // 新库客户端
	config *Config               // 双写配置
}

// NewHookV9 创建双写 Hook
func NewHookV9(newCli redis.UniversalClient, config *Config) *HookV9 {
	// 配置验证：防御性检查
	if config == nil {
		config = &Config{SyncRunning: false}
	}
	return &HookV9{
		newCli: newCli,
		config: config,
	}
}

// DialHook 连接钩子（v9 中间件模式）
func (h *HookV9) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		// 直接调用下一个钩子，不做额外处理
		return next(ctx, network, addr)
	}
}

// ProcessHook 单个命令处理钩子（v9 中间件模式）
func (h *HookV9) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		// 先执行旧库命令
		err := next(ctx, cmd)

		// 判断是否需要写入新库
		cmdName := cmd.Name()
		if ShouldWriteToNew(cmdName, h.config) {
			// 异步写入新库（传递 context 支持 tracing）
			SafeGo("writeToNew", func() {
				h.writeToNew(ctx, cmd)
			})
		}

		return err
	}
}

// ProcessPipelineHook Pipeline 处理钩子（v9 中间件模式）
func (h *HookV9) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		// 先执行旧库 Pipeline
		err := next(ctx, cmds)

		// 收集需要双写的命令
		var writeCommands []redis.Cmder
		for _, cmd := range cmds {
			if ShouldWriteToNew(cmd.Name(), h.config) {
				writeCommands = append(writeCommands, cmd)
			}
		}

		if len(writeCommands) > 0 {
			// 异步写入新库（传递 context 支持 tracing）
			SafeGo("writePipelineToNew", func() {
				h.writePipelineToNew(ctx, writeCommands)
			})
		}

		return err
	}
}

// writeToNew 写入新库
func (h *HookV9) writeToNew(parentCtx context.Context, cmd redis.Cmder) {
	// 设置超时（基于父 context 支持 tracing）
	ctx, cancel := context.WithTimeout(parentCtx, SingleCmdTimeout)
	defer cancel()

	// 获取命令参数
	args := cmd.Args()
	if len(args) == 0 {
		return
	}

	// 抽样日志：千分之一概率记录关键信息
	if rand.Intn(SampleRate) == 0 {
		LogSampleWrite(cmd.Name(), args, h.config)
	}

	// 对 DEL/UNLINK 等多key命令，在集群模式下需要逐key执行，避免CROSSSLOT错误
	if IsMultiKeyCmd(cmd.Name()) && len(args) > 2 {
		pipe := h.newCli.Pipeline()
		for _, key := range args[1:] {
			pipe.Do(ctx, args[0], key)
		}
		if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
			LogWriteError(cmd.Name(), args, err)
		}
		return
	}

	// 执行命令
	newCmd := h.newCli.Do(ctx, args...)
	if err := newCmd.Err(); err != nil && !errors.Is(err, redis.Nil) {
		LogWriteError(cmd.Name(), args, err)
	}
}

// writePipelineToNew 批量写入新库
func (h *HookV9) writePipelineToNew(parentCtx context.Context, cmds []redis.Cmder) {
	// 设置超时（基于父 context 支持 tracing）
	ctx, cancel := context.WithTimeout(parentCtx, PipelineCmdTimeout)
	defer cancel()

	// 抽样日志：千分之一概率记录关键信息
	if rand.Intn(SampleRate) == 0 {
		cmdList := make([]interface{}, len(cmds))
		for i, cmd := range cmds {
			cmdList[i] = cmd
		}
		LogSamplePipeline(cmdList, h.config)
	}

	pipe := h.newCli.Pipeline()
	for _, cmd := range cmds {
		args := cmd.Args()
		if len(args) > 0 {
			pipe.Do(ctx, args...)
		}
	}

	_, err := pipe.Exec(ctx)
	if err != nil && !errors.Is(err, redis.Nil) {
		cmdList := make([]interface{}, len(cmds))
		for i, cmd := range cmds {
			cmdList[i] = cmd
		}
		LogPipelineError(cmdList, err)
	}
}
