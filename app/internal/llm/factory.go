package llm

import (
	"context"
	"contract_review/app/internal/global"
	"contract_review/app/internal/modelconfig"
	"errors"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/ark"
	fmodel "github.com/cloudwego/eino/components/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func NewChatModel(ctx context.Context) (fmodel.BaseChatModel, error) {
	cfg, err := currentConfig(ctx)
	if err != nil {
		return nil, err
	}

	provider := resolveProvider(cfg)
	global.Log.Info("create chat model",
		zap.String("provider", provider),
		zap.String("model", cfg.ModelName),
		zap.String("api_url", cfg.APIURL),
	)

	switch provider {
	case modelconfig.ProviderArk:
		apiURL := normalizeArkBaseURL(cfg.APIURL)
		return ark.NewChatModel(ctx, &ark.ChatModelConfig{
			Model:   cfg.ModelName,
			APIKey:  cfg.APIKey,
			BaseURL: apiURL,
		})
	default:
		return NewOpenAICompatibleModel(cfg.ModelName, cfg.APIURL, cfg.APIKey), nil
	}
}

func normalizeArkBaseURL(apiURL string) string {
	url := strings.TrimRight(strings.TrimSpace(apiURL), "/")
	url = strings.TrimSuffix(url, "/bots/chat/completions")
	url = strings.TrimSuffix(url, "/chat/completions")
	return url
}

func resolveProvider(cfg *modelconfig.ModelConfig) string {
	inferred := modelconfig.InferProvider(cfg.APIURL, cfg.ModelName)
	if cfg.ProviderType == "" {
		return inferred
	}
	if cfg.ProviderType == modelconfig.ProviderArk && inferred != modelconfig.ProviderArk {
		return inferred
	}
	return cfg.ProviderType
}

func currentConfig(ctx context.Context) (*modelconfig.ModelConfig, error) {
	if global.DB != nil {
		repo := modelconfig.NewRepo(global.DB)
		cfg, err := repo.GetDefault(ctx)
		if err == nil {
			return cfg, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	return &modelconfig.ModelConfig{
		ProviderType: modelconfig.InferProvider(global.Config.LLMConfig.APIBase, global.Config.LLMConfig.Model),
		ModelName:    global.Config.LLMConfig.Model,
		APIURL:       global.Config.LLMConfig.APIBase,
		APIKey:       global.Config.LLMConfig.APIKey,
		IsDefault:    true,
	}, nil
}
