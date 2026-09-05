package service

import (
	"context"
	"errors"
	"testing"

	infra_web_search "github.com/Tencent/WeKnora/internal/infrastructure/web_search"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

// platformWebSearchRepoFake records the assignment mutations the service layer
// drives so the tests assert what was handed to the repository (the repository
// semantics themselves are covered by the repository package's sqlite tests).
type platformWebSearchRepoFake struct {
	providers map[string]*types.WebSearchProviderEntity

	getByIDArgTenant uint64
	getByIDArgID     string
	getByIDResult    *types.WebSearchProviderEntity

	getDefaultArgTenant uint64
	defaultResult       *types.WebSearchProviderEntity

	deletedIDs []string
	purgedIDs  []string
	purgeErr   error

	replacedProviderID string
	replacedRows       []types.TenantWebSearchProviderAssignment
	replacedBy         string
}

func (f *platformWebSearchRepoFake) Create(context.Context, *types.WebSearchProviderEntity) error {
	return nil
}

func (f *platformWebSearchRepoFake) GetByID(_ context.Context, tenantID uint64, id string) (*types.WebSearchProviderEntity, error) {
	f.getByIDArgTenant = tenantID
	f.getByIDArgID = id
	return f.getByIDResult, nil
}

func (f *platformWebSearchRepoFake) GetDefault(_ context.Context, tenantID uint64) (*types.WebSearchProviderEntity, error) {
	f.getDefaultArgTenant = tenantID
	return f.defaultResult, nil
}

func (f *platformWebSearchRepoFake) List(context.Context, uint64) ([]*types.WebSearchProviderEntity, error) {
	return nil, nil
}

func (f *platformWebSearchRepoFake) GetByIDAnyTenant(_ context.Context, id string) (*types.WebSearchProviderEntity, error) {
	return f.providers[id], nil
}

func (f *platformWebSearchRepoFake) ListAll(context.Context) ([]*types.WebSearchProviderEntity, error) {
	return nil, nil
}

func (f *platformWebSearchRepoFake) Update(context.Context, *types.WebSearchProviderEntity) error { return nil }

func (f *platformWebSearchRepoFake) Delete(_ context.Context, id string) error {
	f.deletedIDs = append(f.deletedIDs, id)
	return nil
}

func (f *platformWebSearchRepoFake) ListAssignmentInfos(context.Context, string) ([]types.WebSearchProviderAssignmentInfo, error) {
	return nil, nil
}

func (f *platformWebSearchRepoFake) ReplaceProviderAssignments(
	_ context.Context, providerID string,
	rows []types.TenantWebSearchProviderAssignment, assignedBy string,
) error {
	f.replacedProviderID = providerID
	f.replacedRows = rows
	f.replacedBy = assignedBy
	return nil
}

func (f *platformWebSearchRepoFake) DeleteAssignmentsByProviderID(_ context.Context, providerID string) error {
	f.purgedIDs = append(f.purgedIDs, providerID)
	return f.purgeErr
}

func TestSetProviderAssignmentsDedupesAndForcesProviderID(t *testing.T) {
	repo := &platformWebSearchRepoFake{providers: map[string]*types.WebSearchProviderEntity{
		"svc-1": {ID: "svc-1", Provider: types.WebSearchProviderTypeTavily},
	}}
	svc := NewWebSearchProviderService(repo)

	err := svc.SetProviderAssignments(context.Background(), "svc-1", []types.TenantWebSearchProviderAssignment{
		{TenantID: 7, ProviderID: "stale", IsDefault: true},
		{TenantID: 7, IsDefault: false}, // duplicate tenant, first row wins
		{TenantID: 0, IsDefault: true},  // zero tenant dropped
		{TenantID: 8},
	}, "admin")
	require.NoError(t, err)

	require.Equal(t, "svc-1", repo.replacedProviderID)
	require.Equal(t, "admin", repo.replacedBy)
	require.Equal(t, []types.TenantWebSearchProviderAssignment{
		{TenantID: 7, ProviderID: "svc-1", IsDefault: true},
		{TenantID: 8, ProviderID: "svc-1"},
	}, repo.replacedRows)
}

func TestSetProviderAssignmentsUnknownProviderErrors(t *testing.T) {
	repo := &platformWebSearchRepoFake{providers: map[string]*types.WebSearchProviderEntity{}}
	svc := NewWebSearchProviderService(repo)

	err := svc.SetProviderAssignments(context.Background(), "ghost", []types.TenantWebSearchProviderAssignment{
		{TenantID: 7},
	}, "admin")
	require.ErrorIs(t, err, ErrWebSearchProviderNotFound)
	require.Empty(t, repo.replacedRows, "repository must not be touched for an unknown provider")
}

func TestDeleteProviderPurgesAssignments(t *testing.T) {
	repo := &platformWebSearchRepoFake{providers: map[string]*types.WebSearchProviderEntity{
		"svc-1": {ID: "svc-1"},
	}}
	svc := NewWebSearchProviderService(repo)

	require.NoError(t, svc.DeleteProvider(context.Background(), "svc-1"))
	require.Equal(t, []string{"svc-1"}, repo.deletedIDs)
	require.Equal(t, []string{"svc-1"}, repo.purgedIDs)

	// The row is already soft-deleted when the purge runs; a purge failure is
	// logged, not surfaced as a failed delete.
	repo.purgeErr = errors.New("boom")
	require.NoError(t, svc.DeleteProvider(context.Background(), "svc-1"))
}

func TestDeleteProviderUnknownErrors(t *testing.T) {
	repo := &platformWebSearchRepoFake{providers: map[string]*types.WebSearchProviderEntity{}}
	svc := NewWebSearchProviderService(repo)

	require.ErrorIs(t, svc.DeleteProvider(context.Background(), "ghost"), ErrWebSearchProviderNotFound)
}

type stubWebSearchProviderInstance struct{}

func (stubWebSearchProviderInstance) Name() string { return "stub" }
func (stubWebSearchProviderInstance) Search(context.Context, string, int, bool) ([]*types.WebSearchResult, error) {
	return nil, nil
}

// A pinned provider that was deleted or unassigned degrades to the workspace
// default instead of hard-failing the search call.
func TestResolveProviderFallsBackToWorkspaceDefault(t *testing.T) {
	registry := infra_web_search.NewRegistry()
	registry.Register(string(types.WebSearchProviderTypeDuckDuckGo), func(types.WebSearchProviderParameters) (interfaces.WebSearchProvider, error) {
		return stubWebSearchProviderInstance{}, nil
	})
	repo := &platformWebSearchRepoFake{
		// GetByID returns nil: the pinned provider is no longer assigned.
		defaultResult: &types.WebSearchProviderEntity{
			ID:       "dflt",
			Provider: types.WebSearchProviderTypeDuckDuckGo,
		},
	}
	svc := &WebSearchService{registry: registry, providerRepo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))

	provider, err := svc.resolveProvider(ctx, "pinned-gone", &types.WebSearchConfig{})
	require.NoError(t, err)
	require.NotNil(t, provider)
	require.Equal(t, uint64(7), repo.getByIDArgTenant)
	require.Equal(t, "pinned-gone", repo.getByIDArgID)
	require.Equal(t, uint64(7), repo.getDefaultArgTenant)
}

// Neither a pinned provider nor a workspace default → the caller gets the
// explicit "not configured" error.
func TestResolveProviderErrorsWithoutAnyAssignment(t *testing.T) {
	registry := infra_web_search.NewRegistry()
	svc := &WebSearchService{registry: registry, providerRepo: &platformWebSearchRepoFake{}}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))

	_, err := svc.resolveProvider(ctx, "pinned-gone", &types.WebSearchConfig{})
	require.ErrorContains(t, err, "no web search provider configured")

	_, err = svc.resolveProvider(ctx, "", &types.WebSearchConfig{})
	require.ErrorContains(t, err, "no web search provider configured")
}
