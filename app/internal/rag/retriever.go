package rag

import (
	"context"
	"log"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
)

// RAGRetriever 混合检索器
//
// 支持三种检索通道的并行融合:
//  1. 向量检索 (Dense Vector) — Milvus AUTOINDEX 语义匹配
//  2. BM25 全文检索 — Milvus LIKE 表达式文本匹配
//  3. 关键词索引 — SimpleKeywordIndex (BM25 不可用时的降级方案)
//
// 管线: 并行检索 → RRF 三路融合 → 去重 → [Rerank] → [MMR] → final TopK
type RAGRetriever struct {
	vectorStore  VectorStore
	keywordIndex KeywordIndex
	embedder     EmbeddingModel
	reranker     Reranker
	config       RetrieverConfig
	rerankerCfg  RerankerConfig
}

// RAGRetrieverOption 可选的检索器配置
type RAGRetrieverOption func(*RAGRetriever)

// WithReranker 设置 Reranker
func WithReranker(r Reranker, cfg RerankerConfig) RAGRetrieverOption {
	return func(ret *RAGRetriever) {
		ret.reranker = r
		ret.rerankerCfg = cfg
		ret.config.EnableRerank = r != nil
	}
}

// NewRAGRetriever 创建混合检索器
func NewRAGRetriever(
	vs VectorStore,
	ki KeywordIndex,
	emb EmbeddingModel,
	config RetrieverConfig,
	opts ...RAGRetrieverOption,
) *RAGRetriever {
	r := &RAGRetriever{
		vectorStore:  vs,
		keywordIndex: ki,
		embedder:     emb,
		config:       config,
		rerankerCfg:  DefaultRerankerConfig(),
	}
	for _, opt := range opts {
		opt(r)
	}

	// 自动检测 BM25 可用性
	if vs != nil {
		r.config.EnableBM25 = vs.BM25Enabled()
	}
	return r
}

// Retrieve 混合检索主入口
func (r *RAGRetriever) Retrieve(query string, filters map[string]string) ([]SearchResult, error) {
	return r.retrieveWithOverride(query, filters, 0)
}

// RetrieveTopK 指定返回数量的混合检索
func (r *RAGRetriever) RetrieveTopK(query string, filters map[string]string, topK int) ([]SearchResult, error) {
	return r.retrieveWithOverride(query, filters, topK)
}

// retrieveWithOverride 统一的检索实现
func (r *RAGRetriever) retrieveWithOverride(query string, filters map[string]string, topKOverride int) ([]SearchResult, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}

	finalTopK := r.config.effectiveFinalTopK()
	if topKOverride > 0 {
		finalTopK = topKOverride
	}

	// Step 0: 计算过采样 TopK
	oversampleTopK := computeOversampleTopK(finalTopK, r.config)

	// Step 1: 并行三路检索
	var (
		vectorResults  []SearchResult
		bm25Results    []SearchResult
		keywordResults []SearchResult
		embedding      []float32
		wg             sync.WaitGroup
		errChan        = make(chan error, 3)
	)

	// 1a. 向量检索 (需要先做 embedding)
	if r.vectorStore != nil && r.embedder != nil {
		var embedErr error
		embedding, embedErr = r.embedder.Embed(query)
		if embedErr != nil {
			log.Printf("rag: 向量 Embedding 失败, 跳过向量检索: %v", embedErr)
		}
	}

	if len(embedding) > 0 && r.vectorStore != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			vr, err := r.vectorStore.Search(embedding, oversampleTopK, filters)
			if err != nil {
				log.Printf("rag: 向量检索失败: %v", err)
				errChan <- err
				return
			}
			for i := range vr {
				vr[i].BaseScore = vr[i].Score
				vr[i].ChunkType = "vector"
			}
			vectorResults = vr
		}()
	}

	// 1b. BM25 全文检索 (Milvus LIKE 表达式)
	if r.config.EnableBM25 && r.vectorStore != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			br, err := r.vectorStore.SearchBM25(query, oversampleTopK, filters)
			if err != nil {
				log.Printf("rag: BM25 检索失败: %v", err)
				errChan <- err
				return
			}
			for i := range br {
				br[i].ChunkType = "bm25"
			}
			bm25Results = br
		}()
	}

	// 1c. 关键词索引 (BM25 不可用时的降级方案 / 补充通道)
	if r.keywordIndex != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			kr, err := r.keywordIndex.Search(query, oversampleTopK, filters)
			if err != nil {
				log.Printf("rag: 关键词检索失败: %v", err)
				errChan <- err
				return
			}
			for i := range kr {
				kr[i].ChunkType = "keyword"
			}
			keywordResults = kr
		}()
	}

	wg.Wait()
	close(errChan)

	// 所有通道都失败
	if len(vectorResults) == 0 && len(bm25Results) == 0 && len(keywordResults) == 0 {
		return nil, nil
	}

	// Step 2: 三路 RRF 融合
	effectiveVW, effectiveBW, effectiveKW := computeEffectiveRRFWeights(
		vectorResults, bm25Results, keywordResults,
		r.config.VectorWeight, r.config.BM25Weight, r.config.KeywordWeight,
	)

	merged := reciprocalRankFusion3Way(
		vectorResults, bm25Results, keywordResults,
		effectiveVW, effectiveBW, effectiveKW,
		r.config.RRFK,
	)

	// Step 3: 去重
	merged = dedupByChunkID(merged)

	// Step 4: 截断到过采样上限
	if len(merged) > oversampleTopK {
		merged = merged[:oversampleTopK]
	}

	// Step 5: Rerank 管线
	if r.config.EnableRerank && r.reranker != nil && len(merged) > 0 {
		merged = applyRerank(context.Background(), r.reranker, query, merged, r.config, r.rerankerCfg)
	}

	// Step 6: MMR 多样性去重
	if r.config.MMRLambda < 1.0 && len(merged) > finalTopK {
		merged = maximalMarginalRelevance(merged, r.config.MMRLambda, finalTopK*2)
	}

	// Step 7: 截断到最终 TopK
	if len(merged) > finalTopK {
		merged = merged[:finalTopK]
	}

	// Step 8: 阈值过滤 (MinRelevance > 0 时生效)
	if r.config.MinRelevance > 0 {
		merged = filterByRelevance(merged, r.config.MinRelevance)
	}

	return merged, nil
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

// SetReranker 动态设置 Reranker
func (r *RAGRetriever) SetReranker(reranker Reranker, cfg RerankerConfig) {
	r.reranker = reranker
	r.rerankerCfg = cfg
	r.config.EnableRerank = reranker != nil
}

// ============ 三路 RRF 融合 ============

// reciprocalRankFusion3Way 三路加权 RRF 融合
// 参考 WeKnora knowledgebase_search_fusion.go:84-142
func reciprocalRankFusion3Way(
	vectorResults, bm25Results, keywordResults []SearchResult,
	vectorWeight, bm25Weight, keywordWeight float64,
	rrfK float64,
) []SearchResult {
	if rrfK <= 0 {
		rrfK = 60
	}

	// 单路结果直接返回（不需要 RRF）
	activeCount := 0
	if len(vectorResults) > 0 {
		activeCount++
	}
	if len(bm25Results) > 0 {
		activeCount++
	}
	if len(keywordResults) > 0 {
		activeCount++
	}

	if activeCount <= 1 {
		all := append(append(vectorResults, bm25Results...), keywordResults...)
		sort.Slice(all, func(i, j int) bool { return all[i].Score > all[j].Score })
		return all
	}

	// 构建 1-indexed 排名映射
	vectorRanks := buildRankMap(vectorResults)
	bm25Ranks := buildRankMap(bm25Results)
	keywordRanks := buildRankMap(keywordResults)

	// 收集所有结果的 chunkID
	scoreMap := make(map[string]float64)
	resultMap := make(map[string]SearchResult)

	accumulate := func(results []SearchResult, ranks map[string]int, weight float64) {
		if weight <= 0 {
			return
		}
		for _, r := range results {
			id := r.ChunkID
			if rank, ok := ranks[id]; ok {
				scoreMap[id] += weight / (rrfK + float64(rank))
			}
			if _, exists := resultMap[id]; !exists {
				resultMap[id] = r
			}
		}
	}

	accumulate(vectorResults, vectorRanks, vectorWeight)
	accumulate(bm25Results, bm25Ranks, bm25Weight)
	accumulate(keywordResults, keywordRanks, keywordWeight)

	// 构建排序结果
	var merged []SearchResult
	for id, score := range scoreMap {
		result := resultMap[id]
		result.Score = score
		result.BaseScore = score
		merged = append(merged, result)
	}

	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Score > merged[j].Score
	})

	return merged
}

func buildRankMap(results []SearchResult) map[string]int {
	ranks := make(map[string]int, len(results))
	for i, r := range results {
		if _, exists := ranks[r.ChunkID]; !exists {
			ranks[r.ChunkID] = i + 1 // 1-indexed
		}
	}
	return ranks
}

// reciprocalRankFusion 两路 RRF (保留兼容旧调用)
func reciprocalRankFusion(vectorResults, keywordResults []SearchResult, vectorWeight, keywordWeight float64) []SearchResult {
	return reciprocalRankFusion3Way(vectorResults, nil, keywordResults, vectorWeight, 0, keywordWeight, 60)
}

// ============ 过滤 & 工具函数 ============

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

// ============ SimpleKeywordIndex (降级关键词索引) ============

// SimpleKeywordIndex 基于内存的关键词索引
// 用于 BM25 不可用时的降级方案或补充检索通道
type SimpleKeywordIndex struct {
	chunks []Chunk
	mu     sync.RWMutex
}

func NewSimpleKeywordIndex() *SimpleKeywordIndex {
	return &SimpleKeywordIndex{}
}

func (ski *SimpleKeywordIndex) Index(chunks []Chunk) error {
	ski.mu.Lock()
	defer ski.mu.Unlock()
	ski.chunks = append(ski.chunks, chunks...)
	return nil
}

func (ski *SimpleKeywordIndex) SearchableChunks() []Chunk {
	ski.mu.RLock()
	defer ski.mu.RUnlock()
	cp := make([]Chunk, len(ski.chunks))
	copy(cp, ski.chunks)
	return cp
}

// Search 基于 TF 的关键词检索
// 中文: CJK bigram + 法律关键词
// 英文: 单词匹配
func (ski *SimpleKeywordIndex) Search(query string, topK int, filters map[string]string) ([]SearchResult, error) {
	queryTerms := tokenizeQuery(query)
	if len(queryTerms) == 0 {
		return nil, nil
	}

	ski.mu.RLock()
	defer ski.mu.RUnlock()

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
			BaseScore: scored[i].score,
			Source:   scored[i].chunk.Metadata["source"],
			Metadata: scored[i].chunk.Metadata,
			ChunkType: "keyword",
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

	for _, term := range regexp.MustCompile(`[[:punct:]\s，。；：、（）《》【】"']+`).Split(normalized, -1) {
		add(term)
	}

	legalKeywords := []string{
		"付款", "支付", "验收", "交付", "违约", "赔偿", "违约金", "解除", "终止",
		"知识产权", "著作权", "保密", "管辖", "争议", "发票", "逾期", "单方",
		"服务范围", "服务内容", "质量", "期限", "义务", "责任", "免责",
		"垄断", "仲裁", "诉讼", "不可抗力", "连带",
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
	return (r >= '一' && r <= '鿿') || (r >= '㐀' && r <= '䶿')
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
