package comparison

import "time"

// ComparisonTask 比对任务表
type ComparisonTask struct {
	ID               uint64    `gorm:"primaryKey;autoIncrement;comment:比对任务ID" json:"id"`             // 比对任务ID
	SessionID        uint64    `gorm:"not null;index;comment:关联会话ID" json:"session_id"`               // 关联会话ID
	UserID           uint64    `gorm:"not null;index;comment:用户ID" json:"user_id"`                    // 用户ID
	StandardFileID   uint64    `gorm:"not null;comment:标准文档ID" json:"standard_file_id"`               // 标准文档ID
	ComparisonFileID uint64    `gorm:"not null;comment:比对文档ID" json:"comparison_file_id"`             // 比对文档ID
	Status           string    `gorm:"type:varchar(32);default:'pending';comment:任务状态" json:"status"` // 任务状态
	DiffSummary      string    `gorm:"type:text;comment:差异摘要JSON" json:"diff_summary,omitempty"`      // 差异摘要JSON
	DiffResult       string    `gorm:"type:text;comment:差异详情JSON" json:"diff_result,omitempty"`       // 差异详情JSON
	Similarity       float64   `gorm:"type:decimal(5,2);comment:相似度" json:"similarity"`               // 相似度
	CreatedAt        time.Time `gorm:"autoCreateTime;comment:创建时间" json:"created_at"`                 // 创建时间
	CompletedAt      time.Time `gorm:"comment:完成时间" json:"completed_at,omitempty"`                    // 完成时间
}

// TableName 指定表名
func (ComparisonTask) TableName() string {
	return "comparison_tasks"
}
