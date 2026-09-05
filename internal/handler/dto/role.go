package dto

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// RoleFromContext returns the caller's tenant role from ctx.
func RoleFromContext(ctx context.Context) types.TenantRole {
	return types.TenantRoleFromContext(ctx)
}

// CanViewIntegrationSecrets is true for Admin+ tenant members, for system
// admins (the admin console reads the integration KV keys through
// X-Tenant-ID with a Viewer tenant role — 按空间代管), and for API keys with
// full tenant access or the manage_tenant_settings capability.
func CanViewIntegrationSecrets(ctx context.Context) bool {
	if types.IsSystemAdminFromContext(ctx) {
		return true
	}
	if RoleFromContext(ctx).HasPermission(types.TenantRoleAdmin) {
		return true
	}
	return apiKeyCanManageIntegrationSecrets(ctx)
}

// CanManageIntegrationSecrets is the write-side counterpart for the
// integration KV keys (parser-engine-config / storage-engine-config /
// web-search-config). Since the admin-console 按空间代管 migration workspace
// admins lost these writes: only system admins and API keys (full access or
// manage_tenant_settings — API-key sessions never carry the system-admin
// context key, so their branch must survive) pass.
func CanManageIntegrationSecrets(ctx context.Context) bool {
	if types.IsSystemAdminFromContext(ctx) {
		return true
	}
	return apiKeyCanManageIntegrationSecrets(ctx)
}

func apiKeyCanManageIntegrationSecrets(ctx context.Context) bool {
	scope, ok := types.TenantAPIKeyScopeFromContext(ctx)
	if !ok {
		return false
	}
	if scope.FullAccess {
		return true
	}
	return scope.HasCapability(types.APIKeyCapabilityManageTenantSettings)
}
