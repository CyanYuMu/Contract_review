package qa

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

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

type qaStreamEvent struct {
	delta string
	msg   *QAMessage
	err   error
	done  bool
}

func NewQAHandler(service *QAService) *QAHandler { return &QAHandler{service: service} }

// Ask 提问（SSE 流式响应）
// POST /api/qa/ask  {session_id, message}
func (h *QAHandler) Ask(ctx context.Context, c *app.RequestContext) {
	scope, ok := middleware.GetScope(c)
	if !ok {
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

	// 模型回调只写入事件通道，所有 HTTP 写操作都由当前 handler goroutine
	// 串行完成，避免 token 回调和 heartbeat 并发写同一个响应。
	events := make(chan qaStreamEvent, 128)
	go func() {
		msg, askErr := h.service.Ask(ctx, req.SessionID, scope.UserID, scope.Account, req.Message, func(delta string) {
			select {
			case events <- qaStreamEvent{delta: delta}:
			case <-ctx.Done():
			}
		})
		select {
		case events <- qaStreamEvent{msg: msg, err: askErr, done: true}:
		case <-ctx.Done():
		}
	}()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case event := <-events:
			if !event.done {
				h.sendSSE(c, SSEEventDelta, SSEDeltaData{Content: event.delta})
				continue
			}
			if event.err != nil {
				global.Log.Error("合同问答失败", zap.Error(event.err))
				h.sendSSE(c, SSEEventError, SSEErrorData{Message: event.err.Error()})
				return
			}
			if event.msg == nil {
				h.sendSSE(c, SSEEventError, SSEErrorData{Message: "回答生成失败"})
				return
			}
			h.sendSSE(c, SSEEventEnd, SSEEndData{
				MessageID: event.msg.ID,
				Tokens:    event.msg.Tokens,
			})
			return
		case <-heartbeat.C:
			c.Write([]byte(": ping\n\n"))
			c.Flush()
		case <-ctx.Done():
			return
		}
	}
}

// GetMessages 获取会话消息历史
// GET /api/qa/messages?session_id=&limit=
func (h *QAHandler) GetMessages(ctx context.Context, c *app.RequestContext) {
	scope, ok := middleware.GetScope(c)
	if !ok {
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
	msgs, err := h.service.ListMessages(ctx, sessionID, scope.UserID, limit)
	if err != nil {
		c.JSON(400, response.FailWithMsg(err.Error()))
		return
	}
	c.JSON(200, response.OkWithData(msgs))
}

// ClearMessages 清空会话问答历史
// POST /api/qa/clear  {session_id}
func (h *QAHandler) ClearMessages(ctx context.Context, c *app.RequestContext) {
	scope, ok := middleware.GetScope(c)
	if !ok {
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
	if err := h.service.DeleteMessages(ctx, req.SessionID, scope.UserID); err != nil {
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
