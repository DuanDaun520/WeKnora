package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

// 000096 platform rework: MCP services are platform-owned rows
// (tenant_id = 0) shared with workspaces through explicit assignments. These
// tests assert the service-layer behaviors the admin console relies on.

// A created service is always platform-owned regardless of what tenant the
// calling surface had in context — visibility comes from assignments, never
// from row ownership.
func TestCreateMCPServiceForcesPlatformOwnership(t *testing.T) {
	svc, repo := newTestService()
	url := "https://example.com/sse"
	created := &types.MCPService{
		ID:            "svc-new",
		TenantID:      7, // hostile/stale context value; must be reset
		Name:          "shared",
		Enabled:       true,
		TransportType: types.MCPTransportSSE,
		URL:           &url,
	}
	require.NoError(t, svc.CreateMCPService(context.Background(), created))
	require.Equal(t, uint64(0), repo.store["svc-new"].TenantID)
	require.NotNil(t, repo.store["svc-new"].AdvancedConfig,
		"default advanced config is stamped on create")
}

// 无 URL / 无名称的行连接不到任何地方：运行时 MCP 客户端会以
// "URL is required for SSE transport" 失败，agent 静默零工具注册。创建
// 与更新都必须拒绝落库这种行（管理台抽屉的自定义表单曾放行过全空 POST）。
func TestCreateMCPServiceRejectsMissingNameOrURL(t *testing.T) {
	svc, repo := newTestService()
	url := "https://example.com/sse"

	err := svc.CreateMCPService(context.Background(), &types.MCPService{
		Name:          "no-url",
		TransportType: types.MCPTransportSSE,
	})
	require.ErrorContains(t, err, "URL is required")
	require.NotContains(t, repo.store, "svc-new", "rejected row must not be persisted")

	err = svc.CreateMCPService(context.Background(), &types.MCPService{
		Name:          "",
		TransportType: types.MCPTransportSSE,
		URL:           &url,
	})
	require.ErrorContains(t, err, "name is required")
}

func TestUpdateMCPServiceRejectsURLlessFinalState(t *testing.T) {
	svc, repo := newTestService()
	id := seedService(t, repo, "", "")

	// 显式清空 URL 的部分更新：合并后非 stdio 行无 URL，必须拒绝且不落库。
	emptyURL := ""
	err := svc.UpdateMCPService(context.Background(), &types.MCPService{
		ID:  id,
		URL: &emptyURL,
	}, map[string]bool{})
	require.ErrorContains(t, err, "URL is required")
	stored := repo.store[id]
	require.NotNil(t, stored.URL)
	require.Equal(t, "https://example.com/sse", *stored.URL)
}

func TestSetServiceAssignmentsDedupesAndForcesServiceID(t *testing.T) {
	svc, repo := newTestService()
	id := seedService(t, repo, "", "")

	err := svc.SetServiceAssignments(context.Background(), id, []types.TenantMCPServiceAssignment{
		{TenantID: 7, ServiceID: "stale"},
		{TenantID: 7}, // duplicate tenant, first row wins
		{TenantID: 0}, // zero tenant dropped
		{TenantID: 8},
	}, "admin")
	require.NoError(t, err)

	require.Equal(t, id, repo.replacedServiceID)
	require.Equal(t, "admin", repo.replacedBy)
	require.Equal(t, []types.TenantMCPServiceAssignment{
		{TenantID: 7, ServiceID: id},
		{TenantID: 8, ServiceID: id},
	}, repo.replacedRows)
}

func TestSetServiceAssignmentsUnknownServiceErrors(t *testing.T) {
	svc, repo := newTestService()

	err := svc.SetServiceAssignments(context.Background(), "ghost", []types.TenantMCPServiceAssignment{
		{TenantID: 7},
	}, "admin")
	require.ErrorIs(t, err, ErrMCPServiceNotFound)
	require.Empty(t, repo.replacedRows, "repository must not be touched for an unknown service")
}

// Builtin services are visible to every workspace by design — assignment
// management would be meaningless and is rejected.
func TestSetServiceAssignmentsRejectsBuiltin(t *testing.T) {
	svc, repo := newTestService()
	id := seedService(t, repo, "", "")
	repo.store[id].IsBuiltin = true

	err := svc.SetServiceAssignments(context.Background(), id, []types.TenantMCPServiceAssignment{
		{TenantID: 7},
	}, "admin")
	require.ErrorContains(t, err, "builtin")
	require.Empty(t, repo.replacedRows)
}

func TestDeleteMCPServicePurgesAssignments(t *testing.T) {
	svc, repo := newTestService()
	id := seedService(t, repo, "", "")

	require.NoError(t, svc.DeleteMCPService(context.Background(), 0, id))
	require.Equal(t, []string{id}, repo.purgedIDs)

	// The row is already soft-deleted when the purge runs; a purge failure is
	// logged, not surfaced as a failed delete.
	repo.purgeErr = errors.New("boom")
	repo.store[id] = &types.MCPService{ID: id, Enabled: true, TransportType: types.MCPTransportSSE}
	require.NoError(t, svc.DeleteMCPService(context.Background(), 0, id))
}

func TestDeleteMCPServiceUnknownErrors(t *testing.T) {
	svc, _ := newTestService()

	err := svc.DeleteMCPService(context.Background(), 0, "ghost")
	require.ErrorIs(t, err, ErrMCPServiceNotFound)
}
