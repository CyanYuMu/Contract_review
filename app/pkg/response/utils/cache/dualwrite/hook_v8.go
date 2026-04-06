package dualwrite

import (
	"context"
	"errors"
	"math/rand"

	"github.com/go-redis/redis/v8"
)

// HookV8 双写 Hook（v8 版本）
// 实现 redis.Hook 接口，用于拦截 Redis 命令并实现双写
type HookV8 struct {
	newCli redis.UniversalClient // 新库客户端
	config *Config               // 双写配置
}

// NewHookV8 创建双写 Hook
func NewHookV8(newCli redis.UniversalClient, config *Config) *HookV8 {
	// 配置验证：防御性检查
	if config == nil {
		config = &Config{SyncRunning: false}
	}
	return &HookV8{
		newCli: newCli,
		config: config,
	}
}

// BeforeProcess 单个命令执行前
func (h *HookV8) BeforeProcess(ctx context.Context, cmd redis.Cmder) (context.Context, error) {
	return ctx, nil
}

// AfterProcess 单个命令执行后
func (h *HookV8) AfterProcess(ctx context.Context, cmd redis.Cmder) error {
	cmdName := cmd.Name()

	// 判断是否需要写入新库
	if !ShouldWriteToNew(cmdName, h.config) {
		return nil
	}

	// 异步写入新库（传递 context 支持 tracing）
	SafeGo("writeToNew", func() {
		h.writeToNew(ctx, cmd)
	})

	return nil
}

// BeforeProcessPipeline Pipeline 执行前
func (h *HookV8) BeforeProcessPipeline(ctx context.Context, cmds []redis.Cmder) (context.Context, error) {
	return ctx, nil
}

// AfterProcessPipeline Pipeline 执行后
func (h *HookV8) AfterProcessPipeline(ctx context.Context, cmds []redis.Cmder) error {
	// 收集需要双写的命令
	var writeCommands []redis.Cmder
	for _, cmd := range cmds {
		if ShouldWriteToNew(cmd.Name(), h.config) {
			writeCommands = append(writeCommands, cmd)
		}
	}

	if len(writeCommands) == 0 {
		return nil
	}

	// 异步写入新库（传递 context 支持 tracing）
	SafeGo("writePipelineToNew", func() {
		h.writePipelineToNew(ctx, writeCommands)
	})

	return nil
}

// writeToNew 写入新库
func (h *HookV8) writeToNew(parentCtx context.Context, cmd redis.Cmder) {
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
		// 日志增强：添加 args 详情便于排查
		LogWriteError(cmd.Name(), args, err)
	}
}

// writePipelineToNew 批量写入新库
func (h *HookV8) writePipelineToNew(parentCtx context.Context, cmds []redis.Cmder) {
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
