package agent

import (
	"testing"

	"contract_review/app/internal/rag"
)

func candidateSet(clauseID string, candidates ...RiskCandidate) []candidateClauseSet {
	return []candidateClauseSet{
		{clause: Clause{ID: clauseID, Title: "条款", Content: "条款正文"}, candidates: candidates},
	}
}

func TestRiskCandidateFromSearchResultPrefersMetadata(t *testing.T) {
	content := `【风险点配置】
合同类型：服务合同
风险类型：旧文本
法律依据：未配置

风险内容：
旧文本内容`
	result := rag.SearchResult{
		ChunkID: "risk-1-chunk-0",
		Content: content,
		Metadata: map[string]string{
			"risk_id":              "RP000001",
			"risk_type":            "单方解除",
			"risk_level":           "高",
			"trigger_condition":    "甲方可单方解除",
			"keywords":             "解除、单方",
			"applicable_clauses":   "解除条款",
			"legal_basis":          "《民法典》第五百六十三条",
			"recommended_template": "双方协商一致方可解除",
			"risk_content":         "服务合同中单方解除权应设置对等触发条件。",
			"source":               "风险点配置",
		},
		Source: "风险点配置",
	}

	c := riskCandidateFromSearchResult(result)
	if c.RiskType != "单方解除" {
		t.Fatalf("RiskType 应来自 metadata，got %q", c.RiskType)
	}
	if c.LegalBasis != "《民法典》第五百六十三条" {
		t.Fatalf("LegalBasis 应来自 metadata，got %q", c.LegalBasis)
	}
	if c.RiskContent != "服务合同中单方解除权应设置对等触发条件。" {
		t.Fatalf("RiskContent 应来自 metadata，got %q", c.RiskContent)
	}
	if len(c.Keywords) != 2 || c.Keywords[0] != "解除" || c.Keywords[1] != "单方" {
		t.Fatalf("Keywords 应解析为 [解除 单方]，got %v", c.Keywords)
	}
}

func TestRiskCandidateFromSearchResultFallsBackToContentParsing(t *testing.T) {
	// 老数据：无 metadata，回退到从拍扁文本反解字段。
	content := "风险类型：单方解除\n风险等级：高\n法律依据：未配置\n触发条件：甲方可单方解除\n"
	result := rag.SearchResult{
		ChunkID:  "risk-1-chunk-0",
		Content:  content,
		Metadata: map[string]string{"title": "风险点配置"},
		Source:   "风险点配置",
	}
	c := riskCandidateFromSearchResult(result)
	if c.RiskType != "单方解除" {
		t.Fatalf("旧数据应回退解析 RiskType，got %q", c.RiskType)
	}
	if c.LegalBasis != "未配置" {
		t.Fatalf("旧数据应回退解析 LegalBasis，got %q", c.LegalBasis)
	}
}

func TestParseCandidateRiskFindingsPlaceholderBasisNotVerified(t *testing.T) {
	// 候选只有"未配置"占位，且无风险内容、无实质指引 → 不应 verified。
	candidates := candidateSet("clause-1", RiskCandidate{
		ID:          "RP1",
		RiskType:    "单方解除",
		LegalBasis:  "未配置",
		RiskContent: "",
		Content:     "【风险点配置】合同类型：服务合同\n风险内容：未配置",
		Source:      "风险点配置",
	})
	text := `{"findings":[{"clause_id":"clause-1","candidate_ids":["RP1"],"risk_type":"单方解除","risk_level":"高","risk_description":"单方解除风险","verified":true,"confidence":0.9}]}`

	findings := parseCandidateRiskFindings(text, candidates)
	if len(findings) != 1 {
		t.Fatalf("应解析出 1 条风险，got %d", len(findings))
	}
	if findings[0].Verified {
		t.Fatalf("候选无实质依据时不应 verified")
	}
	if !findings[0].RequiresHumanReview {
		t.Fatalf("无依据时应标记待人工确认")
	}
	if len(findings[0].LegalBasis) != 0 {
		t.Fatalf("不应产生占位依据，got %v", findings[0].LegalBasis)
	}
}

func TestParseCandidateRiskFindingsRiskContentAsReviewBasisVerified(t *testing.T) {
	// 法律依据为占位，但风险内容可作"审阅规范"依据 → verified，且依据不是占位文本。
	candidates := candidateSet("clause-1", RiskCandidate{
		ID:          "RP1",
		RiskType:    "单方解除",
		LegalBasis:  "未配置",
		RiskContent: "服务合同单方解除权应设置对等触发条件。",
		Source:      "风险点配置",
	})
	text := `{"findings":[{"clause_id":"clause-1","candidate_ids":["RP1"],"risk_type":"单方解除","risk_level":"高","risk_description":"单方解除风险","verified":true,"confidence":0.9}]}`

	findings := parseCandidateRiskFindings(text, candidates)
	if len(findings) != 1 {
		t.Fatalf("应解析出 1 条风险，got %d", len(findings))
	}
	if !findings[0].Verified {
		t.Fatalf("有风险内容作为审阅规范依据时应 verified")
	}
	for _, lb := range findings[0].LegalBasis {
		if lb.Content == "未配置" {
			t.Fatalf("依据内容不应是占位文本，got %q", lb.Content)
		}
	}
}

func TestParseCandidateRiskFindingsLowConfidenceDowngraded(t *testing.T) {
	candidates := candidateSet("clause-1", RiskCandidate{
		ID:         "RP1",
		RiskType:   "单方解除",
		LegalBasis: "《民法典》第五百六十三条",
		Source:     "风险点配置",
	})
	text := `{"findings":[{"clause_id":"clause-1","candidate_ids":["RP1"],"risk_type":"单方解除","risk_description":"单方解除风险","verified":true,"confidence":0.1}]}`

	findings := parseCandidateRiskFindings(text, candidates)
	if len(findings) != 1 {
		t.Fatalf("应解析出 1 条风险，got %d", len(findings))
	}
	if findings[0].Verified {
		t.Fatalf("LLM 自评置信度过低时应降级为未验证")
	}
	if !findings[0].RequiresHumanReview {
		t.Fatalf("降级后应标记待人工确认")
	}
}

func TestCandidateBasisFieldsExcludesFlattenedTemplate(t *testing.T) {
	c := RiskCandidate{
		ID:          "RP1",
		RiskType:    "单方解除",
		LegalBasis:  "未配置",
		RiskContent: "",
		Content:     "【风险点配置】合同类型：服务合同\n风险内容：未配置",
		Source:      "风险点配置",
	}
	content, _, _ := candidateBasisFields(c)
	if content != "" {
		t.Fatalf("拍扁模板不应作为依据，got %q", content)
	}

	// 纯文本审阅指引（种子知识）可直接作为依据。
	c2 := RiskCandidate{ID: "builtin-1", RiskType: "违约责任", Content: "违约责任应覆盖逾期付款、逾期交付等场景。", Source: "内置审阅指引"}
	content2, source2, _ := candidateBasisFields(c2)
	if content2 != c2.Content {
		t.Fatalf("纯文本审阅指引应作为依据，got %q", content2)
	}
	if source2 != "内置审阅指引" {
		t.Fatalf("来源应为内置审阅指引，got %q", source2)
	}
}
