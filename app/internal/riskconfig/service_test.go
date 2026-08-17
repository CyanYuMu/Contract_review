package riskconfig

import "testing"

func TestBuildRiskPointMetadata(t *testing.T) {
	rp := &RiskPoint{
		ID:                  7,
		ContractTypeName:    "服务合同",
		RiskType:            "单方解除",
		RiskLevel:           "高",
		ApplicableScope:     "platform",
		TriggerCondition:    "甲方可单方解除",
		Keywords:            `["解除","单方"]`,
		ApplicableClauses:   `["解除条款"]`,
		LegalBasis:          "《民法典》第五百六十三条",
		RecommendedTemplate: "双方协商一致方可解除",
		RiskContent:         "服务合同中单方解除权应设置对等触发条件。",
	}

	m := buildRiskPointMetadata(rp)
	if m["risk_id"] != "RP000007" {
		t.Fatalf("risk_id got %q", m["risk_id"])
	}
	if m["risk_type"] != "单方解除" {
		t.Fatalf("risk_type got %q", m["risk_type"])
	}
	if m["risk_level"] != "高" {
		t.Fatalf("risk_level got %q", m["risk_level"])
	}
	if m["legal_basis"] != "《民法典》第五百六十三条" {
		t.Fatalf("legal_basis got %q", m["legal_basis"])
	}
	if m["risk_content"] != "服务合同中单方解除权应设置对等触发条件。" {
		t.Fatalf("risk_content got %q", m["risk_content"])
	}
	if m["keywords"] != "解除、单方" {
		t.Fatalf("keywords 应为顿号连接串，got %q", m["keywords"])
	}
	if m["applicable_clauses"] != "解除条款" {
		t.Fatalf("applicable_clauses got %q", m["applicable_clauses"])
	}

	// JSON 序列化必须可逆且非空。
	jsonStr := buildRiskPointMetadataJSON(rp)
	if jsonStr == "" || jsonStr == "{}" {
		t.Fatalf("metadata JSON 不应为空，got %q", jsonStr)
	}
}
