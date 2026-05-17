package riskconfig

import "time"

// RiskPoint 可配置风险点，保存后会同步到审阅知识库供 RAG 检索。
type RiskPoint struct {
	ID               uint64    `gorm:"primaryKey;autoIncrement;comment:风险点ID" json:"id"`
	ContractTypeID   uint64    `gorm:"not null;index;comment:合同类型ID" json:"contract_type_id"`
	ContractTypeName string    `gorm:"type:varchar(64);not null;index;comment:合同类型名称" json:"contract_type_name"`
	RiskContent      string    `gorm:"type:longtext;not null;comment:风险点内容" json:"risk_content"`
	RiskType         string    `gorm:"type:varchar(64);index;comment:风险类型" json:"risk_type"`
	RiskLevel        string    `gorm:"type:varchar(16);index;comment:风险等级" json:"risk_level"`
	ApplicableScope  string    `gorm:"type:varchar(32);default:platform;comment:适用范围 individual/department/platform" json:"applicable_scope"`
	Departments      string    `gorm:"type:varchar(512);comment:适用部门JSON" json:"departments"`
	Creator          string    `gorm:"type:varchar(64);index;comment:创建者账号" json:"creator"`
	Status           string    `gorm:"type:varchar(16);default:enabled;index;comment:enabled/disabled" json:"status"`
	KnowledgeDocID   uint64    `gorm:"index;comment:同步的知识库文档ID" json:"knowledge_doc_id"`
	CreatedAt        time.Time `gorm:"autoCreateTime;comment:创建时间" json:"created_at"`
	UpdatedAt        time.Time `gorm:"autoUpdateTime;comment:更新时间" json:"updated_at"`
}

func (RiskPoint) TableName() string {
	return "review_risk_points"
}
