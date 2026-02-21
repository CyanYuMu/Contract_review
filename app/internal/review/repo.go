package review

import (
	"context"
	"contract_review/app/internal/global"
	"errors"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ============ ReviewRepo 审阅任务仓储 ============

type ReviewRepo struct {
	db *gorm.DB
}

func NewReviewRepo(db *gorm.DB) *ReviewRepo {
	return &ReviewRepo{db: db}
}

// Create 创建审阅任务
func (r *ReviewRepo) Create(ctx context.Context, task *ReviewTask) error {
	err := r.db.WithContext(ctx).Create(task).Error
	if err != nil {
		global.Log.Error("ReviewRepo.Create failed", zap.Error(err))
		return errors.New("create review task failed")
	}
	return nil
}

// GetByID 根据ID获取审阅任务
func (r *ReviewRepo) GetByID(ctx context.Context, id uint64) (*ReviewTask, error) {
	var task ReviewTask
	err := r.db.WithContext(ctx).First(&task, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		global.Log.Error("ReviewRepo.GetByID failed", zap.Error(err))
		return nil, err
	}
	return &task, nil
}

// GetBySessionID 根据SessionID获取审阅任务（单条）
func (r *ReviewRepo) GetBySessionID(ctx context.Context, sessionID uint64) (*ReviewTask, error) {
	var task ReviewTask
	err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).First(&task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		global.Log.Error("ReviewRepo.GetBySessionID failed", zap.Error(err))
		return nil, err
	}
	return &task, nil
}

// GetBySessionIDList 根据SessionID获取审阅任务列表
func (r *ReviewRepo) GetBySessionIDList(ctx context.Context, sessionID uint64) ([]ReviewTask, error) {
	var tasks []ReviewTask
	err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).Find(&tasks).Error
	if err != nil {
		global.Log.Error("ReviewRepo.GetBySessionIDList failed", zap.Error(err))
	}
	return tasks, err
}

// GetBySessionIDAndStatus 根据SessionID和状态获取审阅任务
func (r *ReviewRepo) GetBySessionIDAndStatus(ctx context.Context, sessionID uint64, status string) ([]ReviewTask, error) {
	var tasks []ReviewTask
	err := r.db.WithContext(ctx).Where("session_id = ? AND status = ?", sessionID, status).Find(&tasks).Error
	if err != nil {
		global.Log.Error("ReviewRepo.GetBySessionIDAndStatus failed", zap.Error(err))
	}
	return tasks, err
}

// GetByFileID 根据文件ID获取审阅任务列表
func (r *ReviewRepo) GetByFileID(ctx context.Context, fileID uint64) ([]ReviewTask, error) {
	var tasks []ReviewTask
	err := r.db.WithContext(ctx).Where("file_id = ?", fileID).Find(&tasks).Error
	if err != nil {
		global.Log.Error("ReviewRepo.GetByFileID failed", zap.Error(err))
	}
	return tasks, err
}

// GetByUserID 根据用户ID获取审阅任务列表
func (r *ReviewRepo) GetByUserID(ctx context.Context, userID uint64) ([]ReviewTask, error) {
	var tasks []ReviewTask
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC").Find(&tasks).Error
	if err != nil {
		global.Log.Error("ReviewRepo.GetByUserID failed", zap.Error(err))
	}
	return tasks, err
}

// List 获取审阅任务列表（分页）
func (r *ReviewRepo) List(ctx context.Context, offset, limit int) ([]ReviewTask, int64, error) {
	var tasks []ReviewTask
	var count int64

	if err := r.db.WithContext(ctx).Model(&ReviewTask{}).Count(&count).Error; err != nil {
		global.Log.Error("ReviewRepo.List count failed", zap.Error(err))
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).Offset(offset).Limit(limit).Order("created_at DESC").Find(&tasks).Error
	if err != nil {
		global.Log.Error("ReviewRepo.List find failed", zap.Error(err))
	}
	return tasks, count, err
}

// ListBySessionID 根据SessionID分页获取审阅任务
func (r *ReviewRepo) ListBySessionID(ctx context.Context, sessionID uint64, offset, limit int) ([]ReviewTask, int64, error) {
	var tasks []ReviewTask
	var count int64

	if err := r.db.WithContext(ctx).Model(&ReviewTask{}).Where("session_id = ?", sessionID).Count(&count).Error; err != nil {
		global.Log.Error("ReviewRepo.ListBySessionID count failed", zap.Error(err))
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).Offset(offset).Limit(limit).Order("created_at DESC").Find(&tasks).Error
	if err != nil {
		global.Log.Error("ReviewRepo.ListBySessionID find failed", zap.Error(err))
	}
	return tasks, count, err
}

// ListByUserID 根据用户ID分页获取审阅任务
func (r *ReviewRepo) ListByUserID(ctx context.Context, userID uint64, offset, limit int) ([]ReviewTask, int64, error) {
	var tasks []ReviewTask
	var count int64

	if err := r.db.WithContext(ctx).Model(&ReviewTask{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		global.Log.Error("ReviewRepo.ListByUserID count failed", zap.Error(err))
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Offset(offset).Limit(limit).Order("created_at DESC").Find(&tasks).Error
	if err != nil {
		global.Log.Error("ReviewRepo.ListByUserID find failed", zap.Error(err))
	}
	return tasks, count, err
}

// Update 更新审阅任务
func (r *ReviewRepo) Update(ctx context.Context, task *ReviewTask) error {
	err := r.db.WithContext(ctx).Model(task).Updates(task).Error
	if err != nil {
		global.Log.Error("ReviewRepo.Update failed", zap.Error(err))
		return errors.New("update review task failed")
	}
	return nil
}

// UpdateStatus 更新任务状态
func (r *ReviewRepo) UpdateStatus(ctx context.Context, id uint64, status string) error {
	updates := map[string]interface{}{
		"status": status,
	}
	if status == "completed" {
		updates["completed_at"] = time.Now()
	}
	err := r.db.WithContext(ctx).Model(&ReviewTask{}).Where("id = ?", id).Updates(updates).Error
	if err != nil {
		global.Log.Error("ReviewRepo.UpdateStatus failed", zap.Error(err))
		return errors.New("update review task status failed")
	}
	return nil
}

// UpdateByID 按字段更新审阅任务
func (r *ReviewRepo) UpdateByID(ctx context.Context, id uint64, updates map[string]interface{}) error {
	err := r.db.WithContext(ctx).Model(&ReviewTask{}).Where("id = ?", id).Updates(updates).Error
	if err != nil {
		global.Log.Error("ReviewRepo.UpdateByID failed", zap.Error(err))
		return errors.New("update review task failed")
	}
	return nil
}

// Delete 删除审阅任务
func (r *ReviewRepo) Delete(ctx context.Context, id uint64) error {
	err := r.db.WithContext(ctx).Delete(&ReviewTask{}, id).Error
	if err != nil {
		global.Log.Error("ReviewRepo.Delete failed", zap.Error(err))
		return errors.New("delete review task failed")
	}
	return nil
}

// DeleteBySessionID 根据SessionID删除审阅任务
func (r *ReviewRepo) DeleteBySessionID(ctx context.Context, sessionID uint64) error {
	err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).Delete(&ReviewTask{}).Error
	if err != nil {
		global.Log.Error("ReviewRepo.DeleteBySessionID failed", zap.Error(err))
		return errors.New("delete review task failed")
	}
	return nil
}

// Count 统计审阅任务总数
func (r *ReviewRepo) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&ReviewTask{}).Count(&count).Error
	if err != nil {
		global.Log.Error("ReviewRepo.Count failed", zap.Error(err))
	}
	return count, err
}

// CountBySessionID 统计指定SessionID的审阅任务数量
func (r *ReviewRepo) CountBySessionID(ctx context.Context, sessionID uint64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&ReviewTask{}).Where("session_id = ?", sessionID).Count(&count).Error
	if err != nil {
		global.Log.Error("ReviewRepo.CountBySessionID failed", zap.Error(err))
	}
	return count, err
}

// CountByStatus 统计指定状态的审阅任务数量
func (r *ReviewRepo) CountByStatus(ctx context.Context, status string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&ReviewTask{}).Where("status = ?", status).Count(&count).Error
	if err != nil {
		global.Log.Error("ReviewRepo.CountByStatus failed", zap.Error(err))
	}
	return count, err
}

// ============ ReviewResultRepo 审阅结果仓储 ============

type ReviewResultRepo struct {
	db *gorm.DB
}

func NewReviewResultRepo(db *gorm.DB) *ReviewResultRepo {
	return &ReviewResultRepo{db: db}
}

// Create 创建审阅结果
func (r *ReviewResultRepo) Create(ctx context.Context, result *ReviewResult) error {
	err := r.db.WithContext(ctx).Create(result).Error
	if err != nil {
		global.Log.Error("ReviewResultRepo.Create failed", zap.Error(err))
		return errors.New("create review result failed")
	}
	return nil
}

// GetByID 根据ID获取审阅结果
func (r *ReviewResultRepo) GetByID(ctx context.Context, id uint64) (*ReviewResult, error) {
	var result ReviewResult
	err := r.db.WithContext(ctx).First(&result, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		global.Log.Error("ReviewResultRepo.GetByID failed", zap.Error(err))
		return nil, err
	}
	return &result, nil
}

// GetByTaskID 根据任务ID获取审阅结果列表（按索引排序）
func (r *ReviewResultRepo) GetByTaskID(ctx context.Context, taskID uint64) ([]ReviewResult, error) {
	var results []ReviewResult
	err := r.db.WithContext(ctx).Where("task_id = ?", taskID).Order("index ASC").Find(&results).Error
	if err != nil {
		global.Log.Error("ReviewResultRepo.GetByTaskID failed", zap.Error(err))
	}
	return results, err
}

// GetBySessionID 根据会话ID获取审阅结果列表（按索引排序）
func (r *ReviewResultRepo) GetBySessionID(ctx context.Context, sessionID uint64) ([]ReviewResult, error) {
	var results []ReviewResult
	err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).Order("index ASC").Find(&results).Error
	if err != nil {
		global.Log.Error("ReviewResultRepo.GetBySessionID failed", zap.Error(err))
	}
	return results, err
}

// ListByTaskID 根据任务ID分页获取审阅结果
func (r *ReviewResultRepo) ListByTaskID(ctx context.Context, taskID uint64, offset, limit int) ([]ReviewResult, int64, error) {
	var results []ReviewResult
	var count int64

	if err := r.db.WithContext(ctx).Model(&ReviewResult{}).Where("task_id = ?", taskID).Count(&count).Error; err != nil {
		global.Log.Error("ReviewResultRepo.ListByTaskID count failed", zap.Error(err))
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).Where("task_id = ?", taskID).Offset(offset).Limit(limit).Order("index ASC").Find(&results).Error
	if err != nil {
		global.Log.Error("ReviewResultRepo.ListByTaskID find failed", zap.Error(err))
	}
	return results, count, err
}

// Update 更新审阅结果
func (r *ReviewResultRepo) Update(ctx context.Context, result *ReviewResult) error {
	err := r.db.WithContext(ctx).Model(result).Updates(result).Error
	if err != nil {
		global.Log.Error("ReviewResultRepo.Update failed", zap.Error(err))
		return errors.New("update review result failed")
	}
	return nil
}

// Delete 删除审阅结果
func (r *ReviewResultRepo) Delete(ctx context.Context, id uint64) error {
	err := r.db.WithContext(ctx).Delete(&ReviewResult{}, id).Error
	if err != nil {
		global.Log.Error("ReviewResultRepo.Delete failed", zap.Error(err))
		return errors.New("delete review result failed")
	}
	return nil
}

// DeleteByTaskID 根据任务ID删除审阅结果
func (r *ReviewResultRepo) DeleteByTaskID(ctx context.Context, taskID uint64) error {
	err := r.db.WithContext(ctx).Where("task_id = ?", taskID).Delete(&ReviewResult{}).Error
	if err != nil {
		global.Log.Error("ReviewResultRepo.DeleteByTaskID failed", zap.Error(err))
		return errors.New("delete review results by task_id failed")
	}
	return nil
}

// DeleteBySessionID 根据会话ID删除审阅结果
func (r *ReviewResultRepo) DeleteBySessionID(ctx context.Context, sessionID uint64) error {
	err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).Delete(&ReviewResult{}).Error
	if err != nil {
		global.Log.Error("ReviewResultRepo.DeleteBySessionID failed", zap.Error(err))
		return errors.New("delete review results by session_id failed")
	}
	return nil
}

// CountByTaskID 统计指定任务的审阅结果数量
func (r *ReviewResultRepo) CountByTaskID(ctx context.Context, taskID uint64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&ReviewResult{}).Where("task_id = ?", taskID).Count(&count).Error
	if err != nil {
		global.Log.Error("ReviewResultRepo.CountByTaskID failed", zap.Error(err))
	}
	return count, err
}

// CountBySessionID 统计指定会话的审阅结果数量
func (r *ReviewResultRepo) CountBySessionID(ctx context.Context, sessionID uint64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&ReviewResult{}).Where("session_id = ?", sessionID).Count(&count).Error
	if err != nil {
		global.Log.Error("ReviewResultRepo.CountBySessionID failed", zap.Error(err))
	}
	return count, err
}

// CountByRiskLevel 统计指定任务中各风险等级的数量
func (r *ReviewResultRepo) CountByRiskLevel(ctx context.Context, taskID uint64, riskLevel string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&ReviewResult{}).Where("task_id = ? AND risk_level = ?", taskID, riskLevel).Count(&count).Error
	if err != nil {
		global.Log.Error("ReviewResultRepo.CountByRiskLevel failed", zap.Error(err))
	}
	return count, err
}
