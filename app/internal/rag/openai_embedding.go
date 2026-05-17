package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAIEmbeddingModel 调用 OpenAI-compatible /embeddings 接口生成向量。
type OpenAIEmbeddingModel struct {
	model   string
	apiURL  string
	apiKey  string
	timeout time.Duration
}

type embeddingRequest struct {
	Model string      `json:"model"`
	Input interface{} `json:"input"`
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Error *struct {
		Message string      `json:"message"`
		Type    string      `json:"type"`
		Code    interface{} `json:"code"`
	} `json:"error,omitempty"`
}

func NewOpenAIEmbeddingModel(modelName, apiURL, apiKey string, timeout time.Duration) *OpenAIEmbeddingModel {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &OpenAIEmbeddingModel{
		model:   strings.TrimSpace(modelName),
		apiURL:  strings.TrimSpace(apiURL),
		apiKey:  strings.TrimSpace(apiKey),
		timeout: timeout,
	}
}

func (m *OpenAIEmbeddingModel) Embed(text string) ([]float32, error) {
	vectors, err := m.EmbedBatch([]string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("embedding 响应为空")
	}
	return vectors[0], nil
}

func (m *OpenAIEmbeddingModel) EmbedBatch(texts []string) ([][]float32, error) {
	if strings.TrimSpace(m.model) == "" {
		return nil, fmt.Errorf("embedding model 不能为空")
	}
	if strings.TrimSpace(m.apiURL) == "" {
		return nil, fmt.Errorf("embedding api_url 不能为空")
	}

	inputs := make([]string, 0, len(texts))
	for _, text := range texts {
		if trimmed := strings.TrimSpace(text); trimmed != "" {
			inputs = append(inputs, trimmed)
		}
	}
	if len(inputs) == 0 {
		return nil, nil
	}

	reqBody := embeddingRequest{
		Model: m.model,
		Input: inputs,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, embeddingURL(m.apiURL), bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if m.apiKey != "" {
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

	var parsed embeddingResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, fmt.Errorf("解析 embedding 响应失败: %w, body: %s", err, string(respBytes))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if parsed.Error != nil && parsed.Error.Message != "" {
			return nil, fmt.Errorf("embedding 请求失败（%d）: %s", resp.StatusCode, parsed.Error.Message)
		}
		return nil, fmt.Errorf("embedding 请求失败（%d）: %s", resp.StatusCode, string(respBytes))
	}

	vectors := make([][]float32, len(parsed.Data))
	for _, item := range parsed.Data {
		if item.Index >= 0 && item.Index < len(vectors) {
			vectors[item.Index] = item.Embedding
		}
	}
	return vectors, nil
}

func embeddingURL(apiURL string) string {
	base := strings.TrimRight(strings.TrimSpace(apiURL), "/")
	if strings.HasSuffix(base, "/embeddings") {
		return base
	}
	return base + "/embeddings"
}
