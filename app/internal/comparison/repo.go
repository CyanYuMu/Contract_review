package comparison

import (
	"context"
	"contract_review/app/internal/global"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type ComparisonRepo struct {
	db *gorm.DB
}

func NewComparisonRepo(db *gorm.DB) *ComparisonRepo {
	return &ComparisonRepo{db: db}
}

// Create 创建比对任务
func (r *ComparisonRepo) Create(ctx context.Context, task *ComparisonTask) error {
	err := r.db.WithContext(ctx).Create(task).Error
	if err != nil {
		global.Log.Error("ComparisonRepo.Create failed", zap.Error(err))
		return fmt.Errorf("create comparison task failed: %w", err)
	}
	return nil
}

// GetByID 根据ID获取比对任务
func (r *ComparisonRepo) GetByID(ctx context.Context, id uint64) (*ComparisonTask, error) {
	var task ComparisonTask
	err := r.db.WithContext(ctx).First(&task, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		global.Log.Error("ComparisonRepo.GetByID failed", zap.Error(err))
		return nil, err
	}
	return &task, nil
}

// GetBySessionID 根据SessionID获取比对任务（单条）
func (r *ComparisonRepo) GetBySessionID(ctx context.Context, sessionID uint64) (*ComparisonTask, error) {
	var task ComparisonTask
	err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).First(&task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		global.Log.Error("ComparisonRepo.GetBySessionID failed", zap.Error(err))
		return nil, err
	}
	return &task, nil
}

// GetBySessionIDList 根据SessionID获取比对任务列表
func (r *ComparisonRepo) GetBySessionIDList(ctx context.Context, sessionID uint64) ([]ComparisonTask, error) {
	var tasks []ComparisonTask
	err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).Find(&tasks).Error
	if err != nil {
		global.Log.Error("ComparisonRepo.GetBySessionIDList failed", zap.Error(err))
	}
	return tasks, err
}

// GetByUserID 根据用户ID获取比对任务列表
func (r *ComparisonRepo) GetByUserID(ctx context.Context, userID uint64) ([]ComparisonTask, error) {
	var tasks []ComparisonTask
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC").Find(&tasks).Error
	if err != nil {
		global.Log.Error("ComparisonRepo.GetByUserID failed", zap.Error(err))
	}
	return tasks, err
}

// GetBySessionIDAndStatus 根据SessionID和状态获取比对任务
func (r *ComparisonRepo) GetBySessionIDAndStatus(ctx context.Context, sessionID uint64, status string) ([]ComparisonTask, error) {
	var tasks []ComparisonTask
	err := r.db.WithContext(ctx).Where("session_id = ? AND status = ?", sessionID, status).Find(&tasks).Error
	if err != nil {
		global.Log.Error("ComparisonRepo.GetBySessionIDAndStatus failed", zap.Error(err))
	}
	return tasks, err
}

// List 获取比对任务列表（分页）
func (r *ComparisonRepo) List(ctx context.Context, offset, limit int) ([]ComparisonTask, int64, error) {
	var tasks []ComparisonTask
	var count int64

	if err := r.db.WithContext(ctx).Model(&ComparisonTask{}).Count(&count).Error; err != nil {
		global.Log.Error("ComparisonRepo.List count failed", zap.Error(err))
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).Offset(offset).Limit(limit).Order("created_at DESC").Find(&tasks).Error
	if err != nil {
		global.Log.Error("ComparisonRepo.List find failed", zap.Error(err))
	}
	return tasks, count, err
}

// ListByUserID 根据用户ID分页获取比对任务
func (r *ComparisonRepo) ListByUserID(ctx context.Context, userID uint64, offset, limit int) ([]ComparisonTask, int64, error) {
	var tasks []ComparisonTask
	var count int64

	if err := r.db.WithContext(ctx).Model(&ComparisonTask{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		global.Log.Error("ComparisonRepo.ListByUserID count failed", zap.Error(err))
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Offset(offset).Limit(limit).Order("created_at DESC").Find(&tasks).Error
	if err != nil {
		global.Log.Error("ComparisonRepo.ListByUserID find failed", zap.Error(err))
	}
	return tasks, count, err
}

// Update 更新比对任务
func (r *ComparisonRepo) Update(ctx context.Context, task *ComparisonTask) error {
	err := r.db.WithContext(ctx).Model(task).Updates(task).Error
	if err != nil {
		global.Log.Error("ComparisonRepo.Update failed", zap.Error(err))
		return errors.New("update comparison task failed")
	}
	return nil
}

// UpdateStatus 更新任务状态
func (r *ComparisonRepo) UpdateStatus(ctx context.Context, id uint64, status string) error {
	err := r.db.WithContext(ctx).Model(&ComparisonTask{}).Where("id = ?", id).Update("status", status).Error
	if err != nil {
		global.Log.Error("ComparisonRepo.UpdateStatus failed", zap.Error(err))
		return errors.New("update comparison task status failed")
	}
	return nil
}

// UpdateByID 按字段更新比对任务
func (r *ComparisonRepo) UpdateByID(ctx context.Context, id uint64, updates map[string]interface{}) error {
	err := r.db.WithContext(ctx).Model(&ComparisonTask{}).Where("id = ?", id).Updates(updates).Error
	if err != nil {
		global.Log.Error("ComparisonRepo.UpdateByID failed", zap.Error(err))
		return errors.New("update comparison task failed")
	}
	return nil
}

// UpdateTaskFiles 更新任务的文件ID（重新比对）
func (r *ComparisonRepo) UpdateTaskFiles(ctx context.Context, task *ComparisonTask, standardFileID, comparisonFileID uint64) error {
	updates := map[string]interface{}{
		"standard_file_id":   standardFileID,
		"comparison_file_id": comparisonFileID,
		"status":             "pending",
		"diff_summary":       nil,
		"diff_result":        nil,
		"similarity":         0,
		"completed_at":       nil,
	}
	err := r.db.WithContext(ctx).Model(task).Updates(updates).Error
	if err != nil {
		global.Log.Error("ComparisonRepo.UpdateTaskFiles failed", zap.Error(err))
		return errors.New("update comparison task files failed")
	}
	return nil
}

// SaveDiffResult 保存比对结果
func (r *ComparisonRepo) SaveDiffResult(ctx context.Context, task *ComparisonTask, diffSummary, diffResult string, similarity float64) error {
	updates := map[string]interface{}{
		"diff_summary": diffSummary,
		"diff_result":  diffResult,
		"similarity":   similarity,
		"status":       "completed",
		"completed_at": time.Now(),
	}
	err := r.db.WithContext(ctx).Model(task).Updates(updates).Error
	if err != nil {
		global.Log.Error("ComparisonRepo.SaveDiffResult failed", zap.Error(err))
		return errors.New("save comparison result failed")
	}
	return nil
}

// Delete 删除比对任务
func (r *ComparisonRepo) Delete(ctx context.Context, id uint64) error {
	err := r.db.WithContext(ctx).Delete(&ComparisonTask{}, id).Error
	if err != nil {
		global.Log.Error("ComparisonRepo.Delete failed", zap.Error(err))
		return errors.New("delete comparison task failed")
	}
	return nil
}

// DeleteBySessionID 根据SessionID删除比对任务
func (r *ComparisonRepo) DeleteBySessionID(ctx context.Context, sessionID uint64) error {
	err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).Delete(&ComparisonTask{}).Error
	if err != nil {
		global.Log.Error("ComparisonRepo.DeleteBySessionID failed", zap.Error(err))
		return errors.New("delete comparison task failed")
	}
	return nil
}

// Count 统计比对任务数量
func (r *ComparisonRepo) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&ComparisonTask{}).Count(&count).Error
	if err != nil {
		global.Log.Error("ComparisonRepo.Count failed", zap.Error(err))
	}
	return count, err
}

// CountBySessionID 统计指定SessionID的比对任务数量
func (r *ComparisonRepo) CountBySessionID(ctx context.Context, sessionID uint64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&ComparisonTask{}).Where("session_id = ?", sessionID).Count(&count).Error
	if err != nil {
		global.Log.Error("ComparisonRepo.CountBySessionID failed", zap.Error(err))
	}
	return count, err
}

// CountByUserID 统计指定用户的比对任务数量
func (r *ComparisonRepo) CountByUserID(ctx context.Context, userID uint64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&ComparisonTask{}).Where("user_id = ?", userID).Count(&count).Error
	if err != nil {
		global.Log.Error("ComparisonRepo.CountByUserID failed", zap.Error(err))
	}
	return count, err
}

// CountByStatus 统计指定状态的比对任务数量
func (r *ComparisonRepo) CountByStatus(ctx context.Context, status string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&ComparisonTask{}).Where("status = ?", status).Count(&count).Error
	if err != nil {
		global.Log.Error("ComparisonRepo.CountByStatus failed", zap.Error(err))
	}
	return count, err
}
