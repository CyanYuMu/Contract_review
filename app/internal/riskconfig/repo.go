package riskconfig

import (
	"context"
	"strings"

	"contract_review/app/internal/global"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

type ListFilters struct {
	RiskID           uint64
	RiskContent      string
	Status           string
	ContractTypeName string
	Creator          string
	StartDate        string
	EndDate          string
}

func (r *Repo) Create(ctx context.Context, riskPoint *RiskPoint) error {
	if err := r.db.WithContext(ctx).Create(riskPoint).Error; err != nil {
		global.Log.Error("RiskPointRepo.Create failed", zap.Error(err))
		return err
	}
	return nil
}

func (r *Repo) GetByID(ctx context.Context, id uint64) (*RiskPoint, error) {
	var riskPoint RiskPoint
	err := r.db.WithContext(ctx).First(&riskPoint, id).Error
	if err != nil {
		global.Log.Error("RiskPointRepo.GetByID failed", zap.Error(err))
		return nil, err
	}
	return &riskPoint, nil
}

func (r *Repo) List(ctx context.Context, filters ListFilters, offset, limit int) ([]RiskPoint, int64, error) {
	var riskPoints []RiskPoint
	var count int64

	query := r.db.WithContext(ctx).Model(&RiskPoint{})
	query = applyFilters(query, filters)

	if err := query.Count(&count).Error; err != nil {
		global.Log.Error("RiskPointRepo.List count failed", zap.Error(err))
		return nil, 0, err
	}

	err := query.Offset(offset).Limit(limit).Order("updated_at DESC, id DESC").Find(&riskPoints).Error
	if err != nil {
		global.Log.Error("RiskPointRepo.List find failed", zap.Error(err))
	}
	return riskPoints, count, err
}

func (r *Repo) UpdateByID(ctx context.Context, id uint64, updates map[string]interface{}) error {
	if err := r.db.WithContext(ctx).Model(&RiskPoint{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		global.Log.Error("RiskPointRepo.UpdateByID failed", zap.Error(err))
		return err
	}
	return nil
}

func (r *Repo) Delete(ctx context.Context, id uint64) error {
	if err := r.db.WithContext(ctx).Delete(&RiskPoint{}, id).Error; err != nil {
		global.Log.Error("RiskPointRepo.Delete failed", zap.Error(err))
		return err
	}
	return nil
}

func (r *Repo) Stats(ctx context.Context, contractTypeName string) (RiskPointStatsResponse, error) {
	var stats RiskPointStatsResponse
	base := r.db.WithContext(ctx).Model(&RiskPoint{})
	if strings.TrimSpace(contractTypeName) != "" {
		base = base.Where("contract_type_name = ?", strings.TrimSpace(contractTypeName))
	}

	if err := base.Count(&stats.Total).Error; err != nil {
		return stats, err
	}
	if err := base.Where("status = ?", "enabled").Count(&stats.Enabled).Error; err != nil {
		return stats, err
	}
	if err := base.Where("status = ?", "disabled").Count(&stats.Disabled).Error; err != nil {
		return stats, err
	}
	if err := base.Where("knowledge_doc_id > 0 AND status = ?", "enabled").Count(&stats.Indexed).Error; err != nil {
		return stats, err
	}

	stats.ByLevel = make([]RiskPointStatItem, 0)
	if err := groupedStats(base, "risk_level", &stats.ByLevel); err != nil {
		return stats, err
	}
	stats.ByType = make([]RiskPointStatItem, 0)
	if err := groupedStats(base, "risk_type", &stats.ByType); err != nil {
		return stats, err
	}
	stats.ByContractType = make([]RiskPointStatItem, 0)
	if err := groupedStats(base, "contract_type_name", &stats.ByContractType); err != nil {
		return stats, err
	}
	return stats, nil
}

func applyFilters(query *gorm.DB, filters ListFilters) *gorm.DB {
	if filters.RiskID > 0 {
		query = query.Where("id = ?", filters.RiskID)
	}
	if value := strings.TrimSpace(filters.RiskContent); value != "" {
		query = query.Where("risk_content LIKE ?", "%"+value+"%")
	}
	if value := strings.TrimSpace(filters.Status); value != "" {
		query = query.Where("status = ?", value)
	}
	if value := strings.TrimSpace(filters.ContractTypeName); value != "" {
		query = query.Where("contract_type_name = ?", value)
	}
	if value := strings.TrimSpace(filters.Creator); value != "" {
		query = query.Where("creator LIKE ?", "%"+value+"%")
	}
	if value := strings.TrimSpace(filters.StartDate); value != "" {
		query = query.Where("DATE(updated_at) >= ?", value)
	}
	if value := strings.TrimSpace(filters.EndDate); value != "" {
		query = query.Where("DATE(updated_at) <= ?", value)
	}
	return query
}

func groupedStats(base *gorm.DB, column string, out *[]RiskPointStatItem) error {
	var rows []struct {
		Name  string `gorm:"column:name"`
		Value int64  `gorm:"column:value"`
	}
	if err := base.
		Select(column + " AS name, COUNT(*) AS value").
		Where(column + " <> ''").
		Group(column).
		Order("value DESC").
		Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		*out = append(*out, RiskPointStatItem{Name: row.Name, Value: row.Value})
	}
	return nil
}
