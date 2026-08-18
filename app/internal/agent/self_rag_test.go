package agent

import (
	"strings"
	"testing"
)

func TestIsBoilerplateClause(t *testing.T) {
	if !isBoilerplateClause(Clause{ID: "preamble", Title: "合同序言/首部"}) {
		t.Fatal("preamble 应视为 boilerplate")
	}
	if !isBoilerplateClause(Clause{ID: "clause-1", Title: "第X条 通知与送达"}) {
		t.Fatal("送达条款应视为 boilerplate")
	}
	if !isBoilerplateClause(Clause{ID: "clause-1", Title: "签章页"}) {
		t.Fatal("签章页应视为 boilerplate")
	}
	if isBoilerplateClause(Clause{ID: "clause-1", Title: "第X条 违约责任"}) {
		t.Fatal("违约责任条款不应视为 boilerplate")
	}
	if isBoilerplateClause(Clause{ID: "clause-1", Title: "第X条 付款方式"}) {
		t.Fatal("付款条款不应视为 boilerplate")
	}
}

func TestFilterReviewableClauses(t *testing.T) {
	clauses := []Clause{
		{ID: "preamble", Title: "合同序言/首部"},
		{ID: "clause-1", Title: "第X条 违约责任"},
		{ID: "clause-2", Title: "第X条 通知与送达"},
		{ID: "clause-3", Title: "第X条 付款方式"},
	}
	reviewable, skipped := filterReviewableClauses(clauses)
	if skipped != 2 {
		t.Fatalf("应跳过 2 个 boilerplate，got %d", skipped)
	}
	if len(reviewable) != 2 {
		t.Fatalf("应剩余 2 个可审阅条款，got %d", len(reviewable))
	}
	for _, c := range reviewable {
		if isBoilerplateClause(c) {
			t.Fatalf("过滤后不应包含 boilerplate 条款：%s", c.ID)
		}
	}
}

func TestBuildGeneralizedQuery(t *testing.T) {
	meta := ContractMeta{ContractType: "服务合同", Stance: "甲方"}
	q := buildGeneralizedQuery(Clause{Content: "逾期付款应承担违约金，可单方解除合同。"}, meta)
	if q == "" {
		t.Fatal("含法律关键词时应生成泛化查询")
	}
	for _, kw := range []string{"违约金", "解除"} {
		if !strings.Contains(q, kw) {
			t.Fatalf("泛化查询应包含关键词 %q，got %q", kw, q)
		}
	}
	if !strings.Contains(q, "服务合同") {
		t.Fatalf("泛化查询应包含合同类型，got %q", q)
	}

	if buildGeneralizedQuery(Clause{Content: "甲乙双方友好协商。"}, meta) != "" {
		t.Fatal("无法律关键词时应返回空查询")
	}
}

func TestGeneralizedRetrievalThresholdFor(t *testing.T) {
	cases := map[string]int{"严格": 5, "宽松": 1, "标准": 3, "": 3, "未知": 3}
	for intensity, want := range cases {
		if got := generalizedRetrievalThresholdFor(intensity); got != want {
			t.Fatalf("intensity=%q 阈值应为 %d，got %d", intensity, want, got)
		}
	}
}

func TestBuildContractOverview(t *testing.T) {
	clauses := []Clause{
		{ID: "clause-1", Title: "第一条 服务范围", Category: "权利义务"},
		{ID: "clause-2", Title: "第二条 付款方式", Category: "付款条款"},
	}
	overview := buildContractOverview(clauses, ContractMeta{ContractType: "服务合同", Stance: "甲方", Amount: "1000000.00"})
	for _, want := range []string{"服务合同", "甲方", "1000000.00", "clause-1", "第一条 服务范围", "付款方式", "共 2 个条款"} {
		if !strings.Contains(overview, want) {
			t.Fatalf("overview 应包含 %q，got:\n%s", want, overview)
		}
	}
}
