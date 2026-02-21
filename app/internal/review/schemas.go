package review

// ============ Review Task 请求结构 ============

// CreateReviewTaskRequest 创建审阅任务请求
type CreateReviewTaskRequest struct {
	SessionID     uint64 `json:"session_id" binding:"required"`     // 会话ID
	Stance        string `json:"stance" binding:"required"`         // 审查立场（甲方/乙方）
	Intensity     string `json:"intensity" binding:"required"`      // 审查尺度（严格/标准/宽松）
	Description   string `json:"description"`                       // 审查需求描述
	ContractType  string `json:"contract_type"`                     // 合同类型
	MaxConcurrent int    `json:"max_concurrent" binding:"required"` // 最大并发数，默认20
}

// GetReviewTaskRequest 获取审阅任务请求
type GetReviewTaskRequest struct {
	SessionID uint64 `json:"session_id" binding:"required"` // 会话ID
}

// ListReviewTasksRequest 获取审阅任务列表请求
type ListReviewTasksRequest struct {
	Page     int `json:"page"`      // 页码
	PageSize int `json:"page_size"` // 每页数量
}

// ============ Review Task 响应结构 ============

// ReviewTaskResponse 审阅任务响应
type ReviewTaskResponse struct {
	ID           uint64 `json:"id"`            // 任务ID
	SessionID    uint64 `json:"session_id"`    // 会话ID
	FileID       uint64 `json:"file_id"`       // 所属文件ID
	Stance       string `json:"stance"`        // 审查立场
	Intensity    string `json:"intensity"`     // 审查尺度
	ContractType string `json:"contract_type"` // 合同类型
	Description  string `json:"description"`   // 审查需求描述
	Status       string `json:"status"`        // 状态
	CreatedAt    string `json:"created_at"`    // 创建时间
	CompletedAt  string `json:"completed_at"`  // 完成时间
}

// ReviewTaskListResponse 审阅任务列表响应
type ReviewTaskListResponse struct {
	List     []ReviewTaskResponse `json:"list"`
	Total    int64                `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
}

// ============ Review Result 响应结构 ============

// ReviewResultResponse 审阅结果响应
type ReviewResultResponse struct {
	ID               uint64 `json:"id"`                // 结果ID
	SessionID        uint64 `json:"session_id"`        // 会话ID
	TaskID           uint64 `json:"task_id"`           // 任务ID
	Index            int    `json:"index"`             // 结果索引
	OriginalContent  string `json:"original_content"`  // 原始文本
	RiskAnalysis     string `json:"risk_analysis"`     // 风险分析
	RiskLevel        string `json:"risk_level"`        // 风险等级
	SuggestedContent string `json:"suggested_content"` // 建议内容
	Reason           string `json:"reason"`            // 修改理由
	RiskType         string `json:"risk_type"`         // 风险类型
	CreatedAt        string `json:"created_at"`        // 创建时间
}

// ReviewResultListResponse 审阅结果列表响应
type ReviewResultListResponse struct {
	List  []ReviewResultResponse `json:"list"`
	Total int64                  `json:"total"`
}

// ============ SSE 响应结构 ============

// SSEEvent SSE事件类型
type SSEEvent string

const (
	SSEEventMessage SSEEvent = "message" // 消息事件
	SSEEventError   SSEEvent = "error"   // 错误事件
	SSEEventEnd     SSEEvent = "end"     // 结束事件
)

// ReviewSSEResponse 审阅任务SSE流式响应
type ReviewSSEResponse struct {
	Event SSEEvent    `json:"event"` // 事件类型: message, error, end
	Data  interface{} `json:"data"`  // 数据内容
}

// ReviewSSEMessageData SSE消息数据
type ReviewSSEMessageData struct {
	ID               uint64 `json:"id"`                // 结果ID
	SessionID        uint64 `json:"session_id"`        // 会话ID
	TaskID           uint64 `json:"task_id"`           // 任务ID
	Index            int    `json:"index"`             // 结果索引
	OriginalContent  string `json:"original_content"`  // 原始文本
	RiskAnalysis     string `json:"risk_analysis"`     // 风险分析
	RiskLevel        string `json:"risk_level"`        // 风险等级
	SuggestedContent string `json:"suggested_content"` // 建议内容
	Reason           string `json:"reason"`            // 修改理由
	RiskType         string `json:"risk_type"`         // 风险类型
}

// ReviewSSEErrorData SSE错误数据
type ReviewSSEErrorData struct {
	Message string `json:"message"` // 错误信息
}

// ReviewSSESummaryData SSE摘要数据（结束事件）
type ReviewSSESummaryData struct {
	Type       string `json:"type"`        // 类型: summary
	Summary    string `json:"summary"`     // 审阅摘要
	Suggestion string `json:"suggestion"`  // 建议
	TotalCount int    `json:"total_count"` // 总风险点数量
	RiskLevel  string `json:"risk_level"`  // 整体风险等级
}

// ============ 内部使用的结构 ============

// ModificationItem LLM解析后的修改项
type ModificationItem struct {
	Position         string `json:"position"`          // 位置
	OriginalContent  string `json:"original_content"`  // 原文内容
	RiskAnalysis     string `json:"risk_analysis"`     // 风险分析
	RiskLevel        string `json:"risk_level"`        // 风险等级
	SuggestedContent string `json:"suggested_content"` // 修改后内容
	Reason           string `json:"reason"`            // 修改理由
	RiskType         string `json:"risk_type"`         // 风险类型
}

// ChunkResult 分块处理结果
type ChunkResult struct {
	Index         int                // 分块索引
	Modifications []ModificationItem // 修改项列表
	Error         error              // 错误信息
}
