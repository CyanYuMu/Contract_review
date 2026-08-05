package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"contract_review/app/internal/global"

	"go.uber.org/zap"
)

// === Reranker 实现: OpenAI-compatible /rerank 端点 ===

// openAIReranker 基于 OpenAI-compatible /rerank API 的 Reranker 实现
type openAIReranker struct {
	config RerankerConfig
	client *http.Client
}

// NewOpenAIReranker 创建 OpenAI-compatible Reranker
func NewOpenAIReranker(config RerankerConfig) Reranker {
	if config.TopK <= 0 {
		config.TopK = 5
	}
	if config.Threshold <= 0 {
		config.Threshold = 0.2
	}
	if config.TimeoutSeconds <= 0 {
		config.TimeoutSeconds = 10
	}
	if config.DegradationFactor <= 0 {
		config.DegradationFactor = 0.7
	}
	if config.MinThreshold <= 0 {
		config.MinThreshold = 0.15
	}
	return &openAIReranker{
		config: config,
		client: &http.Client{Timeout: time.Duration(config.TimeoutSeconds) * time.Second},
	}
}

type rerankRequest struct {
	Model     string   `json:"model"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int      `json:"top_n,omitempty"`
}

type rerankResponse struct {
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"results"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (r *openAIReranker) Rerank(ctx context.Context, query string, documents []string, topK int) ([]RerankResult, error) {
	if len(documents) == 0 {
		return nil, nil
	}
	if topK <= 0 {
		topK = r.config.TopK
	}
	if topK > len(documents) {
		topK = len(documents)
	}

	reqBody := rerankRequest{
		Model:     r.config.Model,
		Query:     query,
		Documents: documents,
		TopN:      topK,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化 rerank 请求失败: %w", err)
	}

	apiURL := rerankURL(r.config.APIBase)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("创建 rerank 请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if r.config.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+r.config.APIKey)
	}

	resp, err := r.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("rerank API 请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 rerank 响应失败: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errResp rerankResponse
		if json.Unmarshal(respBytes, &errResp) == nil && errResp.Error != nil {
			return nil, fmt.Errorf("rerank API 返回错误 (%d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return nil, fmt.Errorf("rerank API 请求失败 (%d): %s", resp.StatusCode, string(respBytes))
	}

	var parsed rerankResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, fmt.Errorf("解析 rerank 响应失败: %w, body: %s", err, string(respBytes))
	}

	results := make([]RerankResult, 0, len(parsed.Results))
	for _, r := range parsed.Results {
		results = append(results, RerankResult{
			Index: r.Index,
			Score: r.RelevanceScore,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results, nil
}

func rerankURL(apiURL string) string {
	base := strings.TrimRight(strings.TrimSpace(apiURL), "/")
	if strings.HasSuffix(base, "/rerank") {
		return base
	}
	return base + "/rerank"
}

// ============ Passage Cleaning ============

// 预编译的清洗正则（参考 WeKnora chat_pipeline/rerank.go:566-661）
var (
	reCodeBlock    = regexp.MustCompile("(?s)```[^`]*```")
	reLatexBlock   = regexp.MustCompile(`(?s)\$\$[^$]*\$\$`)
	reHTMLTag      = regexp.MustCompile(`<[^>]*>`)
	reLinkedImage  = regexp.MustCompile(`\[!\[([^\]]*)\]\([^)]+\)\]\([^)]+\)`)
	reMarkdownImg  = regexp.MustCompile(`!\[[^\]]*\]\([^)]+\)`)
	reMarkdownLink = regexp.MustCompile(`\[([^\]]*)\]\([^)]+\)`)
	reRawURL       = regexp.MustCompile(`https?://\S+`)
	reTableSep     = regexp.MustCompile(`(?m)^\|[\s\-:|]+\|$`)
	reHeadingPre   = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	reBlockquote   = regexp.MustCompile(`(?m)^>\s?`)
	reBoldItalic   = regexp.MustCompile(`\*{1,3}([^*]+)\*{1,3}`)
	reListMarker   = regexp.MustCompile(`(?m)^\s*[-*+]\s`)
	reExtraNewline = regexp.MustCompile(`\n{3,}`)
)

// cleanPassageForRerank 清除 Markdown 结构噪声，保留纯自然语言内容
// 设计理念: Rerank 模型基于语义文本相似度，
// Markdown 格式/URL/图片引用/表格分隔符等结构语法是噪声，会稀释语义信号
func cleanPassageForRerank(text string) string {
	text = reCodeBlock.ReplaceAllString(text, "")           // 1. 删除代码块
	text = reLatexBlock.ReplaceAllString(text, "")           // 2. 删除 LaTeX 块
	text = reHTMLTag.ReplaceAllString(text, "")              // 3. 删除 HTML 标签
	text = reLinkedImage.ReplaceAllString(text, "$1")        // 3.5 解包嵌套图片链接，保留描述文本
	text = reMarkdownImg.ReplaceAllString(text, "")          // 4. 删除图片引用
	text = reMarkdownLink.ReplaceAllString(text, "$1")       // 5. 链接保留文本丢弃 URL
	text = reRawURL.ReplaceAllString(text, "")               // 6. 删除裸 URL
	text = reTableSep.ReplaceAllString(text, "")             // 7. 删除表格分隔行
	text = reHeadingPre.ReplaceAllString(text, "")           // 8. 去标题标记
	text = reBlockquote.ReplaceAllString(text, "")           // 9. 去引用标记
	text = reBoldItalic.ReplaceAllString(text, "$1")         // 10. 去粗斜体，保留内容
	text = reListMarker.ReplaceAllString(text, "")           // 11. 去列表标记
	text = reExtraNewline.ReplaceAllString(text, "\n\n")     // 12. 压缩多余换行

	return strings.TrimSpace(text)
}

// ============ Rerank 应用函数 ============

// applyRerank 对检索结果进行 Rerank + 阈值过滤 + 阈值降级兜底
// 返回: rerank 后的结果列表（按 composite score 降序）
func applyRerank(
	ctx context.Context,
	reranker Reranker,
	query string,
	results []SearchResult,
	config RetrieverConfig,
	rerankerCfg RerankerConfig,
) []SearchResult {
	if reranker == nil || len(results) == 0 {
		return results
	}

	// Step 1: 构建 enriched passages + 清洗
	documents := make([]string, len(results))
	for i, r := range results {
		documents[i] = buildEnrichedPassage(r)
		documents[i] = cleanPassageForRerank(documents[i])
	}

	// Step 2: 调用 Rerank 模型
	rerankResults, err := reranker.Rerank(ctx, query, documents, len(documents))
	if err != nil {
		global.Log.Warn("Rerank 失败，降级为原始检索结果", zap.Error(err))
		return results
	}

	// Step 3: 阈值过滤 + 降级兜底
	filtered := filterByRerankThreshold(results, rerankResults, config, rerankerCfg)

	return filtered
}

// filterByRerankThreshold 按 Rerank 阈值过滤，带降级兜底
func filterByRerankThreshold(
	searchResults []SearchResult,
	rerankResults []RerankResult,
	config RetrieverConfig,
	rerankerCfg RerankerConfig,
) []SearchResult {
	threshold := rerankerCfg.Threshold

	// 第一轮: 用原始阈值过滤
	filtered := filterWithThreshold(searchResults, rerankResults, threshold, config)

	// 如果全部被过滤且启用了降级
	if len(filtered) == 0 && rerankerCfg.DegradationEnabled && threshold > rerankerCfg.MinThreshold {
		degradedThreshold := threshold * rerankerCfg.DegradationFactor
		if degradedThreshold < rerankerCfg.MinThreshold {
			degradedThreshold = rerankerCfg.MinThreshold
		}
		global.Log.Info("Rerank 阈值过滤后无结果，触发降级",
			zap.Float64("original_threshold", threshold),
			zap.Float64("degraded_threshold", degradedThreshold))
		filtered = filterWithThreshold(searchResults, rerankResults, degradedThreshold, config)
	}

	// 安全网: 如果降级后仍无结果但 top-1 分数 >= 0.15，保留 top-1
	if len(filtered) == 0 && len(rerankResults) > 0 && rerankResults[0].Score >= 0.15 {
		idx := rerankResults[0].Index
		if idx < len(searchResults) {
			searchResults[idx].RerankScore = rerankResults[0].Score
			filtered = []SearchResult{searchResults[idx]}
		}
	}

	return filtered
}

func filterWithThreshold(
	searchResults []SearchResult,
	rerankResults []RerankResult,
	threshold float64,
	config RetrieverConfig,
) []SearchResult {
	// 构建 index → rerank score 映射
	rerankScoreMap := make(map[int]float64)
	for _, rr := range rerankResults {
		rerankScoreMap[rr.Index] = rr.Score
	}

	var filtered []SearchResult
	for i := range searchResults {
		rerankScore, hasRerankScore := rerankScoreMap[i]
		if hasRerankScore && rerankScore < threshold {
			continue
		}

		result := searchResults[i]
		result.RerankScore = rerankScore

		// 计算 composite score
		if hasRerankScore {
			result.Score = compositeScore(&result, rerankScore, result.BaseScore, false)
		}

		filtered = append(filtered, result)
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Score > filtered[j].Score
	})

	return filtered
}

// computeOversampleTopK 计算过采样后的 TopK
func computeOversampleTopK(baseTopK int, config RetrieverConfig) int {
	topK := baseTopK * config.OversampleMultiplier
	if topK < config.OversampleMin {
		topK = config.OversampleMin
	}
	if topK > config.OversampleMax {
		topK = config.OversampleMax
	}
	return topK
}
