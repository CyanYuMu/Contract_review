package Initialize

import (
	"context"

	"contract_review/app/internal/agent"
	"contract_review/app/internal/global"
	"contract_review/app/internal/llm"

	fmodel "github.com/cloudwego/eino/components/model"
	"go.uber.org/zap"
)

// InitLLM 初始化LLM客户端（保留原有函数）
func InitLLM(ctx context.Context) (fmodel.BaseChatModel, error) {
	chatModel, err := llm.NewChatModel(ctx)
	if err != nil {
		zap.L().Error("init chat model failed", zap.Error(err))
		return nil, err
	}

	return chatModel, nil
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
