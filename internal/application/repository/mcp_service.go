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

// mcpServiceRepository implements the MCPServiceRepository interface
type mcpServiceRepository struct {
	db *gorm.DB
}

// NewMCPServiceRepository creates a new MCP service repository
func NewMCPServiceRepository(db *gorm.DB) interfaces.MCPServiceRepository {
	return &mcpServiceRepository{db: db}
}

// visibleTenantScope applies the workspace visibility rule of the 000096
// platform rework: a workspace sees its assigned services plus the builtin
// rows. tenantID == 0 means platform scope (admin console / platform write
// paths) and adds no filter.
func visibleTenantScope(db *gorm.DB, tenantID uint64) *gorm.DB {
	if tenantID == 0 {
		return db
	}
	return db.Where(
		"is_builtin = true OR id IN (SELECT service_id FROM tenant_mcp_service_assignments WHERE tenant_id = ?)",
		tenantID,
	)
}

// Create creates a new MCP service
func (r *mcpServiceRepository) Create(ctx context.Context, service *types.MCPService) error {
	return r.db.WithContext(ctx).Create(service).Error
}

// GetByIDAnyTenant retrieves an MCP service by ID regardless of workspace —
// the platform catalog access used by the admin console.
func (r *mcpServiceRepository) GetByIDAnyTenant(ctx context.Context, id string) (*types.MCPService, error) {
	var service types.MCPService
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&service).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &service, nil
}

// GetByID retrieves an MCP service by ID if it is visible to the given
// workspace (assigned or builtin). Builtin MCP services are visible to all
// workspaces. tenantID == 0 means platform scope (any row).
func (r *mcpServiceRepository) GetByID(ctx context.Context, tenantID uint64, id string) (*types.MCPService, error) {
	var service types.MCPService
	err := visibleTenantScope(r.db.WithContext(ctx), tenantID).
		Where("id = ?", id).
		First(&service).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return &service, nil
}

// List retrieves the MCP services visible to a tenant (assigned + builtin).
func (r *mcpServiceRepository) List(ctx context.Context, tenantID uint64) ([]*types.MCPService, error) {
	var services []*types.MCPService
	err := visibleTenantScope(r.db.WithContext(ctx), tenantID).
		Order("created_at DESC").
		Find(&services).Error
	if err != nil {
		return nil, err
	}

	return services, nil
}

// ListEnabled retrieves the enabled MCP services visible to a tenant.
func (r *mcpServiceRepository) ListEnabled(ctx context.Context, tenantID uint64) ([]*types.MCPService, error) {
	var services []*types.MCPService
	err := visibleTenantScope(r.db.WithContext(ctx), tenantID).
		Where("enabled = ?", true).
		Order("created_at DESC").
		Find(&services).Error
	if err != nil {
		return nil, err
	}

	return services, nil
}

// ListByIDs retrieves MCP services by multiple IDs for a tenant.
func (r *mcpServiceRepository) ListByIDs(
	ctx context.Context,
	tenantID uint64,
	ids []string,
) ([]*types.MCPService, error) {
	if len(ids) == 0 {
		return []*types.MCPService{}, nil
	}

	var services []*types.MCPService
	err := visibleTenantScope(r.db.WithContext(ctx), tenantID).
		Where("id IN ?", ids).
		Find(&services).Error
	if err != nil {
		return nil, err
	}

	return services, nil
}

// ListAll lists every platform service (admin console catalog).
func (r *mcpServiceRepository) ListAll(ctx context.Context) ([]*types.MCPService, error) {
	var services []*types.MCPService
	err := r.db.WithContext(ctx).
		Order("created_at DESC").
		Find(&services).Error
	if err != nil {
		return nil, err
	}
	return services, nil
}

// Update updates an MCP service. Rows are platform-owned since the 000096
// rework, so the update keys on ID alone; the tenantID visibility check is
// the caller's (service layer) responsibility.
func (r *mcpServiceRepository) Update(ctx context.Context, service *types.MCPService) error {
	// Build update map with only non-zero fields (except enabled which should always be updated if set)
	updateMap := make(map[string]interface{})
	updateMap["updated_at"] = service.UpdatedAt

	// Always include enabled field if it's being updated (service layer ensures it's set correctly)
	updateMap["enabled"] = service.Enabled

	if service.Name != "" {
		updateMap["name"] = service.Name
	}
	// Description can be empty, so we check if it's different from existing
	// For now, we'll always update it if provided
	updateMap["description"] = service.Description
	// Category follows the same always-write pattern: the service layer has
	// already merged the incoming value into `existing` (presence-map
	// guarded), so what lands here is the intended final value.
	updateMap["category"] = service.Category

	if service.TransportType != "" {
		updateMap["transport_type"] = service.TransportType
	}
	if service.URL != nil {
		updateMap["url"] = *service.URL
	}
	if service.StdioConfig != nil {
		updateMap["stdio_config"] = service.StdioConfig
	}
	if service.EnvVars != nil {
		updateMap["env_vars"] = service.EnvVars
	}
	if service.Headers != nil {
		updateMap["headers"] = service.Headers
	}
	if service.AuthConfig != nil {
		updateMap["auth_config"] = service.AuthConfig
	}
	if service.AdvancedConfig != nil {
		updateMap["advanced_config"] = service.AdvancedConfig
	}

	return r.db.WithContext(ctx).
		Model(&types.MCPService{}).
		Where("id = ?", service.ID).
		Updates(updateMap).Error
}

// Delete soft-deletes an MCP service by ID. Rows are platform-owned since
// the 000096 rework; the caller (service layer) enforces tenant visibility
// before reaching this point.
func (r *mcpServiceRepository) Delete(ctx context.Context, _ uint64, id string) error {
	return r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&types.MCPService{}).Error
}

// ListAssignmentInfos lists a service's assignments joined with workspace
// names (admin console "assigned workspaces" list).
func (r *mcpServiceRepository) ListAssignmentInfos(
	ctx context.Context, serviceID string,
) ([]types.MCPServiceAssignmentInfo, error) {
	var rows []types.MCPServiceAssignmentInfo
	err := r.db.WithContext(ctx).
		Table("tenant_mcp_service_assignments AS a").
		Select("a.tenant_id, t.name AS tenant_name, a.service_id, a.assigned_at").
		Joins("LEFT JOIN tenants t ON t.id = a.tenant_id").
		Where("a.service_id = ?", serviceID).
		Order("a.assigned_at ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// ReplaceServiceAssignments atomically sets the full assignment list of a
// service: removes rows whose workspace is absent from the list and upserts
// the rest. There is no default flag for MCP services (unlike web search).
func (r *mcpServiceRepository) ReplaceServiceAssignments(
	ctx context.Context, serviceID string,
	rows []types.TenantMCPServiceAssignment, assignedBy string,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		keepIDs := make([]uint64, 0, len(rows))
		for _, row := range rows {
			keepIDs = append(keepIDs, row.TenantID)
		}
		if len(keepIDs) == 0 {
			if err := tx.Where("service_id = ?", serviceID).
				Delete(&types.TenantMCPServiceAssignment{}).Error; err != nil {
				return err
			}
		} else if err := tx.
			Where("service_id = ? AND tenant_id NOT IN ?", serviceID, keepIDs).
			Delete(&types.TenantMCPServiceAssignment{}).Error; err != nil {
			return err
		}

		for _, row := range rows {
			// gorm would insert the zero time over the column's CURRENT_TIMESTAMP
			// default; stamp first-assignment time here so the drawer's
			// assigned_at render stays meaningful.
			assignedAt := row.AssignedAt
			if assignedAt.IsZero() {
				assignedAt = time.Now().UTC()
			}
			updated := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "tenant_id"}, {Name: "service_id"}},
				DoUpdates: clause.Assignments(map[string]interface{}{
					"updated_at": gorm.Expr("CURRENT_TIMESTAMP"),
				}),
			}).Create(&types.TenantMCPServiceAssignment{
				TenantID:   row.TenantID,
				ServiceID:  serviceID,
				AssignedBy: assignedBy,
				AssignedAt: assignedAt,
			})
			if err := updated.Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteAssignmentsByServiceID removes every workspace assignment of a
// service. Called after a service is deleted so no dangling rows remain.
func (r *mcpServiceRepository) DeleteAssignmentsByServiceID(ctx context.Context, serviceID string) error {
	return r.db.WithContext(ctx).
		Where("service_id = ?", serviceID).
		Delete(&types.TenantMCPServiceAssignment{}).Error
}
