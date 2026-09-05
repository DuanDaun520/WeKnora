package types

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
)

// UserPreferences holds per-user preferences persisted server-side
// so they sync across devices/browsers. Fields are pointers so we can
// distinguish "client didn't send this key" (leave existing value alone)
// from "client explicitly set false" — the partial-update merge in
// UpdateUserPreferences relies on this.
//
// Adding a new preference key:
//  1. Add a *T field below + JSON tag (snake_case, must match the front-end key).
//  2. Extend the merge logic in service.UserService.UpdateUserPreferences.
//  3. Surface the new knob in the frontend settings store.
//
// No DB DDL is required — preferences is a single jsonb column.
type UserPreferences struct {
	// LastActiveTenantID remembers the last workspace the user actively
	// switched into, so a fresh login (new device, cleared browser, new
	// refresh token) lands them back in that workspace instead of always
	// bouncing to their home workspace. Written by the SPA's preferences
	// PUT and by service-level SwitchTenant (including when switching
	// home, which stores the home ID). Login / RefreshToken validate that
	// the workspace still exists and the user still has an active membership
	// (or CanAccessAllTenants) before honouring this preference; an
	// invalid pointer is best-effort cleared and the user falls back to
	// home. Refresh JWT claims have no tenant_id, so RefreshToken
	// re-resolves from this field.
	//
	// nil  = no preference (use user.TenantID, i.e. home)
	// *0   = "clear preference" sentinel for the partial-update endpoint
	//        (UpdateUserPreferences turns this into nil). Otherwise treat
	//        a stored *0 the same as nil.
	// *N   = preferred workspace id.
	LastActiveTenantID *uint64 `json:"last_active_tenant_id,omitempty"`
}

// Value implements driver.Valuer so GORM persists UserPreferences as
// JSON text (Postgres jsonb column / SQLite TEXT). Empty struct serialises
// to "{}", matching the NOT NULL DEFAULT '{}' column constraint.
func (p UserPreferences) Value() (driver.Value, error) {
	return json.Marshal(p)
}

// Scan implements sql.Scanner so GORM can hydrate UserPreferences back
// from the underlying column. Accept []byte (Postgres jsonb / SQLite blob)
// and string (some drivers hand TEXT as string) for portability.
func (p *UserPreferences) Scan(value interface{}) error {
	if value == nil {
		*p = UserPreferences{}
		return nil
	}
	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return errors.New("UserPreferences.Scan: unsupported type")
	}
	if len(data) == 0 {
		*p = UserPreferences{}
		return nil
	}
	return json.Unmarshal(data, p)
}

// User represents a user in the system
type User struct {
	// Unique identifier of the user
	ID string `json:"id"         gorm:"type:varchar(36);primaryKey"`
	// Employee ID — the primary account identifier and login name.
	// Unique among non-soft-deleted rows (partial unique index created by
	// migration 000091 / sqlite 000013; the gorm tag stays a plain index so
	// AutoMigrate never creates a full unique index that would block
	// employee-id reuse after soft delete).
	EmployeeID string `json:"employee_id" gorm:"type:varchar(64);index"`
	// Display name (real name) of the user. NOT unique — real names collide.
	Username string `json:"username"   gorm:"type:varchar(100);not null"`
	// Email address of the user (optional contact field; NOT a login
	// identifier, NOT unique)
	Email string `json:"email"      gorm:"type:varchar(255);not null"`
	// Hashed password of the user
	PasswordHash string `json:"-"          gorm:"type:varchar(255);not null"`
	// Avatar URL of the user
	Avatar string `json:"avatar"     gorm:"type:varchar(500)"`
	// Workspace ID that the user belongs to
	TenantID uint64 `json:"tenant_id"  gorm:"index"`
	// Whether the user is active
	IsActive bool `json:"is_active"  gorm:"default:true"`
	// Whether the user can access all workspaces (cross-workspace access)
	CanAccessAllTenants bool `json:"can_access_all_tenants" gorm:"default:false"`
	// Whether the user is a system administrator (independent of workspace roles)
	IsSystemAdmin bool `json:"is_system_admin" gorm:"default:false;index"`
	// MustChangePassword forces the user to rotate the password (admin-set
	// initial password or admin reset). While true, the auth middleware
	// rejects every request except the password-change allowlist.
	MustChangePassword bool `json:"must_change_password" gorm:"default:false"`
	// Per-user UI/feature preferences.
	// Stored as JSON (jsonb on Postgres, TEXT on SQLite) via the
	// driver.Valuer / sql.Scanner methods on UserPreferences.
	Preferences UserPreferences `json:"preferences" gorm:"type:jsonb;not null;default:'{}'"`
	// LastLoginAt records the most recent successful login (000101). NULL
	// means the user has not logged in since the column was introduced —
	// the member-management UI uses this to render 最后登录时间 "-” and to
	// keep the 邀请 (invite) affordance enabled. Updated best-effort by the
	// login path; a failed write never fails the login itself.
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	// Creation time of the user
	CreatedAt time.Time `json:"created_at"`
	// Last updated time of the user
	UpdatedAt time.Time `json:"updated_at"`
	// Deletion time of the user
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Association relationship, not stored in the database
	Tenant *Tenant `json:"tenant,omitempty" gorm:"foreignKey:TenantID"`
}

// AuthToken represents an authentication token
type AuthToken struct {
	// Unique identifier of the token
	ID string `json:"id"         gorm:"type:varchar(36);primaryKey"`
	// User ID that owns this token
	UserID string `json:"user_id"    gorm:"type:varchar(36);index;not null"`
	// Token value (JWT or other format)
	Token string `json:"token"      gorm:"type:text;not null"`
	// Token type (access_token, refresh_token)
	TokenType string `json:"token_type" gorm:"type:varchar(50);not null"`
	// Token expiration time
	ExpiresAt time.Time `json:"expires_at"`
	// Whether the token is revoked
	IsRevoked bool `json:"is_revoked" gorm:"default:false"`
	// Creation time of the token
	CreatedAt time.Time `json:"created_at"`
	// Last updated time of the token
	UpdatedAt time.Time `json:"updated_at"`

	// Association relationship
	User *User `json:"user,omitempty" gorm:"foreignKey:UserID"`
}

// LoginRequest represents a login request. Employee ID (工号) is the only
// supported login identifier; email login is intentionally removed.
type LoginRequest struct {
	EmployeeID string `json:"employee_id" binding:"required"`
	Password   string `json:"password"    binding:"required,min=6"`
}

// AdminCreateUserRequest is the payload for a SystemAdmin provisioning a
// new local user via POST /api/v1/system/admin/users(/create).
//
// EmployeeID (工号) is required and immutable after creation. Password is
// optional: when absent (or null), the service generates a random one and
// returns it exactly once; regardless of origin the new user starts with
// MustChangePassword=true. Email is an optional contact field.
type AdminCreateUserRequest struct {
	EmployeeID string  `json:"employee_id" binding:"required,min=1,max=64"`
	Username   string  `json:"username"    binding:"required,min=2,max=50"`
	Password   *string `json:"password"`
	Email      *string `json:"email" binding:"omitempty,email,max=255"`
}

// LoginResponse represents a login response
type LoginResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	User    *User  `json:"user,omitempty"`
	// ActiveTenant is the workspace whose ID is encoded in the issued JWT;
	// future requests are scoped to it until the client calls /auth/switch-tenant.
	// Defaults to the user's home workspace on a fresh login.
	ActiveTenant *Tenant `json:"active_tenant,omitempty"`
	// Memberships lists every workspace the user can authenticate into,
	// along with their role in each. Always populated (length 1 for users
	// who only belong to their home workspace) so frontends can render a
	// workspace switcher without a follow-up request. Serialised without
	// omitempty so the field is always present as a JSON array (possibly
	// empty) — the "always populated" contract relies on the server side
	// guaranteeing a non-nil slice.
	Memberships  []Membership `json:"memberships"`
	Token        string       `json:"token,omitempty"`
	RefreshToken string       `json:"refresh_token,omitempty"`
	// MustChangePassword is true when the issued credentials ride on an
	// admin-set password (initial or reset) that the user must rotate; the
	// auth middleware blocks non-identity endpoints until they do.
	MustChangePassword bool `json:"must_change_password,omitempty"`
}

// UserInfo represents user information for API responses
type UserInfo struct {
	ID                  string          `json:"id"`
	EmployeeID          string          `json:"employee_id"`
	Username            string          `json:"username"`
	Email               string          `json:"email"`
	Avatar              string          `json:"avatar"`
	TenantID            uint64          `json:"tenant_id"`
	IsActive            bool            `json:"is_active"`
	CanAccessAllTenants bool            `json:"can_access_all_tenants"`
	IsSystemAdmin       bool            `json:"is_system_admin"`
	MustChangePassword  bool            `json:"must_change_password"`
	Preferences         UserPreferences `json:"preferences"`
	LastLoginAt         *time.Time      `json:"last_login_at,omitempty"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

// ToUserInfo converts User to UserInfo (without sensitive data)
func (u *User) ToUserInfo() *UserInfo {
	return &UserInfo{
		ID:                  u.ID,
		EmployeeID:          u.EmployeeID,
		Username:            u.Username,
		Email:               u.Email,
		Avatar:              u.Avatar,
		TenantID:            u.TenantID,
		IsActive:            u.IsActive,
		CanAccessAllTenants: u.CanAccessAllTenants,
		IsSystemAdmin:       u.IsSystemAdmin,
		MustChangePassword:  u.MustChangePassword,
		Preferences:         u.Preferences,
		CreatedAt:           u.CreatedAt,
		UpdatedAt:           u.UpdatedAt,
	}
}
