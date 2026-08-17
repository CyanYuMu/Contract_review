package rag

import "testing"

func TestSimpleKeywordIndexBM25Search(t *testing.T) {
	idx := NewSimpleKeywordIndex()
	chunks := []Chunk{
		{ID: "a", DocID: "d1", Content: "合同违约责任应覆盖逾期付款、逾期交付等场景。", Metadata: map[string]string{"source": "s1"}},
		{ID: "b", DocID: "d2", Content: "合同应当明确双方权利义务与责任。", Metadata: map[string]string{"source": "s2"}},
	}
	if err := idx.Index(chunks); err != nil {
		t.Fatal(err)
	}

	res, err := idx.Search("违约金", 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) == 0 {
		t.Fatal("expected results for query 违约金")
	}
	if res[0].ChunkID != "a" {
		t.Fatalf("expected chunk a first, got %s", res[0].ChunkID)
	}
	// 分数应归一化到 (0,1)
	if res[0].Score <= 0 || res[0].Score >= 1 {
		t.Fatalf("score out of (0,1): %f", res[0].Score)
	}
}

func TestSimpleKeywordIndexFilters(t *testing.T) {
	idx := NewSimpleKeywordIndex()
	_ = idx.Index([]Chunk{
		{ID: "a", DocID: "d1", Content: "违约金条款", Metadata: map[string]string{"sub_category": "买卖合同"}},
		{ID: "b", DocID: "d2", Content: "违约金条款", Metadata: map[string]string{"sub_category": "服务合同"}},
	})
	res, err := idx.Search("违约金", 10, map[string]string{"sub_category": "服务合同"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].ChunkID != "b" {
		t.Fatalf("filter failed: %+v", res)
	}
}

func TestTokenizeProducesBigramAndLegalKeyword(t *testing.T) {
	terms := tokenize("违约责任不合理")
	has := func(s string) bool {
		for _, term := range terms {
			if term == s {
				return true
			}
		}
		return false
	}
	for _, want := range []string{"违约", "责任", "约责"} {
		if !has(want) {
			t.Fatalf("tokenize 缺少 %q: %v", want, terms)
		}
	}
}
