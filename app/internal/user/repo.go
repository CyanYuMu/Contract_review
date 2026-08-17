package user

import (
	"context"
	"contract_review/app/internal/global"
	"errors"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type UserRepo struct {
	db *gorm.DB
}

func NewUserRepo(db *gorm.DB) *UserRepo {
	return &UserRepo{db: db}
}

// ============ User 相关操作 ============

// CreateUser 创建用户
func (r *UserRepo) CreateUser(ctx context.Context, user *User) error {
	err := r.db.WithContext(ctx).Create(user).Error
	if err != nil {
		global.Log.Error("UserRepo.CreateUser failed", zap.Error(err))
		return errors.New("create user failed")
	}
	return nil
}

// EnsureSystemOwner prevents the administrative surface from becoming
// permanently locked after introducing system_role. If no owner/admin exists,
// the earliest user is promoted to owner. Repeated calls are idempotent.
func (r *UserRepo) EnsureSystemOwner(ctx context.Context) error {
	var privilegedCount int64
	if err := r.db.WithContext(ctx).
		Model(&User{}).
		Where("system_role IN ?", []string{"owner", "admin"}).
		Count(&privilegedCount).Error; err != nil {
		return err
	}
	if privilegedCount > 0 {
		return nil
	}

	var first User
	if err := r.db.WithContext(ctx).
		Select("id").
		Order("id ASC").
		First(&first).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}

	return r.db.WithContext(ctx).
		Model(&User{}).
		Where("id = ?", first.ID).
		Update("system_role", "owner").Error
}

// GetUserByID 根据ID获取用户
func (r *UserRepo) GetUserByID(ctx context.Context, id uint) (*User, error) {
	var user User
	err := r.db.WithContext(ctx).First(&user, id).Error
	if err != nil {
		global.Log.Error("UserRepo.GetUserByID failed", zap.Error(err))
		return nil, err
	}
	return &user, nil
}

// GetUserByAccount 根据账号获取用户
func (r *UserRepo) GetUserByAccount(ctx context.Context, account string) (*User, error) {
	var user User
	err := r.db.WithContext(ctx).Where("account = ?", account).First(&user).Error
	if err != nil {
		global.Log.Error("UserRepo.GetUserByAccount failed", zap.Error(err))
		return nil, err
	}
	return &user, nil
}

// GetUserByUsername 根据用户名获取用户
func (r *UserRepo) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	var user User
	err := r.db.WithContext(ctx).Where("username = ?", username).First(&user).Error
	if err != nil {
		global.Log.Error("UserRepo.GetUserByUsername failed", zap.Error(err))
		return nil, err
	}
	return &user, nil
}

// GetUsersByDepartment 根据部门获取用户列表
func (r *UserRepo) GetUsersByDepartment(ctx context.Context, department string) ([]User, error) {
	var users []User
	err := r.db.WithContext(ctx).Where("department = ?", department).Find(&users).Error
	if err != nil {
		global.Log.Error("UserRepo.GetUsersByDepartment failed", zap.Error(err))
	}
	return users, err
}

// GetUsersByRole 根据角色获取用户列表
func (r *UserRepo) GetUsersByRole(ctx context.Context, role string) ([]User, error) {
	var users []User
	err := r.db.WithContext(ctx).Where("role = ?", role).Find(&users).Error
	if err != nil {
		global.Log.Error("UserRepo.GetUsersByRole failed", zap.Error(err))
	}
	return users, err
}

// ListUsers 获取用户列表（分页）
func (r *UserRepo) ListUsers(ctx context.Context, offset, limit int) ([]User, int64, error) {
	var users []User
	var count int64

	if err := r.db.WithContext(ctx).Model(&User{}).Count(&count).Error; err != nil {
		global.Log.Error("UserRepo.ListUsers count failed", zap.Error(err))
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).Offset(offset).Limit(limit).Order("created_at DESC").Find(&users).Error
	if err != nil {
		global.Log.Error("UserRepo.ListUsers find failed", zap.Error(err))
	}
	return users, count, err
}

// ListUsersByDepartment 根据部门分页获取用户列表
func (r *UserRepo) ListUsersByDepartment(ctx context.Context, department string, offset, limit int) ([]User, int64, error) {
	var users []User
	var count int64

	if err := r.db.WithContext(ctx).Model(&User{}).Where("department = ?", department).Count(&count).Error; err != nil {
		global.Log.Error("UserRepo.ListUsersByDepartment count failed", zap.Error(err))
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).Where("department = ?", department).Offset(offset).Limit(limit).Order("created_at DESC").Find(&users).Error
	if err != nil {
		global.Log.Error("UserRepo.ListUsersByDepartment find failed", zap.Error(err))
	}
	return users, count, err
}

// UpdateUser 更新用户
func (r *UserRepo) UpdateUser(ctx context.Context, user *User) error {
	err := r.db.WithContext(ctx).Model(user).Updates(user).Error
	if err != nil {
		global.Log.Error("UserRepo.UpdateUser failed", zap.Error(err))
		return errors.New("update user failed")
	}
	return nil
}

// UpdateUserByID 按字段更新用户
func (r *UserRepo) UpdateUserByID(ctx context.Context, id uint, updates map[string]interface{}) error {
	err := r.db.WithContext(ctx).Model(&User{}).Where("id = ?", id).Updates(updates).Error
	if err != nil {
		global.Log.Error("UserRepo.UpdateUserByID failed", zap.Error(err))
		return errors.New("update user failed")
	}
	return nil
}

// UpdatePassword 更新用户密码
func (r *UserRepo) UpdatePassword(ctx context.Context, id uint, newPassword string) error {
	err := r.db.WithContext(ctx).Model(&User{}).Where("id = ?", id).Update("password", newPassword).Error
	if err != nil {
		global.Log.Error("UserRepo.UpdatePassword failed", zap.Error(err))
		return errors.New("update password failed")
	}
	return nil
}

// DeleteUser 删除用户
func (r *UserRepo) DeleteUser(ctx context.Context, id uint) error {
	err := r.db.WithContext(ctx).Delete(&User{}, id).Error
	if err != nil {
		global.Log.Error("UserRepo.DeleteUser failed", zap.Error(err))
		return errors.New("delete user failed")
	}
	return nil
}

// CountUsers 统计用户总数
func (r *UserRepo) CountUsers(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&User{}).Count(&count).Error
	if err != nil {
		global.Log.Error("UserRepo.CountUsers failed", zap.Error(err))
	}
	return count, err
}

// CountUsersByDepartment 统计指定部门的用户数量
func (r *UserRepo) CountUsersByDepartment(ctx context.Context, department string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&User{}).Where("department = ?", department).Count(&count).Error
	if err != nil {
		global.Log.Error("UserRepo.CountUsersByDepartment failed", zap.Error(err))
	}
	return count, err
}

// CountUsersByRole 统计指定角色的用户数量
func (r *UserRepo) CountUsersByRole(ctx context.Context, role string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&User{}).Where("role = ?", role).Count(&count).Error
	if err != nil {
		global.Log.Error("UserRepo.CountUsersByRole failed", zap.Error(err))
	}
	return count, err
}

// ExistsAccount 检查账号是否存在
func (r *UserRepo) ExistsAccount(ctx context.Context, account string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&User{}).Where("account = ?", account).Count(&count).Error
	if err != nil {
		global.Log.Error("UserRepo.ExistsAccount failed", zap.Error(err))
	}
	return count > 0, err
}

// ============ UserLLMConfig 相关操作 ============

// CreateUserLLMConfig 创建用户LLM配置
//func (r *UserRepo) CreateUserLLMConfig(ctx context.Context, config *UserLLMConfig) error {
//	err := r.db.WithContext(ctx).Create(config).Error
//	if err != nil {
//		global.Log.Error("UserRepo.CreateUserLLMConfig failed", zap.Error(err))
//		return errors.New("create user llm config failed")
//	}
//	return nil
//}
//
//// GetUserLLMConfigByID 根据ID获取用户LLM配置
//func (r *UserRepo) GetUserLLMConfigByID(ctx context.Context, id uint) (*UserLLMConfig, error) {
//	var config UserLLMConfig
//	err := r.db.WithContext(ctx).First(&config, id).Error
//	if err != nil {
//		global.Log.Error("UserRepo.GetUserLLMConfigByID failed", zap.Error(err))
//		return nil, err
//	}
//	return &config, nil
//}
//
//// GetUserLLMConfigByAccount 根据账号获取用户LLM配置
//func (r *UserRepo) GetUserLLMConfigByAccount(ctx context.Context, account string) (*UserLLMConfig, error) {
//	var config UserLLMConfig
//	err := r.db.WithContext(ctx).Where("account = ?", account).First(&config).Error
//	if err != nil {
//		global.Log.Error("UserRepo.GetUserLLMConfigByAccount failed", zap.Error(err))
//		return nil, err
//	}
//	return &config, nil
//}
//
//// ListUserLLMConfigs 获取用户LLM配置列表（分页）
//func (r *UserRepo) ListUserLLMConfigs(ctx context.Context, offset, limit int) ([]UserLLMConfig, int64, error) {
//	var configs []UserLLMConfig
//	var count int64
//
//	if err := r.db.WithContext(ctx).Model(&UserLLMConfig{}).Count(&count).Error; err != nil {
//		global.Log.Error("UserRepo.ListUserLLMConfigs count failed", zap.Error(err))
//		return nil, 0, err
//	}
//
//	err := r.db.WithContext(ctx).Offset(offset).Limit(limit).Find(&configs).Error
//	if err != nil {
//		global.Log.Error("UserRepo.ListUserLLMConfigs find failed", zap.Error(err))
//	}
//	return configs, count, err
//}
//
//// UpdateUserLLMConfig 更新用户LLM配置
//func (r *UserRepo) UpdateUserLLMConfig(ctx context.Context, config *UserLLMConfig) error {
//	err := r.db.WithContext(ctx).Model(config).Updates(config).Error
//	if err != nil {
//		global.Log.Error("UserRepo.UpdateUserLLMConfig failed", zap.Error(err))
//		return errors.New("update user llm config failed")
//	}
//	return nil
//}
//
//// UpdateUserLLMConfigByID 按字段更新用户LLM配置
//func (r *UserRepo) UpdateUserLLMConfigByID(ctx context.Context, id uint, updates map[string]interface{}) error {
//	err := r.db.WithContext(ctx).Model(&UserLLMConfig{}).Where("id = ?", id).Updates(updates).Error
//	if err != nil {
//		global.Log.Error("UserRepo.UpdateUserLLMConfigByID failed", zap.Error(err))
//		return errors.New("update user llm config failed")
//	}
//	return nil
//}
//
//// UpdateUserLLMConfigByAccount 按账号更新用户LLM配置
//func (r *UserRepo) UpdateUserLLMConfigByAccount(ctx context.Context, account string, updates map[string]interface{}) error {
//	err := r.db.WithContext(ctx).Model(&UserLLMConfig{}).Where("account = ?", account).Updates(updates).Error
//	if err != nil {
//		global.Log.Error("UserRepo.UpdateUserLLMConfigByAccount failed", zap.Error(err))
//		return errors.New("update user llm config failed")
//	}
//	return nil
//}
//
//// DeleteUserLLMConfig 删除用户LLM配置
//func (r *UserRepo) DeleteUserLLMConfig(ctx context.Context, id uint) error {
//	err := r.db.WithContext(ctx).Delete(&UserLLMConfig{}, id).Error
//	if err != nil {
//		global.Log.Error("UserRepo.DeleteUserLLMConfig failed", zap.Error(err))
//		return errors.New("delete user llm config failed")
//	}
//	return nil
//}
//
//// DeleteUserLLMConfigByAccount 根据账号删除用户LLM配置
//func (r *UserRepo) DeleteUserLLMConfigByAccount(ctx context.Context, account string) error {
//	err := r.db.WithContext(ctx).Where("account = ?", account).Delete(&UserLLMConfig{}).Error
//	if err != nil {
//		global.Log.Error("UserRepo.DeleteUserLLMConfigByAccount failed", zap.Error(err))
//		return errors.New("delete user llm config failed")
//	}
//	return nil
//}
//
//// CountUserLLMConfigs 统计用户LLM配置总数
//func (r *UserRepo) CountUserLLMConfigs(ctx context.Context) (int64, error) {
//	var count int64
//	err := r.db.WithContext(ctx).Model(&UserLLMConfig{}).Count(&count).Error
//	if err != nil {
//		global.Log.Error("UserRepo.CountUserLLMConfigs failed", zap.Error(err))
//	}
//	return count, err
//}
