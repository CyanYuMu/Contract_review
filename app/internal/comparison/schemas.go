package comparison

// ============ 请求结构 ============

// StartComparisonRequest 启动比对任务请求
type StartComparisonRequest struct {
	StandardFileID   uint64 `json:"standard_file_id" binding:"required"`   // 标准文档ID
	ComparisonFileID uint64 `json:"comparison_file_id" binding:"required"` // 比对文档ID
	SessionID        uint64 `json:"session_id"`                            // 已有会话ID（可选）
	Title            string `json:"title"`                                 // 会话标题（仅新建时生效）
}

// ============ 响应结构 ============

// FileInfo 文件信息
type FileInfo struct {
	FileID      uint64 `json:"file_id"`      // 文件ID
	Title       string `json:"title"`        // 文件名
	FileType    string `json:"file_type"`    // 文件类型
	FilePath    string `json:"file_path"`    // 服务器存储路径
	DownloadURL string `json:"download_url"` // 下载接口
}

// ComparisonDiffDetail 字符级差异详情
type ComparisonDiffDetail struct {
	Operation string `json:"operation"`           // diff操作类型: equal, delete, insert, replace
	StdText   string `json:"std_text"`            // 标准文档片段
	CmpText   string `json:"cmp_text"`            // 比对文档片段
	StdRange  []int  `json:"std_range,omitempty"` // 标准文档内字符区间 [start, end]
	CmpRange  []int  `json:"cmp_range,omitempty"` // 比对文档内字符区间 [start, end]
}

// ComparisonParagraphDiff 段落级差异
type ComparisonParagraphDiff struct {
	Operation      string                 `json:"operation"`           // 段落级操作: delete, insert, replace
	StdIndex       *int                   `json:"std_index,omitempty"` // 标准文档段落索引
	CmpIndex       *int                   `json:"cmp_index,omitempty"` // 比对文档段落索引
	StandardText   string                 `json:"standard_text"`       // 标准文档文本
	ComparisonText string                 `json:"comparison_text"`     // 比对文档文本
	CharDiff       []ComparisonDiffDetail `json:"char_diff,omitempty"` // 字符级差异
}

// ComparisonSummary 比对摘要
type ComparisonSummary struct {
	StandardParagraphs   int     `json:"standard_paragraphs"`   // 标准文档段落数
	ComparisonParagraphs int     `json:"comparison_paragraphs"` // 比对文档段落数
	DifferenceCount      int     `json:"difference_count"`      // 差异段落总数
	Similarity           float64 `json:"similarity"`            // 相似度百分比（0-100）
}

// ComparisonTaskResponse 比对任务响应
type ComparisonTaskResponse struct {
	TaskID         uint64                    `json:"task_id"`         // 比对任务ID
	SessionID      uint64                    `json:"session_id"`      // 会话ID
	DiffSummary    ComparisonSummary         `json:"diff_summary"`    // 差异摘要
	Diffs          []ComparisonParagraphDiff `json:"diffs"`           // 差异详情
	StandardFile   FileInfo                  `json:"standard_file"`   // 标准文档信息
	ComparisonFile FileInfo                  `json:"comparison_file"` // 比对文档信息
	Similarity     float64                   `json:"similarity"`      // 相似度百分比（0-100）
}

// ComparisonTaskListResponse 比对任务列表响应
type ComparisonTaskListResponse struct {
	List     []ComparisonTaskItem `json:"list"`
	Total    int64                `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
}

// ComparisonTaskItem 比对任务列表项
type ComparisonTaskItem struct {
	ID               uint64  `json:"id"`
	SessionID        uint64  `json:"session_id"`
	StandardFileID   uint64  `json:"standard_file_id"`
	ComparisonFileID uint64  `json:"comparison_file_id"`
	Status           string  `json:"status"`
	Similarity       float64 `json:"similarity"`
	CreatedAt        string  `json:"created_at"`
	CompletedAt      string  `json:"completed_at,omitempty"`
}

// ComparisonHistoryResponse 比对历史响应
type ComparisonHistoryResponse struct {
	TaskID         uint64                    `json:"task_id"`
	SessionID      uint64                    `json:"session_id"`
	DiffSummary    ComparisonSummary         `json:"diff_summary"`
	Diffs          []ComparisonParagraphDiff `json:"diffs"`
	StandardFile   FileInfo                  `json:"standard_file"`
	ComparisonFile FileInfo                  `json:"comparison_file"`
	Similarity     float64                   `json:"similarity"`
	CreatedAt      string                    `json:"created_at"`
}

// ============ 内部使用的结构 ============

// DiffResult 比对结果（内部使用）
type DiffResult struct {
	Summary    ComparisonSummary         `json:"summary"`
	Diffs      []ComparisonParagraphDiff `json:"diffs"`
	Similarity float64                   `json:"similarity"`
}
