package agent

import (
	"context"
	"strings"
	"testing"

	"contract_review/app/internal/global"

	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

func TestFindGapClauses(t *testing.T) {
	clauses := []Clause{
		{ID: "c1", Title: "一"},
		{ID: "c2", Title: "二"},
		{ID: "c3", Title: "三"},
	}
	findings := []RiskFinding{
		{ClauseID: "c1"},
		{ClauseID: "c3", ClauseIDs: []string{"c3", "c2"}}, // 合并后覆盖 c3 + c2
	}
	gaps := findGapClauses(clauses, findings)
	if len(gaps) != 0 {
		t.Fatalf("所有条款都应有发现覆盖，got gaps=%v", gapIDs(gaps))
	}

	gaps = findGapClauses(clauses, []RiskFinding{{ClauseID: "c1"}})
	if len(gaps) != 2 || gaps[0].ID != "c2" || gaps[1].ID != "c3" {
		t.Fatalf("应返回 c2、c3，got %v", gapIDs(gaps))
	}
}

func TestBuildReflectionHints(t *testing.T) {
	eval := &QualityEvaluation{
		CriticalGaps: []string{"覆盖面不足", "存在未验证风险"},
		Feedback:     "重点检查违约责任",
	}
	hints := buildReflectionHints(eval)
	if len(hints) != 3 {
		t.Fatalf("应返回 3 条提示，got %v", hints)
	}
	if hints[2] != "改进建议：重点检查违约责任" {
		t.Fatalf("反馈提示格式错误，got %q", hints[2])
	}
	if buildReflectionHints(nil) != nil {
		t.Fatal("nil eval 应返回 nil")
	}
}

func TestReviewOrchestratorReflectionReReviewsGapClauses(t *testing.T) {
	previousLogger := global.Log
	global.Log = zap.NewNop()
	defer func() { global.Log = previousLogger }()

	riskCalls := 0
	fakeLLM := func(_ context.Context, messages []*schema.Message) (*schema.Message, error) {
		sys := ""
		if len(messages) > 0 {
			sys = messages[0].Content
		}
		if strings.Contains(sys, "质量评估") {
			// 质量评估：低分 + 建议重试
			return &schema.Message{Role: schema.Assistant,
				Content: `{"overall_score":0.5,"critical_gaps":["覆盖面不足"],"feedback":"重点检查违约责任","should_retry":true}`}, nil
		}
		riskCalls++
		if riskCalls == 1 {
			// 首次审阅：无发现（两个条款都成为缺口）
			return &schema.Message{Role: schema.Assistant, Content: `{"findings":[]}`}, nil
		}
		// 反思重审：为两个缺口条款各发现一个风险
		return &schema.Message{Role: schema.Assistant,
			Content: `{"findings":[{"clause_id":"clause-1","risk_type":"违约责任","risk_level":"中","risk_description":"缺少违约责任条款","verified":false,"confidence":0.5},{"clause_id":"clause-2","risk_type":"付款风险","risk_level":"低","risk_description":"付款节点不明确","verified":false,"confidence":0.4}]}`}, nil
	}

	config := DefaultOrchestratorConfig()
	config.MaxConcurrentAgents = 1
	config.RiskReviewBatchSize = 2 // 两条款一批，首轮与反思各一次 LLM
	orch := NewReviewOrchestrator(fakeLLM, nil, nil, config)

	// 两个"第X条"结构 → structuralSplit 直接产出 clause-1/clause-2，不触发 LLM 拆分。
	report, err := orch.ReviewContract(context.Background(),
		"第一条 服务范围\n乙方提供服务。\n第二条 付款方式\n甲方按期付款。",
		ContractMeta{ContractType: "服务合同"},
		ReviewCallbacks{})
	if err != nil {
		t.Fatal(err)
	}
	if report.ReflectionCount != 1 {
		t.Fatalf("应触发 1 轮反思，got %d", report.ReflectionCount)
	}
	if len(report.Findings) != 2 {
		t.Fatalf("反思后应有 2 条发现，got %d", len(report.Findings))
	}
	if riskCalls != 2 {
		t.Fatalf("风险审阅应被调用 2 次（首轮+反思），got %d", riskCalls)
	}
}

func gapIDs(clauses []Clause) []string {
	ids := make([]string, len(clauses))
	for i, c := range clauses {
		ids[i] = c.ID
	}
	return ids
}
