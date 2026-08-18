package rag

import (
	"context"
	"testing"

	"gorm.io/gorm"
)

func TestIngestKnowledgeDocumentValidation(t *testing.T) {
	ctx := context.Background()

	if _, err := IngestKnowledgeDocument(ctx, nil, KnowledgeDocumentInput{Title: "x", Content: "y"}); err == nil {
		t.Fatal("nil db 应报错")
	}

	// 非 nil 但不会被实际使用的 db（校验在访问 db 前返回）。
	db := &gorm.DB{}
	if _, err := IngestKnowledgeDocument(ctx, db, KnowledgeDocumentInput{Title: "", Content: "y"}); err == nil {
		t.Fatal("空标题应报错")
	}
	if _, err := IngestKnowledgeDocument(ctx, db, KnowledgeDocumentInput{Title: "x", Content: "   "}); err == nil {
		t.Fatal("空内容应报错")
	}
}
