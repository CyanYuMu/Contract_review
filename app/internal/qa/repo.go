package qa

import (
	"context"

	"gorm.io/gorm"
)

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// Create 写入一条消息
func (r *Repo) Create(ctx context.Context, msg *QAMessage) error {
	return r.db.WithContext(ctx).Create(msg).Error
}

// ListBySession 取会话最近 limit 条消息（按时间正序返回）
func (r *Repo) ListBySession(ctx context.Context, sessionID uint64, limit int) ([]QAMessage, error) {
	if limit <= 0 {
		limit = 20
	}
	var msgs []QAMessage
	// 子查询取最近 limit 条，再正序返回，保证多轮顺序正确
	sub := r.db.WithContext(ctx).Model(&QAMessage{}).
		Where("session_id = ?", sessionID).
		Order("id DESC").Limit(limit)
	err := r.db.WithContext(ctx).Raw("SELECT * FROM (?) AS sub ORDER BY id ASC", sub).Scan(&msgs).Error
	return msgs, err
}

// DeleteBySession 删除会话全部消息
func (r *Repo) DeleteBySession(ctx context.Context, sessionID uint64) error {
	return r.db.WithContext(ctx).Where("session_id = ?", sessionID).Delete(&QAMessage{}).Error
}
