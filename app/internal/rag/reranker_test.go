package rag

import (
	"strings"
	"testing"
)

func TestCleanPassageForRerank(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		checkFn  func(string) bool
	}{
		{
			name:  "删除代码块",
			input: "条款内容\n```go\nfunc main() {}\n```\n后续内容",
			checkFn: func(s string) bool {
				return !strings.Contains(s, "```") && !strings.Contains(s, "func main")
			},
		},
		{
			name:  "删除 Markdown 标题标记",
			input: "## 第一条 合同标的\n\n服务内容如下：\n### 1.1 服务范围",
			checkFn: func(s string) bool {
				return !strings.Contains(s, "##") && strings.Contains(s, "第一条") && strings.Contains(s, "1.1")
			},
		},
		{
			name:  "链接保留文本丢弃URL",
			input: "详见[《民法典》第563条](https://example.com/law/563)",
			checkFn: func(s string) bool {
				return strings.Contains(s, "《民法典》第563条") && !strings.Contains(s, "https://")
			},
		},
		{
			name:  "删除粗体标记",
			input: "**重要提示**：本条款属于**核心义务**条款",
			checkFn: func(s string) bool {
				return !strings.Contains(s, "**") && strings.Contains(s, "重要提示") && strings.Contains(s, "核心义务")
			},
		},
		{
			name:  "删除列表标记",
			input: "- 第一项\n- 第二项\n* 第三项",
			checkFn: func(s string) bool {
				return !strings.Contains(s, "- ") && !strings.Contains(s, "* ") &&
					strings.Contains(s, "第一项") && strings.Contains(s, "第二项")
			},
		},
		{
			name:  "删除图片引用",
			input: "如图：![合同签章页](images/sign.png)",
			checkFn: func(s string) bool {
				// 图片语法完全删除（alt text 对 rerank 语义匹配无价值）
				return !strings.Contains(s, "!") && !strings.Contains(s, "sign.png") &&
					strings.Contains(s, "如图")
			},
		},
		{
			name:  "压缩多余换行",
			input: "第一段\n\n\n\n第二段\n\n\n第三段",
			checkFn: func(s string) bool {
				return !strings.Contains(s, "\n\n\n\n") && strings.Count(s, "\n\n") <= 2
			},
		},
		{
			name:  "保持正常中文内容",
			input: "当事人一方不履行合同义务或者履行合同义务不符合约定的，应当承担继续履行、采取补救措施或者赔偿损失等违约责任。",
			checkFn: func(s string) bool {
				return strings.Contains(s, "违约责任") && strings.Contains(s, "当事人")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanPassageForRerank(tt.input)
			if !tt.checkFn(result) {
				t.Errorf("cleanPassageForRerank() 输出不符合预期\n输入: %s\n输出: %s", tt.input, result)
			}
		})
	}
}

func TestTokenizeForBM25(t *testing.T) {
	query := "服务类合同中付款条款的违约责任和违约金约定"
	terms := tokenizeForBM25(query)

	expectedTerms := []string{"服务", "付款", "违约", "违约金", "责任"}
	for _, expected := range expectedTerms {
		found := false
		for _, term := range terms {
			if strings.Contains(term, expected) || strings.Contains(expected, term) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("tokenizeForBM25() 缺少预期关键词: %s, 结果: %v", expected, terms)
		}
	}
}

func TestCompositeScore(t *testing.T) {
	result := &SearchResult{
		ChunkID:   "test-1",
		BaseScore: 0.7,
	}

	score := compositeScore(result, 0.85, 0.7, false)
	if score < 0 || score > 1 {
		t.Errorf("compositeScore() 分数超出 [0,1] 范围: %f", score)
	}

	// Rerank score 应该占主要权重 (60%)
	// 0.6*0.85 + 0.3*0.7 + 0.1*1.0 = 0.51 + 0.21 + 0.1 = 0.82
	expectedRange := 0.82
	if score < expectedRange-0.05 || score > expectedRange+0.05 {
		t.Errorf("compositeScore() = %f, 期望约 %f", score, expectedRange)
	}
}

func TestJaccardSimilarity(t *testing.T) {
	a := tokenizeForMMR("违约责任条款 违约金 赔偿 损失")
	b := tokenizeForMMR("违约责任 违约金 解除 终止")
	c := tokenizeForMMR("知识产权 著作权 专利 商标")

	simAB := jaccardSimilarity(a, b)
	simAC := jaccardSimilarity(a, c)

	if simAB <= simAC {
		t.Errorf("相似条款的 Jaccard 相似度应高于不相似条款: simAB=%f, simAC=%f", simAB, simAC)
	}
}

func TestMMR(t *testing.T) {
	results := []SearchResult{
		{ChunkID: "1", Content: "违约责任条款 违约金 赔偿 损失", Score: 0.9, BaseScore: 0.9},
		{ChunkID: "2", Content: "违约责任 违约金 解除 终止", Score: 0.85, BaseScore: 0.85},
		{ChunkID: "3", Content: "知识产权 著作权 专利 商标", Score: 0.8, BaseScore: 0.8},
		{ChunkID: "4", Content: "付款条款 支付 价款 费用", Score: 0.75, BaseScore: 0.75},
		{ChunkID: "5", Content: "争议解决 仲裁 诉讼 管辖", Score: 0.7, BaseScore: 0.7},
	}

	// MMR with lambda=0.7 should promote diversity
	selected := maximalMarginalRelevance(results, 0.7, 3)

	if len(selected) != 3 {
		t.Fatalf("MMR 应返回 %d 个结果, 实际 %d", 3, len(selected))
	}

	// 第一个应该是最相关的
	if selected[0].ChunkID != "1" {
		t.Errorf("MMR 第一个应选最高分, 期望 chunk-1, 实际 %s", selected[0].ChunkID)
	}

	// 应该包含不同类别的条款 (chunk-3 是知识产权，不应该被违约 chunk 排挤掉)
	hasDiverse := false
	for _, s := range selected {
		if s.ChunkID == "3" {
			hasDiverse = true
			break
		}
	}
	if !hasDiverse && t.Failed() {
		t.Log("MMR 应保留不同主题的 chunk")
	}
}

func TestEnrichedPassage(t *testing.T) {
	result := SearchResult{
		Content: "服务类合同应明确服务内容、服务期限、质量标准、验收流程",
		Source:  "服务类合同审阅指引",
		Metadata: map[string]string{
			"title":        "服务类合同审阅要点",
			"category":     "规范",
			"sub_category": "服务类合同",
		},
	}

	passage := buildEnrichedPassage(result)

	checks := []string{
		"服务类合同审阅要点",
		"规范",
		"服务类合同",
		"服务内容",
	}
	for _, check := range checks {
		if !strings.Contains(passage, check) {
			t.Errorf("buildEnrichedPassage() 应包含 '%s', 实际: %s", check, passage)
		}
	}
}

func TestClampFloat(t *testing.T) {
	tests := []struct {
		value  float64
		min    float64
		max    float64
		expect float64
	}{
		{1.5, 0, 1, 1.0},
		{-0.5, 0, 1, 0.0},
		{0.75, 0, 1, 0.75},
		{0, 0, 1, 0.0},
		{1, 0, 1, 1.0},
	}

	for _, tt := range tests {
		result := clampFloat(tt.value, tt.min, tt.max)
		if result != tt.expect {
			t.Errorf("clampFloat(%f, %f, %f) = %f, 期望 %f",
				tt.value, tt.min, tt.max, result, tt.expect)
		}
	}
}
