package qa

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"contract_review/app/internal/global"
	"contract_review/app/internal/middleware"
	"contract_review/app/pkg/response"

	"github.com/cloudwego/hertz/pkg/app"
	"go.uber.org/zap"
)

// QAHandler 合同问答处理器
type QAHandler struct {
	service *QAService
}

func NewQAHandler(service *QAService) *QAHandler { return &QAHandler{service: service} }

// Ask 提问（SSE 流式响应）
// POST /api/qa/ask  {session_id, message}
func (h *QAHandler) Ask(ctx context.Context, c *app.RequestContext) {
	account := middleware.GetCurrentUserID(c)
	userID, err := h.service.ResolveUserID(ctx, account)
	if err != nil {
		c.JSON(401, response.Unauthorized())
		return
	}

	var req AskRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, response.FailWithMsg("请求参数错误"))
		return
	}
	if req.SessionID == 0 || req.Message == "" {
		c.JSON(400, response.FailWithMsg("session_id 与 message 不能为空"))
		return
	}

	// SSE 响应头
	c.SetContentType("text/event-stream")
	c.Response.Header.Set("Cache-Control", "no-cache")
	c.Response.Header.Set("Connection", "keep-alive")
	c.Response.Header.Set("X-Accel-Buffering", "no")

	// 流式回调：每收到一段 token 即推送 delta 事件
	onDelta := func(delta string) {
		h.sendSSE(c, SSEEventDelta, SSEDeltaData{Content: delta})
	}

	msg, err := h.service.Ask(ctx, req.SessionID, userID, account, req.Message, onDelta)
	if err != nil {
		global.Log.Error("合同问答失败", zap.Error(err))
		h.sendSSE(c, SSEEventError, SSEErrorData{Message: err.Error()})
		return
	}

	// 结束事件
	h.sendSSE(c, SSEEventEnd, SSEEndData{
		MessageID: msg.ID,
		Tokens:    msg.Tokens,
	})
}

// GetMessages 获取会话消息历史
// GET /api/qa/messages?session_id=&limit=
func (h *QAHandler) GetMessages(ctx context.Context, c *app.RequestContext) {
	account := middleware.GetCurrentUserID(c)
	userID, err := h.service.ResolveUserID(ctx, account)
	if err != nil {
		c.JSON(401, response.Unauthorized())
		return
	}
	sessionID, _ := strconv.ParseUint(string(c.Query("session_id")), 10, 64)
	if sessionID == 0 {
		c.JSON(400, response.FailWithMsg("session_id 不能为空"))
		return
	}
	limit, _ := strconv.Atoi(string(c.Query("limit")))
	if limit <= 0 {
		limit = 50
	}
	msgs, err := h.service.ListMessages(ctx, sessionID, userID, limit)
	if err != nil {
		c.JSON(400, response.FailWithMsg(err.Error()))
		return
	}
	c.JSON(200, response.OkWithData(msgs))
}

// ClearMessages 清空会话问答历史
// POST /api/qa/clear  {session_id}
func (h *QAHandler) ClearMessages(ctx context.Context, c *app.RequestContext) {
	account := middleware.GetCurrentUserID(c)
	userID, err := h.service.ResolveUserID(ctx, account)
	if err != nil {
		c.JSON(401, response.Unauthorized())
		return
	}
	var req struct {
		SessionID uint64 `json:"session_id"`
	}
	if err := c.BindJSON(&req); err != nil || req.SessionID == 0 {
		c.JSON(400, response.FailWithMsg("session_id 不能为空"))
		return
	}
	if err := h.service.DeleteMessages(ctx, req.SessionID, userID); err != nil {
		c.JSON(400, response.FailWithMsg(err.Error()))
		return
	}
	c.JSON(200, response.Ok())
}

// sendSSE 发送一条 SSE 事件
func (h *QAHandler) sendSSE(c *app.RequestContext, event string, data interface{}) {
	sseResp := SSEResponse{Event: event, Data: data}
	jsonData, _ := json.Marshal(sseResp)
	c.Write([]byte(fmt.Sprintf("data: %s\n\n", jsonData)))
	c.Flush()
}
