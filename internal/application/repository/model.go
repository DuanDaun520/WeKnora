package repository

import (
	"context"
	"errors"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// modelRepository implements the model repository interface
type modelRepository struct {
	db *gorm.DB
}

// NewModelRepository creates a new model repository
func NewModelRepository(db *gorm.DB) interfaces.ModelRepository {
	return &modelRepository{db: db}
}

// tenantModelVisibility scopes a models query to what `tenantID` may see:
// its own (legacy pre-000094) rows, builtin rows, and rows explicitly
// assigned via tenant_model_assignments. Since 000094 non-builtin rows are
// platform-owned (tenant_id = 0), the assignment subquery is the live
// sharing mechanism and the tenant_id term is a legacy safety net.
func tenantModelVisibility(db *gorm.DB, tenantID uint64) *gorm.DB {
	return db.Where(
		"(tenant_id = ? OR is_builtin = true OR id IN ("+
			"SELECT model_id FROM tenant_model_assignments WHERE tenant_id = ?))",
		tenantID, tenantID,
	)
}

// Create creates a new model
func (r *modelRepository) Create(ctx context.Context, m *types.Model) error {
	return r.db.WithContext(ctx).Create(m).Error
}

// GetByID retrieves a model by ID
func (r *modelRepository) GetByID(ctx context.Context, tenantID uint64, id string) (*types.Model, error) {
	var m types.Model
	if err := tenantModelVisibility(r.db.WithContext(ctx).Where("id = ?", id), tenantID).
		First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

// GetByIDAnyTenant retrieves a model by ID with no workspace visibility
// filter. System-admin console surface only (models are platform-managed
// since 000094); regular request paths must go through GetByID.
func (r *modelRepository) GetByIDAnyTenant(ctx context.Context, id string) (*types.Model, error) {
	var m types.Model
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

// List lists models with optional filtering
func (r *modelRepository) List(
	ctx context.Context, tenantID uint64, modelType types.ModelType, source types.ModelSource,
) ([]*types.Model, error) {
	var models []*types.Model
	query := tenantModelVisibility(r.db.WithContext(ctx), tenantID)

	if modelType != "" {
		query = query.Where("type = ?", modelType)
	}

	if source != "" {
		query = query.Where("source = ?", source)
	}

	if err := query.Find(&models).Error; err != nil {
		return nil, err
	}

	return models, nil
}

// ListAll lists every model in the platform catalog (builtin included),
// optionally filtered by type/source and a case-insensitive name /
// display-name substring. System-admin console surface only.
func (r *modelRepository) ListAll(
	ctx context.Context, modelType types.ModelType, source types.ModelSource, nameQuery string,
) ([]*types.Model, error) {
	var models []*types.Model
	query := r.db.WithContext(ctx)

	if modelType != "" {
		query = query.Where("type = ?", modelType)
	}
	if source != "" {
		query = query.Where("source = ?", source)
	}
	if nameQuery != "" {
		like := "%" + strings.ToLower(nameQuery) + "%"
		query = query.Where("(LOWER(name) LIKE ? OR LOWER(display_name) LIKE ?)", like, like)
	}

	if err := query.Order("is_builtin DESC, type, name").Find(&models).Error; err != nil {
		return nil, err
	}
	return models, nil
}

// Update updates a model
func (r *modelRepository) Update(ctx context.Context, m *types.Model) error {
	// Use Select to explicitly update all fields, including zero values like false
	return r.db.WithContext(ctx).Model(&types.Model{}).Where(
		"id = ?", m.ID,
	).Select("*").Updates(m).Error
}

// Delete deletes a model owned by the given workspace (soft delete)
func (r *modelRepository) Delete(ctx context.Context, tenantID uint64, id string) error {
	return r.db.WithContext(ctx).Where(
		"id = ? AND tenant_id = ?", id, tenantID,
	).Delete(&types.Model{}).Error
}

// DeleteAnyTenant deletes a platform model regardless of owner workspace.
// System-admin console surface only.
func (r *modelRepository) DeleteAnyTenant(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&types.Model{}).Error
}

// ClearDefaultByType clears the default flag for all models of a specific type
// This is a batch operation that updates all matching records in one query
func (r *modelRepository) ClearDefaultByType(
	ctx context.Context,
	tenantID uint,
	modelType types.ModelType,
	excludeID string,
) error {
	query := r.db.WithContext(ctx).Model(&types.Model{}).Where(
		"tenant_id = ? AND type = ? AND is_default = ?", tenantID, modelType, true,
	)

	// If excludeID is provided, exclude that model from the update
	if excludeID != "" {
		query = query.Where("id != ?", excludeID)
	}

	// Batch update: set is_default to false for all matching records
	return query.Update("is_default", false).Error
}

// CountModelUsages counts rows that reference modelID. tenantID > 0 scopes
// the count to one workspace (assignment-removal guard); tenantID == 0
// counts across the whole platform (model-deletion guard). memoryCount
// probes tenant memory_config pins in addition to KB / agent references.
func (r *modelRepository) CountModelUsages(
	ctx context.Context, tenantID uint64, modelID string,
) (kbCount, agentCount, memoryCount int64, err error) {
	kbQuery := scopeKnowledgeBasesByModelID(r.db.WithContext(ctx).Model(&types.KnowledgeBase{}), modelID)
	agentQuery := scopeCustomAgentsByModelID(r.db.WithContext(ctx).Model(&types.CustomAgent{}), modelID)
	if tenantID > 0 {
		kbQuery = kbQuery.Where("tenant_id = ?", tenantID)
		agentQuery = agentQuery.Where("tenant_id = ?", tenantID)
	}
	if err = kbQuery.Count(&kbCount).Error; err != nil {
		return
	}
	if err = agentQuery.Count(&agentCount).Error; err != nil {
		return
	}

	memoryQuery := r.db.WithContext(ctx).Model(&types.Tenant{})
	if r.db.Dialector.Name() == "postgres" {
		memoryQuery = memoryQuery.Where(
			"memory_config->>'embedding_model_id' = ? OR memory_config->>'extract_model_id' = ?",
			modelID, modelID,
		)
	} else {
		memoryQuery = memoryQuery.Where(
			"json_extract(memory_config, '$.embedding_model_id') = ? OR "+
				"json_extract(memory_config, '$.extract_model_id') = ?",
			modelID, modelID,
		)
	}
	if err = memoryQuery.Count(&memoryCount).Error; err != nil {
		return
	}
	return kbCount, agentCount, memoryCount, nil
}

// DeleteAssignmentsByModelID removes every workspace assignment of a model.
// Called after a platform model is deleted so no dangling rows remain.
func (r *modelRepository) DeleteAssignmentsByModelID(ctx context.Context, modelID string) error {
	return r.db.WithContext(ctx).Where("model_id = ?", modelID).
		Delete(&types.TenantModelAssignment{}).Error
}

// ListAssignedModelIDs returns the IDs of the models assigned to a workspace.
func (r *modelRepository) ListAssignedModelIDs(ctx context.Context, tenantID uint64) ([]string, error) {
	var ids []string
	err := r.db.WithContext(ctx).Model(&types.TenantModelAssignment{}).
		Where("tenant_id = ?", tenantID).
		Order("assigned_at").Pluck("model_id", &ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// ReplaceAssignments atomically sets the full assignment list of a
// workspace: removes rows whose model is absent from modelIDs and inserts
// the missing ones. Rows are hard-deleted (the audit log is the history of
// record), so a plain UNIQUE (tenant_id, model_id) plus OnConflict DoNothing
// keeps the operation idempotent.
func (r *modelRepository) ReplaceAssignments(
	ctx context.Context, tenantID uint64, modelIDs []string, assignedBy string,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(modelIDs) == 0 {
			return tx.Where("tenant_id = ?", tenantID).Delete(&types.TenantModelAssignment{}).Error
		}
		if err := tx.Where("tenant_id = ? AND model_id NOT IN ?", tenantID, modelIDs).
			Delete(&types.TenantModelAssignment{}).Error; err != nil {
			return err
		}
		rows := make([]types.TenantModelAssignment, 0, len(modelIDs))
		for _, id := range modelIDs {
			rows = append(rows, types.TenantModelAssignment{
				TenantID:   tenantID,
				ModelID:    id,
				AssignedBy: assignedBy,
			})
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
	})
}
