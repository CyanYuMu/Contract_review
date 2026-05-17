package session

import (
	"context"
	"contract_review/app/internal/contract"
	"contract_review/app/internal/global"
	"contract_review/app/internal/middleware/redis"
	"encoding/json"
	"errors"
	"fmt"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// SessionService 会话服务
type SessionService struct {
	sessionRepo  *SessionRepo
	contractRepo *contract.ContractRepo
	db           *gorm.DB
	cache        *redis.RedisClient
}

// NewSessionService 创建会话服务
func NewSessionService(
	sessionRepo *SessionRepo,
	contractRepo *contract.ContractRepo,
	db *gorm.DB,
	cache *redis.RedisClient,
) *SessionService {
	return &SessionService{
		sessionRepo:  sessionRepo,
		contractRepo: contractRepo,
		db:           db,
		cache:        cache,
	}
}

// CreateSession 创建会话
func (s *SessionService) CreateSession(ctx context.Context, account string, req *CreateSessionRequest) (*Session, error) {
	// 验证关联合同文件
	if req.FileID > 0 {
		contractFile, err := s.contractRepo.GetContractByID(ctx, req.FileID)
		if err != nil || contractFile == nil {
			return nil, fmt.Errorf("关联的合同文件不存在")
		}
	}

	// 验证会话类型
	if req.SessionType != SessionTypeReview && req.SessionType != SessionTypeCompare && req.SessionType != SessionTypeChat {
		return nil, fmt.Errorf("无效的会话类型: %s", req.SessionType)
	}

	var userRecord struct {
		ID uint `gorm:"column:id"`
	}
	if err := s.db.WithContext(ctx).Table("users").Select("id").Where("account = ?", account).First(&userRecord).Error; err != nil {
		return nil, fmt.Errorf("获取用户信息失败: %w", err)
	}

	session := &Session{
		UserID:      userRecord.ID,
		Title:       req.Title,
		SessionType: req.SessionType,
		FileID:      uint(req.FileID),
	}

	if err := s.sessionRepo.Create(ctx, session); err != nil {
		return nil, fmt.Errorf("创建会话失败: %w", err)
	}

	return session, nil
}

// GetSessionByID 根据ID获取会话
func (s *SessionService) GetSessionByID(ctx context.Context, sessionID uint64) (*Session, error) {
	return s.sessionRepo.GetByID(ctx, uint(sessionID))
}

// ListSessions 获取用户会话列表
func (s *SessionService) ListSessions(ctx context.Context, userID uint64, req *ListSessionsRequest) (interface{}, int64, error) {
	offset := (req.Page - 1) * req.PageSize

	switch req.SessionType {
	case SessionTypeReview:
		return s.listReviewSessions(ctx, userID, offset, req.PageSize)
	case SessionTypeCompare, SessionTypeCompareLegacy:
		return s.listCompareSessions(ctx, userID, offset, req.PageSize)
	default:
		return nil, 0, fmt.Errorf("无效的会话类型: %s", req.SessionType)
	}
}

// listReviewSessions 获取审阅类型会话列表
func (s *SessionService) listReviewSessions(ctx context.Context, userID uint64, offset, limit int) (*ReviewSessionListResponse, int64, error) {
	sessions, total, err := s.sessionRepo.ListReviewSessionsWithFileInfo(ctx, uint(userID), offset, limit)
	if err != nil {
		return nil, 0, err
	}

	var response []ReviewSessionResponse
	for _, sess := range sessions {
		response = append(response, ReviewSessionResponse{
			SessionID:    uint64(sess.ID),
			Title:        sess.Title,
			SessionType:  sess.SessionType,
			FileID:       uint64(sess.FileID),
			CreatedAt:    sess.CreatedAt.Format("2006-01-02 15:04:05"),
			PartyA:       sess.PartyA,
			PartyB:       sess.PartyB,
			FileName:     sess.FileName,
			FilePath:     sess.FilePath,
			IsAccepted:   sess.IsAccepted,
			ContractType: sess.ContractType,
		})
	}

	return &ReviewSessionListResponse{
		Sessions: response,
		Total:    total,
	}, total, nil
}

// listCompareSessions 获取比对类型会话列表
func (s *SessionService) listCompareSessions(ctx context.Context, userID uint64, offset, limit int) (*CompareSessionListResponse, int64, error) {
	sessions, total, err := s.sessionRepo.ListCompareSessionsWithInfo(ctx, uint(userID), offset, limit)
	if err != nil {
		return nil, 0, err
	}

	var response []CompareSessionResponse
	for _, sess := range sessions {
		response = append(response, CompareSessionResponse{
			SessionID:   uint64(sess.ID),
			Title:       sess.Title,
			SessionType: sess.SessionType,
			CreatedAt:   sess.CreatedAt.Format("2006-01-02 15:04:05"),
			FileID1:     uint64(sess.FileID1),
			PartyA1:     sess.PartyA1,
			PartyB1:     sess.PartyB1,
			FileName1:   sess.FileName1,
			FilePath1:   sess.FilePath1,
			IsAccepted1: sess.IsAccepted1,
			FileID2:     uint64(sess.FileID2),
			PartyA2:     sess.PartyA2,
			PartyB2:     sess.PartyB2,
			FileName2:   sess.FileName2,
			FilePath2:   sess.FilePath2,
			IsAccepted2: sess.IsAccepted2,
			Similarity:  sess.Similarity,
		})
	}

	return &CompareSessionListResponse{
		Sessions: response,
		Total:    total,
	}, total, nil
}

// UpdateSessionTitle 更新会话标题
func (s *SessionService) UpdateSessionTitle(ctx context.Context, userID, sessionID uint64, newTitle string) (*Session, error) {
	session, err := s.sessionRepo.GetByIDAndUserID(ctx, uint(sessionID), uint(userID))
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, errors.New("会话不存在或无权限修改")
	}

	if err := s.sessionRepo.UpdateTitle(ctx, uint(sessionID), uint(userID), newTitle); err != nil {
		return nil, err
	}

	return s.sessionRepo.GetByID(ctx, uint(sessionID))
}

// DeleteSession 删除会话
func (s *SessionService) DeleteSession(ctx context.Context, userID, sessionID uint64) error {
	session, err := s.sessionRepo.GetByIDAndUserID(ctx, uint(sessionID), uint(userID))
	if err != nil {
		return err
	}
	if session == nil {
		return errors.New("会话不存在或无权限删除")
	}

	// 根据会话类型删除关联数据
	switch session.SessionType {
	case SessionTypeReview:
		// 删除关联的审阅任务和结果
		if err := s.deleteReviewRelatedData(ctx, session.ID); err != nil {
			global.Log.Warn("删除审阅关联数据失败", zap.Error(err))
		}
	case SessionTypeCompare, SessionTypeCompareLegacy:
		if err := s.db.WithContext(ctx).Exec("DELETE FROM comparison_tasks WHERE session_id = ?", session.ID).Error; err != nil {
			global.Log.Warn("删除比对关联数据失败", zap.Error(err))
		}
	}

	return s.sessionRepo.DeleteByIDAndUserID(ctx, uint(sessionID), uint(userID))
}

// deleteReviewRelatedData 删除审阅关联数据
func (s *SessionService) deleteReviewRelatedData(ctx context.Context, sessionID uint) error {
	sid := uint64(sessionID)
	if err := s.db.WithContext(ctx).Exec("DELETE FROM review_results WHERE session_id = ?", sid).Error; err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Exec("DELETE FROM review_tasks WHERE session_id = ?", sid).Error; err != nil {
		return err
	}
	return nil
}

// GetSessionHistoryDetail 获取会话历史详情
func (s *SessionService) GetSessionHistoryDetail(ctx context.Context, sessionID uint64) (*SessionHistoryDetailResponse, error) {
	session, err := s.sessionRepo.GetByID(ctx, uint(sessionID))
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, errors.New("会话不存在")
	}

	var data interface{}

	switch session.SessionType {
	case SessionTypeReview:
		data, err = s.getReviewHistoryData(ctx, session)
	case SessionTypeCompare, SessionTypeCompareLegacy:
		data, err = s.getCompareHistoryData(ctx, session)
	default:
		data = nil
	}

	if err != nil {
		return nil, err
	}

	return &SessionHistoryDetailResponse{
		SessionID:   uint64(session.ID),
		Title:       session.Title,
		SessionType: session.SessionType,
		FileID:      uint64(session.FileID),
		CreatedAt:   session.CreatedAt.Format("2006-01-02 15:04:05"),
		Data:        data,
	}, nil
}

// getReviewHistoryData 获取审阅历史数据
func (s *SessionService) getReviewHistoryData(ctx context.Context, sess *Session) ([]map[string]interface{}, error) {
	var count int64
	if err := s.db.WithContext(ctx).Table("review_tasks").Where("session_id = ?", sess.ID).Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, errors.New("审阅任务不存在")
	}

	type reviewResultRow struct {
		ID               uint64 `gorm:"column:id"`
		SessionID        uint64 `gorm:"column:session_id"`
		TaskID           uint64 `gorm:"column:task_id"`
		Index            int    `gorm:"column:index"`
		OriginalContent  string `gorm:"column:original_content"`
		RiskAnalysis     string `gorm:"column:risk_analysis"`
		RiskLevel        string `gorm:"column:risk_level"`
		SuggestedContent string `gorm:"column:suggested_content"`
		Reason           string `gorm:"column:reason"`
		RiskType         string `gorm:"column:risk_type"`
		CreatedAt        string `gorm:"column:created_at"`
	}
	var rows []reviewResultRow
	if err := s.db.WithContext(ctx).Table("review_results").
		Where("session_id = ?", sess.ID).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}

	var data []map[string]interface{}
	for _, r := range rows {
		data = append(data, map[string]interface{}{
			"id":                r.ID,
			"session_id":        r.SessionID,
			"task_id":           r.TaskID,
			"index":             r.Index,
			"original_content":  r.OriginalContent,
			"risk_analysis":     r.RiskAnalysis,
			"risk_level":        r.RiskLevel,
			"suggested_content": r.SuggestedContent,
			"reason":            r.Reason,
			"risk_type":         r.RiskType,
			"created_at":        r.CreatedAt,
		})
	}
	return data, nil
}

// getCompareHistoryData 获取比对历史数据
func (s *SessionService) getCompareHistoryData(ctx context.Context, sess *Session) (map[string]interface{}, error) {
	type compTaskRow struct {
		ID               uint64  `gorm:"column:id"`
		StandardFileID   uint64  `gorm:"column:standard_file_id"`
		ComparisonFileID uint64  `gorm:"column:comparison_file_id"`
		Similarity       float64 `gorm:"column:similarity"`
		DiffSummary      string  `gorm:"column:diff_summary"`
		DiffResult       string  `gorm:"column:diff_result"`
	}
	var task compTaskRow
	if err := s.db.WithContext(ctx).Table("comparison_tasks").
		Where("session_id = ?", sess.ID).First(&task).Error; err != nil {
		return nil, errors.New("比对任务不存在")
	}

	stdFile, _ := s.contractRepo.GetContractByID(ctx, task.StandardFileID)
	cmpFile, _ := s.contractRepo.GetContractByID(ctx, task.ComparisonFileID)

	var summary map[string]interface{}
	var diffs []interface{}
	if task.DiffSummary != "" {
		if err := json.Unmarshal([]byte(task.DiffSummary), &summary); err != nil {
			global.Log.Warn("解析比对摘要失败", zap.Error(err))
		}
	}
	if task.DiffResult != "" {
		if err := json.Unmarshal([]byte(task.DiffResult), &diffs); err != nil {
			global.Log.Warn("解析比对结果失败", zap.Error(err))
		}
	}

	result := map[string]interface{}{
		"task_id":      task.ID,
		"session_id":   sess.ID,
		"similarity":   task.Similarity,
		"diff_summary": summary,
		"diffs":        diffs,
	}

	if stdFile != nil {
		result["standard_file"] = map[string]interface{}{
			"file_id":      stdFile.ID,
			"title":        stdFile.Title,
			"file_type":    stdFile.FileType,
			"file_path":    stdFile.FilePath,
			"download_url": fmt.Sprintf("/api/contract/download/%d", stdFile.ID),
		}
	}
	if cmpFile != nil {
		result["comparison_file"] = map[string]interface{}{
			"file_id":      cmpFile.ID,
			"title":        cmpFile.Title,
			"file_type":    cmpFile.FileType,
			"file_path":    cmpFile.FilePath,
			"download_url": fmt.Sprintf("/api/contract/download/%d", cmpFile.ID),
		}
	}

	return result, nil
}

// CountUserSessions 统计用户会话数量
func (s *SessionService) CountUserSessions(ctx context.Context, userID uint64, sessionType string) (int64, error) {
	if sessionType != "" {
		return s.sessionRepo.CountByUserIDAndType(ctx, uint(userID), sessionType)
	}
	return s.sessionRepo.CountByUserID(ctx, uint(userID))
}
