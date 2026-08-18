package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// dashScopeReranker 基于阿里云百炼（DashScope）原生重排序 API 的 Reranker 实现。
//
// DashScope 的 rerank 不走 OpenAI 兼容模式，而是原生端点：
//
//	POST https://dashscope.aliyuncs.com/api/v1/services/rerank/text-rerank/text-rerank
//	模型：gte-rerank / gte-rerank-v2
type dashScopeReranker struct {
	config RerankerConfig
	client *http.Client
}

// NewDashScopeReranker 创建 DashScope 原生 reranker。
func NewDashScopeReranker(config RerankerConfig) Reranker {
	if config.TopK <= 0 {
		config.TopK = 5
	}
	if config.Threshold <= 0 {
		config.Threshold = 0.2
	}
	if config.TimeoutSeconds <= 0 {
		config.TimeoutSeconds = 10
	}
	return &dashScopeReranker{
		config: config,
		client: &http.Client{Timeout: time.Duration(config.TimeoutSeconds) * time.Second},
	}
}

type dashScopeRerankRequest struct {
	Model      string                    `json:"model"`
	Input      dashScopeRerankInput      `json:"input"`
	Parameters dashScopeRerankParameters `json:"parameters"`
}

type dashScopeRerankInput struct {
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
}

type dashScopeRerankParameters struct {
	ReturnDocuments bool `json:"return_documents"`
	TopN            int  `json:"top_n,omitempty"`
}

type dashScopeRerankResponse struct {
	Output struct {
		Results []struct {
			Index          int     `json:"index"`
			RelevanceScore float64 `json:"relevance_score"`
		} `json:"results"`
	} `json:"output"`
	Message string `json:"message,omitempty"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (r *dashScopeReranker) Rerank(ctx context.Context, query string, documents []string, topK int) ([]RerankResult, error) {
	if len(documents) == 0 {
		return nil, nil
	}
	if topK <= 0 {
		topK = r.config.TopK
	}
	if topK > len(documents) {
		topK = len(documents)
	}

	reqBody := dashScopeRerankRequest{
		Model: r.config.Model,
		Input: dashScopeRerankInput{Query: query, Documents: documents},
		Parameters: dashScopeRerankParameters{
			ReturnDocuments: false,
			TopN:            topK,
		},
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化 dashscope rerank 请求失败: %w", err)
	}

	apiURL := dashScopeRerankURL(r.config.APIBase)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("创建 dashscope rerank 请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if r.config.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+r.config.APIKey)
	}

	resp, err := r.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("dashscope rerank API 请求失败: %w", err)
	}
	defer resp.Body.Close()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 dashscope rerank 响应失败: %w", err)
	}

	var parsed dashScopeRerankResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, fmt.Errorf("解析 dashscope rerank 响应失败: %w, body: %s", err, string(respBytes))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if parsed.Error != nil {
			return nil, fmt.Errorf("dashscope rerank 错误 (%d): %s %s", resp.StatusCode, parsed.Error.Code, parsed.Error.Message)
		}
		return nil, fmt.Errorf("dashscope rerank 请求失败 (%d): %s", resp.StatusCode, parsed.Message)
	}

	results := make([]RerankResult, 0, len(parsed.Output.Results))
	for _, r := range parsed.Output.Results {
		results = append(results, RerankResult{Index: r.Index, Score: r.RelevanceScore})
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	return results, nil
}

// dashScopeRerankURL 由 API base 构造 DashScope 原生 rerank 端点。
// 兼容 base 形如 https://dashscope.aliyuncs.com 或 https://dashscope.aliyuncs.com/compatible-mode/v1。
func dashScopeRerankURL(apiBase string) string {
	const nativePath = "/api/v1/services/rerank/text-rerank/text-rerank"
	base := strings.TrimRight(strings.TrimSpace(apiBase), "/")
	if strings.HasSuffix(base, nativePath) {
		return base
	}
	// 去掉 OpenAI 兼容模式后缀，回到根后拼接原生路径。
	base = strings.TrimSuffix(base, "/compatible-mode/v1")
	base = strings.TrimSuffix(base, "/compatible-mode")
	return strings.TrimRight(base, "/") + nativePath
}

// UseDashScopeReranker 根据 API base 判断是否应使用 DashScope 原生 reranker。
func UseDashScopeReranker(apiBase string) bool {
	base := strings.ToLower(apiBase)
	return strings.Contains(base, "dashscope") || strings.Contains(base, "aliyuncs.com")
}
