package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// MCPToolApprovalRepository implements interfaces.MCPToolApprovalRepository.
type MCPToolApprovalRepository struct {
	db *gorm.DB
}

// NewMCPToolApprovalRepository creates a repository backed by GORM.
func NewMCPToolApprovalRepository(db *gorm.DB) interfaces.MCPToolApprovalRepository {
	return &MCPToolApprovalRepository{db: db}
}

// ListByService returns all stored approval rows for an MCP service (may be empty).
func (r *MCPToolApprovalRepository) ListByService(ctx context.Context, tenantID uint64, serviceID string) ([]*types.MCPToolApproval, error) {
	var rows []*types.MCPToolApproval
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND service_id = ?", tenantID, serviceID).
		Order("tool_name ASC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list mcp tool approvals: %w", err)
	}
	return rows, nil
}

// IsRequired returns true when a row exists with require_approval = true.
//
// Since the 000096 platform rework the policy has two layers: a workspace row
// (tenant_id > 0) overrides the platform default row (tenant_id = 0) that the
// admin console writes for shared services. The workspace row is checked
// first; only when it is absent does the platform row apply.
func (r *MCPToolApprovalRepository) IsRequired(ctx context.Context, tenantID uint64, serviceID, toolName string) (bool, error) {
	var row types.MCPToolApproval
	err := r.db.WithContext(ctx).
		Select("require_approval").
		Where("tenant_id = ? AND service_id = ? AND tool_name = ?", tenantID, serviceID, toolName).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if tenantID == 0 {
			return false, nil
		}
		// Fall back to the platform default policy for shared services.
		var platformRow types.MCPToolApproval
		err = r.db.WithContext(ctx).
			Select("require_approval").
			Where("tenant_id = 0 AND service_id = ? AND tool_name = ?", serviceID, toolName).
			First(&platformRow).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("get mcp tool approval: %w", err)
		}
		return platformRow.RequireApproval, nil
	}
	if err != nil {
		return false, fmt.Errorf("get mcp tool approval: %w", err)
	}
	return row.RequireApproval, nil
}

// IsEnabled returns true when a tool has no stored policy row. This preserves
// the historical behaviour for tools discovered before per-tool settings.
//
// Mirrors IsRequired's two-layer 000096 platform policy: a workspace row
// (tenant_id > 0) overrides the platform default row (tenant_id = 0) that the
// admin console writes. Without the fallback, a tool the platform admin
// disabled would stay callable in every workspace without its own override.
func (r *MCPToolApprovalRepository) IsEnabled(ctx context.Context, tenantID uint64, serviceID, toolName string) (bool, error) {
	var row types.MCPToolApproval
	err := r.db.WithContext(ctx).
		Select("enabled").
		Where("tenant_id = ? AND service_id = ? AND tool_name = ?", tenantID, serviceID, toolName).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if tenantID == 0 {
			return true, nil
		}
		// Fall back to the platform default policy for shared services.
		var platformRow types.MCPToolApproval
		err = r.db.WithContext(ctx).
			Select("enabled").
			Where("tenant_id = 0 AND service_id = ? AND tool_name = ?", serviceID, toolName).
			First(&platformRow).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return true, nil
		}
		if err != nil {
			return false, fmt.Errorf("get mcp tool enabled state: %w", err)
		}
		return platformRow.Enabled, nil
	}
	if err != nil {
		return false, fmt.Errorf("get mcp tool enabled state: %w", err)
	}
	return row.Enabled, nil
}

// UpsertPolicy creates or updates only the supplied policy columns.
// Concurrent patches of different fields cannot clobber each other because
// ON CONFLICT updates omit unchanged columns. A first insert uses
// require_approval=false and enabled=true for any field the patch omits.
func (r *MCPToolApprovalRepository) UpsertPolicy(
	ctx context.Context, tenantID uint64, serviceID, toolName string, patch types.MCPToolPolicyPatch,
) error {
	if serviceID == "" || toolName == "" {
		return errors.New("service_id and tool_name are required")
	}
	if patch.RequireApproval == nil && patch.Enabled == nil {
		return errors.New("require_approval or enabled is required")
	}

	now := time.Now()
	requireApproval := false
	if patch.RequireApproval != nil {
		requireApproval = *patch.RequireApproval
	}
	enabled := true
	if patch.Enabled != nil {
		enabled = *patch.Enabled
	}

	updates := map[string]interface{}{"updated_at": now}
	if patch.RequireApproval != nil {
		updates["require_approval"] = *patch.RequireApproval
	}
	if patch.Enabled != nil {
		updates["enabled"] = *patch.Enabled
	}

	row := map[string]interface{}{
		"id":               uuid.New().String(),
		"tenant_id":        tenantID,
		"service_id":       serviceID,
		"tool_name":        toolName,
		"require_approval": requireApproval,
		"enabled":          enabled,
		"created_at":       now,
		"updated_at":       now,
	}
	// Create via map so enabled=false is persisted. GORM struct inserts omit
	// bool zero values when the field has a default tag.
	err := r.db.WithContext(ctx).Model(&types.MCPToolApproval{}).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "tenant_id"},
			{Name: "service_id"},
			{Name: "tool_name"},
		},
		DoUpdates: clause.Assignments(updates),
	}).Create(row).Error
	if err != nil {
		return fmt.Errorf("upsert mcp tool policy: %w", err)
	}
	return nil
}
