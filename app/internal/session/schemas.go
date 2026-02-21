package session

// ============ 请求结构 ============

// CreateSessionRequest 创建会话请求
type CreateSessionRequest struct {
	Title       string `json:"title" binding:"required"`        // 会话主题
	SessionType string `json:"session_type" binding:"required"` // 会话类型: review/compare/chat
	FileID      uint64 `json:"file_id"`                         // 关联文件ID（可选）
}

// ListSessionsRequest 获取会话列表请求
type ListSessionsRequest struct {
	Page        int    `json:"page" binding:"required"`         // 页码
	PageSize    int    `json:"page_size" binding:"required"`    // 每页数量
	SessionType string `json:"session_type" binding:"required"` // 会话类型: review/compare
}

// UpdateSessionTitleRequest 更新会话标题请求
type UpdateSessionTitleRequest struct {
	SessionID uint64 `json:"session_id" binding:"required"` // 会话ID
	NewTitle  string `json:"new_title" binding:"required"`  // 新标题
}

// DeleteSessionRequest 删除会话请求
type DeleteSessionRequest struct {
	SessionID uint64 `json:"session_id" binding:"required"` // 会话ID
}

// SessionHistoryDetailRequest 获取会话历史详情请求
type SessionHistoryDetailRequest struct {
	SessionID uint64 `json:"session_id" binding:"required"` // 会话ID
}

// ============ 响应结构 ============

// SessionResponse 会话基础响应
type SessionResponse struct {
	SessionID   uint64 `json:"session_id"`   // 会话ID
	Title       string `json:"title"`        // 会话主题
	SessionType string `json:"session_type"` // 会话类型
	FileID      uint64 `json:"file_id"`      // 关联文件ID
	CreatedAt   string `json:"created_at"`   // 创建时间
}

// ReviewSessionResponse 审阅类型会话响应
type ReviewSessionResponse struct {
	SessionID   uint64 `json:"session_id"`   // 会话ID
	Title       string `json:"title"`        // 会话主题
	SessionType string `json:"session_type"` // 会话类型
	FileID      uint64 `json:"file_id"`      // 关联文件ID
	CreatedAt   string `json:"created_at"`   // 创建时间
	PartyA      string `json:"party_a"`      // 甲方信息
	PartyB      string `json:"party_b"`      // 乙方信息
	FileName    string `json:"file_name"`    // 文件名
	FilePath    string `json:"file_path"`    // 文件URL
	IsAccepted  bool   `json:"is_accepted"`  // 是否已接受修订
}

// ReviewSessionListResponse 审阅类型会话列表响应
type ReviewSessionListResponse struct {
	Sessions []ReviewSessionResponse `json:"sessions"`
	Total    int64                   `json:"total"`
}

// CompareSessionResponse 比对类型会话响应
type CompareSessionResponse struct {
	SessionID   uint64  `json:"session_id"`    // 会话ID
	Title       string  `json:"title"`         // 会话主题
	SessionType string  `json:"session_type"`  // 会话类型
	CreatedAt   string  `json:"created_at"`    // 创建时间
	FileID1     uint64  `json:"file_id_1"`     // 合同1关联文件ID
	PartyA1     string  `json:"party_a_1"`     // 合同1甲方信息
	PartyB1     string  `json:"party_b_1"`     // 合同1乙方信息
	FileName1   string  `json:"file_name_1"`   // 文件1名
	FilePath1   string  `json:"file_path_1"`   // 文件1 URL
	IsAccepted1 bool    `json:"is_accepted_1"` // 合同1是否已接受修订
	FileID2     uint64  `json:"file_id_2"`     // 合同2关联文件ID
	PartyA2     string  `json:"party_a_2"`     // 合同2甲方信息
	PartyB2     string  `json:"party_b_2"`     // 合同2乙方信息
	FileName2   string  `json:"file_name_2"`   // 文件2名
	FilePath2   string  `json:"file_path_2"`   // 文件2 URL
	IsAccepted2 bool    `json:"is_accepted_2"` // 合同2是否已接受修订
	Similarity  float64 `json:"similarity"`    // 相似度
}

// CompareSessionListResponse 比对类型会话列表响应
type CompareSessionListResponse struct {
	Sessions []CompareSessionResponse `json:"sessions"`
	Total    int64                    `json:"total"`
}

// SessionHistoryDetailResponse 会话历史详情响应
type SessionHistoryDetailResponse struct {
	SessionID   uint64      `json:"session_id"`
	Title       string      `json:"title"`
	SessionType string      `json:"session_type"`
	FileID      uint64      `json:"file_id"`
	CreatedAt   string      `json:"created_at"`
	Data        interface{} `json:"data"` // 根据类型返回不同的历史数据
}

// SessionListResponse 会话列表响应（通用）
type SessionListResponse struct {
	List     interface{} `json:"list"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
}
