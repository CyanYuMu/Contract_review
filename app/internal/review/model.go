package review

import "time"

// ReviewTask 审阅任务表
type ReviewTask struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement;comment:审阅任务主键ID" json:"id"`                                            // 审阅任务主键ID
	SessionID    uint64    `gorm:"not null;index;comment:关联会话ID" json:"session_id"`                                                // 关联会话ID
	FileID       uint64    `gorm:"not null;index;comment:所属文件ID" json:"file_id"`                                                   // 所属文件ID
	UserID       uint64    `gorm:"not null;index;comment:发起用户ID" json:"user_id"`                                                   // 发起用户ID
	Stance       string    `gorm:"type:varchar(32);comment:审查立场(甲方/乙方)" json:"stance"`                                             // 审查立场
	Intensity    string    `gorm:"type:varchar(32);comment:审查尺度(严格/标准/宽松)" json:"intensity"`                                       // 审查尺度
	ContractType string    `gorm:"type:varchar(64);comment:合同类型" json:"contract_type"`                                             // 合同类型
	Description  string    `gorm:"type:text;comment:审查需求描述" json:"description"`                                                    // 审查需求描述
	Status       string    `gorm:"type:varchar(32);default:pending;comment:状态(pending/processing/completed/failed)" json:"status"` // 状态
	CreatedAt    time.Time `gorm:"autoCreateTime;comment:创建时间" json:"created_at"`                                                  // 创建时间
	CompletedAt  time.Time `gorm:"comment:完成时间" json:"completed_at"`                                                               // 完成时间
}

// TableName 自定义表名
func (ReviewTask) TableName() string {
	return "review_tasks"
}

// ReviewResult 审查结果表
type ReviewResult struct {
	ID               uint64    `gorm:"primaryKey;autoIncrement;comment:审查结果ID" json:"id"`      // 审查结果ID
	SessionID        uint64    `gorm:"not null;index;comment:会话ID" json:"session_id"`          // 会话ID
	TaskID           uint64    `gorm:"not null;index;comment:任务ID" json:"task_id"`             // 任务ID
	Index            int       `gorm:"comment:结果索引" json:"index"`                              // 结果索引
	OriginalContent  string    `gorm:"type:text;comment:原始文本" json:"original_content"`         // 原始文本
	RiskAnalysis     string    `gorm:"type:text;comment:风险分析" json:"risk_analysis"`            // 风险分析
	RiskLevel        string    `gorm:"type:varchar(16);comment:风险等级(高/中/低)" json:"risk_level"` // 风险等级
	SuggestedContent string    `gorm:"type:text;comment:建议内容" json:"suggested_content"`        // 建议内容
	Reason           string    `gorm:"type:text;comment:修改理由" json:"reason"`                   // 修改理由
	RiskType         string    `gorm:"type:varchar(64);comment:风险类型" json:"risk_type"`         // 风险类型
	CreatedAt        time.Time `gorm:"autoCreateTime;comment:创建时间" json:"created_at"`          // 创建时间
}

// TableName 自定义表名
func (ReviewResult) TableName() string {
	return "review_results"
}
