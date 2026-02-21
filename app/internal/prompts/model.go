package prompts

import "time"

// SystemPrompt 系统级 Prompt
type SystemPrompt struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement;comment:合同审阅系统promptID" json:"id"` // 合同审阅系统promptID
	ContractTypeID uint64    `gorm:"not null;comment:合同类型ID" json:"contract_type_id"`           // 合同类型ID
	PromptName     string    `gorm:"type:varchar(255);comment:提示prompt名称" json:"prompt_name"`   // 提示prompt名称
	PromptText     string    `gorm:"type:text;comment:提示prompt文本" json:"prompt_text"`           // 提示prompt文本
	CreatedAt      time.Time `gorm:"autoCreateTime;comment:创建时间" json:"created_at"`             // 创建时间
	UpdatedAt      time.Time `gorm:"autoUpdateTime;comment:更新时间" json:"updated_at"`             // 更新时间
}

// InstitutionPrompt 机构级 Prompt
type InstitutionPrompt struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement;comment:针对机构的promptID" json:"id"` // 针对机构的promptID
	ContractTypeID uint64    `gorm:"not null;comment:合同类型ID" json:"contract_type_id"`          // 合同类型ID
	OrganizationID uint64    `gorm:"not null;comment:机构ID" json:"organization_id"`             // 机构ID
	SystemPromptID uint64    `gorm:"not null;comment:系统promptID" json:"system_prompt_id"`      // 系统promptID
	PromptName     string    `gorm:"type:varchar(255);comment:prompt名称" json:"prompt_name"`    // prompt名称
	PromptText     string    `gorm:"type:text;comment:prompt文本" json:"prompt_text"`            // prompt文本
	CreatedAt      time.Time `gorm:"autoCreateTime;comment:创建时间" json:"created_at"`            // 创建时间
	UpdatedAt      time.Time `gorm:"autoUpdateTime;comment:更新时间" json:"updated_at"`            // 更新时间
}

// PersonalPrompt 个人级 Prompt
type PersonalPrompt struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement;comment:个人promptID" json:"id"`      // 个人promptID
	UserID         uint64    `gorm:"not null;comment:绑定的用户ID" json:"user_id"`                    // 绑定的用户ID
	ContractTypeID uint64    `gorm:"not null;comment:合同类型ID" json:"contract_type_id"`            // 合同类型ID
	OverrideName   string    `gorm:"type:varchar(255);comment:个性化prompt名称" json:"override_name"` // 个性化prompt名称
	OverrideText   string    `gorm:"type:text;comment:个性化prompt文本" json:"override_text"`         // 个性化prompt文本
	CreatedAt      time.Time `gorm:"autoCreateTime;comment:创建时间" json:"created_at"`              // 创建时间
	UpdatedAt      time.Time `gorm:"autoUpdateTime;comment:更新时间" json:"updated_at"`              // 更新时间
}
