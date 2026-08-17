package rag

import (
	"strings"
	"testing"
)

func TestDocumentProcessorProducesParentAndChildren(t *testing.T) {
	dp := NewDocumentProcessor(nil, 50, 10)
	doc := Document{
		ID:          "doc1",
		Title:       "测试审阅指引",
		Category:    "规范",
		SubCategory: "通用",
		Source:      "测试来源",
		Content:     strings.Repeat("违约责任应覆盖逾期付款、逾期交付、质量不合格等场景。", 20),
	}

	chunks, err := dp.ProcessDocument(doc)
	if err != nil {
		t.Fatal(err)
	}

	parentCount, childCount := 0, 0
	var parentID string
	for _, c := range chunks {
		switch c.ChunkType {
		case ChunkTypeParent:
			parentCount++
			parentID = c.ID
		case ChunkTypeChild:
			childCount++
		}
	}
	if parentCount != 1 {
		t.Fatalf("应恰好 1 个 parent 分块，got %d", parentCount)
	}
	if childCount < 2 {
		t.Fatalf("长文档应拆出多个 child 分块，got %d", childCount)
	}
	for _, c := range chunks {
		if c.ChunkType == ChunkTypeChild {
			if c.ParentChunkID != parentID {
				t.Fatalf("child %s 的 ParentChunkID=%q，应为 %q", c.ID, c.ParentChunkID, parentID)
			}
			if c.Embedding != nil {
				t.Fatalf("child 不应有 embedding（embedder 为 nil）")
			}
		}
	}
}

func TestPartitionChunks(t *testing.T) {
	chunks := []Chunk{
		{ID: "p1", ChunkType: ChunkTypeParent, Content: "parent"},
		{ID: "c1", ChunkType: ChunkTypeChild, Content: "child1", ParentChunkID: "p1"},
		{ID: "s1", Content: "standalone"},
	}
	retrievable, parents := PartitionChunks(chunks)
	if len(retrievable) != 2 {
		t.Fatalf("检索单元应为 2（child + standalone），got %d", len(retrievable))
	}
	if len(parents) != 1 {
		t.Fatalf("父分块应为 1，got %d", len(parents))
	}
	if parents["p1"].ID != "p1" {
		t.Fatalf("父分块映射缺失 p1")
	}
}

func TestRAGRetrieverExpandParents(t *testing.T) {
	parent := Chunk{ID: "p1", DocID: "d1", Content: "完整章节：违约责任应覆盖逾期付款、逾期交付、质量不合格等场景。", ChunkType: ChunkTypeParent}
	child := Chunk{ID: "c1", DocID: "d1", Content: "违约金条款", Metadata: map[string]string{"source": "s1"}, ParentChunkID: "p1", ChunkType: ChunkTypeChild}

	keyword := NewSimpleKeywordIndex()
	if err := keyword.Index([]Chunk{child}); err != nil {
		t.Fatal(err)
	}

	config := DefaultRetrieverConfig()
	config.FinalTopK = 5
	config.EnableBM25 = false
	config.EnableRerank = false
	config.MMRLambda = 1.0 // 关闭 MMR，简化断言
	retriever := NewRAGRetriever(nil, keyword, nil, config)
	retriever.SetParents(map[string]Chunk{parent.ID: parent})

	res, err := retriever.Retrieve("违约金", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("父子回填后应去重为 1 条，got %d", len(res))
	}
	if res[0].Content != parent.Content {
		t.Fatalf("应回填 parent 内容，got %q", res[0].Content)
	}
	if res[0].Metadata["child_content"] != child.Content {
		t.Fatalf("child_content 应保留子块原文，got %q", res[0].Metadata["child_content"])
	}
}

func TestRAGRetrieverExpandParentsDedupesByParent(t *testing.T) {
	parent := Chunk{ID: "p1", DocID: "d1", Content: "完整章节内容", ChunkType: ChunkTypeParent}
	child1 := Chunk{ID: "c1", DocID: "d1", Content: "违约金条款甲", ParentChunkID: "p1", ChunkType: ChunkTypeChild}
	child2 := Chunk{ID: "c2", DocID: "d1", Content: "违约金条款乙", ParentChunkID: "p1", ChunkType: ChunkTypeChild}

	keyword := NewSimpleKeywordIndex()
	if err := keyword.Index([]Chunk{child1, child2}); err != nil {
		t.Fatal(err)
	}

	config := DefaultRetrieverConfig()
	config.FinalTopK = 5
	config.EnableBM25 = false
	config.EnableRerank = false
	config.MMRLambda = 1.0
	retriever := NewRAGRetriever(nil, keyword, nil, config)
	retriever.SetParents(map[string]Chunk{parent.ID: parent})

	res, err := retriever.Retrieve("违约金", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("同一 parent 的多个 child 应去重为 1 条，got %d", len(res))
	}
	if res[0].Content != parent.Content {
		t.Fatalf("应回填 parent 内容，got %q", res[0].Content)
	}
}
