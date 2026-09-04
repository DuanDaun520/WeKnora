package types

import "time"

// TenantModelAssignment records that a platform-owned model is shared with a
// workspace (000094 enterprise model governance). Rows are hard-deleted on
// removal — the audit log (system.tenant_models_assigned) is the history of
// record — so a plain UNIQUE (tenant_id, model_id) is enough.
type TenantModelAssignment struct {
	ID uint64 `gorm:"primaryKey" json:"id"`
	// Workspace the model is assigned to.
	TenantID uint64 `json:"tenant_id"`
	// ID of the shared model (models.id, platform-owned).
	ModelID string `gorm:"type:varchar(64)" json:"model_id"`
	// Actor that granted the assignment; "system" for the 000094 backfill.
	AssignedBy string     `gorm:"type:varchar(64)" json:"assigned_by"`
	AssignedAt time.Time  `json:"assigned_at"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}
