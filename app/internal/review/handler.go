package review

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"contract_review/app/internal/contract"
	"contract_review/app/internal/global"
	"contract_review/app/internal/middleware"
	"contract_review/app/pkg/response"
	"contract_review/app/pkg/utils"

	"github.com/cloudwego/hertz/pkg/app"
	"go.uber.org/zap"
)

// ReviewHandler 审阅处理器
type ReviewHandler struct {
	reviewService   *ReviewService
	contractService *contract.ContractService
}

// NewReviewHandler 创建审阅处理器
func NewReviewHandler(reviewService *ReviewService, contractService *contract.ContractService) *ReviewHandler {
	return &ReviewHandler{
		reviewService:   reviewService,
		contractService: contractService,
	}
}

// StartReviewTask 启动审阅任务（SSE流式响应）
// @Summary 启动审阅任务
// @Description 启动合同审阅任务，通过SSE实时返回审阅结果
// @Accept json
// @Produce text/event-stream
// @Param body body CreateReviewTaskRequest true "请求体"
// @Success 200 {object} ReviewSSEResponse
// @Router /api/review/start [post]
func (h *ReviewHandler) StartReviewTask(ctx context.Context, c *app.RequestContext) {
	// 1. 验证用户登录状态
	userIDStr := middleware.GetCurrentUserID(c)
	if userIDStr == "" {
		global.Log.Error("用户未登录")
		c.JSON(401, response.Unauthorized())
		return
	}
	userID, _ := strconv.ParseUint(userIDStr, 10, 64)

	// 2. 绑定请求参数
	var req CreateReviewTaskRequest
	if err := c.BindJSON(&req); err != nil {
		global.Log.Error("参数绑定失败", zap.Error(err))
		c.JSON(400, response.FailWithMsg("请求参数错误"))
		return
	}

	// 3. 验证参数
	if req.SessionID == 0 {
		c.JSON(400, response.FailWithMsg("会话ID不能为空"))
		return
	}
	if req.MaxConcurrent <= 0 {
		req.MaxConcurrent = 20 // 默认并发数
	}

	// 4. 设置SSE响应头
	c.SetContentType("text/event-stream")
	c.Response.Header.Set("Cache-Control", "no-cache")
	c.Response.Header.Set("Connection", "keep-alive")
	c.Response.Header.Set("X-Accel-Buffering", "no")

	// 5. 创建审阅任务
	task, err := h.reviewService.CreateReviewTask(ctx, userID, &req)
	if err != nil {
		global.Log.Error("创建审阅任务失败", zap.Error(err))
		h.sendSSEError(c, err.Error())
		return
	}

	// 6. 获取合同内容
	contractInfo, err := h.contractService.GetContractByID(ctx, task.FileID)
	if err != nil || contractInfo == nil {
		global.Log.Error("获取合同信息失败", zap.Error(err))
		h.sendSSEError(c, "获取合同信息失败")
		return
	}

	// 检查合同状态
	if contractInfo.Status == "uploaded" {
		h.sendSSEError(c, "当前文件为上传文件，不支持审阅")
		return
	}

	// 7. 读取合同内容
	var contractContent string
	if contractInfo.FilePath != "" {
		// 从文件读取
		content, err := os.ReadFile(contractInfo.FilePath)
		if err != nil {
			// 尝试使用文档提取工具
			contractContent, err = utils.ExtractText(contractInfo.FilePath)
			if err != nil {
				global.Log.Error("读取合同内容失败", zap.Error(err))
				h.sendSSEError(c, "读取合同内容失败")
				return
			}
		} else {
			contractContent = string(content)
		}
	}

	if contractContent == "" {
		h.sendSSEError(c, "合同内容为空")
		return
	}

	// 8. 创建结果通道
	resultChan := make(chan ChunkResult, 100)
	doneChan := make(chan error, 1)

	// 9. 启动后台处理
	go func() {
		err := h.reviewService.ProcessContractReview(ctx, task, contractContent, req.MaxConcurrent, resultChan)
		doneChan <- err
		close(resultChan)
	}()

	// 10. 流式输出结果
	globalIndex := 1
	totalIssues := 0

	for result := range resultChan {
		if result.Error != nil {
			global.Log.Error("分块处理错误", zap.Int("index", result.Index), zap.Error(result.Error))
			continue
		}

		// 处理每个修改点
		for _, mod := range result.Modifications {
			// 创建审阅结果记录
			reviewResult := &ReviewResult{
				SessionID:        task.SessionID,
				TaskID:           task.ID,
				Index:            globalIndex,
				OriginalContent:  mod.OriginalContent,
				RiskAnalysis:     mod.RiskAnalysis,
				RiskLevel:        mod.RiskLevel,
				SuggestedContent: mod.SuggestedContent,
				Reason:           mod.Reason,
				RiskType:         mod.RiskType,
			}

			// 保存到数据库
			if err := h.reviewService.CreateReviewResult(ctx, reviewResult); err != nil {
				global.Log.Error("保存审阅结果失败", zap.Error(err))
			}

			// 发送SSE消息
			sseData := ReviewSSEMessageData{
				ID:               reviewResult.ID,
				SessionID:        reviewResult.SessionID,
				TaskID:           reviewResult.TaskID,
				Index:            globalIndex,
				OriginalContent:  reviewResult.OriginalContent,
				RiskAnalysis:     reviewResult.RiskAnalysis,
				RiskLevel:        reviewResult.RiskLevel,
				SuggestedContent: reviewResult.SuggestedContent,
				Reason:           reviewResult.Reason,
				RiskType:         reviewResult.RiskType,
			}

			h.sendSSEMessage(c, SSEEventMessage, sseData)
			globalIndex++
			totalIssues++
		}
	}

	// 11. 等待处理完成
	if err := <-doneChan; err != nil {
		global.Log.Error("审阅处理错误", zap.Error(err))
		// 更新任务状态为失败
		h.reviewService.UpdateTaskStatus(ctx, task.ID, "failed")
	} else {
		// 更新任务状态为完成
		h.reviewService.UpdateTaskStatus(ctx, task.ID, "completed")
	}

	// 12. 发送结束消息
	overallRisk := h.reviewService.CalculateOverallRisk(totalIssues)
	summaryData := ReviewSSESummaryData{
		Type:       "summary",
		Summary:    fmt.Sprintf("共发现 %d 个潜在风险点，整体风险等级为 %s。", totalIssues, overallRisk),
		Suggestion: fmt.Sprintf("建议重点关注高风险条款，并根据 %s 立场调整。", task.Stance),
		TotalCount: totalIssues,
		RiskLevel:  overallRisk,
	}

	h.sendSSEMessage(c, SSEEventEnd, summaryData)
}

// GetReviewTask 获取审阅任务
// @Summary 获取审阅任务
// @Description 根据会话ID获取审阅任务详情
// @Produce json
// @Param session_id query int true "会话ID"
// @Success 200 {object} ReviewTaskResponse
// @Router /api/review/task [get]
func (h *ReviewHandler) GetReviewTask(ctx context.Context, c *app.RequestContext) {
	sessionIDStr := c.Query("session_id")
	sessionID, err := strconv.ParseUint(sessionIDStr, 10, 64)
	if err != nil {
		c.JSON(400, response.FailWithMsg("无效的会话ID"))
		return
	}

	task, err := h.reviewService.GetReviewTask(ctx, sessionID)
	if err != nil {
		c.JSON(500, response.ServerError())
		return
	}
	if task == nil {
		c.JSON(404, response.NotFound())
		return
	}

	c.JSON(200, response.OkWithData(h.taskToResponse(task)))
}

// GetReviewTaskByID 根据ID获取审阅任务
// @Summary 根据ID获取审阅任务
// @Description 根据任务ID获取审阅任务详情
// @Produce json
// @Param id path int true "任务ID"
// @Success 200 {object} ReviewTaskResponse
// @Router /api/review/task/:id [get]
func (h *ReviewHandler) GetReviewTaskByID(ctx context.Context, c *app.RequestContext) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(400, response.FailWithMsg("无效的任务ID"))
		return
	}

	task, err := h.reviewService.GetReviewTaskByID(ctx, id)
	if err != nil {
		c.JSON(500, response.ServerError())
		return
	}
	if task == nil {
		c.JSON(404, response.NotFound())
		return
	}

	c.JSON(200, response.OkWithData(h.taskToResponse(task)))
}

// ListReviewTasks 获取用户审阅任务列表
// @Summary 获取用户审阅任务列表
// @Description 分页获取当前用户的审阅任务列表
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Success 200 {object} ReviewTaskListResponse
// @Router /api/review/tasks [get]
func (h *ReviewHandler) ListReviewTasks(ctx context.Context, c *app.RequestContext) {
	userIDStr := middleware.GetCurrentUserID(c)
	if userIDStr == "" {
		c.JSON(401, response.Unauthorized())
		return
	}
	userID, _ := strconv.ParseUint(userIDStr, 10, 64)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	tasks, total, err := h.reviewService.ListUserReviewTasks(ctx, userID, page, pageSize)
	if err != nil {
		c.JSON(500, response.ServerError())
		return
	}

	var list []ReviewTaskResponse
	for _, task := range tasks {
		list = append(list, h.taskToResponse(&task))
	}

	c.JSON(200, response.OkWithData(map[string]interface{}{
		"list":      list,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}))
}

// GetReviewResults 获取审阅结果列表
// @Summary 获取审阅结果列表
// @Description 根据任务ID获取审阅结果列表
// @Produce json
// @Param task_id query int true "任务ID"
// @Success 200 {object} ReviewResultListResponse
// @Router /api/review/results [get]
func (h *ReviewHandler) GetReviewResults(ctx context.Context, c *app.RequestContext) {
	taskIDStr := c.Query("task_id")
	taskID, err := strconv.ParseUint(taskIDStr, 10, 64)
	if err != nil {
		c.JSON(400, response.FailWithMsg("无效的任务ID"))
		return
	}

	results, err := h.reviewService.GetReviewResults(ctx, taskID)
	if err != nil {
		c.JSON(500, response.ServerError())
		return
	}

	var list []ReviewResultResponse
	for _, result := range results {
		list = append(list, h.resultToResponse(&result))
	}

	c.JSON(200, response.OkWithData(map[string]interface{}{
		"list":  list,
		"total": len(list),
	}))
}

// GetReviewResultsBySessionID 根据会话ID获取审阅结果
// @Summary 根据会话ID获取审阅结果
// @Description 根据会话ID获取审阅结果列表
// @Produce json
// @Param session_id query int true "会话ID"
// @Success 200 {object} ReviewResultListResponse
// @Router /api/review/results/session [get]
func (h *ReviewHandler) GetReviewResultsBySessionID(ctx context.Context, c *app.RequestContext) {
	sessionIDStr := c.Query("session_id")
	sessionID, err := strconv.ParseUint(sessionIDStr, 10, 64)
	if err != nil {
		c.JSON(400, response.FailWithMsg("无效的会话ID"))
		return
	}

	results, err := h.reviewService.GetReviewResultsBySessionID(ctx, sessionID)
	if err != nil {
		c.JSON(500, response.ServerError())
		return
	}

	var list []ReviewResultResponse
	for _, result := range results {
		list = append(list, h.resultToResponse(&result))
	}

	c.JSON(200, response.OkWithData(map[string]interface{}{
		"list":  list,
		"total": len(list),
	}))
}

// DeleteReviewTask 删除审阅任务
// @Summary 删除审阅任务
// @Description 根据任务ID删除审阅任务及其结果
// @Produce json
// @Param id path int true "任务ID"
// @Success 200 {object} response.Result
// @Router /api/review/task/:id [delete]
func (h *ReviewHandler) DeleteReviewTask(ctx context.Context, c *app.RequestContext) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(400, response.FailWithMsg("无效的任务ID"))
		return
	}

	if err := h.reviewService.DeleteReviewTask(ctx, id); err != nil {
		c.JSON(500, response.FailWithMsg("删除任务失败"))
		return
	}

	c.JSON(200, response.Ok())
}

// ============ 辅助方法 ============

// sendSSEMessage 发送SSE消息
func (h *ReviewHandler) sendSSEMessage(c *app.RequestContext, event SSEEvent, data interface{}) {
	sseResp := ReviewSSEResponse{
		Event: event,
		Data:  data,
	}
	jsonData, _ := json.Marshal(sseResp)
	c.Write([]byte(fmt.Sprintf("data: %s\n\n", jsonData)))
	c.Flush()
}

// sendSSEError 发送SSE错误消息
func (h *ReviewHandler) sendSSEError(c *app.RequestContext, message string) {
	h.sendSSEMessage(c, SSEEventError, ReviewSSEErrorData{Message: message})
}

// taskToResponse 将ReviewTask转换为响应结构
func (h *ReviewHandler) taskToResponse(task *ReviewTask) ReviewTaskResponse {
	completedAt := ""
	if !task.CompletedAt.IsZero() {
		completedAt = task.CompletedAt.Format("2006-01-02 15:04:05")
	}

	return ReviewTaskResponse{
		ID:           task.ID,
		SessionID:    task.SessionID,
		FileID:       task.FileID,
		Stance:       task.Stance,
		Intensity:    task.Intensity,
		ContractType: task.ContractType,
		Description:  task.Description,
		Status:       task.Status,
		CreatedAt:    task.CreatedAt.Format("2006-01-02 15:04:05"),
		CompletedAt:  completedAt,
	}
}

// resultToResponse 将ReviewResult转换为响应结构
func (h *ReviewHandler) resultToResponse(result *ReviewResult) ReviewResultResponse {
	return ReviewResultResponse{
		ID:               result.ID,
		SessionID:        result.SessionID,
		TaskID:           result.TaskID,
		Index:            result.Index,
		OriginalContent:  result.OriginalContent,
		RiskAnalysis:     result.RiskAnalysis,
		RiskLevel:        result.RiskLevel,
		SuggestedContent: result.SuggestedContent,
		Reason:           result.Reason,
		RiskType:         result.RiskType,
		CreatedAt:        result.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}
