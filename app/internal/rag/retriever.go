package rag

import (
	"log"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// RAGRetriever 混合检索器
// 支持向量检索 + 关键词检索 + RRF 融合 + 重排序
type RAGRetriever struct {
	vectorStore  VectorStore
	keywordIndex KeywordIndex
	embedder     EmbeddingModel
	config       RetrieverConfig
}

// NewRAGRetriever 创建混合检索器
func NewRAGRetriever(vs VectorStore, ki KeywordIndex, emb EmbeddingModel, config RetrieverConfig) *RAGRetriever {
	return &RAGRetriever{
		vectorStore:  vs,
		keywordIndex: ki,
		embedder:     emb,
		config:       config,
	}
}

// Retrieve 混合检索
func (r *RAGRetriever) Retrieve(query string, filters map[string]string) ([]SearchResult, error) {
	var vectorResults, keywordResults []SearchResult

	if r.vectorStore != nil && r.embedder != nil {
		embedding, err := r.embedder.Embed(query)
		if err != nil {
			log.Printf("rag: 向量 Embedding 失败，跳过向量检索: %v", err)
		} else {
			vr, err := r.vectorStore.Search(embedding, r.config.TopK, filters)
			if err != nil {
				log.Printf("rag: 向量检索失败: %v", err)
			} else {
				vectorResults = vr
			}
		}
	}

	if r.keywordIndex != nil {
		kr, err := r.keywordIndex.Search(query, r.config.TopK, filters)
		if err != nil {
			log.Printf("rag: 关键词检索失败: %v", err)
		} else {
			keywordResults = kr
		}
	}

	if len(vectorResults) == 0 && len(keywordResults) == 0 {
		return nil, nil
	}

	merged := reciprocalRankFusion(vectorResults, keywordResults, r.config.VectorWeight, r.config.KeywordWeight)

	if len(merged) > r.config.FinalTopK {
		merged = merged[:r.config.FinalTopK]
	}

	filtered := filterByRelevance(merged, r.config.MinRelevance)
	return filtered, nil
}

// LayeredRetrieve 分层检索: 先精确匹配合同类型 → 再泛化到通用
func (r *RAGRetriever) LayeredRetrieve(query string, contractType string) ([]SearchResult, error) {
	specificResults, err := r.Retrieve(query, map[string]string{
		"sub_category": contractType,
	})
	if err != nil {
		return nil, err
	}

	if len(specificResults) >= 3 {
		return specificResults, nil
	}

	generalResults, err := r.Retrieve(query, map[string]string{
		"sub_category": "通用",
	})
	if err != nil {
		return specificResults, nil
	}

	return mergeAndDedup(specificResults, generalResults), nil
}

// reciprocalRankFusion RRF 融合算法
// 将多路检索结果通过排名倒数加权融合
func reciprocalRankFusion(vectorResults, keywordResults []SearchResult, vectorWeight, keywordWeight float64) []SearchResult {
	const k = 60.0

	scoreMap := make(map[string]float64)
	resultMap := make(map[string]SearchResult)

	for rank, r := range vectorResults {
		id := r.ChunkID
		scoreMap[id] += vectorWeight * (1.0 / (k + float64(rank+1)))
		resultMap[id] = r
	}

	for rank, r := range keywordResults {
		id := r.ChunkID
		scoreMap[id] += keywordWeight * (1.0 / (k + float64(rank+1)))
		if _, exists := resultMap[id]; !exists {
			resultMap[id] = r
		}
	}

	var merged []SearchResult
	for id, score := range scoreMap {
		result := resultMap[id]
		result.Score = score
		merged = append(merged, result)
	}

	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Score > merged[j].Score
	})

	return merged
}

// filterByRelevance 过滤低相关度结果
func filterByRelevance(results []SearchResult, minRelevance float64) []SearchResult {
	if minRelevance <= 0 {
		return results
	}

	var filtered []SearchResult
	for _, r := range results {
		if r.Score >= minRelevance {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// mergeAndDedup 合并去重
func mergeAndDedup(a, b []SearchResult) []SearchResult {
	seen := make(map[string]bool)
	var merged []SearchResult

	for _, r := range a {
		if !seen[r.ChunkID] {
			seen[r.ChunkID] = true
			merged = append(merged, r)
		}
	}
	for _, r := range b {
		if !seen[r.ChunkID] {
			seen[r.ChunkID] = true
			merged = append(merged, r)
		}
	}

	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Score > merged[j].Score
	})

	return merged
}

// SimpleKeywordIndex 基于内存的简单关键词索引（用于开发/测试阶段）
type SimpleKeywordIndex struct {
	chunks []Chunk
}

// NewSimpleKeywordIndex 创建简单关键词索引
func NewSimpleKeywordIndex() *SimpleKeywordIndex {
	return &SimpleKeywordIndex{}
}

// Index 建立索引
func (ski *SimpleKeywordIndex) Index(chunks []Chunk) error {
	ski.chunks = append(ski.chunks, chunks...)
	return nil
}

func (ski *SimpleKeywordIndex) SearchableChunks() []Chunk {
	return ski.chunks
}

// Search 基于 TF 的简单关键词检索
func (ski *SimpleKeywordIndex) Search(query string, topK int, filters map[string]string) ([]SearchResult, error) {
	queryTerms := tokenizeQuery(query)
	if len(queryTerms) == 0 {
		return nil, nil
	}

	type scoredChunk struct {
		chunk Chunk
		score float64
	}
	var scored []scoredChunk

	for _, chunk := range ski.chunks {
		if !matchFilters(chunk.Metadata, filters) {
			continue
		}

		contentLower := strings.ToLower(chunk.Content)
		matchCount := 0
		for _, term := range queryTerms {
			if strings.Contains(contentLower, term) {
				matchCount++
			}
		}

		if matchCount > 0 {
			score := float64(matchCount) / float64(len(queryTerms))
			scored = append(scored, scoredChunk{chunk: chunk, score: score})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	limit := int(math.Min(float64(topK), float64(len(scored))))
	results := make([]SearchResult, limit)
	for i := 0; i < limit; i++ {
		results[i] = SearchResult{
			ChunkID:  scored[i].chunk.ID,
			DocID:    scored[i].chunk.DocID,
			Content:  scored[i].chunk.Content,
			Score:    scored[i].score,
			Source:   scored[i].chunk.Metadata["source"],
			Metadata: scored[i].chunk.Metadata,
		}
	}

	return results, nil
}

func tokenizeQuery(query string) []string {
	normalized := strings.ToLower(strings.TrimSpace(query))
	if normalized == "" {
		return nil
	}

	seen := make(map[string]bool)
	var terms []string
	add := func(term string) {
		term = strings.TrimSpace(term)
		if utf8.RuneCountInString(term) < 2 || seen[term] {
			return
		}
		seen[term] = true
		terms = append(terms, term)
	}

	for _, term := range strings.Fields(normalized) {
		add(term)
	}

	for _, term := range regexp.MustCompile(`[[:punct:]\s，。；：、（）《》【】“”"']+`).Split(normalized, -1) {
		add(term)
	}

	legalKeywords := []string{
		"付款", "支付", "验收", "交付", "违约", "赔偿", "违约金", "解除", "终止",
		"知识产权", "著作权", "保密", "管辖", "争议", "发票", "逾期", "单方",
		"服务范围", "服务内容", "质量", "期限", "义务", "责任", "免责",
	}
	for _, kw := range legalKeywords {
		if strings.Contains(normalized, kw) {
			add(kw)
		}
	}

	runes := []rune(normalized)
	for i := 0; i < len(runes)-1; i++ {
		if isCJK(runes[i]) && isCJK(runes[i+1]) {
			add(string(runes[i : i+2]))
		}
	}

	return terms
}

func isCJK(r rune) bool {
	return (r >= '\u4e00' && r <= '\u9fff') || (r >= '\u3400' && r <= '\u4dbf')
}

func matchFilters(metadata map[string]string, filters map[string]string) bool {
	for k, v := range filters {
		if metaVal, ok := metadata[k]; ok {
			if metaVal != v {
				return false
			}
		}
	}
	return true
}
