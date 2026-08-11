package gateway

// priceEntry 每 1M token 单价（人民币元）
type priceEntry struct {
	prompt     float64
	completion float64
}

// pricingTable 部分常见模型每 1M token 单价（元），未命中时按 provider 默认估算。
// 仅用于成本可观测，非计费口径，可按需在配置中覆盖。
var pricingTable = map[string]priceEntry{
	// qwen
	"qwen-plus":       {prompt: 0.8, completion: 2},
	"qwen-turbo":      {prompt: 0.3, completion: 0.6},
	"qwen-max":        {prompt: 20, completion: 60},
	"qwen-long":       {prompt: 0.5, completion: 2},
	// deepseek
	"deepseek-chat":   {prompt: 1, completion: 2},
	"deepseek-reasoner": {prompt: 4, completion: 16},
	// openai
	"gpt-4o":          {prompt: 18, completion: 72},
	"gpt-4o-mini":     {prompt: 1, completion: 4},
	"gpt-4.1":         {prompt: 15, completion: 60},
	"gpt-4.1-mini":    {prompt: 2, completion: 8},
	// glm
	"glm-4":           {prompt: 50, completion: 50},
	"glm-4-flash":     {prompt: 0, completion: 0},
	"glm-4-air":       {prompt: 0.5, completion: 0.5},
	// moonshot
	"moonshot-v1-8k":  {prompt: 12, completion: 12},
	"moonshot-v1-32k": {prompt: 24, completion: 24},
	// siliconflow 通用
	"Qwen/Qwen2.5-7B-Instruct": {prompt: 0, completion: 0},
}

// provider 默认每 1M token 单价（未命中具体模型时使用）
var providerDefaultPrice = map[string]priceEntry{
	"openai_compatible": {prompt: 1, completion: 4},
	"dashscope":         {prompt: 0.8, completion: 2},
	"deepseek":          {prompt: 1, completion: 2},
	"openai":            {prompt: 10, completion: 30},
	"moonshot":          {prompt: 12, completion: 12},
	"zhipu":             {prompt: 5, completion: 5},
	"siliconflow":       {prompt: 0.5, completion: 0.5},
	"ark":               {prompt: 2, completion: 6},
	"ollama":            {prompt: 0, completion: 0},
}

// estimateCost 按 provider+model+usage 估算成本（元）
func estimateCost(provider, model string, u Usage) float64 {
	entry, ok := pricingTable[model]
	if !ok {
		entry, ok = providerDefaultPrice[provider]
		if !ok {
			entry = providerDefaultPrice["openai_compatible"]
		}
	}
	prompt := float64(u.PromptTokens) / 1_000_000 * entry.prompt
	completion := float64(u.CompletionTokens) / 1_000_000 * entry.completion
	return round2(prompt + completion)
}

func round2(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}

// estimateTokens 粗略估算 token 数（当 provider 不返回 usage 时使用）
// 中文约 1 字 ≈ 1 token，英文约 4 字符 ≈ 1 token。
func estimateTokens(text string) int {
	if text == "" {
		return 0
	}
	cjk, other := 0, 0
	for _, r := range text {
		if r >= 0x4E00 && r <= 0x9FFF {
			cjk++
		} else if r > 32 {
			other++
		}
	}
	return cjk + other/4
}
