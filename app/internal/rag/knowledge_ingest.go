package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"contract_review/app/internal/knowledge"

	"gorm.io/gorm"
)

// KnowledgeDocumentInput 长文档入库入参。
type KnowledgeDocumentInput struct {
	Title       string // 文档标题
	Category    string // 规范/法规/案例/示范
	SubCategory string // 合同类型子类（如 买卖合同/通用）
	Source      string // 来源
	Content     string // 全文（长文档会做父子分块）
}

// IngestResult 长文档入库结果。
type IngestResult struct {
	DocID       uint64 // 新文档 ID
	ChunkCount  int    // 总分块数（parent+child）
	ParentCount int    // parent 分块数
	ChildCount  int    // child 分块数
}

// 长文档父子的 child 分块大小与 overlap。
const (
	DefaultChildChunkSize = 300
	DefaultChunkOverlap   = 50
)

// IngestKnowledgeDocument 将一篇长文档做父子分块后入库（status=indexed，供 RAG 加载）。
// 分块不做 embedding —— embedding 由审阅编排器加载知识库时按需生成（与风险点分块一致）。
//
// 注意：分块在事务内、创建 doc 之后执行，用真实 doc.ID 生成 chunk ID，保证跨文档唯一。
func IngestKnowledgeDocument(ctx context.Context, db *gorm.DB, input KnowledgeDocumentInput) (*IngestResult, error) {
	if db == nil {
		return nil, fmt.Errorf("knowledge ingest: db is nil")
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return nil, fmt.Errorf("文档标题不能为空")
	}
	if strings.TrimSpace(input.Content) == "" {
		return nil, fmt.Errorf("文档内容不能为空")
	}

	result := &IngestResult{}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := knowledge.NewRepo(tx)
		kbDoc := &knowledge.ReviewKnowledgeDoc{
			Title:       title,
			Category:    strings.TrimSpace(input.Category),
			SubCategory: strings.TrimSpace(input.SubCategory),
			Source:      strings.TrimSpace(input.Source),
			Content:     input.Content,
			Status:      "indexed",
		}
		if err := repo.CreateDocument(ctx, kbDoc); err != nil {
			return err
		}
		result.DocID = kbDoc.ID

		doc := Document{
			ID:          strconv.FormatUint(kbDoc.ID, 10),
			Title:       title,
			Category:    kbDoc.Category,
			SubCategory: kbDoc.SubCategory,
			Source:      kbDoc.Source,
			Content:     input.Content,
		}
		processor := NewDocumentProcessor(nil, DefaultChildChunkSize, DefaultChunkOverlap)
		chunks, err := processor.ProcessDocument(doc)
		if err != nil {
			return err
		}

		kbChunks := make([]knowledge.ReviewKnowledgeChunk, 0, len(chunks))
		for i, c := range chunks {
			meta := "{}"
			if len(c.Metadata) > 0 {
				if b, err := json.Marshal(c.Metadata); err == nil {
					meta = string(b)
				}
			}
			kbChunks = append(kbChunks, knowledge.ReviewKnowledgeChunk{
				DocID:         kbDoc.ID,
				ChunkIndex:    i,
				Content:       c.Content,
				VectorID:      c.ID,
				ParentChunkID: c.ParentChunkID,
				ChunkType:     c.ChunkType,
				Metadata:      meta,
			})
			if c.ChunkType == ChunkTypeParent {
				result.ParentCount++
			} else {
				result.ChildCount++
			}
		}
		result.ChunkCount = len(kbChunks)

		// 回写分块数
		if err := tx.Model(&knowledge.ReviewKnowledgeDoc{}).
			Where("id = ?", kbDoc.ID).
			Update("chunk_count", len(kbChunks)).Error; err != nil {
			return err
		}
		return repo.CreateChunks(ctx, kbChunks)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
