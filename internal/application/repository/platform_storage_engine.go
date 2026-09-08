package repository

import (
	"context"
	"errors"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

type platformStorageEngineRepository struct{ db *gorm.DB }

func NewPlatformStorageEngineRepository(db *gorm.DB) interfaces.PlatformStorageEngineRepository {
	return &platformStorageEngineRepository{db: db}
}

func (r *platformStorageEngineRepository) Create(ctx context.Context, engine *types.PlatformStorageEngine) error {
	return r.db.WithContext(ctx).Create(engine).Error
}

func (r *platformStorageEngineRepository) GetByID(ctx context.Context, id string) (*types.PlatformStorageEngine, error) {
	var engine types.PlatformStorageEngine
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&engine).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &engine, nil
}

func (r *platformStorageEngineRepository) GetByName(ctx context.Context, name string) (*types.PlatformStorageEngine, error) {
	var engine types.PlatformStorageEngine
	if err := r.db.WithContext(ctx).Where("name = ?", name).First(&engine).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &engine, nil
}

func (r *platformStorageEngineRepository) List(ctx context.Context) ([]*types.PlatformStorageEngine, error) {
	var engines []*types.PlatformStorageEngine
	err := r.db.WithContext(ctx).Order("created_at DESC").Find(&engines).Error
	return engines, err
}

func (r *platformStorageEngineRepository) Update(ctx context.Context, engine *types.PlatformStorageEngine) error {
	return r.db.WithContext(ctx).Model(&types.PlatformStorageEngine{}).
		Where("id = ?", engine.ID).
		Select("name", "config", "status", "updated_at").Updates(engine).Error
}

func (r *platformStorageEngineRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&types.PlatformStorageEngine{}).Error
}