package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// webSearchProviderRepository implements the WebSearchProviderRepository interface
type webSearchProviderRepository struct {
	db *gorm.DB
}

// NewWebSearchProviderRepository creates a new web search provider repository
func NewWebSearchProviderRepository(db *gorm.DB) interfaces.WebSearchProviderRepository {
	return &webSearchProviderRepository{db: db}
}

// Create creates a new platform web search provider
func (r *webSearchProviderRepository) Create(ctx context.Context, provider *types.WebSearchProviderEntity) error {
	return r.db.WithContext(ctx).Create(provider).Error
}

// GetByIDAnyTenant retrieves a provider by ID regardless of workspace — the
// platform catalog access used by the admin console.
func (r *webSearchProviderRepository) GetByIDAnyTenant(ctx context.Context, id string) (*types.WebSearchProviderEntity, error) {
	var provider types.WebSearchProviderEntity
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&provider).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &provider, nil
}

// GetByID retrieves a provider by ID if it is assigned to the given workspace.
// Providers are platform rows (000095 rework): visibility IS the assignment.
// IsDefault carries the workspace-effective default, same rule as List.
func (r *webSearchProviderRepository) GetByID(ctx context.Context, tenantID uint64, id string) (*types.WebSearchProviderEntity, error) {
	rows, err := r.tenantAssignments(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	assigned := false
	for _, row := range rows {
		if row.ProviderID == id {
			assigned = true
			break
		}
	}
	if !assigned {
		return nil, nil
	}
	provider, err := r.GetByIDAnyTenant(ctx, id)
	if err != nil || provider == nil {
		return provider, err
	}
	provider.IsDefault = id == effectiveDefaultProviderID(rows)
	return provider, nil
}

// tenantAssignments loads a workspace's assignment rows ordered so that the
// effective default comes first: the flagged row before unflagged ones, then
// earliest assigned.
func (r *webSearchProviderRepository) tenantAssignments(
	ctx context.Context, tenantID uint64,
) ([]types.TenantWebSearchProviderAssignment, error) {
	var rows []types.TenantWebSearchProviderAssignment
	err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("is_default DESC").Order("assigned_at ASC").Order("id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// effectiveDefaultProviderID resolves which assigned provider acts as the
// workspace default: the flagged assignment, else the earliest one. The same
// rule backs GetDefault and the IsDefault markers on List so the chat
// readiness check and the runtime never disagree.
func effectiveDefaultProviderID(rows []types.TenantWebSearchProviderAssignment) string {
	for _, row := range rows {
		if row.IsDefault {
			return row.ProviderID
		}
	}
	if len(rows) > 0 {
		return rows[0].ProviderID
	}
	return ""
}

// GetDefault retrieves the workspace's effective default provider, or nil if
// the workspace has no assignment at all.
func (r *webSearchProviderRepository) GetDefault(ctx context.Context, tenantID uint64) (*types.WebSearchProviderEntity, error) {
	rows, err := r.tenantAssignments(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	defaultID := effectiveDefaultProviderID(rows)
	if defaultID == "" {
		return nil, nil
	}
	return r.GetByIDAnyTenant(ctx, defaultID)
}

// List lists the web search providers assigned to a workspace. Each entity's
// IsDefault is the workspace-effective default (not the dead row-level
// column), keeping the tenant-scoped read contract of chat and agent editors.
func (r *webSearchProviderRepository) List(ctx context.Context, tenantID uint64) ([]*types.WebSearchProviderEntity, error) {
	rows, err := r.tenantAssignments(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []*types.WebSearchProviderEntity{}, nil
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ProviderID)
	}
	var providers []*types.WebSearchProviderEntity
	if err := r.db.WithContext(ctx).
		Where("id IN ?", ids).
		Order("created_at ASC").
		Find(&providers).Error; err != nil {
		return nil, err
	}
	defaultID := effectiveDefaultProviderID(rows)
	for _, provider := range providers {
		provider.IsDefault = provider.ID == defaultID
	}
	return providers, nil
}

// ListAll lists every platform provider (admin console catalog).
func (r *webSearchProviderRepository) ListAll(ctx context.Context) ([]*types.WebSearchProviderEntity, error) {
	var providers []*types.WebSearchProviderEntity
	if err := r.db.WithContext(ctx).
		Order("created_at ASC").
		Find(&providers).Error; err != nil {
		return nil, err
	}
	return providers, nil
}

// Update updates a platform web search provider by ID
func (r *webSearchProviderRepository) Update(ctx context.Context, provider *types.WebSearchProviderEntity) error {
	return r.db.WithContext(ctx).Model(&types.WebSearchProviderEntity{}).
		Where("id = ?", provider.ID).
		Select("*").Updates(provider).Error
}

// Delete soft-deletes a platform web search provider by ID
func (r *webSearchProviderRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&types.WebSearchProviderEntity{}).Error
}

// ListAssignmentInfos lists a provider's assignments joined with workspace
// names (admin console "assigned workspaces" list).
func (r *webSearchProviderRepository) ListAssignmentInfos(
	ctx context.Context, providerID string,
) ([]types.WebSearchProviderAssignmentInfo, error) {
	var rows []types.WebSearchProviderAssignmentInfo
	err := r.db.WithContext(ctx).
		Table("tenant_web_search_provider_assignments AS a").
		Select("a.tenant_id, t.name AS tenant_name, a.provider_id, a.is_default, a.assigned_at").
		Joins("LEFT JOIN tenants t ON t.id = a.tenant_id").
		Where("a.provider_id = ?", providerID).
		Order("a.assigned_at ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// ReplaceProviderAssignments atomically sets the full assignment list of a
// provider: removes rows whose workspace is absent from the list, upserts the
// rest (is_default included — OnConflict DoNothing would silently keep a
// stale flag), and clears the default flag on OTHER providers of a workspace
// that just received a new default, so at most one default per workspace
// holds across the whole catalog.
func (r *webSearchProviderRepository) ReplaceProviderAssignments(
	ctx context.Context, providerID string,
	rows []types.TenantWebSearchProviderAssignment, assignedBy string,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		keepIDs := make([]uint64, 0, len(rows))
		for _, row := range rows {
			keepIDs = append(keepIDs, row.TenantID)
		}
		if len(keepIDs) == 0 {
			if err := tx.Where("provider_id = ?", providerID).
				Delete(&types.TenantWebSearchProviderAssignment{}).Error; err != nil {
				return err
			}
		} else if err := tx.
			Where("provider_id = ? AND tenant_id NOT IN ?", providerID, keepIDs).
			Delete(&types.TenantWebSearchProviderAssignment{}).Error; err != nil {
			return err
		}

		for _, row := range rows {
			// gorm would insert the zero time over the column's CURRENT_TIMESTAMP
			// default; stamp first-assignment time here so the earliest-fallback
			// ordering and the drawer's assigned_at render stay meaningful.
			assignedAt := row.AssignedAt
			if assignedAt.IsZero() {
				assignedAt = time.Now().UTC()
			}
			updated := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "tenant_id"}, {Name: "provider_id"}},
				DoUpdates: clause.Assignments(map[string]interface{}{
					"is_default": row.IsDefault,
					"updated_at": gorm.Expr("CURRENT_TIMESTAMP"),
				}),
			}).Create(&types.TenantWebSearchProviderAssignment{
				TenantID:   row.TenantID,
				ProviderID: providerID,
				IsDefault:  row.IsDefault,
				AssignedBy: assignedBy,
				AssignedAt: assignedAt,
			})
			if err := updated.Error; err != nil {
				return err
			}
			if row.IsDefault {
				if err := tx.Model(&types.TenantWebSearchProviderAssignment{}).
					Where("tenant_id = ? AND provider_id != ?", row.TenantID, providerID).
					Update("is_default", false).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// DeleteAssignmentsByProviderID removes every workspace assignment of a
// provider. Called after a provider is deleted so no dangling rows remain.
func (r *webSearchProviderRepository) DeleteAssignmentsByProviderID(ctx context.Context, providerID string) error {
	return r.db.WithContext(ctx).
		Where("provider_id = ?", providerID).
		Delete(&types.TenantWebSearchProviderAssignment{}).Error
}
