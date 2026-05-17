package modelconfig

import (
	"context"
	"contract_review/app/config"
	"errors"
	"strings"

	"gorm.io/gorm"
)

type Service struct {
	repo *Repo
}

func NewService(repo *Repo) *Service {
	return &Service{repo: repo}
}

func (s *Service) EnsureDefaultFromConfig(ctx context.Context, cfg *config.LLMConfig) error {
	_, err := s.repo.GetDefault(ctx)
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	defaultCfg := &ModelConfig{
		ProviderType: InferProvider(cfg.APIBase, cfg.Model),
		ModelName:    cfg.Model,
		APIURL:       cfg.APIBase,
		APIKey:       cfg.APIKey,
		IsDefault:    true,
	}
	return s.repo.Create(ctx, defaultCfg)
}

func (s *Service) GetDefault(ctx context.Context) (*ModelConfig, error) {
	return s.repo.GetDefault(ctx)
}

func (s *Service) List(ctx context.Context) ([]ModelConfig, error) {
	return s.repo.List(ctx)
}

func (s *Service) Create(ctx context.Context, req ModelConfigRequest) (*ModelConfig, error) {
	cfg, err := buildConfig(req)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (s *Service) Update(ctx context.Context, id uint, req ModelConfigRequest) (*ModelConfig, error) {
	cfg, err := buildConfig(req)
	if err != nil {
		return nil, err
	}
	return s.repo.Update(ctx, id, map[string]interface{}{
		"provider_type": cfg.ProviderType,
		"model_name":    cfg.ModelName,
		"api_url":       cfg.APIURL,
		"api_key":       cfg.APIKey,
		"is_default":    cfg.IsDefault,
	})
}

func buildConfig(req ModelConfigRequest) (*ModelConfig, error) {
	provider := normalizeProvider(req.ProviderType)
	apiURL := normalizeAPIURL(provider, req.APIURL)
	modelName := strings.TrimSpace(req.ModelName)
	apiKey := strings.TrimSpace(req.APIKey)
	if modelName == "" {
		return nil, errors.New("模型名称不能为空")
	}
	if apiURL == "" {
		return nil, errors.New("模型URL不能为空")
	}
	if provider != ProviderOllama && apiKey == "" {
		return nil, errors.New("API Key不能为空")
	}
	isDefault := true
	if req.IsDefault != nil {
		isDefault = *req.IsDefault
	}
	return &ModelConfig{
		ProviderType: provider,
		ModelName:    modelName,
		APIURL:       apiURL,
		APIKey:       apiKey,
		IsDefault:    isDefault,
	}, nil
}

func normalizeAPIURL(provider string, apiURL string) string {
	url := strings.TrimRight(strings.TrimSpace(apiURL), "/")
	if provider == ProviderArk {
		url = strings.TrimSuffix(url, "/bots/chat/completions")
		url = strings.TrimSuffix(url, "/chat/completions")
	}
	return url
}

func normalizeProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case ProviderArk, "volcengine", "volcano":
		return ProviderArk
	case ProviderOpenAI:
		return ProviderOpenAI
	case ProviderDashScope, "aliyun", "qwen":
		return ProviderDashScope
	case ProviderDeepSeek:
		return ProviderDeepSeek
	case ProviderMoonshot, "kimi":
		return ProviderMoonshot
	case ProviderZhipu, "glm":
		return ProviderZhipu
	case ProviderSiliconFlow:
		return ProviderSiliconFlow
	case ProviderOllama:
		return ProviderOllama
	default:
		return ProviderOpenAICompatible
	}
}

func InferProvider(apiURL string, modelName string) string {
	u := strings.ToLower(apiURL)
	m := strings.ToLower(modelName)
	switch {
	case strings.Contains(u, "volces.com") || strings.Contains(u, "ark.cn-") || strings.Contains(m, "ep-"):
		return ProviderArk
	case strings.Contains(u, "dashscope.aliyuncs.com"):
		return ProviderDashScope
	case strings.Contains(u, "api.openai.com"):
		return ProviderOpenAI
	case strings.Contains(u, "api.deepseek.com"):
		return ProviderDeepSeek
	case strings.Contains(u, "moonshot.cn"):
		return ProviderMoonshot
	case strings.Contains(u, "bigmodel.cn"):
		return ProviderZhipu
	case strings.Contains(u, "siliconflow.cn"):
		return ProviderSiliconFlow
	case strings.Contains(u, "localhost") || strings.Contains(u, "127.0.0.1"):
		return ProviderOllama
	default:
		return ProviderOpenAICompatible
	}
}

func ProviderOptions() []ProviderOption {
	return []ProviderOption{
		{Label: "OpenAI", Value: ProviderOpenAI, DefaultURL: "https://api.openai.com/v1", Example: "gpt-4o-mini"},
		{Label: "OpenAI兼容", Value: ProviderOpenAICompatible, DefaultURL: "https://example.com/v1", Example: "model-name"},
		{Label: "阿里DashScope", Value: ProviderDashScope, DefaultURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", Example: "qwen-plus"},
		{Label: "DeepSeek", Value: ProviderDeepSeek, DefaultURL: "https://api.deepseek.com/v1", Example: "deepseek-chat"},
		{Label: "Moonshot/Kimi", Value: ProviderMoonshot, DefaultURL: "https://api.moonshot.cn/v1", Example: "moonshot-v1-8k"},
		{Label: "智谱GLM", Value: ProviderZhipu, DefaultURL: "https://open.bigmodel.cn/api/paas/v4", Example: "glm-4-flash"},
		{Label: "硅基流动", Value: ProviderSiliconFlow, DefaultURL: "https://api.siliconflow.cn/v1", Example: "Qwen/Qwen2.5-7B-Instruct"},
		{Label: "火山方舟Ark", Value: ProviderArk, DefaultURL: "https://ark.cn-beijing.volces.com/api/v3", Example: "ep-xxxxxxxx"},
		{Label: "Ollama本地", Value: ProviderOllama, DefaultURL: "http://localhost:11434/v1", Example: "qwen2.5"},
	}
}
