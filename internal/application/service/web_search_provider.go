package service

import (
	"context"
	"errors"
	"fmt"

	infra_web_search "github.com/Tencent/WeKnora/internal/infrastructure/web_search"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// ErrWebSearchProviderNotFound is returned when a platform provider ID does
// not resolve; handlers map it to 404.
var ErrWebSearchProviderNotFound = errors.New("web search provider not found")

// webSearchProviderService implements interfaces.WebSearchProviderService
type webSearchProviderService struct {
	repo interfaces.WebSearchProviderRepository
}

// NewWebSearchProviderService creates a new web search provider service
func NewWebSearchProviderService(repo interfaces.WebSearchProviderRepository) interfaces.WebSearchProviderService {
	return &webSearchProviderService{repo: repo}
}

// CreateProvider creates a new platform web search provider (000095 rework:
// rows are platform-owned, tenant_id = 0; workspaces receive the service
// through assignments).
func (s *webSearchProviderService) CreateProvider(ctx context.Context, provider *types.WebSearchProviderEntity) error {
	if !isValidProviderType(provider.Provider) {
		return fmt.Errorf("invalid provider type: %s", provider.Provider)
	}

	if err := validateProviderParameters(provider.Provider, provider.Parameters); err != nil {
		return err
	}

	provider.TenantID = 0
	provider.IsDefault = false

	logger.Infof(ctx, "Creating platform web search provider: name=%s, type=%s", provider.Name, provider.Provider)
	return s.repo.Create(ctx, provider)
}

// UpdateProvider updates an existing platform provider.
func (s *webSearchProviderService) UpdateProvider(ctx context.Context, provider *types.WebSearchProviderEntity) error {
	// Validate provider type if set
	if provider.Provider != "" && !isValidProviderType(provider.Provider) {
		return fmt.Errorf("invalid provider type: %s", provider.Provider)
	}

	if provider.Provider != "" {
		if err := validateProviderParameters(provider.Provider, provider.Parameters); err != nil {
			return err
		}
	}

	// The row-level is_default column is dead since the platform rework —
	// defaults live on assignments. Never resurrect a stale value.
	provider.TenantID = 0
	provider.IsDefault = false

	logger.Infof(ctx, "Updating platform web search provider: id=%s", provider.ID)
	return s.repo.Update(ctx, provider)
}

// UpdateProviderCredentials writes the api_key credential field. Web search
// providers are stateless from our side — every search call rebuilds a
// transport from current Parameters — so no cache invalidation is required.
func (s *webSearchProviderService) UpdateProviderCredentials(
	ctx context.Context, id string, apiKey *string,
) (*types.WebSearchProviderEntity, error) {
	existing, err := s.repo.GetByIDAnyTenant(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrWebSearchProviderNotFound
	}

	if apiKey != nil && *apiKey != "" && *apiKey != existing.Parameters.APIKey {
		existing.Parameters.APIKey = *apiKey
		if err := s.repo.Update(ctx, existing); err != nil {
			return nil, err
		}
		logger.Infof(ctx, "Platform web search provider credentials updated: id=%s", id)
	}
	return existing, nil
}

// ClearProviderCredential clears the api_key credential. Idempotent.
func (s *webSearchProviderService) ClearProviderCredential(
	ctx context.Context, id, field string,
) error {
	if field != "api_key" {
		return fmt.Errorf("unknown credential field: %s", field)
	}
	existing, err := s.repo.GetByIDAnyTenant(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrWebSearchProviderNotFound
	}
	if existing.Parameters.APIKey == "" {
		return nil
	}
	existing.Parameters.APIKey = ""
	if err := s.repo.Update(ctx, existing); err != nil {
		return err
	}
	logger.Infof(ctx, "Platform web search provider credential cleared: id=%s field=%s", id, field)
	return nil
}

// DeleteProvider soft-deletes a platform provider and purges its workspace
// assignments — agents pinning the deleted service fall back to their
// workspace default at runtime (WebSearchService.resolveProvider).
func (s *webSearchProviderService) DeleteProvider(ctx context.Context, id string) error {
	existing, err := s.repo.GetByIDAnyTenant(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrWebSearchProviderNotFound
	}
	logger.Infof(ctx, "Deleting platform web search provider: id=%s", id)
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	if err := s.repo.DeleteAssignmentsByProviderID(ctx, id); err != nil {
		// The row is gone; dangling assignments would just hide, so a purge
		// failure is logged, not surfaced as a failed delete.
		logger.Warnf(ctx, "Failed to purge assignments of deleted provider %s: %v", id, err)
	}
	return nil
}

// ListProviderAssignments returns a provider's assignments with workspace names.
func (s *webSearchProviderService) ListProviderAssignments(
	ctx context.Context, providerID string,
) ([]types.WebSearchProviderAssignmentInfo, error) {
	return s.repo.ListAssignmentInfos(ctx, providerID)
}

// SetProviderAssignments replaces the full assignment list of a provider.
// Tenant existence is validated by the handler (tenantSvc); the repository
// enforces at most one default per workspace across the whole catalog.
func (s *webSearchProviderService) SetProviderAssignments(
	ctx context.Context, providerID string,
	rows []types.TenantWebSearchProviderAssignment, assignedBy string,
) error {
	existing, err := s.repo.GetByIDAnyTenant(ctx, providerID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrWebSearchProviderNotFound
	}

	seen := make(map[uint64]bool, len(rows))
	deduped := make([]types.TenantWebSearchProviderAssignment, 0, len(rows))
	for _, row := range rows {
		if row.TenantID == 0 || seen[row.TenantID] {
			continue
		}
		seen[row.TenantID] = true
		row.ProviderID = providerID
		deduped = append(deduped, row)
	}

	logger.Infof(ctx, "Setting web search provider assignments: provider=%s, workspaces=%d", providerID, len(deduped))
	return s.repo.ReplaceProviderAssignments(ctx, providerID, deduped, assignedBy)
}

// isValidProviderType checks if the given provider type is supported
func isValidProviderType(provider types.WebSearchProviderType) bool {
	switch provider {
	case types.WebSearchProviderTypeBing,
		types.WebSearchProviderTypeGoogle,
		types.WebSearchProviderTypeDuckDuckGo,
		types.WebSearchProviderTypeTavily,
		types.WebSearchProviderTypeOllama,
		types.WebSearchProviderTypeBaidu,
		types.WebSearchProviderTypeSearxng,
		types.WebSearchProviderTypeKeenable,
		types.WebSearchProviderTypeMetaso,
		types.WebSearchProviderTypeZhipu,
		types.WebSearchProviderTypeExa,
		types.WebSearchProviderTypeSerpbase:
		return true
	default:
		return false
	}
}

// validateProviderParameters validates required parameters for each provider type
func validateProviderParameters(provider types.WebSearchProviderType, params types.WebSearchProviderParameters) error {
	switch provider {
	case types.WebSearchProviderTypeBing:
		if params.APIKey == "" {
			return fmt.Errorf("API key is required for Bing provider")
		}
	case types.WebSearchProviderTypeGoogle:
		if params.APIKey == "" {
			return fmt.Errorf("API key is required for Google provider")
		}
		if params.EngineID == "" {
			return fmt.Errorf("engine ID is required for Google provider")
		}
	case types.WebSearchProviderTypeTavily:
		if params.APIKey == "" {
			return fmt.Errorf("API key is required for Tavily provider")
		}
	case types.WebSearchProviderTypeOllama:
		if params.APIKey == "" {
			return fmt.Errorf("API key is required for Ollama provider")
		}
	case types.WebSearchProviderTypeBaidu:
		if params.APIKey == "" {
			return fmt.Errorf("API key is required for Baidu provider")
		}
	case types.WebSearchProviderTypeExa:
		if params.APIKey == "" {
			return fmt.Errorf("API key is required for Exa provider")
		}
	case types.WebSearchProviderTypeZhipu:
		if err := infra_web_search.ValidateZhipuParameters(params); err != nil {
			return err
		}
	case types.WebSearchProviderTypeMetaso:
		if err := infra_web_search.ValidateMetasoParameters(params); err != nil {
			return err
		}
	case types.WebSearchProviderTypeSerpbase:
		if err := infra_web_search.ValidateSerpbaseParameters(params); err != nil {
			return err
		}
	case types.WebSearchProviderTypeDuckDuckGo:
		// No API key required
	case types.WebSearchProviderTypeKeenable:
		// No API key required (keyless by default; an optional key lifts the rate limit)
	case types.WebSearchProviderTypeSearxng:
		if err := infra_web_search.ValidateSearxngBaseURL(params.BaseURL); err != nil {
			return err
		}
	}
	if err := validateOptionalProxyURL(params.ProxyURL); err != nil {
		return err
	}
	return nil
}

func validateOptionalProxyURL(proxyURL string) error {
	return infra_web_search.ValidateProxyURL(proxyURL)
}
