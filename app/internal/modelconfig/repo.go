package modelconfig

import (
	"context"

	"gorm.io/gorm"
)

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

func (r *Repo) GetDefault(ctx context.Context) (*ModelConfig, error) {
	var cfg ModelConfig
	err := r.db.WithContext(ctx).Where("is_default = ?", true).Order("updated_at DESC, id DESC").First(&cfg).Error
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (r *Repo) List(ctx context.Context) ([]ModelConfig, error) {
	var configs []ModelConfig
	err := r.db.WithContext(ctx).Order("is_default DESC, updated_at DESC, id DESC").Find(&configs).Error
	return configs, err
}

func (r *Repo) Create(ctx context.Context, cfg *ModelConfig) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if cfg.IsDefault {
			if err := tx.Model(&ModelConfig{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return tx.Create(cfg).Error
	})
}

func (r *Repo) Update(ctx context.Context, id uint, updates map[string]interface{}) (*ModelConfig, error) {
	var cfg ModelConfig
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if isDefault, ok := updates["is_default"].(bool); ok && isDefault {
			if err := tx.Model(&ModelConfig{}).Where("is_default = ? AND id <> ?", true, id).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&ModelConfig{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
		return tx.First(&cfg, id).Error
	})
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}
