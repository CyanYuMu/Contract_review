package knowledge

import "time"

// ReviewKnowledgeDoc 审阅规范 / 法规等知识库文档（对应架构文档 review_knowledge_docs）
type ReviewKnowledgeDoc struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement;comment:文档ID" json:"id"`
	Title       string    `gorm:"type:varchar(255);not null;comment:标题" json:"title"`
	Category    string    `gorm:"type:varchar(16);not null;index;comment:规范/法规/案例/示范" json:"category"`
	SubCategory string    `gorm:"type:varchar(64);index;comment:合同类型子类如服务类/通用" json:"sub_category"`
	Source      string    `gorm:"type:varchar(255);comment:来源" json:"source"`
	Content     string    `gorm:"type:longtext;comment:全文(可分块索引)" json:"content"`
	ChunkCount  int       `gorm:"default:0;comment:分块数" json:"chunk_count"`
	Status      string    `gorm:"type:varchar(16);default:pending;index;comment:pending/indexed/failed" json:"status"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (ReviewKnowledgeDoc) TableName() string {
	return "review_knowledge_docs"
}

// ReviewKnowledgeChunk 文档分块（对应架构文档 review_knowledge_chunks；向量 ID 预留 Milvus）
type ReviewKnowledgeChunk struct {
	ID         uint64 `gorm:"primaryKey;autoIncrement;comment:分块ID" json:"id"`
	DocID      uint64 `gorm:"not null;index;comment:所属文档" json:"doc_id"`
	ChunkIndex int    `gorm:"not null;comment:分块序号" json:"chunk_index"`
	Content    string `gorm:"type:longtext;not null;comment:分块文本" json:"content"`
	VectorID   string `gorm:"type:varchar(128);comment:向量库ID" json:"vector_id"`
	// Metadata 结构化元数据（JSON 字符串）。风险点分块在此存放 risk_type/risk_level/
	// trigger_condition/keywords/applicable_clauses/legal_basis/recommended_template/
	// risk_content 等字段，供 RAG 命中后直接读取，避免从拍扁文本正则反解。
	Metadata  string    `gorm:"type:json;comment:结构化元数据(JSON)" json:"metadata,omitempty"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (ReviewKnowledgeChunk) TableName() string {
	return "review_knowledge_chunks"
}
