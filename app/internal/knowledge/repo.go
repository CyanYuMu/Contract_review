package knowledge

import (
	"context"
	"errors"
	"fmt"

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
	Metadata   string `gorm:"column:metadata"`
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
			COALESCE(c.metadata, '{}') AS metadata,
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

// KnowledgeSignature 返回已索引知识库的廉价签名（COUNT + MAX 聚合），
// 供审阅服务判断知识库是否自上次加载后发生变化，避免每次审阅都全量重载。
// 查询出错时返回 error，调用方应回退到全量加载（保持原行为）。
func (r *Repo) KnowledgeSignature(ctx context.Context) (string, error) {
	if r == nil || r.db == nil {
		return "", errors.New("knowledge repo: db is nil")
	}

	var docCount, chunkCount int64
	var docMaxUp string
	var chunkMaxID uint64

	if err := r.db.WithContext(ctx).
		Table("review_knowledge_docs").
		Where("status = ?", "indexed").
		Count(&docCount).Error; err != nil {
		return "", err
	}

	if docCount > 0 {
		if err := r.db.WithContext(ctx).
			Table("review_knowledge_docs").
			Where("status = ?", "indexed").
			Select("COALESCE(MAX(updated_at), '0')").
			Row().Scan(&docMaxUp); err != nil {
			return "", err
		}
	}

	if err := r.db.WithContext(ctx).
		Table("review_knowledge_chunks").
		Count(&chunkCount).Error; err != nil {
		return "", err
	}

	if chunkCount > 0 {
		if err := r.db.WithContext(ctx).
			Table("review_knowledge_chunks").
			Select("COALESCE(MAX(id), 0)").
			Row().Scan(&chunkMaxID); err != nil {
			return "", err
		}
	}

	return fmt.Sprintf("d:%d|u:%s|c:%d|m:%d", docCount, docMaxUp, chunkCount, chunkMaxID), nil
}
