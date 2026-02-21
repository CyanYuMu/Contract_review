package session

import (
	"context"
	"contract_review/app/internal/comparison"
	"contract_review/app/internal/contract"
	"contract_review/app/internal/global"
	"contract_review/app/internal/middleware/redis"
	"contract_review/app/internal/review"
	"encoding/json"
	"errors"
	"fmt"

	"go.uber.org/zap"
)

// SessionService 会话服务
type SessionService struct {
	sessionRepo      *SessionRepo
	contractRepo     *contract.ContractRepo
	reviewRepo       *review.ReviewRepo
	reviewResultRepo *review.ReviewResultRepo
	comparisonRepo   *comparison.ComparisonRepo
	cache            *redis.RedisClient
}

// NewSessionService 创建会话服务
func NewSessionService(
	sessionRepo *SessionRepo,
	contractRepo *contract.ContractRepo,
	reviewRepo *review.ReviewRepo,
	reviewResultRepo *review.ReviewResultRepo,
	comparisonRepo *comparison.ComparisonRepo,
	cache *redis.RedisClient,
) *SessionService {
	return &SessionService{
		sessionRepo:      sessionRepo,
		contractRepo:     contractRepo,
		reviewRepo:       reviewRepo,
		reviewResultRepo: reviewResultRepo,
		comparisonRepo:   comparisonRepo,
		cache:            cache,
	}
}

// CreateSession 创建会话
func (s *SessionService) CreateSession(ctx context.Context, userID uint64, req *CreateSessionRequest) (*Session, error) {
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

	session := &Session{
		UserID:      uint(userID),
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
	case SessionTypeCompare:
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
			SessionID:   uint64(sess.ID),
			Title:       sess.Title,
			SessionType: sess.SessionType,
			FileID:      uint64(sess.FileID),
			CreatedAt:   sess.CreatedAt.Format("2006-01-02 15:04:05"),
			PartyA:      sess.PartyA,
			PartyB:      sess.PartyB,
			FileName:    sess.FileName,
			FilePath:    sess.FilePath,
			IsAccepted:  sess.IsAccepted,
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
	case SessionTypeCompare:
		// 删除关联的比对任务
		if err := s.comparisonRepo.DeleteBySessionID(ctx, uint64(session.ID)); err != nil {
			global.Log.Warn("删除比对关联数据失败", zap.Error(err))
		}
	}

	return s.sessionRepo.DeleteByIDAndUserID(ctx, uint(sessionID), uint(userID))
}

// deleteReviewRelatedData 删除审阅关联数据
func (s *SessionService) deleteReviewRelatedData(ctx context.Context, sessionID uint) error {
	// 获取审阅任务
	task, err := s.reviewRepo.GetBySessionID(ctx, uint64(sessionID))
	if err != nil {
		return err
	}
	if task != nil {
		// 删除审阅结果
		if err := s.reviewResultRepo.DeleteBySessionID(ctx, uint64(sessionID)); err != nil {
			return err
		}
		// 删除审阅任务
		if err := s.reviewRepo.Delete(ctx, task.ID); err != nil {
			return err
		}
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
	case SessionTypeCompare:
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
	task, err := s.reviewRepo.GetBySessionID(ctx, uint64(sess.ID))
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, errors.New("审阅任务不存在")
	}

	results, err := s.reviewResultRepo.GetBySessionID(ctx, uint64(sess.ID))
	if err != nil {
		return nil, err
	}

	var data []map[string]interface{}
	for _, r := range results {
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
			"created_at":        r.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	return data, nil
}

// getCompareHistoryData 获取比对历史数据
func (s *SessionService) getCompareHistoryData(ctx context.Context, sess *Session) (map[string]interface{}, error) {
	task, err := s.comparisonRepo.GetBySessionID(ctx, uint64(sess.ID))
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, errors.New("比对任务不存在")
	}

	// 获取文件信息
	stdFile, _ := s.contractRepo.GetContractByID(ctx, task.StandardFileID)
	cmpFile, _ := s.contractRepo.GetContractByID(ctx, task.ComparisonFileID)

	// 解析比对结果
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

	// 添加文件信息
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
