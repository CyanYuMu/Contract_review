package riskconfig

// RiskPointRequest 新增/编辑风险点请求。
type RiskPointRequest struct {
	ContractTypeID   uint64   `json:"contractTypeId"`
	ContractTypeName string   `json:"contractType"`
	ApplicableScope  string   `json:"applicableScope"`
	Departments      []string `json:"department"`
	RiskContent      string   `json:"riskContent"`
	RiskType         string   `json:"riskType"`
	RiskLevel        string   `json:"riskLevel"`
	Status           string   `json:"status"`
	IsEnabled        string   `json:"isEnabled"`
}

// RiskPointResponse 风险点列表/详情响应，兼容前端现有字段命名。
type RiskPointResponse struct {
	ID                     uint64   `json:"id"`
	Key                    string   `json:"key"`
	RiskID                 string   `json:"riskId"`
	RiskContent            string   `json:"riskContent"`
	RiskType               string   `json:"riskType"`
	RiskLevel              string   `json:"riskLevel"`
	ContractTypeID         uint64   `json:"contractTypeId"`
	ContractType           string   `json:"contractType"`
	ApplicableContractType string   `json:"applicableContractType"`
	ApplicableScope        string   `json:"applicableScope"`
	Department             []string `json:"department"`
	Creator                string   `json:"creator"`
	Status                 string   `json:"status"`
	IsEnabled              string   `json:"isEnabled"`
	KnowledgeDocID         uint64   `json:"knowledgeDocId"`
	RAGStatus              string   `json:"ragStatus"`
	UpdateDate             string   `json:"updateDate"`
	CreatedAt              string   `json:"createdAt"`
}

type RiskPointStatItem struct {
	Name  string `json:"name"`
	Value int64  `json:"value"`
}

type RiskPointStatsResponse struct {
	Total          int64               `json:"total"`
	Enabled        int64               `json:"enabled"`
	Disabled       int64               `json:"disabled"`
	Indexed        int64               `json:"indexed"`
	ByLevel        []RiskPointStatItem `json:"byLevel"`
	ByType         []RiskPointStatItem `json:"byType"`
	ByContractType []RiskPointStatItem `json:"byContractType"`
}
