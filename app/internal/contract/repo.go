package contract

import (
	"context"
	"errors"
	"strings"

	"contract_review/app/internal/global"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type ContractRepo struct {
	db *gorm.DB
}

func NewContractRepo(db *gorm.DB) *ContractRepo {
	return &ContractRepo{db: db}
}

// ============ Contract 相关操作 ============

// CreateContract 创建合同
func (r *ContractRepo) CreateContract(ctx context.Context, contract *Contract) error {
	err := r.db.WithContext(ctx).Create(contract).Error
	if err != nil {
		global.Log.Error("ContractRepo.CreateContract failed", zap.Error(err))
		return err
	}
	return nil
}

// GetContractByIDForAccount scopes a contract lookup to its owner. Returning
// gorm.ErrRecordNotFound for both a missing and a foreign-owned contract avoids
// leaking resource existence to another authenticated user.
func (r *ContractRepo) GetContractByIDForAccount(ctx context.Context, id uint64, account string) (*Contract, error) {
	var contract Contract
	err := r.db.WithContext(ctx).
		Where("id = ? AND account = ?", id, account).
		First(&contract).Error
	if err != nil {
		return nil, err
	}
	return &contract, nil
}

// GetContractByIDWithTypeForAccount scopes a typed contract lookup to its owner.
func (r *ContractRepo) GetContractByIDWithTypeForAccount(ctx context.Context, id uint64, account string) (*Contract, error) {
	var contract Contract
	err := r.db.WithContext(ctx).
		Preload("ContractType").
		Where("id = ? AND account = ?", id, account).
		First(&contract).Error
	if err != nil {
		return nil, err
	}
	return &contract, nil
}

// GetContractsByAccount 根据账号获取合同列表
func (r *ContractRepo) GetContractsByAccount(ctx context.Context, account string) ([]Contract, error) {
	var contracts []Contract
	err := r.db.WithContext(ctx).Where("account = ?", account).Find(&contracts).Error
	if err != nil {
		global.Log.Error("ContractRepo.GetContractsByAccount failed", zap.Error(err))
	}
	return contracts, err
}

// GetContractsByTypeID 根据类型ID获取合同列表
func (r *ContractRepo) GetContractsByTypeID(ctx context.Context, typeID uint64) ([]Contract, error) {
	var contracts []Contract
	err := r.db.WithContext(ctx).Where("type_id = ?", typeID).Find(&contracts).Error
	if err != nil {
		global.Log.Error("ContractRepo.GetContractsByTypeID failed", zap.Error(err))
	}
	return contracts, err
}

// GetContractsByStatus 根据状态获取合同列表
func (r *ContractRepo) GetContractsByStatus(ctx context.Context, status string) ([]Contract, error) {
	var contracts []Contract
	err := r.db.WithContext(ctx).Where("status = ?", status).Find(&contracts).Error
	if err != nil {
		global.Log.Error("ContractRepo.GetContractsByStatus failed", zap.Error(err))
	}
	return contracts, err
}

// ListContracts 获取合同列表（分页）
func (r *ContractRepo) ListContracts(ctx context.Context, offset, limit int) ([]Contract, int64, error) {
	var contracts []Contract
	var count int64

	if err := r.db.WithContext(ctx).Model(&Contract{}).Count(&count).Error; err != nil {
		global.Log.Error("ContractRepo.ListContracts count failed", zap.Error(err))
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).Offset(offset).Limit(limit).Order("upload_time DESC").Find(&contracts).Error
	if err != nil {
		global.Log.Error("ContractRepo.ListContracts find failed", zap.Error(err))
	}
	return contracts, count, err
}

// ListContractsByAccount 根据账号分页获取合同列表
func (r *ContractRepo) ListContractsByAccount(ctx context.Context, account string, offset, limit int) ([]Contract, int64, error) {
	var contracts []Contract
	var count int64

	if err := r.db.WithContext(ctx).Model(&Contract{}).Where("account = ?", account).Count(&count).Error; err != nil {
		global.Log.Error("ContractRepo.ListContractsByAccount count failed", zap.Error(err))
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).Where("account = ?", account).Offset(offset).Limit(limit).Order("upload_time DESC").Find(&contracts).Error
	if err != nil {
		global.Log.Error("ContractRepo.ListContractsByAccount find failed", zap.Error(err))
	}
	return contracts, count, err
}

// ListContractsByAccountWithType 根据账号分页获取合同列表（包含类型信息）
func (r *ContractRepo) ListContractsByAccountWithType(ctx context.Context, account string, offset, limit int) ([]Contract, int64, error) {
	var contracts []Contract
	var count int64

	if err := r.db.WithContext(ctx).Model(&Contract{}).Where("account = ?", account).Count(&count).Error; err != nil {
		global.Log.Error("ContractRepo.ListContractsByAccountWithType count failed", zap.Error(err))
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).Preload("ContractType").Where("account = ?", account).Offset(offset).Limit(limit).Order("upload_time DESC").Find(&contracts).Error
	if err != nil {
		global.Log.Error("ContractRepo.ListContractsByAccountWithType find failed", zap.Error(err))
	}
	return contracts, count, err
}

// UpdateContract 更新合同
func (r *ContractRepo) UpdateContract(ctx context.Context, contract *Contract) error {
	err := r.db.WithContext(ctx).Model(contract).Updates(contract).Error
	if err != nil {
		global.Log.Error("ContractRepo.UpdateContract failed", zap.Error(err))
		return errors.New("update contract failed")
	}
	return nil
}

// UpdateContractByID 按字段更新合同
func (r *ContractRepo) UpdateContractByID(ctx context.Context, id uint64, updates map[string]interface{}) error {
	err := r.db.WithContext(ctx).Model(&Contract{}).Where("id = ?", id).Updates(updates).Error
	if err != nil {
		global.Log.Error("ContractRepo.UpdateContractByID failed", zap.Error(err))
		return errors.New("update contract failed")
	}
	return nil
}

// UpdateContractByIDForAccount updates only a contract owned by account.
func (r *ContractRepo) UpdateContractByIDForAccount(ctx context.Context, id uint64, account string, updates map[string]interface{}) error {
	result := r.db.WithContext(ctx).
		Model(&Contract{}).
		Where("id = ? AND account = ?", id, account).
		Updates(updates)
	if result.Error != nil {
		global.Log.Error("ContractRepo.UpdateContractByIDForAccount failed", zap.Error(result.Error))
		return errors.New("update contract failed")
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// DeleteContract 删除合同
func (r *ContractRepo) DeleteContract(ctx context.Context, id uint64) error {
	err := r.db.WithContext(ctx).Delete(&Contract{}, id).Error
	if err != nil {
		global.Log.Error("ContractRepo.DeleteContract failed", zap.Error(err))
		return errors.New("delete contract failed")
	}
	return nil
}

// DeleteContractForAccount deletes only a contract owned by account.
func (r *ContractRepo) DeleteContractForAccount(ctx context.Context, id uint64, account string) error {
	result := r.db.WithContext(ctx).
		Where("id = ? AND account = ?", id, account).
		Delete(&Contract{})
	if result.Error != nil {
		global.Log.Error("ContractRepo.DeleteContractForAccount failed", zap.Error(result.Error))
		return errors.New("delete contract failed")
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// DeleteContractsByAccount 根据账号删除合同
func (r *ContractRepo) DeleteContractsByAccount(ctx context.Context, account string) error {
	err := r.db.WithContext(ctx).Where("account = ?", account).Delete(&Contract{}).Error
	if err != nil {
		global.Log.Error("ContractRepo.DeleteContractsByAccount failed", zap.Error(err))
		return errors.New("delete contract failed")
	}
	return nil
}

// CountContracts 统计合同总数
func (r *ContractRepo) CountContracts(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Contract{}).Count(&count).Error
	if err != nil {
		global.Log.Error("ContractRepo.CountContracts failed", zap.Error(err))
	}
	return count, err
}

// CountContractsByAccount 统计指定账号的合同数量
func (r *ContractRepo) CountContractsByAccount(ctx context.Context, account string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Contract{}).Where("account = ?", account).Count(&count).Error
	if err != nil {
		global.Log.Error("ContractRepo.CountContractsByAccount failed", zap.Error(err))
	}
	return count, err
}

// CountContractsByTypeID 统计指定类型的合同数量
func (r *ContractRepo) CountContractsByTypeID(ctx context.Context, typeID uint64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Contract{}).Where("type_id = ?", typeID).Count(&count).Error
	if err != nil {
		global.Log.Error("ContractRepo.CountContractsByTypeID failed", zap.Error(err))
	}
	return count, err
}

// CountContractsByStatus 统计指定状态的合同数量
func (r *ContractRepo) CountContractsByStatus(ctx context.Context, status string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Contract{}).Where("status = ?", status).Count(&count).Error
	if err != nil {
		global.Log.Error("ContractRepo.CountContractsByStatus failed", zap.Error(err))
	}
	return count, err
}

// ============ ContractType 相关操作 ============

// CreateContractType 创建合同类型
func (r *ContractRepo) CreateContractType(ctx context.Context, contractType *ContractType) error {
	err := r.db.WithContext(ctx).Create(contractType).Error
	if err != nil {
		global.Log.Error("ContractRepo.CreateContractType failed", zap.Error(err))
		return err
	}
	return nil
}

// GetContractTypeByID 根据ID获取合同类型
func (r *ContractRepo) GetContractTypeByID(ctx context.Context, id uint64) (*ContractType, error) {
	var contractType ContractType
	err := r.db.WithContext(ctx).First(&contractType, id).Error
	if err != nil {
		global.Log.Error("ContractRepo.GetContractTypeByID failed", zap.Error(err))
		return nil, err
	}
	return &contractType, nil
}

// GetContractTypeByName 根据名称获取合同类型
func (r *ContractRepo) GetContractTypeByName(ctx context.Context, name string) (*ContractType, error) {
	var contractType ContractType
	err := r.db.WithContext(ctx).Where("name = ?", name).First(&contractType).Error
	if err != nil {
		global.Log.Error("ContractRepo.GetContractTypeByName failed", zap.Error(err))
		return nil, err
	}
	return &contractType, nil
}

// ListContractTypes 获取所有合同类型列表
func (r *ContractRepo) ListContractTypes(ctx context.Context) ([]ContractType, error) {
	var contractTypes []ContractType
	err := r.db.WithContext(ctx).Order("created_at DESC").Find(&contractTypes).Error
	if err != nil {
		global.Log.Error("ContractRepo.ListContractTypes failed", zap.Error(err))
	}
	return contractTypes, err
}

// ListContractTypesPaginated 分页获取合同类型列表
func (r *ContractRepo) ListContractTypesPaginated(ctx context.Context, offset, limit int) ([]ContractType, int64, error) {
	var contractTypes []ContractType
	var count int64

	if err := r.db.WithContext(ctx).Model(&ContractType{}).Count(&count).Error; err != nil {
		global.Log.Error("ContractRepo.ListContractTypesPaginated count failed", zap.Error(err))
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).Offset(offset).Limit(limit).Order("created_at DESC").Find(&contractTypes).Error
	if err != nil {
		global.Log.Error("ContractRepo.ListContractTypesPaginated find failed", zap.Error(err))
	}
	return contractTypes, count, err
}

// ListContractTypesFiltered 分页获取合同类型列表（支持名称、创建者和更新时间筛选）
func (r *ContractRepo) ListContractTypesFiltered(ctx context.Context, name, creator, startDate, endDate string, offset, limit int) ([]ContractType, int64, error) {
	var contractTypes []ContractType
	var count int64

	query := r.db.WithContext(ctx).Model(&ContractType{})
	if name = strings.TrimSpace(name); name != "" {
		query = query.Where("name LIKE ?", "%"+name+"%")
	}
	if creator = strings.TrimSpace(creator); creator != "" {
		query = query.Where("creator LIKE ?", "%"+creator+"%")
	}
	if startDate = strings.TrimSpace(startDate); startDate != "" {
		query = query.Where("DATE(updated_at) >= ?", startDate)
	}
	if endDate = strings.TrimSpace(endDate); endDate != "" {
		query = query.Where("DATE(updated_at) <= ?", endDate)
	}

	if err := query.Count(&count).Error; err != nil {
		global.Log.Error("ContractRepo.ListContractTypesFiltered count failed", zap.Error(err))
		return nil, 0, err
	}

	err := query.Offset(offset).Limit(limit).Order("updated_at DESC, created_at DESC").Find(&contractTypes).Error
	if err != nil {
		global.Log.Error("ContractRepo.ListContractTypesFiltered find failed", zap.Error(err))
	}
	return contractTypes, count, err
}

// ListContractTypeCreators 获取合同类型创建人列表
func (r *ContractRepo) ListContractTypeCreators(ctx context.Context) ([]string, error) {
	var creators []string
	err := r.db.WithContext(ctx).Model(&ContractType{}).
		Where("creator <> ''").
		Distinct("creator").
		Order("creator ASC").
		Pluck("creator", &creators).Error
	if err != nil {
		global.Log.Error("ContractRepo.ListContractTypeCreators failed", zap.Error(err))
	}
	return creators, err
}

// UpdateContractType 更新合同类型
func (r *ContractRepo) UpdateContractType(ctx context.Context, contractType *ContractType) error {
	err := r.db.WithContext(ctx).Model(contractType).Updates(contractType).Error
	if err != nil {
		global.Log.Error("ContractRepo.UpdateContractType failed", zap.Error(err))
		return errors.New("update contract type failed")
	}
	return nil
}

// UpdateContractTypeByID 按字段更新合同类型
func (r *ContractRepo) UpdateContractTypeByID(ctx context.Context, id uint64, updates map[string]interface{}) error {
	err := r.db.WithContext(ctx).Model(&ContractType{}).Where("id = ?", id).Updates(updates).Error
	if err != nil {
		global.Log.Error("ContractRepo.UpdateContractTypeByID failed", zap.Error(err))
		return errors.New("update contract type failed")
	}
	return nil
}

// DeleteContractType 删除合同类型
func (r *ContractRepo) DeleteContractType(ctx context.Context, id uint64) error {
	// 检查是否有合同关联此类型
	var count int64
	if err := r.db.WithContext(ctx).Model(&Contract{}).Where("type_id = ?", id).Count(&count).Error; err != nil {
		global.Log.Error("ContractRepo.DeleteContractType check failed", zap.Error(err))
		return errors.New("check contract type usage failed")
	}
	if count > 0 {
		return errors.New("该合同类型下存在合同，无法删除")
	}

	err := r.db.WithContext(ctx).Delete(&ContractType{}, id).Error
	if err != nil {
		global.Log.Error("ContractRepo.DeleteContractType failed", zap.Error(err))
		return errors.New("delete contract type failed")
	}
	return nil
}

// DeleteContractTypes 批量删除合同类型
func (r *ContractRepo) DeleteContractTypes(ctx context.Context, ids []uint64) error {
	if len(ids) == 0 {
		return nil
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&Contract{}).Where("type_id IN ?", ids).Count(&count).Error; err != nil {
		global.Log.Error("ContractRepo.DeleteContractTypes check failed", zap.Error(err))
		return errors.New("check contract type usage failed")
	}
	if count > 0 {
		return errors.New("选中的合同类型下存在合同，无法删除")
	}

	err := r.db.WithContext(ctx).Where("id IN ?", ids).Delete(&ContractType{}).Error
	if err != nil {
		global.Log.Error("ContractRepo.DeleteContractTypes failed", zap.Error(err))
		return errors.New("delete contract types failed")
	}
	return nil
}

// CountContractTypes 统计合同类型总数
func (r *ContractRepo) CountContractTypes(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&ContractType{}).Count(&count).Error
	if err != nil {
		global.Log.Error("ContractRepo.CountContractTypes failed", zap.Error(err))
	}
	return count, err
}

// ExistsContractType 检查合同类型是否存在
func (r *ContractRepo) ExistsContractType(ctx context.Context, id uint64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&ContractType{}).Where("id = ?", id).Count(&count).Error
	if err != nil {
		global.Log.Error("ContractRepo.ExistsContractType failed", zap.Error(err))
		return false, err
	}
	return count > 0, nil
}

// ExistsContractTypeByName 检查合同类型名称是否存在
func (r *ContractRepo) ExistsContractTypeByName(ctx context.Context, name string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&ContractType{}).Where("name = ?", name).Count(&count).Error
	if err != nil {
		global.Log.Error("ContractRepo.ExistsContractTypeByName failed", zap.Error(err))
		return false, err
	}
	return count > 0, nil
}
