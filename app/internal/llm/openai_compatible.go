package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	fmodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type OpenAICompatibleModel struct {
	model   string
	apiURL  string
	apiKey  string
	timeout time.Duration
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message openAIMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error,omitempty"`
}

func NewOpenAICompatibleModel(modelName, apiURL, apiKey string) *OpenAICompatibleModel {
	return &OpenAICompatibleModel{
		model:   modelName,
		apiURL:  apiURL,
		apiKey:  apiKey,
		timeout: 5 * time.Minute,
	}
}

func (m *OpenAICompatibleModel) Generate(ctx context.Context, input []*schema.Message, _ ...fmodel.Option) (*schema.Message, error) {
	reqBody := chatCompletionRequest{
		Model:    m.model,
		Messages: make([]openAIMessage, 0, len(input)),
	}
	for _, msg := range input {
		if msg == nil {
			continue
		}
		role := string(msg.Role)
		if role == "" {
			role = "user"
		}
		reqBody.Messages = append(reqBody.Messages, openAIMessage{
			Role:    role,
			Content: msg.Content,
		})
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, completionURL(m.apiURL), bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(m.apiKey) != "" {
		req.Header.Set("Authorization", "Bearer "+m.apiKey)
	}

	client := &http.Client{Timeout: m.timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var parsed chatCompletionResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, fmt.Errorf("解析模型响应失败: %w, body: %s", err, string(respBytes))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if parsed.Error != nil && parsed.Error.Message != "" {
			return nil, fmt.Errorf("模型请求失败（%d）: %s", resp.StatusCode, parsed.Error.Message)
		}
		return nil, fmt.Errorf("模型请求失败（%d）: %s", resp.StatusCode, string(respBytes))
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("模型响应为空")
	}

	return &schema.Message{
		Role:    schema.Assistant,
		Content: parsed.Choices[0].Message.Content,
	}, nil
}

func (m *OpenAICompatibleModel) Stream(ctx context.Context, input []*schema.Message, opts ...fmodel.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, fmt.Errorf("OpenAI-compatible stream is not implemented, use Generate instead")
}

func completionURL(apiURL string) string {
	base := strings.TrimRight(strings.TrimSpace(apiURL), "/")
	if strings.HasSuffix(base, "/chat/completions") {
		return base
	}
	return base + "/chat/completions"
}
