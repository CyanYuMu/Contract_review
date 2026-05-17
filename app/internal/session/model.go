package session

import "time"

const (
	SessionTypeReview        = "review"
	SessionTypeCompare       = "compare"
	SessionTypeCompareLegacy = "comparison"
	SessionTypeChat          = "chat"
)

// Session 任务会话表
type Session struct {
	ID          uint      `gorm:"primaryKey;autoIncrement;comment:会话ID" json:"id"`
	UserID      uint      `gorm:"not null;index;comment:用户ID" json:"user_id"`
	Title       string    `gorm:"type:varchar(256);comment:会话主题" json:"title"`
	SessionType string    `gorm:"type:varchar(16);index;comment:会话类型(review/compare/chat)" json:"session_type"`
	FileID      uint      `gorm:"comment:关联文件ID(审阅时使用)" json:"file_id"`
	CreatedAt   time.Time `gorm:"autoCreateTime;comment:创建时间" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime;comment:更新时间" json:"updated_at"`
}

// TableName 指定表名
func (Session) TableName() string {
	return "sessions"
}

// SessionWithFileInfo 带文件信息的会话（用于审阅类型）
type SessionWithFileInfo struct {
	Session
	PartyA       string `gorm:"column:party_a" json:"party_a"`
	PartyB       string `gorm:"column:party_b" json:"party_b"`
	FileName     string `gorm:"column:file_name" json:"file_name"`
	FilePath     string `gorm:"column:file_path" json:"file_path"`
	IsAccepted   bool   `gorm:"column:is_accepted" json:"is_accepted"`
	ContractType string `gorm:"column:contract_type" json:"contract_type"`
}

// SessionWithCompareInfo 带比对信息的会话（用于比对类型）
type SessionWithCompareInfo struct {
	Session
	FileID1     uint    `gorm:"column:file_id_1" json:"file_id_1"`
	PartyA1     string  `gorm:"column:party_a_1" json:"party_a_1"`
	PartyB1     string  `gorm:"column:party_b_1" json:"party_b_1"`
	FileName1   string  `gorm:"column:file_name_1" json:"file_name_1"`
	FilePath1   string  `gorm:"column:file_path_1" json:"file_path_1"`
	IsAccepted1 bool    `gorm:"column:is_accepted_1" json:"is_accepted_1"`
	FileID2     uint    `gorm:"column:file_id_2" json:"file_id_2"`
	PartyA2     string  `gorm:"column:party_a_2" json:"party_a_2"`
	PartyB2     string  `gorm:"column:party_b_2" json:"party_b_2"`
	FileName2   string  `gorm:"column:file_name_2" json:"file_name_2"`
	FilePath2   string  `gorm:"column:file_path_2" json:"file_path_2"`
	IsAccepted2 bool    `gorm:"column:is_accepted_2" json:"is_accepted_2"`
	Similarity  float64 `gorm:"column:similarity" json:"similarity"`
}
