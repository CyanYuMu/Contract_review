package knowledgeapi

import (
	"context"
	"strings"

	"contract_review/app/internal/rag"

	"gorm.io/gorm"
)

// Service 知识库文档管理服务。
type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// IngestDocument 入库一篇长文档（父子分块）。
// 分类/子分类为空时给默认值，保证检索按 sub_category 过滤可用。
func (s *Service) IngestDocument(ctx context.Context, req IngestDocumentRequest) (*rag.IngestResult, error) {
	category := strings.TrimSpace(req.Category)
	if category == "" {
		category = "规范"
	}
	subCategory := strings.TrimSpace(req.SubCategory)
	if subCategory == "" {
		subCategory = "通用"
	}
	return rag.IngestKnowledgeDocument(ctx, s.db, rag.KnowledgeDocumentInput{
		Title:       req.Title,
		Category:    category,
		SubCategory: subCategory,
		Source:      strings.TrimSpace(req.Source),
		Content:     req.Content,
	})
}
