package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// WebSearchProviderRepository defines the repository interface for web search
// provider storage.
//
// Since the 000095 platform rework provider rows are platform-owned
// (tenant_id = 0): a search service is configured once and shared with
// workspaces through tenant_web_search_provider_assignments. The tenantID
// arguments below therefore mean "resolve through the workspace's
// assignments", not "filter by the provider's tenant_id column".
type WebSearchProviderRepository interface {
	// Create creates a new platform web search provider
	Create(ctx context.Context, provider *types.WebSearchProviderEntity) error
	// GetByIDAnyTenant loads a provider by ID regardless of workspace
	// (platform catalog access for the admin console).
	GetByIDAnyTenant(ctx context.Context, id string) (*types.WebSearchProviderEntity, error)
	// GetByID retrieves a provider by ID if it is assigned to the given
	// workspace, or nil when the workspace cannot see it.
	GetByID(ctx context.Context, tenantID uint64, id string) (*types.WebSearchProviderEntity, error)
	// GetDefault resolves the workspace's effective default provider: the
	// assignment flagged is_default, else the earliest assignment, else nil.
	GetDefault(ctx context.Context, tenantID uint64) (*types.WebSearchProviderEntity, error)
	// List lists the providers assigned to a workspace; each entity's
	// IsDefault carries the workspace-effective default so chat readiness
	// and pickers keep working unchanged.
	List(ctx context.Context, tenantID uint64) ([]*types.WebSearchProviderEntity, error)
	// ListAll lists every platform provider (admin console catalog).
	ListAll(ctx context.Context) ([]*types.WebSearchProviderEntity, error)
	// Update updates a platform web search provider by ID
	Update(ctx context.Context, provider *types.WebSearchProviderEntity) error
	// Delete soft-deletes a platform web search provider by ID
	Delete(ctx context.Context, id string) error
	// ListAssignmentInfos lists a provider's assignments joined with
	// workspace names (admin console drawer).
	ListAssignmentInfos(ctx context.Context, providerID string) ([]types.WebSearchProviderAssignmentInfo, error)
	// ReplaceProviderAssignments atomically sets the full assignment list of
	// a provider, enforcing at most one default per workspace across all
	// providers.
	ReplaceProviderAssignments(
		ctx context.Context, providerID string,
		rows []types.TenantWebSearchProviderAssignment, assignedBy string,
	) error
	// DeleteAssignmentsByProviderID removes every workspace assignment of a
	// provider. Called after a provider is deleted so no dangling rows remain.
	DeleteAssignmentsByProviderID(ctx context.Context, providerID string) error
}

// WebSearchProviderService defines the service interface for web search
// provider management. All methods operate on platform-owned rows; the
// workspace relationship is expressed exclusively through assignments.
type WebSearchProviderService interface {
	// CreateProvider creates a new platform web search provider
	// (TenantID is forced to 0).
	CreateProvider(ctx context.Context, provider *types.WebSearchProviderEntity) error
	// UpdateProvider updates an existing platform provider by ID.
	UpdateProvider(ctx context.Context, provider *types.WebSearchProviderEntity) error
	// DeleteProvider soft-deletes a provider and purges its assignments.
	DeleteProvider(ctx context.Context, id string) error

	// UpdateProviderCredentials writes one or more credential fields.
	// apiKey nil means "do not touch"; empty string is a no-op (clearing
	// goes through ClearProviderCredential). Returns the updated entity.
	UpdateProviderCredentials(
		ctx context.Context, id string, apiKey *string,
	) (*types.WebSearchProviderEntity, error)
	// ClearProviderCredential removes a single credential field. Currently
	// only "api_key" is recognized. Idempotent on already-empty fields.
	ClearProviderCredential(ctx context.Context, id, field string) error

	// ListProviderAssignments returns a provider's assignments with
	// workspace names.
	ListProviderAssignments(ctx context.Context, providerID string) ([]types.WebSearchProviderAssignmentInfo, error)
	// SetProviderAssignments replaces the full assignment list of a
	// provider. At most one default per workspace is enforced.
	SetProviderAssignments(
		ctx context.Context, providerID string,
		rows []types.TenantWebSearchProviderAssignment, assignedBy string,
	) error
}
