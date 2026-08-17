package session

import (
	"context"
	"contract_review/app/internal/global"
	"contract_review/app/internal/middleware"
	"contract_review/app/pkg/response"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"go.uber.org/zap"
)

// SessionHandler 会话处理器
type SessionHandler struct {
	sessionService *SessionService
}

// NewSessionHandler 创建会话处理器
func NewSessionHandler(sessionService *SessionService) *SessionHandler {
	return &SessionHandler{
		sessionService: sessionService,
	}
}

// CreateSession 创建会话
// @Summary 创建会话
// @Description 创建新的会话（支持关联合同文件或不关联）
// @Accept json
// @Produce json
// @Param body body CreateSessionRequest true "请求体"
// @Success 200 {object} SessionResponse
// @Router /api/session/create [post]
func (h *SessionHandler) CreateSession(ctx context.Context, c *app.RequestContext) {
	// 1. 验证用户登录状态
	scope, ok := middleware.GetScope(c)
	if !ok {
		global.Log.Error("用户未登录")
		c.JSON(401, response.Unauthorized())
		return
	}

	// 2. 绑定请求参数
	var req CreateSessionRequest
	if err := c.BindJSON(&req); err != nil {
		global.Log.Error("参数绑定失败", zap.Error(err))
		c.JSON(400, response.FailWithMsg("请求参数错误"))
		return
	}

	// 3. 创建会话
	session, err := h.sessionService.CreateSession(ctx, scope.Account, &req)
	if err != nil {
		global.Log.Error("创建会话失败", zap.Error(err))
		c.JSON(500, response.FailWithMsg(err.Error()))
		return
	}

	// 4. 构建响应
	resp := SessionResponse{
		SessionID:   uint64(session.ID),
		Title:       session.Title,
		SessionType: session.SessionType,
		FileID:      uint64(session.FileID),
		CreatedAt:   session.CreatedAt.Format("2006-01-02 15:04:05"),
	}

	c.JSON(200, response.OkWithData(resp))
}

// ListSessions 获取会话列表
// @Summary 获取用户会话列表
// @Description 根据会话类型获取用户会话列表
// @Accept json
// @Produce json
// @Param body body ListSessionsRequest true "请求体"
// @Success 200 {object} SessionListResponse
// @Router /api/session/list [post]
func (h *SessionHandler) ListSessions(ctx context.Context, c *app.RequestContext) {
	// 1. 验证用户登录状态
	scope, ok := middleware.GetScope(c)
	if !ok {
		c.JSON(401, response.Unauthorized())
		return
	}
	userID := scope.UserID

	// 2. 绑定请求参数
	var req ListSessionsRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, response.FailWithMsg("请求参数错误"))
		return
	}

	// 3. 验证参数
	if req.Page < 1 || req.PageSize < 1 {
		c.JSON(400, response.FailWithMsg("页码和每页数量必须大于等于1"))
		return
	}

	// 4. 获取会话列表
	data, total, err := h.sessionService.ListSessions(ctx, userID, &req)
	if err != nil {
		global.Log.Error("获取会话列表失败", zap.Error(err))
		c.JSON(500, response.FailWithMsg(err.Error()))
		return
	}

	c.JSON(200, response.OkWithData(map[string]interface{}{
		"data":      data,
		"total":     total,
		"page":      req.Page,
		"page_size": req.PageSize,
	}))
}

// UpdateSessionTitle 更新会话标题
// @Summary 更新会话标题
// @Description 修改指定会话的标题
// @Accept json
// @Produce json
// @Param body body UpdateSessionTitleRequest true "请求体"
// @Success 200 {object} SessionResponse
// @Router /api/session/title [put]
func (h *SessionHandler) UpdateSessionTitle(ctx context.Context, c *app.RequestContext) {
	// 1. 验证用户登录状态
	scope, ok := middleware.GetScope(c)
	if !ok {
		c.JSON(401, response.Unauthorized())
		return
	}
	userID := scope.UserID

	// 2. 绑定请求参数
	var req UpdateSessionTitleRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, response.FailWithMsg("请求参数错误"))
		return
	}

	// 3. 更新标题
	session, err := h.sessionService.UpdateSessionTitle(ctx, userID, req.SessionID, req.NewTitle)
	if err != nil {
		c.JSON(404, response.FailWithMsg(err.Error()))
		return
	}

	// 4. 构建响应
	resp := SessionResponse{
		SessionID:   uint64(session.ID),
		Title:       session.Title,
		SessionType: session.SessionType,
		FileID:      uint64(session.FileID),
		CreatedAt:   session.CreatedAt.Format("2006-01-02 15:04:05"),
	}

	c.JSON(200, response.OkWithData(resp))
}

// DeleteSession 删除会话
// @Summary 删除会话
// @Description 删除指定会话
// @Accept json
// @Produce json
// @Param body body DeleteSessionRequest true "请求体"
// @Success 200 {object} response.Result
// @Router /api/session/delete [post]
func (h *SessionHandler) DeleteSession(ctx context.Context, c *app.RequestContext) {
	// 1. 验证用户登录状态
	scope, ok := middleware.GetScope(c)
	if !ok {
		c.JSON(401, response.Unauthorized())
		return
	}
	userID := scope.UserID

	// 2. 绑定请求参数
	var req DeleteSessionRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, response.FailWithMsg("请求参数错误"))
		return
	}

	// 3. 删除会话
	if err := h.sessionService.DeleteSession(ctx, userID, req.SessionID); err != nil {
		c.JSON(404, response.FailWithMsg(err.Error()))
		return
	}

	c.JSON(200, response.Ok())
}

// GetSessionHistoryDetail 获取会话历史详情
// @Summary 获取会话历史详情
// @Description 根据会话ID获取历史记录详情
// @Accept json
// @Produce json
// @Param body body SessionHistoryDetailRequest true "请求体"
// @Success 200 {object} SessionHistoryDetailResponse
// @Router /api/session/history [post]
func (h *SessionHandler) GetSessionHistoryDetail(ctx context.Context, c *app.RequestContext) {
	// 1. 验证用户登录状态
	scope, ok := middleware.GetScope(c)
	if !ok {
		c.JSON(401, response.Unauthorized())
		return
	}
	userID := scope.UserID

	// 2. 绑定请求参数
	var req SessionHistoryDetailRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, response.FailWithMsg("请求参数错误"))
		return
	}

	// 3. 获取会话历史详情
	detail, err := h.sessionService.GetSessionHistoryDetail(ctx, userID, req.SessionID)
	if err != nil {
		c.JSON(404, response.FailWithMsg(err.Error()))
		return
	}

	c.JSON(200, response.OkWithData(detail))
}

// GetSessionByID 根据ID获取会话
// @Summary 根据ID获取会话
// @Description 根据会话ID获取会话基本信息
// @Produce json
// @Param id path int true "会话ID"
// @Success 200 {object} SessionResponse
// @Router /api/session/:id [get]
func (h *SessionHandler) GetSessionByID(ctx context.Context, c *app.RequestContext) {
	scope, ok := middleware.GetScope(c)
	if !ok {
		c.JSON(401, response.Unauthorized())
		return
	}
	userID := scope.UserID
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(400, response.FailWithMsg("无效的会话ID"))
		return
	}

	session, err := h.sessionService.GetSessionByID(ctx, userID, id)
	if err != nil {
		c.JSON(500, response.ServerError())
		return
	}
	if session == nil {
		c.JSON(404, response.NotFound())
		return
	}

	resp := SessionResponse{
		SessionID:   uint64(session.ID),
		Title:       session.Title,
		SessionType: session.SessionType,
		FileID:      uint64(session.FileID),
		CreatedAt:   session.CreatedAt.Format("2006-01-02 15:04:05"),
	}

	c.JSON(200, response.OkWithData(resp))
}
