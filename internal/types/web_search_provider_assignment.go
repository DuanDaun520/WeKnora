package types

import "time"

// TenantWebSearchProviderAssignment records that a platform-owned web search
// service (000095 platform rework) is shared with a workspace. A workspace
// sees exactly the services it is assigned; IsDefault marks the service used
// when neither the agent pins one nor a default exists (the effective default
// falls back to the earliest assignment). Rows are hard-deleted on removal —
// the audit log (system.web_search_provider_assigned) is the history of
// record — so a plain UNIQUE (tenant_id, provider_id) is enough.
type TenantWebSearchProviderAssignment struct {
	ID uint64 `gorm:"primaryKey" json:"id"`
	// Workspace the search service is assigned to.
	TenantID uint64 `json:"tenant_id"`
	// ID of the shared service (web_search_providers.id, platform-owned).
	ProviderID string `gorm:"type:varchar(36)" json:"provider_id"`
	// Workspace-level default: at most one per tenant, enforced by the
	// repository when assignments are replaced.
	IsDefault bool `json:"is_default"`
	// Actor that granted the assignment; "system" for the 000095 backfill.
	AssignedBy string    `gorm:"type:varchar(64)" json:"assigned_by"`
	AssignedAt time.Time `json:"assigned_at"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// TableName distinguishes the assignment mapping from the provider entity.
func (TenantWebSearchProviderAssignment) TableName() string {
	return "tenant_web_search_provider_assignments"
}

// WebSearchProviderAssignmentInfo is the admin-console view of one assignment
// row, joined with the workspace name so the service drawer can render the
// "assigned workspaces" list without a second round-trip.
type WebSearchProviderAssignmentInfo struct {
	TenantID   uint64    `json:"tenant_id"`
	TenantName string    `json:"tenant_name"`
	ProviderID string    `json:"provider_id"`
	IsDefault  bool      `json:"is_default"`
	AssignedAt time.Time `json:"assigned_at"`
}
