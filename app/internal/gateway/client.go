package gateway

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"contract_review/app/internal/modelconfig"

	"github.com/cloudwego/eino/schema"
)

// ---- OpenAI 兼容请求/响应结构 ----

type oaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type oaiStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type oaiChatRequest struct {
	Model         string          `json:"model"`
	Messages      []oaiMessage    `json:"messages"`
	Temperature   *float64        `json:"temperature,omitempty"`
	MaxTokens     *int            `json:"max_tokens,omitempty"`
	TopP          *float64        `json:"top_p,omitempty"`
	Stream        bool            `json:"stream,omitempty"`
	StreamOptions *oaiStreamOptions `json:"stream_options,omitempty"`
}

type oaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type oaiChoice struct {
	Message      oaiMessage `json:"message"`
	Delta        oaiMessage `json:"delta"`
	FinishReason string     `json:"finish_reason"`
}

type oaiChatResponse struct {
	Choices []oaiChoice `json:"choices"`
	Usage   *oaiUsage   `json:"usage,omitempty"`
	Error   *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error,omitempty"`
}

type oaiStreamChunk struct {
	Choices []oaiChoice `json:"choices"`
	Usage   *oaiUsage   `json:"usage,omitempty"`
}

// resolveModel 按 feature 解析模型配置：路由命中则用路由，否则用默认配置。
func (g *Gateway) resolveModel(ctx context.Context, feature string) (*modelconfig.ModelConfig, error) {
	route, err := g.repo.GetRoute(ctx, feature)
	if err == nil && route != nil {
		var cfg modelconfig.ModelConfig
		if e := g.db.WithContext(ctx).First(&cfg, route.ModelConfigID).Error; e == nil && cfg.ID != 0 {
			return &cfg, nil
		}
	}
	return g.modelRepo.GetDefault(ctx)
}

func completionURL(apiURL string) string {
	base := strings.TrimRight(strings.TrimSpace(apiURL), "/")
	if strings.HasSuffix(base, "/chat/completions") {
		return base
	}
	return base + "/chat/completions"
}

func (g *Gateway) buildRequest(cfg *modelconfig.ModelConfig, messages []*schema.Message, opts *ChatOptions, stream bool) (*http.Request, error) {
	reqBody := oaiChatRequest{
		Model:    cfg.ModelName,
		Messages: make([]oaiMessage, 0, len(messages)),
		Stream:   stream,
	}
	if stream {
		reqBody.StreamOptions = &oaiStreamOptions{IncludeUsage: true}
	}
	if opts != nil {
		if opts.Temperature > 0 {
			t := opts.Temperature
			reqBody.Temperature = &t
		}
		if opts.MaxTokens > 0 {
			m := opts.MaxTokens
			reqBody.MaxTokens = &m
		}
		if opts.TopP > 0 {
			p := opts.TopP
			reqBody.TopP = &p
		}
	}
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		role := string(msg.Role)
		if role == "" {
			role = "user"
		}
		reqBody.Messages = append(reqBody.Messages, oaiMessage{Role: role, Content: msg.Content})
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, completionURL(cfg.APIURL), bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(cfg.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	return req, nil
}

// callGenerate 非流式调用，返回消息内容与 usage。
func (g *Gateway) callGenerate(ctx context.Context, cfg *modelconfig.ModelConfig, messages []*schema.Message, opts *ChatOptions) (string, Usage, error) {
	req, err := g.buildRequest(cfg, messages, opts, false)
	if err != nil {
		return "", Usage{}, err
	}
	req = req.WithContext(ctx)

	client := &http.Client{Timeout: g.requestTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", Usage{}, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", Usage{}, err
	}

	var parsed oaiChatResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return "", Usage{}, fmt.Errorf("解析模型响应失败: %w, body: %s", err, truncate(string(respBytes), 512))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if parsed.Error != nil && parsed.Error.Message != "" {
			return "", Usage{}, fmt.Errorf("模型请求失败(%d): %s", resp.StatusCode, parsed.Error.Message)
		}
		return "", Usage{}, fmt.Errorf("模型请求失败(%d): %s", resp.StatusCode, truncate(string(respBytes), 512))
	}
	if len(parsed.Choices) == 0 {
		return "", Usage{}, fmt.Errorf("模型响应为空")
	}

	content := parsed.Choices[0].Message.Content
	usage := Usage{}
	if parsed.Usage != nil {
		usage.PromptTokens = parsed.Usage.PromptTokens
		usage.CompletionTokens = parsed.Usage.CompletionTokens
		usage.TotalTokens = parsed.Usage.TotalTokens
	} else {
		usage.PromptTokens = estimateTokens(joinMessageTexts(messages))
		usage.CompletionTokens = estimateTokens(content)
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	return content, usage, nil
}

// callStream 流式调用，通过 onDelta 回调逐 token 推送，返回完整内容与 usage。
func (g *Gateway) callStream(ctx context.Context, cfg *modelconfig.ModelConfig, messages []*schema.Message, opts *ChatOptions, onDelta func(string)) (string, Usage, error) {
	req, err := g.buildRequest(cfg, messages, opts, true)
	if err != nil {
		return "", Usage{}, err
	}
	req = req.WithContext(ctx)

	client := &http.Client{Timeout: g.requestTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", Usage{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", Usage{}, fmt.Errorf("模型请求失败(%d): %s", resp.StatusCode, truncate(string(body), 512))
	}

	var content strings.Builder
	usage := Usage{}
	scanner := bufio.NewScanner(resp.Body)
	// 单行可能较大，调高缓冲
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk oaiStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) > 0 {
			delta := chunk.Choices[0].Delta.Content
			if delta != "" {
				content.WriteString(delta)
				if onDelta != nil {
					onDelta(delta)
				}
			}
		}
		if chunk.Usage != nil {
			usage.PromptTokens = chunk.Usage.PromptTokens
			usage.CompletionTokens = chunk.Usage.CompletionTokens
			usage.TotalTokens = chunk.Usage.TotalTokens
		}
	}
	if err := scanner.Err(); err != nil {
		return content.String(), usage, err
	}
	if usage.TotalTokens == 0 {
		usage.PromptTokens = estimateTokens(joinMessageTexts(messages))
		usage.CompletionTokens = estimateTokens(content.String())
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	return content.String(), usage, nil
}

func joinMessageTexts(messages []*schema.Message) string {
	var sb strings.Builder
	for _, m := range messages {
		if m == nil {
			continue
		}
		sb.WriteString(m.Content)
	}
	return sb.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
