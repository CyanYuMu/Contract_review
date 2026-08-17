package rag

import (
	"context"
	"contract_review/app/internal/knowledge"
	"encoding/json"
	"fmt"
	"strings"

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
		base := map[string]string{
			"title":        row.Title,
			"category":     row.Category,
			"sub_category": row.SubCat,
			"source":       row.Source,
			"chunk_index":  fmt.Sprintf("%d", row.ChunkIndex),
			"vector_id":    row.VectorID,
		}
		// 合并结构化元数据（风险点字段等）；结构化值优先，基础字段作兜底。
		meta, err := mergeChunkMetadata(row.Metadata, base)
		if err != nil {
			meta = base
		}
		out = append(out, Chunk{
			ID:       chunkID,
			DocID:    docIDStr,
			Content:  row.Content,
			Metadata: meta,
		})
	}
	return out, nil
}

// mergeChunkMetadata 解析分块的 JSON 元数据并与基础元数据合并。
// 结构化元数据（如 risk_type/legal_basis）优先，基础字段（title/category 等）作兜底。
func mergeChunkMetadata(rawJSON string, base map[string]string) (map[string]string, error) {
	trimmed := strings.TrimSpace(rawJSON)
	if trimmed == "" || trimmed == "{}" {
		return base, nil
	}
	var structured map[string]string
	if err := json.Unmarshal([]byte(trimmed), &structured); err != nil {
		return nil, err
	}
	if len(structured) == 0 {
		return base, nil
	}
	merged := make(map[string]string, len(base)+len(structured))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range structured {
		merged[k] = v
	}
	return merged, nil
}
