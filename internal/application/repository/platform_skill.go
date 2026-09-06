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
	// that bump is the drift driver for every assignment row. Category/author
	// are deliberately NOT in its column list: a bundle re-register never
	// touches admin-managed metadata.
	Update(ctx context.Context, e *types.PlatformSkillEntity) error
	// UpdateMeta writes only the definition metadata (category/author plus the
	// 000106 Chinese display fields) and bumps updated_at the same way, so meta
	// edits also drive assignment drift and reach workspaces through push.
	UpdateMeta(ctx context.Context, id, category, author, zhName, zhDescription string) error

	// Category registry (000105): categories are created FIRST in the console's
	// category-manager dialog and skills then only reference existing names, so
	// these methods administer the registry table plus the skill rows that point
	// at it — never a free-text category a skill accidentally mints.

	// CreateCategory inserts one registry row. A live row holding the same name
	// is a violation of the partial unique index; callers pre-check with
	// GetCategoryByName to render a conflict.
	CreateCategory(ctx context.Context, e *types.PlatformSkillCategoryEntity) error
	// GetCategoryByName returns nil (no error) when no live category holds the
	// name.
	GetCategoryByName(ctx context.Context, name string) (*types.PlatformSkillCategoryEntity, error)
	// ListCategories lists every live registry category with how many live
	// skills reference it (LEFT JOIN, so a fresh unused category still shows).
	ListCategories(ctx context.Context) ([]SkillCategoryCount, error)
	// RenameCategory renames one registry row and moves every live skill that
	// references the old name onto the new one, in one transaction, returning
	// how many skills moved.
	RenameCategory(ctx context.Context, from, to string) (int64, error)
	// ClearCategory deletes one registry row (soft) and empties the category on
	// every live skill that referenced it, returning how many skills were
	// cleared. Deleting clears rather than cascades: skills land uncategorized.
	ClearCategory(ctx context.Context, name string) (int64, error)
	SoftDelete(ctx context.Context, id string) error
}

// SkillCategoryCount is one registry row plus how many live platform skills
// reference it — the console's category-manager directory.
type SkillCategoryCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
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

// UpdateMeta writes only the definition metadata. The updated_at stamp is
// server-side (not from the entity) so meta edits drive drift exactly like
// bundle re-registers do; the soft-delete scope mirrors Update.
func (r *platformSkillRepository) UpdateMeta(
	ctx context.Context, id, category, author, zhName, zhDescription string,
) error {
	return r.db.WithContext(ctx).
		Model(&types.PlatformSkillEntity{}).
		Where("id = ?", id).
		Select("category", "author", "zh_name", "zh_description", "updated_at").
		Updates(map[string]any{
			"category":       category,
			"author":         author,
			"zh_name":        zhName,
			"zh_description": zhDescription,
			"updated_at":     time.Now(),
		}).Error
}

func (r *platformSkillRepository) CreateCategory(
	ctx context.Context, e *types.PlatformSkillCategoryEntity,
) error {
	return r.db.WithContext(ctx).Create(e).Error
}

// GetCategoryByName scopes to live rows only; a deleted-then-recreated name is
// a new category, not this one.
func (r *platformSkillRepository) GetCategoryByName(
	ctx context.Context, name string,
) (*types.PlatformSkillCategoryEntity, error) {
	var e types.PlatformSkillCategoryEntity
	err := r.db.WithContext(ctx).
		Where("name = ? AND deleted_at IS NULL", name).
		First(&e).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// ListCategories is registry-driven: every live category row, LEFT JOINed with
// the live skills referencing it so a freshly created (unused) category still
// appears with count 0.
func (r *platformSkillRepository) ListCategories(
	ctx context.Context,
) ([]SkillCategoryCount, error) {
	var list []SkillCategoryCount
	err := r.db.WithContext(ctx).
		Table("platform_skill_categories AS c").
		Select("c.name AS name, COUNT(s.id) AS count").
		Joins("LEFT JOIN platform_skills AS s ON s.deleted_at IS NULL AND s.category = c.name").
		Where("c.deleted_at IS NULL").
		Group("c.name").
		Order("c.name ASC").
		Scan(&list).Error
	if err != nil {
		return nil, err
	}
	return list, nil
}

// RenameCategory renames the registry row and every referencing skill in one
// transaction: both sides must move together or neither does. RowsAffected of
// the skill UPDATE is the "how many skills moved" answer.
func (r *platformSkillRepository) RenameCategory(
	ctx context.Context, from, to string,
) (int64, error) {
	var moved int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&types.PlatformSkillCategoryEntity{}).
			Where("name = ? AND deleted_at IS NULL", from).
			Updates(map[string]any{"name": to, "updated_at": time.Now()})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			// The registry row is gone (deleted since the caller looked it up);
			// refuse rather than silently rewriting only the skills.
			return gorm.ErrRecordNotFound
		}
		skills := tx.Model(&types.PlatformSkillEntity{}).
			Where("category = ?", from).
			Updates(map[string]any{"category": to, "updated_at": time.Now()})
		if skills.Error != nil {
			return skills.Error
		}
		moved = skills.RowsAffected
		return nil
	})
	return moved, err
}

// ClearCategory deletes the registry row and empties the category on every
// referencing skill, atomically. Deleting clears rather than cascades — skills
// land uncategorized, keeping their identity and assignments.
func (r *platformSkillRepository) ClearCategory(
	ctx context.Context, name string,
) (int64, error) {
	var cleared int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		reg := tx.Model(&types.PlatformSkillCategoryEntity{}).
			Where("name = ? AND deleted_at IS NULL", name).
			Updates(map[string]any{"deleted_at": time.Now()})
		if reg.Error != nil {
			return reg.Error
		}
		skills := tx.Model(&types.PlatformSkillEntity{}).
			Where("category = ?", name).
			Updates(map[string]any{"category": "", "updated_at": time.Now()})
		if skills.Error != nil {
			return skills.Error
		}
		cleared = skills.RowsAffected
		return nil
	})
	return cleared, err
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
