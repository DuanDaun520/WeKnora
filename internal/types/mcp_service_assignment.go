package types

import "time"

// TenantMCPServiceAssignment records that a platform-owned MCP service
// (000096 platform rework) is shared with a workspace. A workspace sees
// exactly the non-builtin services it is assigned plus the is_builtin rows
// that remain visible to everyone. Unlike web search there is no per-workspace
// "default service" concept, so no is_default flag exists. Rows are
// hard-deleted on removal — the audit log
// (system.mcp_service_tenants_assigned) is the history of record — so a plain
// UNIQUE (tenant_id, service_id) is enough.
type TenantMCPServiceAssignment struct {
	ID uint64 `gorm:"primaryKey" json:"id"`
	// Workspace the MCP service is assigned to.
	TenantID uint64 `json:"tenant_id"`
	// ID of the shared service (mcp_services.id, platform-owned).
	ServiceID string `gorm:"type:varchar(36)" json:"service_id"`
	// Actor that granted the assignment; "system" for the 000096 backfill.
	AssignedBy string    `gorm:"type:varchar(64)" json:"assigned_by"`
	AssignedAt time.Time `json:"assigned_at"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// TableName distinguishes the assignment mapping from the service entity.
func (TenantMCPServiceAssignment) TableName() string {
	return "tenant_mcp_service_assignments"
}

// MCPServiceAssignmentInfo is the admin-console view of one assignment row,
// joined with the workspace name so the service drawer can render the
// "assigned workspaces" list without a second round-trip.
type MCPServiceAssignmentInfo struct {
	TenantID   uint64    `json:"tenant_id"`
	TenantName string    `json:"tenant_name"`
	ServiceID  string    `json:"service_id"`
	AssignedAt time.Time `json:"assigned_at"`
}
