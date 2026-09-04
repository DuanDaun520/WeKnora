package dto

import "github.com/Tencent/WeKnora/internal/types"

// AuthLoginResponse is the HTTP-safe login / switch-tenant response shape.
type AuthLoginResponse struct {
	Success      bool               `json:"success"`
	Message      string             `json:"message,omitempty"`
	User         *types.User        `json:"user,omitempty"`
	ActiveTenant *TenantResponse    `json:"active_tenant,omitempty"`
	Memberships  []types.Membership `json:"memberships"`
	Token        string             `json:"token,omitempty"`
	RefreshToken string             `json:"refresh_token,omitempty"`
	// MustChangePassword mirrors User.MustChangePassword at the top level
	// so clients can detect the forced-rotation state without digging into
	// the user object.
	MustChangePassword bool `json:"must_change_password,omitempty"`
}

// NewAuthLoginResponse converts a service-layer login response for HTTP output.
func NewAuthLoginResponse(resp *types.LoginResponse) *AuthLoginResponse {
	if resp == nil {
		return nil
	}
	var role types.TenantRole
	if resp.ActiveTenant != nil {
		role = membershipRoleForTenant(resp.Memberships, resp.ActiveTenant.ID)
	}
	return &AuthLoginResponse{
		Success:            resp.Success,
		Message:            resp.Message,
		User:               resp.User,
		ActiveTenant:       NewTenantResponseWithRole(resp.ActiveTenant, role),
		Memberships:        resp.Memberships,
		Token:              resp.Token,
		RefreshToken:       resp.RefreshToken,
		MustChangePassword: resp.MustChangePassword,
	}
}

func membershipRoleForTenant(memberships []types.Membership, tenantID uint64) types.TenantRole {
	for _, m := range memberships {
		if m.TenantID == tenantID && m.Role.IsValid() {
			return m.Role
		}
	}
	return ""
}
