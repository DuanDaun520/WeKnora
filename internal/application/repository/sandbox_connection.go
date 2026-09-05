// Package repository persists the platform sandbox connection catalog.
//
// Unlike TenantSandboxConfigRepository there is no tenant scope: rows are
// platform-owned and only reachable from the SystemAdmin-gated
// /system/admin/sandbox-connections surface. The credential-bearing payload
// rides the entity's Config and goes through TenantSandboxConfig's encrypted
// Value/Scan hooks like any sandbox config.
package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

// SandboxConnectionRepository persists platform-level sandbox connections.
type SandboxConnectionRepository interface {
	Create(ctx context.Context, e *types.SandboxConnectionEntity) error
	// GetByID returns nil (no error) when the connection does not exist, so
	// callers can render a 404 without inspecting errors.
	GetByID(ctx context.Context, id string) (*types.SandboxConnectionEntity, error)
	List(ctx context.Context) ([]*types.SandboxConnectionEntity, error)
	Update(ctx context.Context, e *types.SandboxConnectionEntity) error
	SoftDelete(ctx context.Context, id string) error
}

type sandboxConnectionRepository struct {
	db *gorm.DB
}

// NewSandboxConnectionRepository returns a GORM-backed implementation.
func NewSandboxConnectionRepository(db *gorm.DB) SandboxConnectionRepository {
	return &sandboxConnectionRepository{db: db}
}

func (r *sandboxConnectionRepository) Create(
	ctx context.Context, e *types.SandboxConnectionEntity,
) error {
	return r.db.WithContext(ctx).Create(e).Error
}

func (r *sandboxConnectionRepository) GetByID(
	ctx context.Context, id string,
) (*types.SandboxConnectionEntity, error) {
	var e types.SandboxConnectionEntity
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&e).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *sandboxConnectionRepository) List(
	ctx context.Context,
) ([]*types.SandboxConnectionEntity, error) {
	var list []*types.SandboxConnectionEntity
	err := r.db.WithContext(ctx).Order("created_at ASC").Find(&list).Error
	if err != nil {
		return nil, err
	}
	return list, nil
}

// Update writes the mutable columns. Select is explicit so no other column —
// in particular created_at — can be clobbered by a stale entity copy.
func (r *sandboxConnectionRepository) Update(
	ctx context.Context, e *types.SandboxConnectionEntity,
) error {
	return r.db.WithContext(ctx).
		Model(&types.SandboxConnectionEntity{}).
		Where("id = ?", e.ID).
		Select("name", "description", "sandbox_type", "config", "updated_at").
		Updates(map[string]any{
			"name":         e.Name,
			"description":  e.Description,
			"sandbox_type": e.SandboxType,
			"config":       e.Config,
			"updated_at":   time.Now(),
		}).Error
}

func (r *sandboxConnectionRepository) SoftDelete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&types.SandboxConnectionEntity{}).Error
}
