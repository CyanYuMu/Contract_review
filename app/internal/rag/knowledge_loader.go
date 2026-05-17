package rag

import (
	"context"
	"contract_review/app/internal/knowledge"
	"fmt"

	"gorm.io/gorm"
)

// LoadKnowledgeChunksFromDB 从审阅知识库表加载已索引分块，供 SimpleKeywordIndex.Index 使用
func LoadKnowledgeChunksFromDB(ctx context.Context, db *gorm.DB) ([]Chunk, error) {
	if db == nil {
		return nil, nil
	}
	repo := knowledge.NewRepo(db)
	rows, err := repo.ListIndexedChunksForRAG(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Chunk, 0, len(rows))
	for _, row := range rows {
		docIDStr := fmt.Sprintf("%d", row.DocID)
		chunkID := fmt.Sprintf("ck-%d", row.ChunkID)
		if row.VectorID != "" {
			chunkID = row.VectorID
		}
		out = append(out, Chunk{
			ID:      chunkID,
			DocID:   docIDStr,
			Content: row.Content,
			Metadata: map[string]string{
				"title":        row.Title,
				"category":     row.Category,
				"sub_category": row.SubCat,
				"source":       row.Source,
				"chunk_index":  fmt.Sprintf("%d", row.ChunkIndex),
				"vector_id":    row.VectorID,
			},
		})
	}
	return out, nil
}
