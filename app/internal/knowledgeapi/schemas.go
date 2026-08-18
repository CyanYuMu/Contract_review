package knowledgeapi

// IngestDocumentRequest 长文档入库请求。
type IngestDocumentRequest struct {
	Title       string `json:"title"`       // 文档标题
	Category    string `json:"category"`    // 规范/法规/案例/示范
	SubCategory string `json:"subCategory"` // 合同类型子类（如 买卖合同/通用）
	Source      string `json:"source"`      // 来源
	Content     string `json:"content"`     // 全文
}

// IngestDocumentResponse 长文档入库响应。
type IngestDocumentResponse struct {
	DocID       uint64 `json:"docId"`
	ChunkCount  int    `json:"chunkCount"`
	ParentCount int    `json:"parentCount"`
	ChildCount  int    `json:"childCount"`
}
