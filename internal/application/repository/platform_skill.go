// Package repository persists the platform skill library (000098).
//
// Unlike TenantSkillRepository there is no tenant scope on the skill table:
// rows are platform-owned and only reachable from the SystemAdmin-gated
// /system/admin/skills surface. Assignments carry the tenant_id link; the
// materialized tenant_skill_catalog rows live in TenantSkillRepository like
// any self-built definition.
package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

// PlatformSkillRepository persists platform-level skill definitions.
type PlatformSkillRepository interface {
	Create(ctx context.Context, e *types.PlatformSkillEntity) error
	// GetByID returns nil (no error) when the skill does not exist, so callers
	// can render a 404 without inspecting errors.
	GetByID(ctx context.Context, id string) (*types.PlatformSkillEntity, error)
	// GetByName returns nil (no error) when no live skill holds the name.
	GetByName(ctx context.Context, name string) (*types.PlatformSkillEntity, error)
	List(ctx context.Context) ([]*types.PlatformSkillEntity, error)
	// Update writes the mutable columns and bumps updated_at server-side —
	// that bump is the drift driver for every assignment row.
	Update(ctx context.Context, e *types.PlatformSkillEntity) error
	SoftDelete(ctx context.Context, id string) error
}

// PlatformSkillAssignmentRepository persists the (skill, workspace) links.
type PlatformSkillAssignmentRepository interface {
	Create(ctx context.Context, e *types.PlatformSkillAssignmentEntity) error
	// GetBySkillAndTenant returns nil (no error) when no live assignment exists.
	GetBySkillAndTenant(
		ctx context.Context, skillID string, tenantID uint64,
	) (*types.PlatformSkillAssignmentEntity, error)
	ListBySkill(ctx context.Context, skillID string) ([]*types.PlatformSkillAssignmentEntity, error)
	// Update writes the bookkeeping columns after a materialize/push succeeded.
	Update(ctx context.Context, e *types.PlatformSkillAssignmentEntity) error
	SoftDelete(ctx context.Context, id string) error
}

type platformSkillRepository struct{ db *gorm.DB }

// NewPlatformSkillRepository returns a GORM-backed implementation.
func NewPlatformSkillRepository(db *gorm.DB) PlatformSkillRepository {
	return &platformSkillRepository{db: db}
}

func (r *platformSkillRepository) Create(ctx context.Context, e *types.PlatformSkillEntity) error {
	return r.db.WithContext(ctx).Create(e).Error
}

func (r *platformSkillRepository) GetByID(
	ctx context.Context, id string,
) (*types.PlatformSkillEntity, error) {
	var e types.PlatformSkillEntity
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&e).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *platformSkillRepository) GetByName(
	ctx context.Context, name string,
) (*types.PlatformSkillEntity, error) {
	var e types.PlatformSkillEntity
	err := r.db.WithContext(ctx).Where("name = ?", name).First(&e).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *platformSkillRepository) List(
	ctx context.Context,
) ([]*types.PlatformSkillEntity, error) {
	var list []*types.PlatformSkillEntity
	err := r.db.WithContext(ctx).Order("created_at ASC").Find(&list).Error
	if err != nil {
		return nil, err
	}
	return list, nil
}

// Update writes the mutable columns. Select is explicit so no other column —
// in particular created_at or source — can be clobbered by a stale entity
// copy, and updated_at is stamped here (not from the entity) so drift is
// driven by DB time, mirroring the sandbox connection repository.
func (r *platformSkillRepository) Update(ctx context.Context, e *types.PlatformSkillEntity) error {
	return r.db.WithContext(ctx).
		Model(&types.PlatformSkillEntity{}).
		Where("id = ?", e.ID).
		Select("name", "version", "description", "instructions", "bundle_ref", "bundle_sha256", "updated_at").
		Updates(map[string]any{
			"name":          e.Name,
			"version":       e.Version,
			"description":   e.Description,
			"instructions":  e.Instructions,
			"bundle_ref":    e.BundleRef,
			"bundle_sha256": e.BundleSHA256,
			"updated_at":    time.Now(),
		}).Error
}

func (r *platformSkillRepository) SoftDelete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&types.PlatformSkillEntity{}).Error
}

type platformSkillAssignmentRepository struct{ db *gorm.DB }

// NewPlatformSkillAssignmentRepository returns a GORM-backed implementation.
func NewPlatformSkillAssignmentRepository(
	db *gorm.DB,
) PlatformSkillAssignmentRepository {
	return &platformSkillAssignmentRepository{db: db}
}

func (r *platformSkillAssignmentRepository) Create(
	ctx context.Context, e *types.PlatformSkillAssignmentEntity,
) error {
	return r.db.WithContext(ctx).Create(e).Error
}

func (r *platformSkillAssignmentRepository) GetBySkillAndTenant(
	ctx context.Context, skillID string, tenantID uint64,
) (*types.PlatformSkillAssignmentEntity, error) {
	var e types.PlatformSkillAssignmentEntity
	err := r.db.WithContext(ctx).
		Where("skill_id = ? AND tenant_id = ?", skillID, tenantID).
		First(&e).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *platformSkillAssignmentRepository) ListBySkill(
	ctx context.Context, skillID string,
) ([]*types.PlatformSkillAssignmentEntity, error) {
	var list []*types.PlatformSkillAssignmentEntity
	err := r.db.WithContext(ctx).
		Where("skill_id = ?", skillID).
		Order("tenant_id ASC").
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	return list, nil
}

// Update writes the bookkeeping columns after a successful materialize or
// push; pushed_at is only ever moved forward by callers that succeeded.
func (r *platformSkillAssignmentRepository) Update(
	ctx context.Context, e *types.PlatformSkillAssignmentEntity,
) error {
	return r.db.WithContext(ctx).
		Model(&types.PlatformSkillAssignmentEntity{}).
		Where("id = ?", e.ID).
		Select("materialized_catalog_id", "pushed_at", "updated_at").
		Updates(map[string]any{
			"materialized_catalog_id": e.MaterializedCatalogID,
			"pushed_at":               e.PushedAt,
			"updated_at":              time.Now(),
		}).Error
}

func (r *platformSkillAssignmentRepository) SoftDelete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&types.PlatformSkillAssignmentEntity{}).Error
}
