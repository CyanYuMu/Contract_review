package contract

import "time"

// Contract 合同表
type Contract struct {
	ID         uint64    `gorm:"primaryKey;comment:合同ID"`
	Account    string    `gorm:"size:64;not null;index;comment:上传者账号"`
	TypeID     uint64    `gorm:"index;comment:合同类型ID"`
	Title      string    `gorm:"size:256;not null;comment:合同标题"`
	FilePath   string    `gorm:"size:512;not null;comment:存储路径"`
	FileType   string    `gorm:"size:16;not null;comment:文件类型:pdf/docx"`
	UploadTime time.Time `gorm:"autoCreateTime;comment:上传时间"`
	Status     string    `gorm:"size:32;default:uploaded;comment:状态:uploading/uploaded"`
	PartyA     string    `gorm:"size:128;comment:甲方"`
	PartyB     string    `gorm:"size:128;comment:乙方"`
	Amount     float64   `gorm:"type:decimal(15,2);comment:合同金额"`
	IsAccepted int8      `gorm:"default:0;comment:是否已接受修订"`

	// 关联
	ContractType *ContractType `gorm:"foreignKey:TypeID;references:ID"`
}

// TableName 指定表名
func (Contract) TableName() string {
	return "contracts"
}

// ContractType 合同类型表
type ContractType struct {
	ID              uint64    `gorm:"primaryKey;comment:类型ID" json:"id"`
	Name            string    `gorm:"size:64;not null;unique;comment:类型名称" json:"name"`
	TemplateContent string    `gorm:"type:text;comment:该合同类型的审阅提示词模板" json:"template_content"`
	Creator         string    `gorm:"size:64;index;comment:创建者账号" json:"creator"`
	CreatedAt       time.Time `gorm:"autoCreateTime;comment:创建时间" json:"created_at"`
	UpdatedAt       time.Time `gorm:"autoUpdateTime;comment:更新时间" json:"updated_at"`

	// 关联的合同列表
	Contracts []Contract `gorm:"foreignKey:TypeID"`
}

// TableName 指定表名
func (ContractType) TableName() string {
	return "contract_types"
}
