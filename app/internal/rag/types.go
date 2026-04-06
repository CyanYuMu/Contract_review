package rag

// Document 知识库文档
type Document struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Content     string            `json:"content"`
	Category    string            `json:"category"`     // 规范/法规/案例/示范
	SubCategory string            `json:"sub_category"` // 服务类/货物类/基建类/通用
	Source      string            `json:"source"`
	Metadata    map[string]string `json:"metadata"`
}

// Chunk 文档分块
type Chunk struct {
	ID        string            `json:"id"`
	DocID     string            `json:"doc_id"`
	Content   string            `json:"content"`
	Embedding []float32         `json:"embedding,omitempty"`
	Metadata  map[string]string `json:"metadata"`
}

// SearchResult 检索结果
type SearchResult struct {
	ChunkID  string            `json:"chunk_id"`
	DocID    string            `json:"doc_id"`
	Content  string            `json:"content"`
	Score    float64           `json:"score"`
	Source   string            `json:"source"`
	Metadata map[string]string `json:"metadata"`
}

// SearchRequest 检索请求
type SearchRequest struct {
	Query       string            `json:"query"`
	TopK        int               `json:"top_k"`
	Filters     map[string]string `json:"filters,omitempty"`
	MinScore    float64           `json:"min_score"`
}

// RetrieverConfig 检索器配置
type RetrieverConfig struct {
	TopK          int     `json:"top_k"`
	FinalTopK     int     `json:"final_top_k"`
	VectorWeight  float64 `json:"vector_weight"`
	KeywordWeight float64 `json:"keyword_weight"`
	MinRelevance  float64 `json:"min_relevance"`
}

// DefaultRetrieverConfig 默认检索配置
func DefaultRetrieverConfig() RetrieverConfig {
	return RetrieverConfig{
		TopK:          10,
		FinalTopK:     5,
		VectorWeight:  0.6,
		KeywordWeight: 0.4,
		MinRelevance:  0.5,
	}
}

// VectorStore 向量数据库接口
type VectorStore interface {
	// Insert 插入向量
	Insert(chunks []Chunk) error
	// Search 向量相似度检索
	Search(query []float32, topK int, filters map[string]string) ([]SearchResult, error)
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

// KeywordIndex 关键词索引接口
type KeywordIndex interface {
	// Index 建立索引
	Index(chunks []Chunk) error
	// Search 关键词检索
	Search(query string, topK int, filters map[string]string) ([]SearchResult, error)
}
