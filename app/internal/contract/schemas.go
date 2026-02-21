package contract

// ============ Contract 请求结构 ============

// UpdateContractTypeRequest 更新合同类型ID请求
type UpdateContractTypeRequest struct {
	TypeID uint64 `json:"type_id"`
}

// ============ Contract 响应结构 ============

// UploadContractResponse 上传合同响应
type UploadContractResponse struct {
	FileID         uint64  `json:"file_id"`
	Title          string  `json:"title"`
	FilePathURL    string  `json:"file_path_url"`
	FileType       string  `json:"file_type"`
	ContractTypeID uint64  `json:"contract_type_id"`
	PartyA         string  `json:"party_a"` // 甲方
	PartyB         string  `json:"party_b"` // 乙方
	Amount         float64 `json:"amount"`  // 合同金额
}

// ContractListResponse 合同列表响应
type ContractListResponse struct {
	List     []ContractItem `json:"list"`
	Total    int64          `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
}

// ContractItem 合同列表项
type ContractItem struct {
	ID         uint64  `json:"id"`
	Title      string  `json:"title"`
	FileType   string  `json:"file_type"`
	PartyA     string  `json:"party_a"`
	PartyB     string  `json:"party_b"`
	Amount     float64 `json:"amount"`
	Status     string  `json:"status"`
	TypeID     uint64  `json:"type_id"`
	TypeName   string  `json:"type_name"`
	UploadTime string  `json:"upload_time"`
}

// ContractDetailResponse 合同详情响应
type ContractDetailResponse struct {
	ID         uint64  `json:"id"`
	Account    string  `json:"account"`
	TypeID     uint64  `json:"type_id"`
	TypeName   string  `json:"type_name"`
	Title      string  `json:"title"`
	FilePath   string  `json:"file_path"`
	FileType   string  `json:"file_type"`
	UploadTime string  `json:"upload_time"`
	Status     string  `json:"status"`
	PartyA     string  `json:"party_a"`
	PartyB     string  `json:"party_b"`
	Amount     float64 `json:"amount"`
	IsAccepted int8    `json:"is_accepted"`
}

// ============ ContractType 请求结构 ============

// CreateContractTypeRequest 创建合同类型请求
type CreateContractTypeRequest struct {
	Name string `json:"name"`
}

// UpdateContractTypeNameRequest 更新合同类型名称请求
type UpdateContractTypeNameRequest struct {
	Name string `json:"name"`
}

// ============ ContractType 响应结构 ============

// ContractTypeResponse 合同类型响应
type ContractTypeResponse struct {
	ID        uint64 `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// ContractTypeDetailResponse 合同类型详情响应（包含使用数量）
type ContractTypeDetailResponse struct {
	ID            uint64 `json:"id"`
	Name          string `json:"name"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
	ContractCount int64  `json:"contract_count"` // 该类型下的合同数量
}

// ContractTypeListResponse 合同类型列表响应
type ContractTypeListResponse struct {
	List     []ContractTypeResponse `json:"list"`
	Total    int64                  `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
}
