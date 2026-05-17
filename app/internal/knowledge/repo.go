package knowledge

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// Repo 审阅知识库仓储
type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// IndexedChunkRow JOIN 查询结果，供 rag 包组装为 Chunk
type IndexedChunkRow struct {
	ChunkID    uint64 `gorm:"column:chunk_id"`
	DocID      uint64 `gorm:"column:doc_id"`
	ChunkIndex int    `gorm:"column:chunk_index"`
	Content    string `gorm:"column:content"`
	VectorID   string `gorm:"column:vector_id"`
	Title      string `gorm:"column:title"`
	Category   string `gorm:"column:category"`
	SubCat     string `gorm:"column:sub_category"`
	Source     string `gorm:"column:source"`
}

// ListIndexedChunksForRAG 列出已索引文档的全部分块
func (r *Repo) ListIndexedChunksForRAG(ctx context.Context) ([]IndexedChunkRow, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("knowledge repo: db is nil")
	}

	var rows []IndexedChunkRow
	err := r.db.WithContext(ctx).Table("review_knowledge_chunks AS c").
		Select(`c.id AS chunk_id, c.doc_id, c.chunk_index, c.content, c.vector_id,
			d.title, d.category, d.sub_category, d.source`).
		Joins("INNER JOIN review_knowledge_docs AS d ON d.id = c.doc_id").
		Where("d.status = ?", "indexed").
		Order("c.doc_id ASC, c.chunk_index ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}
