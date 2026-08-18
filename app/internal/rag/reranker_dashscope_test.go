package rag

import "testing"

func TestDashScopeRerankURL(t *testing.T) {
	const native = "https://dashscope.aliyuncs.com/api/v1/services/rerank/text-rerank/text-rerank"

	cases := map[string]string{
		"https://dashscope.aliyuncs.com/compatible-mode/v1": native,
		"https://dashscope.aliyuncs.com":                    native,
		"https://dashscope.aliyuncs.com/":                   native,
		native:                                              native,
	}
	for input, want := range cases {
		if got := dashScopeRerankURL(input); got != want {
			t.Fatalf("dashScopeRerankURL(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestUseDashScopeReranker(t *testing.T) {
	if !UseDashScopeReranker("https://dashscope.aliyuncs.com/compatible-mode/v1") {
		t.Fatal("dashscope 端点应判定为使用 DashScope reranker")
	}
	if !UseDashScopeReranker("https://xxx.aliyuncs.com/v1") {
		t.Fatal("aliyuncs.com 端点应判定为使用 DashScope reranker")
	}
	if UseDashScopeReranker("https://api.openai.com/v1") {
		t.Fatal("openai 端点不应判定为 DashScope reranker")
	}
	if UseDashScopeReranker("https://api.deepseek.com/v1") {
		t.Fatal("deepseek 端点不应判定为 DashScope reranker")
	}
}
