package comparison

import (
	"context"
	"contract_review/app/internal/contract"
	"contract_review/app/internal/global"
	"contract_review/app/internal/middleware/redis"
	"contract_review/app/internal/session"
	"contract_review/app/pkg/utils"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
)

// ComparisonService 比对服务
type ComparisonService struct {
	comparisonRepo *ComparisonRepo
	contractRepo   *contract.ContractRepo
	sessionRepo    *session.SessionRepo
	cache          *redis.RedisClient
}

// NewComparisonService 创建比对服务
func NewComparisonService(
	comparisonRepo *ComparisonRepo,
	contractRepo *contract.ContractRepo,
	sessionRepo *session.SessionRepo,
	cache *redis.RedisClient,
) *ComparisonService {
	return &ComparisonService{
		comparisonRepo: comparisonRepo,
		contractRepo:   contractRepo,
		sessionRepo:    sessionRepo,
		cache:          cache,
	}
}

// StartComparison 启动比对任务
func (s *ComparisonService) StartComparison(ctx context.Context, userID uint64, req *StartComparisonRequest) (*ComparisonTaskResponse, error) {
	// 1. 获取标准文档和比对文档
	stdFile, err := s.contractRepo.GetContractByID(ctx, req.StandardFileID)
	if err != nil || stdFile == nil {
		return nil, fmt.Errorf("标准文档不存在")
	}

	cmpFile, err := s.contractRepo.GetContractByID(ctx, req.ComparisonFileID)
	if err != nil || cmpFile == nil {
		return nil, fmt.Errorf("比对文档不存在")
	}

	// 2. 验证文件类型
	if !strings.EqualFold(stdFile.FileType, "docx") || !strings.EqualFold(cmpFile.FileType, "docx") {
		return nil, fmt.Errorf("目前仅支持 docx 文件比对")
	}

	// 3. 验证文件是否存在
	if _, err := os.Stat(stdFile.FilePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("标准文档文件不存在: %s", stdFile.FilePath)
	}
	if _, err := os.Stat(cmpFile.FilePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("比对文档文件不存在: %s", cmpFile.FilePath)
	}

	// 4. 确保或创建会话
	sessionID, err := s.ensureComparisonSession(ctx, userID, req.SessionID, req.Title, stdFile.Title, cmpFile.Title)
	if err != nil {
		return nil, err
	}

	// 5. 创建或更新比对任务
	task, err := s.comparisonRepo.GetBySessionID(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("获取比对任务失败: %w", err)
	}

	if task != nil {
		// 更新现有任务
		if err := s.comparisonRepo.UpdateTaskFiles(ctx, task, req.StandardFileID, req.ComparisonFileID); err != nil {
			return nil, fmt.Errorf("更新比对任务失败: %w", err)
		}
	} else {
		// 创建新任务
		task = &ComparisonTask{
			SessionID:        sessionID,
			UserID:           userID,
			StandardFileID:   req.StandardFileID,
			ComparisonFileID: req.ComparisonFileID,
			Status:           "pending",
		}
		if err := s.comparisonRepo.Create(ctx, task); err != nil {
			return nil, fmt.Errorf("创建比对任务失败: %w", err)
		}
	}

	// 6. 执行文档比对
	diffResult, err := s.compareDocuments(stdFile.FilePath, cmpFile.FilePath)
	if err != nil {
		s.comparisonRepo.UpdateStatus(ctx, task.ID, "failed")
		return nil, fmt.Errorf("文档比对失败: %w", err)
	}

	// 7. 保存比对结果
	summaryJSON, _ := json.Marshal(diffResult.Summary)
	diffsJSON, _ := json.Marshal(diffResult.Diffs)

	if err := s.comparisonRepo.SaveDiffResult(ctx, task, string(summaryJSON), string(diffsJSON), diffResult.Similarity); err != nil {
		return nil, fmt.Errorf("保存比对结果失败: %w", err)
	}

	// 8. 构建响应
	response := &ComparisonTaskResponse{
		TaskID:         task.ID,
		SessionID:      sessionID,
		DiffSummary:    diffResult.Summary,
		Diffs:          diffResult.Diffs,
		Similarity:     diffResult.Similarity,
		StandardFile:   s.buildFileInfo(stdFile),
		ComparisonFile: s.buildFileInfo(cmpFile),
	}

	return response, nil
}

// ensureComparisonSession 确保或创建比对会话
func (s *ComparisonService) ensureComparisonSession(ctx context.Context, userID, sessionID uint64, title, stdTitle, cmpTitle string) (uint64, error) {
	if sessionID > 0 {
		// 验证现有会话
		sess, err := s.sessionRepo.GetByID(ctx, uint(sessionID))
		if err != nil {
			return 0, fmt.Errorf("获取会话失败: %w", err)
		}
		if sess == nil {
			return 0, fmt.Errorf("会话不存在")
		}
		if sess.UserID != uint(userID) {
			return 0, fmt.Errorf("无权限访问此会话")
		}
		if sess.SessionType != "comparison" {
			return 0, fmt.Errorf("会话类型必须为comparison")
		}
		return uint64(sess.ID), nil
	}

	// 创建新会话
	sessionTitle := title
	if sessionTitle == "" {
		sessionTitle = fmt.Sprintf("%s VS %s", stdTitle, cmpTitle)
	}

	sess := &session.Session{
		UserID:      uint(userID),
		Title:       sessionTitle,
		SessionType: "comparison",
	}

	if err := s.sessionRepo.Create(ctx, sess); err != nil {
		return 0, fmt.Errorf("创建会话失败: %w", err)
	}

	return uint64(sess.ID), nil
}

// compareDocuments 比对两个文档
func (s *ComparisonService) compareDocuments(stdPath, cmpPath string) (*DiffResult, error) {
	// 读取标准文档段落
	stdLines, err := utils.ExtractParagraphsFromDOCX(stdPath)
	if err != nil {
		global.Log.Error("读取标准文档失败", zap.Error(err))
		return nil, fmt.Errorf("读取标准文档失败: %w", err)
	}

	// 读取比对文档段落
	cmpLines, err := utils.ExtractParagraphsFromDOCX(cmpPath)
	if err != nil {
		global.Log.Error("读取比对文档失败", zap.Error(err))
		return nil, fmt.Errorf("读取比对文档失败: %w", err)
	}

	global.Log.Info("文档段落提取完成",
		zap.Int("stdParagraphs", len(stdLines)),
		zap.Int("cmpParagraphs", len(cmpLines)))

	// 执行比对
	diffResult := utils.DiffDocuments(stdLines, cmpLines)

	return &diffResult, nil
}

// buildFileInfo 构建文件信息
func (s *ComparisonService) buildFileInfo(c *contract.Contract) FileInfo {
	return FileInfo{
		FileID:      c.ID,
		Title:       c.Title,
		FileType:    c.FileType,
		FilePath:    c.FilePath,
		DownloadURL: fmt.Sprintf("/api/contract/download/%d", c.ID),
	}
}

// GetComparisonTask 获取比对任务
func (s *ComparisonService) GetComparisonTask(ctx context.Context, taskID uint64) (*ComparisonTask, error) {
	return s.comparisonRepo.GetByID(ctx, taskID)
}

// GetComparisonTaskBySession 根据会话ID获取比对任务
func (s *ComparisonService) GetComparisonTaskBySession(ctx context.Context, sessionID uint64) (*ComparisonTask, error) {
	return s.comparisonRepo.GetBySessionID(ctx, sessionID)
}

// ListUserComparisonTasks 获取用户比对任务列表
func (s *ComparisonService) ListUserComparisonTasks(ctx context.Context, userID uint64, page, pageSize int) ([]ComparisonTask, int64, error) {
	offset := (page - 1) * pageSize
	return s.comparisonRepo.ListByUserID(ctx, userID, offset, pageSize)
}

// GetComparisonResult 获取比对结果详情
func (s *ComparisonService) GetComparisonResult(ctx context.Context, taskID uint64) (*ComparisonTaskResponse, error) {
	task, err := s.comparisonRepo.GetByID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, errors.New("比对任务不存在")
	}

	// 解析保存的结果
	var summary ComparisonSummary
	var diffs []ComparisonParagraphDiff

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

	// 获取文件信息
	stdFile, _ := s.contractRepo.GetContractByID(ctx, task.StandardFileID)
	cmpFile, _ := s.contractRepo.GetContractByID(ctx, task.ComparisonFileID)

	var stdFileInfo, cmpFileInfo FileInfo
	if stdFile != nil {
		stdFileInfo = s.buildFileInfo(stdFile)
	}
	if cmpFile != nil {
		cmpFileInfo = s.buildFileInfo(cmpFile)
	}

	return &ComparisonTaskResponse{
		TaskID:         task.ID,
		SessionID:      task.SessionID,
		DiffSummary:    summary,
		Diffs:          diffs,
		Similarity:     task.Similarity,
		StandardFile:   stdFileInfo,
		ComparisonFile: cmpFileInfo,
	}, nil
}

// DeleteComparisonTask 删除比对任务
func (s *ComparisonService) DeleteComparisonTask(ctx context.Context, taskID uint64) error {
	return s.comparisonRepo.Delete(ctx, taskID)
}
