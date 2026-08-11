package qa

import "time"

// QAMessage 合同问答消息（多轮对话历史）
type QAMessage struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	SessionID uint64    `gorm:"index;not null;comment:会话ID" json:"session_id"`
	UserID    uint64    `gorm:"not null;comment:用户ID" json:"user_id"`
	Role      string    `gorm:"type:varchar(16);not null;comment:user/assistant" json:"role"`
	Content   string    `gorm:"type:longtext;not null" json:"content"`
	Tokens    int       `gorm:"default:0;comment:assistant 消耗 token 数" json:"tokens"`
	CreatedAt time.Time `gorm:"index;autoCreateTime" json:"created_at"`
}

func (QAMessage) TableName() string { return "qa_messages" }

const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)
