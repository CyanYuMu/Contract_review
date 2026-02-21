package comparison

import (
	"context"
	"contract_review/app/internal/global"
	"contract_review/app/internal/middleware"
	"contract_review/app/pkg/response"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"go.uber.org/zap"
)

// ComparisonHandler 比对处理器
type ComparisonHandler struct {
	comparisonService *ComparisonService
}

// NewComparisonHandler 创建比对处理器
func NewComparisonHandler(comparisonService *ComparisonService) *ComparisonHandler {
	return &ComparisonHandler{
		comparisonService: comparisonService,
	}
}

// StartComparison 启动比对任务
// @Summary 启动合同比对任务
// @Description 启动两个合同的比对任务，返回差异详情
// @Accept json
// @Produce json
// @Param body body StartComparisonRequest true "请求体"
// @Success 200 {object} ComparisonTaskResponse
// @Router /api/comparison/start [post]
func (h *ComparisonHandler) StartComparison(ctx context.Context, c *app.RequestContext) {
	// 1. 验证用户登录状态
	userIDStr := middleware.GetCurrentUserID(c)
	if userIDStr == "" {
		global.Log.Error("用户未登录")
		c.JSON(401, response.Unauthorized())
		return
	}
	userID, _ := strconv.ParseUint(userIDStr, 10, 64)

	// 2. 绑定请求参数
	var req StartComparisonRequest
	if err := c.BindJSON(&req); err != nil {
		global.Log.Error("参数绑定失败", zap.Error(err))
		c.JSON(400, response.FailWithMsg("请求参数错误"))
		return
	}

	// 3. 验证参数
	if req.StandardFileID == 0 || req.ComparisonFileID == 0 {
		c.JSON(400, response.FailWithMsg("标准文档ID和比对文档ID不能为空"))
		return
	}

	// 4. 调用服务层执行比对
	result, err := h.comparisonService.StartComparison(ctx, userID, &req)
	if err != nil {
		global.Log.Error("比对任务执行失败", zap.Error(err))
		c.JSON(500, response.FailWithMsg(err.Error()))
		return
	}

	c.JSON(200, response.OkWithData(result))
}

// GetComparisonTask 获取比对任务详情
// @Summary 获取比对任务详情
// @Description 根据任务ID获取比对任务详情
// @Produce json
// @Param id path int true "任务ID"
// @Success 200 {object} ComparisonTaskResponse
// @Router /api/comparison/task/:id [get]
func (h *ComparisonHandler) GetComparisonTask(ctx context.Context, c *app.RequestContext) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(400, response.FailWithMsg("无效的任务ID"))
		return
	}

	result, err := h.comparisonService.GetComparisonResult(ctx, id)
	if err != nil {
		c.JSON(500, response.ServerError())
		return
	}
	if result == nil {
		c.JSON(404, response.NotFound())
		return
	}

	c.JSON(200, response.OkWithData(result))
}

// GetComparisonTaskBySession 根据会话ID获取比对任务
// @Summary 根据会话ID获取比对任务
// @Description 根据会话ID获取比对任务详情
// @Produce json
// @Param session_id query int true "会话ID"
// @Success 200 {object} ComparisonTaskResponse
// @Router /api/comparison/task/session [get]
func (h *ComparisonHandler) GetComparisonTaskBySession(ctx context.Context, c *app.RequestContext) {
	sessionIDStr := c.Query("session_id")
	sessionID, err := strconv.ParseUint(sessionIDStr, 10, 64)
	if err != nil {
		c.JSON(400, response.FailWithMsg("无效的会话ID"))
		return
	}

	task, err := h.comparisonService.GetComparisonTaskBySession(ctx, sessionID)
	if err != nil {
		c.JSON(500, response.ServerError())
		return
	}
	if task == nil {
		c.JSON(404, response.NotFound())
		return
	}

	result, err := h.comparisonService.GetComparisonResult(ctx, task.ID)
	if err != nil {
		c.JSON(500, response.ServerError())
		return
	}

	c.JSON(200, response.OkWithData(result))
}

// ListComparisonTasks 获取用户比对任务列表
// @Summary 获取用户比对任务列表
// @Description 分页获取当前用户的比对任务列表
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Success 200 {object} ComparisonTaskListResponse
// @Router /api/comparison/tasks [get]
func (h *ComparisonHandler) ListComparisonTasks(ctx context.Context, c *app.RequestContext) {
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

	tasks, total, err := h.comparisonService.ListUserComparisonTasks(ctx, userID, page, pageSize)
	if err != nil {
		c.JSON(500, response.ServerError())
		return
	}

	var list []ComparisonTaskItem
	for _, task := range tasks {
		list = append(list, h.taskToItem(&task))
	}

	c.JSON(200, response.OkWithData(map[string]interface{}{
		"list":      list,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}))
}

// DeleteComparisonTask 删除比对任务
// @Summary 删除比对任务
// @Description 根据任务ID删除比对任务
// @Produce json
// @Param id path int true "任务ID"
// @Success 200 {object} response.Result
// @Router /api/comparison/task/:id [delete]
func (h *ComparisonHandler) DeleteComparisonTask(ctx context.Context, c *app.RequestContext) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(400, response.FailWithMsg("无效的任务ID"))
		return
	}

	if err := h.comparisonService.DeleteComparisonTask(ctx, id); err != nil {
		c.JSON(500, response.FailWithMsg("删除任务失败"))
		return
	}

	c.JSON(200, response.Ok())
}

// ============ 辅助方法 ============

// taskToItem 将ComparisonTask转换为列表项
func (h *ComparisonHandler) taskToItem(task *ComparisonTask) ComparisonTaskItem {
	completedAt := ""
	if !task.CompletedAt.IsZero() {
		completedAt = task.CompletedAt.Format("2006-01-02 15:04:05")
	}

	return ComparisonTaskItem{
		ID:               task.ID,
		SessionID:        task.SessionID,
		StandardFileID:   task.StandardFileID,
		ComparisonFileID: task.ComparisonFileID,
		Status:           task.Status,
		Similarity:       task.Similarity,
		CreatedAt:        task.CreatedAt.Format("2006-01-02 15:04:05"),
		CompletedAt:      completedAt,
	}
}
