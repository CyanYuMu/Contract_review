package rag

import "context"

// Document 知识库文档
type Document struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Content     string            `json:"content"`
	Category    string            `json:"category"`     // 规范/法规/案例/示范
	SubCategory string            `json:"sub_category"` // 买卖/服务/劳动/租赁/借款/合作/知识产权/通用
	Source      string            `json:"source"`
	Metadata    map[string]string `json:"metadata"`
}

// Chunk 文档分块
// ContextHeader 不占 Content 字符位置，在 EmbeddingContent() 时前置拼接
type Chunk struct {
	ID            string            `json:"id"`
	DocID         string            `json:"doc_id"`
	Content       string            `json:"content"`
	ContextHeader string            `json:"context_header,omitempty"` // 标题面包屑，embedding 时前置
	Embedding     []float32         `json:"embedding,omitempty"`
	Metadata      map[string]string `json:"metadata"`
	ParentChunkID string            `json:"parent_chunk_id,omitempty"` // Parent-Child: 指向父 chunk ID
}

// EmbeddingContent 返回用于 embedding 的文本内容
// 如果有 ContextHeader，将其前置拼接，增强语义检索精度
func (c Chunk) EmbeddingContent() string {
	body := c.Content
	if c.ContextHeader != "" {
		return c.ContextHeader + "\n\n" + body
	}
	return body
}

// SearchResult 检索结果
type SearchResult struct {
	ChunkID     string            `json:"chunk_id"`
	DocID       string            `json:"doc_id"`
	Content     string            `json:"content"`
	Score       float64           `json:"score"`
	Source      string            `json:"source"`
	Metadata    map[string]string `json:"metadata"`
	RerankScore float64           `json:"rerank_score,omitempty"` // Rerank 模型分数
	BaseScore   float64           `json:"base_score,omitempty"`   // 原始检索分数
	ChunkType   string            `json:"chunk_type,omitempty"`   // "parent" / "child"
}

// SearchRequest 检索请求
type SearchRequest struct {
	Query    string            `json:"query"`
	TopK     int               `json:"top_k"`
	Filters  map[string]string `json:"filters,omitempty"`
	MinScore float64           `json:"min_score"`
}

// RetrieverConfig 检索器配置
type RetrieverConfig struct {
	TopK                int     `json:"top_k"`                 // 最终返回数量
	FinalTopK           int     `json:"final_top_k"`           // 最终返回数量（重排后）
	VectorWeight        float64 `json:"vector_weight"`         // RRF 向量检索权重
	KeywordWeight       float64 `json:"keyword_weight"`        // RRF 关键词检索权重
	BM25Weight          float64 `json:"bm25_weight"`           // RRF BM25 检索权重
	MinRelevance        float64 `json:"min_relevance"`         // 最低相关度阈值
	OversampleMultiplier int    `json:"oversample_multiplier"` // 过采样倍数，默认 5
	OversampleMin       int     `json:"oversample_min"`        // 过采样最低数量，默认 25
	OversampleMax       int     `json:"oversample_max"`        // 过采样最高数量，默认 100
	EnableBM25          bool    `json:"enable_bm25"`           // 是否启用 BM25
	EnableRerank        bool    `json:"enable_rerank"`         // 是否启用 Rerank
	MMRLambda           float64 `json:"mmr_lambda"`            // MMR 相关性与多样性权衡 (0-1)
	RerankThreshold     float64 `json:"rerank_threshold"`      // Rerank 阈值（低于此分数的结果被过滤）
	RRFK                float64 `json:"rrf_k"`                 // RRF 平滑常数 K
}

// DefaultRetrieverConfig 默认检索配置
func DefaultRetrieverConfig() RetrieverConfig {
	return RetrieverConfig{
		TopK:                10,
		FinalTopK:           5,
		VectorWeight:        0.5,
		KeywordWeight:       0.2,
		BM25Weight:          0.3,
		MinRelevance:        0.0,
		OversampleMultiplier: 5,
		OversampleMin:       25,
		OversampleMax:       100,
		EnableBM25:          false,
		EnableRerank:        false,
		MMRLambda:           0.7,
		RerankThreshold:     0.2,
		RRFK:                60,
	}
}

// VectorStore 向量数据库接口
type VectorStore interface {
	// Insert 插入向量
	Insert(chunks []Chunk) error
	// Search 向量相似度检索
	Search(query []float32, topK int, filters map[string]string) ([]SearchResult, error)
	// SearchBM25 BM25 稀疏向量检索（需 Milvus 2.4+）
	SearchBM25(query string, topK int, filters map[string]string) ([]SearchResult, error)
	// BM25Enabled 返回是否支持 BM25
	BM25Enabled() bool
	// Delete 删除向量
	Delete(ids []string) error
}

// EmbeddingModel Embedding 模型接口
type EmbeddingModel interface {
	// Embed 将文本转为向量
	Embed(text string) ([]float32, error)
	// EmbedBatch 批量转换
	EmbedBatch(texts []string) ([][]float32, error)
}

// CachedEmbeddingModel 带缓存的 Embedding 模型
type CachedEmbeddingModel interface {
	EmbeddingModel
	// EmbedWithCache 带缓存的 embedding，同一 session 内相同文本不重复计算
	EmbedWithCache(key string, text string) ([]float32, error)
	// ClearCache 清除缓存
	ClearCache()
}

// KeywordIndex 关键词索引接口（BM25 未启用时的降级方案）
type KeywordIndex interface {
	// Index 建立索引
	Index(chunks []Chunk) error
	// Search 关键词检索
	Search(query string, topK int, filters map[string]string) ([]SearchResult, error)
	// SearchableChunks 返回已索引的 chunk 数量
	SearchableChunks() []Chunk
}

// ============ Phase 1: Rerank 相关类型 ============

// Reranker 重排序接口
// 实现 Cross-Encoder 精排，支持可插拔的 Rerank 模型
type Reranker interface {
	// Rerank 对候选文档进行重排序
	// query: 原始查询
	// documents: 候选文档内容列表
	// topK: 返回数量
	Rerank(ctx context.Context, query string, documents []string, topK int) ([]RerankResult, error)
}

// RerankResult 重排序结果
type RerankResult struct {
	Index int     `json:"index"` // 对应 documents 中的索引
	Score float64 `json:"score"` // 相关性分数 (0-1)
}

// RerankerConfig Reranker 配置
type RerankerConfig struct {
	Model              string  `json:"model"`               // Rerank 模型名
	APIBase            string  `json:"api_base"`            // API 地址
	APIKey             string  `json:"api_key"`             // API Key
	TopK               int     `json:"top_k"`               // 返回数量
	Threshold          float64 `json:"threshold"`           // 相关性阈值
	TimeoutSeconds     int     `json:"timeout_seconds"`     // 超时秒数
	DegradationEnabled bool    `json:"degradation_enabled"` // 是否启用阈值降级
	DegradationFactor  float64 `json:"degradation_factor"`  // 降级因子 (0-1)
	MinThreshold       float64 `json:"min_threshold"`       // 最低安全阈值
}

// DefaultRerankerConfig 默认 Reranker 配置
func DefaultRerankerConfig() RerankerConfig {
	return RerankerConfig{
		Model:              "BAAI/bge-reranker-v2-m3",
		TopK:               5,
		Threshold:          0.2,
		TimeoutSeconds:     10,
		DegradationEnabled: true,
		DegradationFactor:  0.7,
		MinThreshold:       0.15,
	}
}
