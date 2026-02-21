package prompts

import (
	"context"
	"contract_review/app/internal/global"
	"errors"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type PromptRepo struct {
	db *gorm.DB
}

func NewPromptRepo(db *gorm.DB) *PromptRepo {
	return &PromptRepo{db: db}
}

// ============ SystemPrompt 相关操作 ============

// CreateSystemPrompt 创建系统级Prompt
func (r *PromptRepo) CreateSystemPrompt(ctx context.Context, prompt *SystemPrompt) error {
	err := r.db.WithContext(ctx).Create(prompt).Error
	if err != nil {
		global.Log.Error("PromptRepo.CreateSystemPrompt failed", zap.Error(err))
		return errors.New("create system prompt failed")
	}
	return nil
}

// GetSystemPromptByID 根据ID获取系统级Prompt
func (r *PromptRepo) GetSystemPromptByID(ctx context.Context, id uint) (*SystemPrompt, error) {
	var prompt SystemPrompt
	err := r.db.WithContext(ctx).First(&prompt, id).Error
	if err != nil {
		global.Log.Error("PromptRepo.GetSystemPromptByID failed", zap.Error(err))
		return nil, err
	}
	return &prompt, nil
}

// GetSystemPromptByContractType 根据合同类型ID获取系统级Prompt列表
func (r *PromptRepo) GetSystemPromptByContractType(ctx context.Context, contractTypeID uint) ([]SystemPrompt, error) {
	var prompts []SystemPrompt
	err := r.db.WithContext(ctx).Where("contract_type_id = ?", contractTypeID).Find(&prompts).Error
	if err != nil {
		global.Log.Error("PromptRepo.GetSystemPromptByContractType failed", zap.Error(err))
	}
	return prompts, err
}

// GetSystemPromptByContractTypeAndName 根据合同类型和名称获取系统级Prompt
func (r *PromptRepo) GetSystemPromptByContractTypeAndName(ctx context.Context, contractTypeID uint, promptName string) (*SystemPrompt, error) {
	var prompt SystemPrompt
	err := r.db.WithContext(ctx).Where("contract_type_id = ? AND prompt_name = ?", contractTypeID, promptName).First(&prompt).Error
	if err != nil {
		global.Log.Error("PromptRepo.GetSystemPromptByContractTypeAndName failed", zap.Error(err))
		return nil, err
	}
	return &prompt, nil
}

// ListSystemPrompts 获取系统级Prompt列表（分页）
func (r *PromptRepo) ListSystemPrompts(ctx context.Context, offset, limit int) ([]SystemPrompt, int64, error) {
	var prompts []SystemPrompt
	var count int64

	if err := r.db.WithContext(ctx).Model(&SystemPrompt{}).Count(&count).Error; err != nil {
		global.Log.Error("PromptRepo.ListSystemPrompts count failed", zap.Error(err))
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).Offset(offset).Limit(limit).Order("created_at DESC").Find(&prompts).Error
	if err != nil {
		global.Log.Error("PromptRepo.ListSystemPrompts find failed", zap.Error(err))
	}
	return prompts, count, err
}

// ListSystemPromptsByContractType 根据合同类型分页获取系统级Prompt
func (r *PromptRepo) ListSystemPromptsByContractType(ctx context.Context, contractTypeID uint, offset, limit int) ([]SystemPrompt, int64, error) {
	var prompts []SystemPrompt
	var count int64

	if err := r.db.WithContext(ctx).Model(&SystemPrompt{}).Where("contract_type_id = ?", contractTypeID).Count(&count).Error; err != nil {
		global.Log.Error("PromptRepo.ListSystemPromptsByContractType count failed", zap.Error(err))
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).Where("contract_type_id = ?", contractTypeID).Offset(offset).Limit(limit).Order("created_at DESC").Find(&prompts).Error
	if err != nil {
		global.Log.Error("PromptRepo.ListSystemPromptsByContractType find failed", zap.Error(err))
	}
	return prompts, count, err
}

// UpdateSystemPrompt 更新系统级Prompt
func (r *PromptRepo) UpdateSystemPrompt(ctx context.Context, prompt *SystemPrompt) error {
	err := r.db.WithContext(ctx).Model(prompt).Updates(prompt).Error
	if err != nil {
		global.Log.Error("PromptRepo.UpdateSystemPrompt failed", zap.Error(err))
		return errors.New("update system prompt failed")
	}
	return nil
}

// UpdateSystemPromptByID 按字段更新系统级Prompt
func (r *PromptRepo) UpdateSystemPromptByID(ctx context.Context, id uint, updates map[string]interface{}) error {
	err := r.db.WithContext(ctx).Model(&SystemPrompt{}).Where("id = ?", id).Updates(updates).Error
	if err != nil {
		global.Log.Error("PromptRepo.UpdateSystemPromptByID failed", zap.Error(err))
		return errors.New("update system prompt failed")
	}
	return nil
}

// DeleteSystemPrompt 删除系统级Prompt
func (r *PromptRepo) DeleteSystemPrompt(ctx context.Context, id uint) error {
	err := r.db.WithContext(ctx).Delete(&SystemPrompt{}, id).Error
	if err != nil {
		global.Log.Error("PromptRepo.DeleteSystemPrompt failed", zap.Error(err))
		return errors.New("delete system prompt failed")
	}
	return nil
}

// DeleteSystemPromptByContractType 根据合同类型删除系统级Prompt
func (r *PromptRepo) DeleteSystemPromptByContractType(ctx context.Context, contractTypeID uint) error {
	err := r.db.WithContext(ctx).Where("contract_type_id = ?", contractTypeID).Delete(&SystemPrompt{}).Error
	if err != nil {
		global.Log.Error("PromptRepo.DeleteSystemPromptByContractType failed", zap.Error(err))
		return errors.New("delete system prompt failed")
	}
	return nil
}

// CountSystemPrompts 统计系统级Prompt总数
func (r *PromptRepo) CountSystemPrompts(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&SystemPrompt{}).Count(&count).Error
	if err != nil {
		global.Log.Error("PromptRepo.CountSystemPrompts failed", zap.Error(err))
	}
	return count, err
}

// ============ InstitutionPrompt 相关操作 ============

// CreateInstitutionPrompt 创建机构级Prompt
func (r *PromptRepo) CreateInstitutionPrompt(ctx context.Context, prompt *InstitutionPrompt) error {
	err := r.db.WithContext(ctx).Create(prompt).Error
	if err != nil {
		global.Log.Error("PromptRepo.CreateInstitutionPrompt failed", zap.Error(err))
		return errors.New("create institution prompt failed")
	}
	return nil
}

// GetInstitutionPromptByID 根据ID获取机构级Prompt
func (r *PromptRepo) GetInstitutionPromptByID(ctx context.Context, id uint) (*InstitutionPrompt, error) {
	var prompt InstitutionPrompt
	err := r.db.WithContext(ctx).First(&prompt, id).Error
	if err != nil {
		global.Log.Error("PromptRepo.GetInstitutionPromptByID failed", zap.Error(err))
		return nil, err
	}
	return &prompt, nil
}

// GetInstitutionPromptByOrganization 根据机构ID获取机构级Prompt列表
func (r *PromptRepo) GetInstitutionPromptByOrganization(ctx context.Context, organizationID uint) ([]InstitutionPrompt, error) {
	var prompts []InstitutionPrompt
	err := r.db.WithContext(ctx).Where("organization_id = ?", organizationID).Find(&prompts).Error
	if err != nil {
		global.Log.Error("PromptRepo.GetInstitutionPromptByOrganization failed", zap.Error(err))
	}
	return prompts, err
}

// GetInstitutionPromptByOrganizationAndContractType 根据机构ID和合同类型获取机构级Prompt列表
func (r *PromptRepo) GetInstitutionPromptByOrganizationAndContractType(ctx context.Context, organizationID, contractTypeID uint) ([]InstitutionPrompt, error) {
	var prompts []InstitutionPrompt
	err := r.db.WithContext(ctx).Where("organization_id = ? AND contract_type_id = ?", organizationID, contractTypeID).Find(&prompts).Error
	if err != nil {
		global.Log.Error("PromptRepo.GetInstitutionPromptByOrganizationAndContractType failed", zap.Error(err))
	}
	return prompts, err
}

// ListInstitutionPrompts 获取机构级Prompt列表（分页）
func (r *PromptRepo) ListInstitutionPrompts(ctx context.Context, offset, limit int) ([]InstitutionPrompt, int64, error) {
	var prompts []InstitutionPrompt
	var count int64

	if err := r.db.WithContext(ctx).Model(&InstitutionPrompt{}).Count(&count).Error; err != nil {
		global.Log.Error("PromptRepo.ListInstitutionPrompts count failed", zap.Error(err))
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).Offset(offset).Limit(limit).Order("created_at DESC").Find(&prompts).Error
	if err != nil {
		global.Log.Error("PromptRepo.ListInstitutionPrompts find failed", zap.Error(err))
	}
	return prompts, count, err
}

// ListInstitutionPromptsByOrganization 根据机构ID分页获取机构级Prompt
func (r *PromptRepo) ListInstitutionPromptsByOrganization(ctx context.Context, organizationID uint, offset, limit int) ([]InstitutionPrompt, int64, error) {
	var prompts []InstitutionPrompt
	var count int64

	if err := r.db.WithContext(ctx).Model(&InstitutionPrompt{}).Where("organization_id = ?", organizationID).Count(&count).Error; err != nil {
		global.Log.Error("PromptRepo.ListInstitutionPromptsByOrganization count failed", zap.Error(err))
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).Where("organization_id = ?", organizationID).Offset(offset).Limit(limit).Order("created_at DESC").Find(&prompts).Error
	if err != nil {
		global.Log.Error("PromptRepo.ListInstitutionPromptsByOrganization find failed", zap.Error(err))
	}
	return prompts, count, err
}

// UpdateInstitutionPrompt 更新机构级Prompt
func (r *PromptRepo) UpdateInstitutionPrompt(ctx context.Context, prompt *InstitutionPrompt) error {
	err := r.db.WithContext(ctx).Model(prompt).Updates(prompt).Error
	if err != nil {
		global.Log.Error("PromptRepo.UpdateInstitutionPrompt failed", zap.Error(err))
		return errors.New("update institution prompt failed")
	}
	return nil
}

// UpdateInstitutionPromptByID 按字段更新机构级Prompt
func (r *PromptRepo) UpdateInstitutionPromptByID(ctx context.Context, id uint, updates map[string]interface{}) error {
	err := r.db.WithContext(ctx).Model(&InstitutionPrompt{}).Where("id = ?", id).Updates(updates).Error
	if err != nil {
		global.Log.Error("PromptRepo.UpdateInstitutionPromptByID failed", zap.Error(err))
		return errors.New("update institution prompt failed")
	}
	return nil
}

// DeleteInstitutionPrompt 删除机构级Prompt
func (r *PromptRepo) DeleteInstitutionPrompt(ctx context.Context, id uint) error {
	err := r.db.WithContext(ctx).Delete(&InstitutionPrompt{}, id).Error
	if err != nil {
		global.Log.Error("PromptRepo.DeleteInstitutionPrompt failed", zap.Error(err))
		return errors.New("delete institution prompt failed")
	}
	return nil
}

// DeleteInstitutionPromptByOrganization 根据机构ID删除机构级Prompt
func (r *PromptRepo) DeleteInstitutionPromptByOrganization(ctx context.Context, organizationID uint) error {
	err := r.db.WithContext(ctx).Where("organization_id = ?", organizationID).Delete(&InstitutionPrompt{}).Error
	if err != nil {
		global.Log.Error("PromptRepo.DeleteInstitutionPromptByOrganization failed", zap.Error(err))
		return errors.New("delete institution prompt failed")
	}
	return nil
}

// CountInstitutionPrompts 统计机构级Prompt总数
func (r *PromptRepo) CountInstitutionPrompts(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&InstitutionPrompt{}).Count(&count).Error
	if err != nil {
		global.Log.Error("PromptRepo.CountInstitutionPrompts failed", zap.Error(err))
	}
	return count, err
}

// ============ PersonalPrompt 相关操作 ============

// CreatePersonalPrompt 创建个人级Prompt
func (r *PromptRepo) CreatePersonalPrompt(ctx context.Context, prompt *PersonalPrompt) error {
	err := r.db.WithContext(ctx).Create(prompt).Error
	if err != nil {
		global.Log.Error("PromptRepo.CreatePersonalPrompt failed", zap.Error(err))
		return errors.New("create personal prompt failed")
	}
	return nil
}

// GetPersonalPromptByID 根据ID获取个人级Prompt
func (r *PromptRepo) GetPersonalPromptByID(ctx context.Context, id uint) (*PersonalPrompt, error) {
	var prompt PersonalPrompt
	err := r.db.WithContext(ctx).First(&prompt, id).Error
	if err != nil {
		global.Log.Error("PromptRepo.GetPersonalPromptByID failed", zap.Error(err))
		return nil, err
	}
	return &prompt, nil
}

// GetPersonalPromptByUser 根据用户ID获取个人级Prompt列表
func (r *PromptRepo) GetPersonalPromptByUser(ctx context.Context, userID uint) ([]PersonalPrompt, error) {
	var prompts []PersonalPrompt
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&prompts).Error
	if err != nil {
		global.Log.Error("PromptRepo.GetPersonalPromptByUser failed", zap.Error(err))
	}
	return prompts, err
}

// GetPersonalPromptByUserAndContractType 根据用户ID和合同类型获取个人级Prompt
func (r *PromptRepo) GetPersonalPromptByUserAndContractType(ctx context.Context, userID, contractTypeID uint) (*PersonalPrompt, error) {
	var prompt PersonalPrompt
	err := r.db.WithContext(ctx).Where("user_id = ? AND contract_type_id = ?", userID, contractTypeID).First(&prompt).Error
	if err != nil {
		global.Log.Error("PromptRepo.GetPersonalPromptByUserAndContractType failed", zap.Error(err))
		return nil, err
	}
	return &prompt, nil
}

// ListPersonalPrompts 获取个人级Prompt列表（分页）
func (r *PromptRepo) ListPersonalPrompts(ctx context.Context, offset, limit int) ([]PersonalPrompt, int64, error) {
	var prompts []PersonalPrompt
	var count int64

	if err := r.db.WithContext(ctx).Model(&PersonalPrompt{}).Count(&count).Error; err != nil {
		global.Log.Error("PromptRepo.ListPersonalPrompts count failed", zap.Error(err))
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).Offset(offset).Limit(limit).Order("created_at DESC").Find(&prompts).Error
	if err != nil {
		global.Log.Error("PromptRepo.ListPersonalPrompts find failed", zap.Error(err))
	}
	return prompts, count, err
}

// ListPersonalPromptsByUser 根据用户ID分页获取个人级Prompt
func (r *PromptRepo) ListPersonalPromptsByUser(ctx context.Context, userID uint, offset, limit int) ([]PersonalPrompt, int64, error) {
	var prompts []PersonalPrompt
	var count int64

	if err := r.db.WithContext(ctx).Model(&PersonalPrompt{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		global.Log.Error("PromptRepo.ListPersonalPromptsByUser count failed", zap.Error(err))
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Offset(offset).Limit(limit).Order("created_at DESC").Find(&prompts).Error
	if err != nil {
		global.Log.Error("PromptRepo.ListPersonalPromptsByUser find failed", zap.Error(err))
	}
	return prompts, count, err
}

// UpdatePersonalPrompt 更新个人级Prompt
func (r *PromptRepo) UpdatePersonalPrompt(ctx context.Context, prompt *PersonalPrompt) error {
	err := r.db.WithContext(ctx).Model(prompt).Updates(prompt).Error
	if err != nil {
		global.Log.Error("PromptRepo.UpdatePersonalPrompt failed", zap.Error(err))
		return errors.New("update personal prompt failed")
	}
	return nil
}

// UpdatePersonalPromptByID 按字段更新个人级Prompt
func (r *PromptRepo) UpdatePersonalPromptByID(ctx context.Context, id uint, updates map[string]interface{}) error {
	err := r.db.WithContext(ctx).Model(&PersonalPrompt{}).Where("id = ?", id).Updates(updates).Error
	if err != nil {
		global.Log.Error("PromptRepo.UpdatePersonalPromptByID failed", zap.Error(err))
		return errors.New("update personal prompt failed")
	}
	return nil
}

// DeletePersonalPrompt 删除个人级Prompt
func (r *PromptRepo) DeletePersonalPrompt(ctx context.Context, id uint) error {
	err := r.db.WithContext(ctx).Delete(&PersonalPrompt{}, id).Error
	if err != nil {
		global.Log.Error("PromptRepo.DeletePersonalPrompt failed", zap.Error(err))
		return errors.New("delete personal prompt failed")
	}
	return nil
}

// DeletePersonalPromptByUser 根据用户ID删除个人级Prompt
func (r *PromptRepo) DeletePersonalPromptByUser(ctx context.Context, userID uint) error {
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&PersonalPrompt{}).Error
	if err != nil {
		global.Log.Error("PromptRepo.DeletePersonalPromptByUser failed", zap.Error(err))
		return errors.New("delete personal prompt failed")
	}
	return nil
}

// CountPersonalPrompts 统计个人级Prompt总数
func (r *PromptRepo) CountPersonalPrompts(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&PersonalPrompt{}).Count(&count).Error
	if err != nil {
		global.Log.Error("PromptRepo.CountPersonalPrompts failed", zap.Error(err))
	}
	return count, err
}
