package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// MCPServiceRepository defines the interface for MCP service data access
type MCPServiceRepository interface {
	// Create creates a new MCP service
	Create(ctx context.Context, service *types.MCPService) error

	// GetByID retrieves an MCP service by ID and tenant ID. Since the 000096
	// platform rework tenantID scopes visibility (assigned or builtin
	// services only); tenantID == 0 means platform scope (any row, used by
	// the admin console and platform write paths).
	GetByID(ctx context.Context, tenantID uint64, id string) (*types.MCPService, error)

	// GetByIDAnyTenant retrieves an MCP service by ID regardless of
	// workspace — the platform catalog access used by the admin console.
	GetByIDAnyTenant(ctx context.Context, id string) (*types.MCPService, error)

	// List retrieves the MCP services visible to a tenant (assigned + builtin)
	List(ctx context.Context, tenantID uint64) ([]*types.MCPService, error)

	// ListEnabled retrieves the enabled MCP services visible to a tenant
	ListEnabled(ctx context.Context, tenantID uint64) ([]*types.MCPService, error)

	// ListByIDs retrieves MCP services by multiple IDs for a tenant
	ListByIDs(ctx context.Context, tenantID uint64, ids []string) ([]*types.MCPService, error)

	// ListAll lists every platform service (admin console catalog)
	ListAll(ctx context.Context) ([]*types.MCPService, error)

	// Update updates an MCP service
	Update(ctx context.Context, service *types.MCPService) error

	// Delete deletes an MCP service (soft delete)
	Delete(ctx context.Context, tenantID uint64, id string) error

	// ListAssignmentInfos lists a service's workspace assignments joined
	// with workspace names (admin console "assigned workspaces" list).
	ListAssignmentInfos(ctx context.Context, serviceID string) ([]types.MCPServiceAssignmentInfo, error)

	// ReplaceServiceAssignments atomically sets the full assignment list of
	// a service (rows absent from the list are removed).
	ReplaceServiceAssignments(
		ctx context.Context, serviceID string,
		rows []types.TenantMCPServiceAssignment, assignedBy string,
	) error

	// DeleteAssignmentsByServiceID removes every workspace assignment of a
	// service. Called after a service is deleted so no dangling rows remain.
	DeleteAssignmentsByServiceID(ctx context.Context, serviceID string) error
}

// MCPServiceService defines the interface for MCP service business logic
type MCPServiceService interface {
	// CreateMCPService creates a new MCP service
	CreateMCPService(ctx context.Context, service *types.MCPService) error

	// GetMCPServiceByID retrieves an MCP service by ID
	GetMCPServiceByID(ctx context.Context, tenantID uint64, id string) (*types.MCPService, error)

	// ListMCPServices lists all MCP services for a tenant
	ListMCPServices(ctx context.Context, tenantID uint64) ([]*types.MCPService, error)

	// ListMCPServicesByIDs retrieves multiple MCP services by IDs
	ListMCPServicesByIDs(ctx context.Context, tenantID uint64, ids []string) ([]*types.MCPService, error)

	// UpdateMCPService updates an MCP service. updateFields records presence for
	// scalar fields whose zero values cannot represent omission. Supported keys
	// are "name", "description", and "enabled"; a nil map means none of those
	// scalar fields were provided.
	UpdateMCPService(
		ctx context.Context,
		service *types.MCPService,
		updateFields map[string]bool,
	) error

	// DeleteMCPService deletes an MCP service
	DeleteMCPService(ctx context.Context, tenantID uint64, id string) error

	// TestMCPService tests the connection to an MCP service and returns available tools/resources
	TestMCPService(ctx context.Context, tenantID uint64, id string) (*types.MCPTestResult, error)

	// GetMCPServiceTools retrieves the list of tools from an MCP service
	GetMCPServiceTools(ctx context.Context, tenantID uint64, id string) ([]*types.MCPTool, error)

	// GetMCPServiceResources retrieves the list of resources from an MCP service
	GetMCPServiceResources(ctx context.Context, tenantID uint64, id string) ([]*types.MCPResource, error)

	// UpdateMCPCredentials writes one or more credential fields on the auth
	// config. Nil pointer means "do not touch this field". Returns the updated
	// service (with current AuthConfig) so the handler can derive the
	// configured/not-configured metadata for the response.
	//
	// Implementations MUST close any active MCP client connection for this
	// service so the next upstream call reconnects with the new credential.
	UpdateMCPCredentials(
		ctx context.Context, tenantID uint64, id string, apiKey *string, token *string,
	) (*types.MCPService, error)

	// ClearMCPCredential removes a single credential field. field must be
	// "api_key" or "token"; other values must be rejected by the caller.
	// Implementations MUST close any active MCP client connection for this
	// service. Clearing a field that is already empty is a no-op (no error).
	ClearMCPCredential(ctx context.Context, tenantID uint64, id, field string) error

	// ListServiceAssignments returns a service's assignments with workspace
	// names (admin console).
	ListServiceAssignments(
		ctx context.Context, serviceID string,
	) ([]types.MCPServiceAssignmentInfo, error)

	// SetServiceAssignments replaces the full assignment list of a service.
	// Tenant existence is validated by the handler (tenantSvc).
	SetServiceAssignments(
		ctx context.Context, serviceID string,
		rows []types.TenantMCPServiceAssignment, assignedBy string,
	) error
}
