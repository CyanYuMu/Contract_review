package modelconfig

import "time"

const (
	ProviderArk              = "ark"
	ProviderOpenAI           = "openai"
	ProviderOpenAICompatible = "openai_compatible"
	ProviderDashScope        = "dashscope"
	ProviderDeepSeek         = "deepseek"
	ProviderMoonshot         = "moonshot"
	ProviderZhipu            = "zhipu"
	ProviderSiliconFlow      = "siliconflow"
	ProviderOllama           = "ollama"
)

type ModelConfig struct {
	ID           uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	ProviderType string    `gorm:"type:varchar(64);not null;default:'openai_compatible'" json:"provider_type"`
	ModelName    string    `gorm:"type:varchar(128);not null" json:"model_name"`
	APIURL       string    `gorm:"type:varchar(512);not null" json:"api_url"`
	APIKey       string    `gorm:"type:varchar(512)" json:"api_key"`
	IsDefault    bool      `gorm:"not null;default:true" json:"is_default"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ModelConfigRequest struct {
	ProviderType string `json:"provider_type"`
	ModelName    string `json:"model_name" binding:"required"`
	APIURL       string `json:"api_url" binding:"required"`
	APIKey       string `json:"api_key"`
	IsDefault    *bool  `json:"is_default"`
}

type ProviderOption struct {
	Label      string `json:"label"`
	Value      string `json:"value"`
	DefaultURL string `json:"default_url"`
	Example    string `json:"example"`
}
