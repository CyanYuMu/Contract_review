package qa

// AskRequest 提问请求
type AskRequest struct {
	SessionID uint64 `json:"session_id"`
	Message   string `json:"message"`
}

// MessageResponse 消息响应
type MessageResponse struct {
	ID        uint64 `json:"id"`
	SessionID uint64 `json:"session_id"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	Tokens    int    `json:"tokens"`
	CreatedAt string `json:"created_at"`
}

// SSE 事件类型
const (
	SSEEventDelta = "delta" // 增量 token
	SSEEventError = "error"
	SSEEventEnd   = "end"
)

// SSEResponse SSE 统一结构
type SSEResponse struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
}

// SSEDeltaData 增量内容
type SSEDeltaData struct {
	Content string `json:"content"`
}

// SSEEndData 结束信息
type SSEEndData struct {
	MessageID uint64 `json:"message_id"`
	Tokens    int    `json:"tokens"`
	CacheHit  bool   `json:"cache_hit"`
}

// SSEErrorData 错误信息
type SSEErrorData struct {
	Message string `json:"message"`
}
