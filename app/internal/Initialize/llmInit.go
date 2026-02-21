package Initialize

import (
	"context"

	"contract_review/app/internal/agent"
	"contract_review/app/internal/global"

	"github.com/cloudwego/eino-ext/components/model/arkbot"
	"go.uber.org/zap"
)

// InitLLM 初始化LLM客户端（保留原有函数）
func InitLLM(ctx context.Context) (*arkbot.ChatModel, error) {
	llm, err := arkbot.NewChatModel(ctx,
		&arkbot.Config{
			Model:  global.Config.LLMConfig.Model,
			APIKey: global.Config.LLMConfig.APIKey,
		})
	if err != nil {
		global.Log.Error("init arkbot model failed", zap.Error(err))
		return nil, err
	}

	return llm, nil
}

// InitContractAgent 初始化合同解析智能体
func InitContractAgent(ctx context.Context) error {
	err := agent.InitContractAgent(ctx)
	if err != nil {
		global.Log.Error("init contract agent failed", zap.Error(err))
		return err
	}
	global.Log.Info("contract agent initialized successfully")
	return nil
}
