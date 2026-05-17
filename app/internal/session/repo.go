package session

import (
	"context"
	"contract_review/app/internal/global"
	"errors"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type SessionRepo struct {
	db *gorm.DB
}

func NewSessionRepo(db *gorm.DB) *SessionRepo {
	return &SessionRepo{db: db}
}

// Create 创建会话
func (r *SessionRepo) Create(ctx context.Context, session *Session) error {
	err := r.db.WithContext(ctx).Create(session).Error
	if err != nil {
		global.Log.Error("SessionRepo.Create failed", zap.Error(err))
		return errors.New("create session failed")
	}
	return nil
}

// GetByID 根据ID获取会话
func (r *SessionRepo) GetByID(ctx context.Context, id uint) (*Session, error) {
	var session Session
	err := r.db.WithContext(ctx).First(&session, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		global.Log.Error("SessionRepo.GetByID failed", zap.Error(err))
		return nil, err
	}
	return &session, nil
}

// GetByIDAndUserID 根据ID和用户ID获取会话
func (r *SessionRepo) GetByIDAndUserID(ctx context.Context, id, userID uint) (*Session, error) {
	var session Session
	err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&session).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		global.Log.Error("SessionRepo.GetByIDAndUserID failed", zap.Error(err))
		return nil, err
	}
	return &session, nil
}

// GetByFileID 根据文件ID获取会话列表
func (r *SessionRepo) GetByFileID(ctx context.Context, fileID uint) ([]Session, error) {
	var sessions []Session
	err := r.db.WithContext(ctx).Where("file_id = ?", fileID).Find(&sessions).Error
	if err != nil {
		global.Log.Error("SessionRepo.GetByFileID failed", zap.Error(err))
	}
	return sessions, err
}

// GetByUserID 根据用户ID获取会话列表
func (r *SessionRepo) GetByUserID(ctx context.Context, userID uint) ([]Session, error) {
	var sessions []Session
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC").Find(&sessions).Error
	if err != nil {
		global.Log.Error("SessionRepo.GetByUserID failed", zap.Error(err))
	}
	return sessions, err
}

// GetBySessionType 根据会话类型获取会话列表
func (r *SessionRepo) GetBySessionType(ctx context.Context, sessionType string) ([]Session, error) {
	var sessions []Session
	err := r.db.WithContext(ctx).Where("session_type = ?", sessionType).Find(&sessions).Error
	if err != nil {
		global.Log.Error("SessionRepo.GetBySessionType failed", zap.Error(err))
	}
	return sessions, err
}

// GetByUserIDAndType 根据用户ID和会话类型获取会话列表
func (r *SessionRepo) GetByUserIDAndType(ctx context.Context, userID uint, sessionType string) ([]Session, error) {
	var sessions []Session
	err := r.db.WithContext(ctx).Where("user_id = ? AND session_type = ?", userID, sessionType).Order("created_at DESC").Find(&sessions).Error
	if err != nil {
		global.Log.Error("SessionRepo.GetByUserIDAndType failed", zap.Error(err))
	}
	return sessions, err
}

// ListByUserID 分页获取用户会话列表
func (r *SessionRepo) ListByUserID(ctx context.Context, userID uint, offset, limit int) ([]Session, int64, error) {
	var sessions []Session
	var count int64

	if err := r.db.WithContext(ctx).Model(&Session{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		global.Log.Error("SessionRepo.ListByUserID count failed", zap.Error(err))
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Offset(offset).Limit(limit).Order("created_at DESC").Find(&sessions).Error
	if err != nil {
		global.Log.Error("SessionRepo.ListByUserID find failed", zap.Error(err))
	}
	return sessions, count, err
}

// ListByUserIDAndType 分页获取用户指定类型的会话列表
func (r *SessionRepo) ListByUserIDAndType(ctx context.Context, userID uint, sessionType string, offset, limit int) ([]Session, int64, error) {
	var sessions []Session
	var count int64

	query := r.db.WithContext(ctx).Model(&Session{}).Where("user_id = ? AND session_type = ?", userID, sessionType)

	if err := query.Count(&count).Error; err != nil {
		global.Log.Error("SessionRepo.ListByUserIDAndType count failed", zap.Error(err))
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).
		Where("user_id = ? AND session_type = ?", userID, sessionType).
		Offset(offset).
		Limit(limit).
		Order("created_at DESC").
		Find(&sessions).Error
	if err != nil {
		global.Log.Error("SessionRepo.ListByUserIDAndType find failed", zap.Error(err))
	}
	return sessions, count, err
}

// ListReviewSessionsWithFileInfo 获取审阅类型会话列表（带文件信息）
func (r *SessionRepo) ListReviewSessionsWithFileInfo(ctx context.Context, userID uint, offset, limit int) ([]SessionWithFileInfo, int64, error) {
	var sessions []SessionWithFileInfo
	var count int64

	// 统计总数
	if err := r.db.WithContext(ctx).Model(&Session{}).
		Where("user_id = ? AND session_type = ?", userID, SessionTypeReview).
		Count(&count).Error; err != nil {
		global.Log.Error("SessionRepo.ListReviewSessionsWithFileInfo count failed", zap.Error(err))
		return nil, 0, err
	}

	// 联表查询
	err := r.db.WithContext(ctx).
		Table("sessions s").
		Select(`
			s.id, s.user_id, s.title, s.session_type, s.file_id, s.created_at, s.updated_at,
			c.party_a, c.party_b, c.title as file_name, c.file_path, c.is_accepted,
			rt.contract_type
		`).
		Joins("LEFT JOIN contracts c ON s.file_id = c.id").
		Joins("LEFT JOIN review_tasks rt ON rt.session_id = s.id AND rt.id = (SELECT MAX(id) FROM review_tasks WHERE session_id = s.id)").
		Where("s.user_id = ? AND s.session_type = ?", userID, SessionTypeReview).
		Order("s.created_at DESC").
		Offset(offset).
		Limit(limit).
		Scan(&sessions).Error

	if err != nil {
		global.Log.Error("SessionRepo.ListReviewSessionsWithFileInfo query failed", zap.Error(err))
		return nil, 0, err
	}

	return sessions, count, nil
}

// ListCompareSessionsWithInfo 获取比对类型会话列表（带比对信息）
func (r *SessionRepo) ListCompareSessionsWithInfo(ctx context.Context, userID uint, offset, limit int) ([]SessionWithCompareInfo, int64, error) {
	var sessions []SessionWithCompareInfo
	var count int64

	// 统计总数
	if err := r.db.WithContext(ctx).Model(&Session{}).
		Where("user_id = ? AND session_type IN ?", userID, []string{SessionTypeCompare, SessionTypeCompareLegacy}).
		Count(&count).Error; err != nil {
		global.Log.Error("SessionRepo.ListCompareSessionsWithInfo count failed", zap.Error(err))
		return nil, 0, err
	}

	// 联表查询
	err := r.db.WithContext(ctx).
		Table("sessions s").
		Select(`
			s.id, s.user_id, s.title, s.session_type, s.file_id, s.created_at, s.updated_at,
			c1.id as file_id_1, c1.party_a as party_a_1, c1.party_b as party_b_1, c1.title as file_name_1, c1.file_path as file_path_1, c1.is_accepted as is_accepted_1,
			c2.id as file_id_2, c2.party_a as party_a_2, c2.party_b as party_b_2, c2.title as file_name_2, c2.file_path as file_path_2, c2.is_accepted as is_accepted_2,
			ct.similarity
		`).
		Joins("LEFT JOIN comparison_tasks ct ON ct.session_id = s.id").
		Joins("LEFT JOIN contracts c1 ON c1.id = ct.standard_file_id").
		Joins("LEFT JOIN contracts c2 ON c2.id = ct.comparison_file_id").
		Where("s.user_id = ? AND s.session_type IN ?", userID, []string{SessionTypeCompare, SessionTypeCompareLegacy}).
		Order("s.created_at DESC").
		Offset(offset).
		Limit(limit).
		Scan(&sessions).Error

	if err != nil {
		global.Log.Error("SessionRepo.ListCompareSessionsWithInfo query failed", zap.Error(err))
		return nil, 0, err
	}

	return sessions, count, nil
}

// Update 更新会话
func (r *SessionRepo) Update(ctx context.Context, session *Session) error {
	err := r.db.WithContext(ctx).Model(session).Updates(session).Error
	if err != nil {
		global.Log.Error("SessionRepo.Update failed", zap.Error(err))
		return errors.New("update session failed")
	}
	return nil
}

// UpdateByID 按字段更新会话
func (r *SessionRepo) UpdateByID(ctx context.Context, id uint, updates map[string]interface{}) error {
	err := r.db.WithContext(ctx).Model(&Session{}).Where("id = ?", id).Updates(updates).Error
	if err != nil {
		global.Log.Error("SessionRepo.UpdateByID failed", zap.Error(err))
		return errors.New("update session failed")
	}
	return nil
}

// UpdateTitle 更新会话标题
func (r *SessionRepo) UpdateTitle(ctx context.Context, id, userID uint, newTitle string) error {
	result := r.db.WithContext(ctx).Model(&Session{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("title", newTitle)

	if result.Error != nil {
		global.Log.Error("SessionRepo.UpdateTitle failed", zap.Error(result.Error))
		return errors.New("update session title failed")
	}

	if result.RowsAffected == 0 {
		return errors.New("session not found or no permission")
	}

	return nil
}

// Delete 删除会话
func (r *SessionRepo) Delete(ctx context.Context, id uint) error {
	err := r.db.WithContext(ctx).Delete(&Session{}, id).Error
	if err != nil {
		global.Log.Error("SessionRepo.Delete failed", zap.Error(err))
		return errors.New("delete session failed")
	}
	return nil
}

// DeleteByIDAndUserID 根据ID和用户ID删除会话
func (r *SessionRepo) DeleteByIDAndUserID(ctx context.Context, id, userID uint) error {
	result := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		Delete(&Session{})

	if result.Error != nil {
		global.Log.Error("SessionRepo.DeleteByIDAndUserID failed", zap.Error(result.Error))
		return errors.New("delete session failed")
	}

	if result.RowsAffected == 0 {
		return errors.New("session not found or no permission")
	}

	return nil
}

// DeleteByUserID 根据用户ID删除会话
func (r *SessionRepo) DeleteByUserID(ctx context.Context, userID uint) error {
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&Session{}).Error
	if err != nil {
		global.Log.Error("SessionRepo.DeleteByUserID failed", zap.Error(err))
		return errors.New("delete session failed")
	}
	return nil
}

// DeleteByFileID 根据文件ID删除会话
func (r *SessionRepo) DeleteByFileID(ctx context.Context, fileID uint) error {
	err := r.db.WithContext(ctx).Where("file_id = ?", fileID).Delete(&Session{}).Error
	if err != nil {
		global.Log.Error("SessionRepo.DeleteByFileID failed", zap.Error(err))
		return errors.New("delete session failed")
	}
	return nil
}

// Count 统计会话总数
func (r *SessionRepo) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Session{}).Count(&count).Error
	if err != nil {
		global.Log.Error("SessionRepo.Count failed", zap.Error(err))
	}
	return count, err
}

// CountByUserID 统计指定用户的会话数量
func (r *SessionRepo) CountByUserID(ctx context.Context, userID uint) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Session{}).Where("user_id = ?", userID).Count(&count).Error
	if err != nil {
		global.Log.Error("SessionRepo.CountByUserID failed", zap.Error(err))
	}
	return count, err
}

// CountByUserIDAndType 统计指定用户和类型的会话数量
func (r *SessionRepo) CountByUserIDAndType(ctx context.Context, userID uint, sessionType string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Session{}).
		Where("user_id = ? AND session_type = ?", userID, sessionType).
		Count(&count).Error
	if err != nil {
		global.Log.Error("SessionRepo.CountByUserIDAndType failed", zap.Error(err))
	}
	return count, err
}

// CountByFileID 统计指定文件的会话数量
func (r *SessionRepo) CountByFileID(ctx context.Context, fileID uint) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Session{}).Where("file_id = ?", fileID).Count(&count).Error
	if err != nil {
		global.Log.Error("SessionRepo.CountByFileID failed", zap.Error(err))
	}
	return count, err
}
