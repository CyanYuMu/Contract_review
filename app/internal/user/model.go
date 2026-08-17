package user

import "time"

// User 用户表
type User struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`                                            // 用户ID
	Account    string    `gorm:"type:varchar(255);not null;unique" json:"account"`                              // 用户账号
	Username   string    `gorm:"type:varchar(255);not null" json:"username"`                                    // 用户名
	Password   string    `gorm:"type:varchar(255);not null" json:"password"`                                    // 密码
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`                                              // 创建时间
	Department string    `gorm:"type:varchar(255)" json:"department,omitempty"`                                 // 部门
	Role       string    `gorm:"type:varchar(64)" json:"role,omitempty"`                                        // 业务角色：本科生/研究生/老师
	SystemRole string    `gorm:"type:varchar(32);not null;default:'member';index" json:"system_role,omitempty"` // 系统权限角色：member/admin/owner
	LoginType  string    `gorm:"type:varchar(16);default:'password';comment:登录类型" json:"login_type"`            // 登录类型：password-账号密码，cas-CAS认证
}

// UserLLMConfig 用户 LLM 配置表
//type UserLLMConfig struct {
//	ID        uint   `gorm:"primaryKey;autoIncrement" json:"id"`
//	Account   string `gorm:"type:varchar(255);not null" json:"account"`
//	APIKey    string `gorm:"type:varchar(128);comment:APIkey" json:"api_key"`
//	APIURL    string `gorm:"type:varchar(255)" json:"api_url"`
//	ModelName string `gorm:"type:varchar(128);comment:模型名称" json:"model_name"`
//}
