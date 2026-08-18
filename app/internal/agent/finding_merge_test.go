package agent

import "testing"

func TestMergeFindingsSameCandidateAcrossClauses(t *testing.T) {
	findings := []RiskFinding{
		{FindingID: "c1-r1", ClauseID: "clause-1", CandidateIDs: []string{"RP1"}, RiskType: "违约金", RiskLevel: "中", Verified: true, Confidence: 0.7},
		{FindingID: "c2-r1", ClauseID: "clause-2", CandidateIDs: []string{"RP1"}, RiskType: "违约金", RiskLevel: "高", Verified: true, Confidence: 0.8},
	}
	merged := MergeFindings(findings)
	if len(merged) != 1 {
		t.Fatalf("同候选应合并为 1 条，got %d", len(merged))
	}
	m := merged[0]
	if len(m.ClauseIDs) != 2 || m.ClauseIDs[0] != "clause-1" || m.ClauseIDs[1] != "clause-2" {
		t.Fatalf("ClauseIDs 应保留 [clause-1 clause-2]，got %v", m.ClauseIDs)
	}
	if m.ClauseID != "clause-1" {
		t.Fatalf("主条款应为最早条款 clause-1，got %s", m.ClauseID)
	}
	if m.RiskLevel != "高" {
		t.Fatalf("应取最严重等级 高，got %s", m.RiskLevel)
	}
	if m.Confidence != 0.8 {
		t.Fatalf("应取最高置信度 0.8，got %f", m.Confidence)
	}
}

func TestMergeFindingsDifferentCandidateNotMerged(t *testing.T) {
	findings := []RiskFinding{
		{ClauseID: "clause-1", CandidateIDs: []string{"RP1"}, RiskType: "违约金", RiskLevel: "中"},
		{ClauseID: "clause-2", CandidateIDs: []string{"RP2"}, RiskType: "违约金", RiskLevel: "中"},
	}
	merged := MergeFindings(findings)
	if len(merged) != 2 {
		t.Fatalf("不同候选不应合并，got %d", len(merged))
	}
}

func TestMergeFindingsUnverifiedSameRiskTypeMerged(t *testing.T) {
	// 无候选的待人工确认风险，按风险类型归并
	findings := []RiskFinding{
		{ClauseID: "clause-1", RiskType: "违约责任缺失", RiskLevel: "中", RequiresHumanReview: true},
		{ClauseID: "clause-3", RiskType: "违约责任缺失", RiskLevel: "中", RequiresHumanReview: true},
	}
	merged := MergeFindings(findings)
	if len(merged) != 1 {
		t.Fatalf("同类型无候选应合并，got %d", len(merged))
	}
	if len(merged[0].ClauseIDs) != 2 {
		t.Fatalf("应保留 2 个条款，got %v", merged[0].ClauseIDs)
	}
}

func TestMergeFindingsSingleFindingUnchanged(t *testing.T) {
	findings := []RiskFinding{
		{ClauseID: "clause-1", CandidateIDs: []string{"RP1"}, RiskType: "违约金", RiskLevel: "中"},
	}
	merged := MergeFindings(findings)
	if len(merged) != 1 || merged[0].ClauseID != "clause-1" {
		t.Fatalf("单条应原样返回，got %+v", merged)
	}
}
