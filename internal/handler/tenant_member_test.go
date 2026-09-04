package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apprepo "github.com/Tencent/WeKnora/internal/application/repository"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// stubMemberService is a TenantMemberService whose list path can be
// overridden per-test. Embedding the interface keeps the fixture
// minimal — any method not set will nil-panic if reached, which is
// exactly what we want for "the mutation surface no longer exists"
// confidence: the enterprise rework left this handler read-only.
type stubMemberService struct {
	interfaces.TenantMemberService
	listTenant      func(ctx context.Context, tenantID uint64) ([]*types.TenantMember, error)
	listMembersPage func(ctx context.Context, tenantID uint64, query string, page, pageSize int) ([]*types.TenantMember, int64, error)
}

func (s *stubMemberService) ListMembersPage(
	ctx context.Context,
	tenantID uint64,
	query string,
	page, pageSize int,
) ([]*types.TenantMember, int64, error) {
	if s.listMembersPage != nil {
		return s.listMembersPage(ctx, tenantID, query, page, pageSize)
	}
	if s.listTenant != nil {
		members, err := s.listTenant(ctx, tenantID)
		if err != nil {
			return nil, 0, err
		}
		total := int64(len(members))
		if page < 1 {
			page = 1
		}
		if pageSize < 1 {
			pageSize = 20
		}
		off := (page - 1) * pageSize
		if off >= len(members) {
			return []*types.TenantMember{}, total, nil
		}
		end := off + pageSize
		if end > len(members) {
			end = len(members)
		}
		slice := append([]*types.TenantMember(nil), members[off:end]...)
		return slice, total, nil
	}
	return []*types.TenantMember{}, 0, nil
}

func (s *stubMemberService) ListByTenant(ctx context.Context, tenantID uint64) ([]*types.TenantMember, error) {
	return s.listTenant(ctx, tenantID)
}

// stubMemberUserService satisfies just the UserService methods the
// handler reaches: GetUsersByIDs (ListMembers hydration) and GetUserByID
// (fallback so existing tests keep working).
type stubMemberUserService struct {
	interfaces.UserService
	getByID  func(ctx context.Context, id string) (*types.User, error)
	getByIDs func(ctx context.Context, ids []string) (map[string]*types.User, error)
}

func (s *stubMemberUserService) GetUserByID(ctx context.Context, id string) (*types.User, error) {
	return s.getByID(ctx, id)
}

func (s *stubMemberUserService) GetUsersByIDs(ctx context.Context, ids []string) (map[string]*types.User, error) {
	if s.getByIDs != nil {
		return s.getByIDs(ctx, ids)
	}
	out := make(map[string]*types.User, len(ids))
	for _, id := range ids {
		u, err := s.getByID(ctx, id)
		if err != nil || u == nil {
			continue
		}
		out[u.ID] = u
	}
	return out, nil
}

// errorCapture mirrors the production error middleware so c.Error()
// shows up as a real HTTP status in the recorder. Originally defined in
// the deleted auth_register_invite_only_test.go; re-homed here because
// this file (and datasource_test.go) still rely on it.
func errorCapture() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 {
			return
		}
		if appErr, ok := c.Errors.Last().Err.(*apperrors.AppError); ok {
			c.JSON(appErr.HTTPCode, gin.H{"error": appErr})
		}
	}
}

// memberTestRouter wires the handler with the same errorCapture middleware
// production uses. It also mounts middleware.RequirePathTenantMatch on the
// /tenants/:id group, mirroring router.RegisterTenantRoutes — that's
// where the URL-vs-active-tenant cross-check lives, and the tests below
// assert it through this layer.
func memberTestRouter(h *TenantMemberHandler) *gin.Engine {
	return memberTestRouterWithCfg(h, &config.Config{
		Tenant: &config.TenantConfig{EnableCrossTenantAccess: true},
	})
}

// memberTestRouterWithCfg lets a test choose its own config (e.g.
// disabling the cross-tenant superuser carve-out) so it can assert how
// RequirePathTenantMatch behaves under different cluster flags.
func memberTestRouterWithCfg(h *TenantMemberHandler, cfg *config.Config) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(errorCapture())
	tenantByID := r.Group("/tenants/:id", middleware.RequirePathTenantMatch(cfg))
	tenantByID.GET("/members", h.ListMembers)
	return r
}

// defaultTestTenantID is what every test request is "active in" unless
// the test overrides it via doJSONWithTenant. Tenant 1 is also what the
// per-test data fixtures hard-code, so call sites stay short.
const defaultTestTenantID uint64 = 1

// memberCtxOpts lets a test override what the auth middleware would have
// stuffed into the request context. The zero value matches the common
// case ("authenticated, active in tenant 1, no superuser flag").
type memberCtxOpts struct {
	callerID   string
	tenantID   uint64
	user       *types.User
	skipTenant bool // when true, do NOT set TenantIDContextKey at all
}

// withMemberCtx installs the auth-middleware-equivalent values on req's
// context. We intentionally set the tenant ID here so the
// RequirePathTenantMatch middleware has something to compare against;
// the handler trusts that pairing to reject cross-tenant escalation.
func withMemberCtx(req *http.Request, opts memberCtxOpts) *http.Request {
	ctx := req.Context()
	if opts.callerID != "" {
		ctx = context.WithValue(ctx, types.UserIDContextKey, opts.callerID)
	}
	if !opts.skipTenant {
		tid := opts.tenantID
		if tid == 0 {
			tid = defaultTestTenantID
		}
		ctx = context.WithValue(ctx, types.TenantIDContextKey, tid)
	}
	if opts.user != nil {
		ctx = context.WithValue(ctx, types.UserContextKey, opts.user)
	}
	return req.WithContext(ctx)
}

func doJSON(t *testing.T, r *gin.Engine, method, path string, body any, callerID string) *httptest.ResponseRecorder {
	t.Helper()
	return doJSONWithCtx(t, r, method, path, body, memberCtxOpts{callerID: callerID})
}

func doJSONWithCtx(t *testing.T, r *gin.Engine, method, path string, body any, opts memberCtxOpts) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		buf, _ := json.Marshal(body)
		reader = bytes.NewReader(buf)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	req = withMemberCtx(req, opts)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ---------- ListMembers ----------

func TestTenantMember_ListMembers_HappyPath(t *testing.T) {
	now := time.Now()
	ms := &stubMemberService{
		listTenant: func(_ context.Context, tenantID uint64) ([]*types.TenantMember, error) {
			if tenantID != 1 {
				t.Fatalf("tenantID parsed wrong: got %d", tenantID)
			}
			return []*types.TenantMember{
				{UserID: "u-admin", TenantID: 1, Role: types.TenantRoleAdmin, Status: types.TenantMemberStatusActive, JoinedAt: now},
				{UserID: "u-c", TenantID: 1, Role: types.TenantRoleViewer, Status: types.TenantMemberStatusActive, JoinedAt: now},
			}, nil
		},
	}
	us := &stubMemberUserService{
		getByID: func(_ context.Context, id string) (*types.User, error) {
			return &types.User{ID: id, EmployeeID: "EMP-" + id, Username: id, Email: id + "@x.com"}, nil
		},
	}
	h := NewTenantMemberHandler(ms, us)

	w := doJSON(t, memberTestRouter(h), http.MethodGet, "/tenants/1/members", nil, "u-admin")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Members []types.TenantMemberResponse `json:"members"`
			Total   int                          `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.Total != 2 || len(resp.Data.Members) != 2 {
		t.Fatalf("expected 2 members, got total=%d len=%d", resp.Data.Total, len(resp.Data.Members))
	}
	// Hydration must populate employee_id (the primary account key) and
	// email so the roster UI can render both.
	if resp.Data.Members[0].EmployeeID == "" {
		t.Fatalf("expected hydrated employee_id, got empty")
	}
	if resp.Data.Members[0].Email == "" {
		t.Fatalf("expected hydrated email, got empty")
	}
}

func TestTenantMember_ListMembers_TolerantToDeletedUsers(t *testing.T) {
	// A dangling membership (user account deleted) must still appear in
	// the listing so an admin can spot it. The service returned the row;
	// the user lookup error is silently swallowed.
	ms := &stubMemberService{
		listTenant: func(_ context.Context, _ uint64) ([]*types.TenantMember, error) {
			return []*types.TenantMember{
				{UserID: "u-ghost", TenantID: 1, Role: types.TenantRoleAdmin, Status: types.TenantMemberStatusActive},
			}, nil
		},
	}
	us := &stubMemberUserService{
		getByID: func(_ context.Context, _ string) (*types.User, error) {
			return nil, apprepo.ErrUserNotFound
		},
	}
	h := NewTenantMemberHandler(ms, us)

	w := doJSON(t, memberTestRouter(h), http.MethodGet, "/tenants/1/members", nil, "u-admin")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 even when user lookup fails, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"user_id":"u-ghost"`) {
		t.Fatalf("dangling membership must remain in response: %s", w.Body.String())
	}
}

func TestTenantMember_ListMembers_RejectsBadTenantID(t *testing.T) {
	h := NewTenantMemberHandler(&stubMemberService{}, &stubMemberUserService{})
	w := doJSON(t, memberTestRouter(h), http.MethodGet, "/tenants/abc/members", nil, "u1")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("non-numeric tenant id must 400, got %d", w.Code)
	}
}

func TestTenantMember_ListMembers_RejectsInvalidPage(t *testing.T) {
	h := NewTenantMemberHandler(&stubMemberService{}, &stubMemberUserService{})
	w := doJSON(t, memberTestRouter(h), http.MethodGet, "/tenants/1/members?page=0", nil, "u1")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("page=0 must 400, got %d body=%s", w.Code, w.Body.String())
	}
}

// ---------- Cross-tenant guard ----------

// A user whose active tenant context is N must NOT be able to read
// tenant M's roster just by changing the URL. RequirePathTenantMatch
// rejects the pairing before the handler runs.

func TestTenantMember_RejectsCrossTenantURL_List(t *testing.T) {
	called := false
	ms := &stubMemberService{
		listTenant: func(_ context.Context, _ uint64) ([]*types.TenantMember, error) {
			called = true
			return nil, nil
		},
	}
	h := NewTenantMemberHandler(ms, &stubMemberUserService{})

	// Active tenant 1, URL targets tenant 5.
	w := doJSONWithCtx(t, memberTestRouter(h), http.MethodGet, "/tenants/5/members", nil,
		memberCtxOpts{callerID: "u1", tenantID: 1})
	if w.Code != http.StatusForbidden {
		t.Fatalf("cross-tenant URL must 403, got %d body=%s", w.Code, w.Body.String())
	}
	if called {
		t.Fatalf("service must NOT be reached when :id != active tenant")
	}
}

func TestTenantMember_CrossTenantSuperuserBypassesURLCheck(t *testing.T) {
	// CanAccessAllTenants + EnableCrossTenantAccess is the documented
	// escape hatch (mirrors middleware/rbac.go). Once flipped on, an
	// org-level operator can read any tenant's member list from any
	// session — including a session whose active tenant != URL.
	called := false
	ms := &stubMemberService{
		listTenant: func(_ context.Context, tenantID uint64) ([]*types.TenantMember, error) {
			called = true
			if tenantID != 5 {
				t.Fatalf("expected tenant 5 to reach service, got %d", tenantID)
			}
			return nil, nil
		},
	}
	us := &stubMemberUserService{
		getByID: func(_ context.Context, _ string) (*types.User, error) { return nil, apprepo.ErrUserNotFound },
	}
	h := NewTenantMemberHandler(ms, us)
	w := doJSONWithCtx(t, memberTestRouter(h), http.MethodGet, "/tenants/5/members", nil,
		memberCtxOpts{
			callerID: "u-superuser",
			tenantID: 1,
			user:     &types.User{ID: "u-superuser", CanAccessAllTenants: true},
		})
	if w.Code != http.StatusOK {
		t.Fatalf("superuser bypass must reach service, got %d body=%s", w.Code, w.Body.String())
	}
	if !called {
		t.Fatalf("service must be reached for the superuser bypass")
	}
}

func TestTenantMember_SuperuserBypassRequiresFeatureFlag(t *testing.T) {
	// Just having user.CanAccessAllTenants on the User struct is not
	// enough — the cluster operator must also have flipped
	// cfg.Tenant.EnableCrossTenantAccess. Otherwise a stale token
	// claim couldn't be revoked operationally.
	called := false
	ms := &stubMemberService{
		listTenant: func(_ context.Context, _ uint64) ([]*types.TenantMember, error) {
			called = true
			return nil, nil
		},
	}
	// Build the router with the flag explicitly off — the carve-out
	// lives in middleware.RequirePathTenantMatch, which the router
	// helper mounts.
	h := NewTenantMemberHandler(ms, &stubMemberUserService{})
	router := memberTestRouterWithCfg(h, &config.Config{
		Tenant: &config.TenantConfig{EnableCrossTenantAccess: false},
	})
	w := doJSONWithCtx(t, router, http.MethodGet, "/tenants/5/members", nil,
		memberCtxOpts{
			callerID: "u-superuser",
			tenantID: 1,
			user:     &types.User{ID: "u-superuser", CanAccessAllTenants: true},
		})
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when feature flag is off, got %d body=%s", w.Code, w.Body.String())
	}
	if called {
		t.Fatalf("service must not be reached without the feature flag")
	}
}

// ---------- Hydration ----------

func TestTenantMember_ListMembers_UsesBatchedUserLookup(t *testing.T) {
	// Regression for the N+1 finding: the handler should call
	// GetUsersByIDs exactly once, NOT GetUserByID per row.
	now := time.Now()
	ms := &stubMemberService{
		listTenant: func(_ context.Context, _ uint64) ([]*types.TenantMember, error) {
			return []*types.TenantMember{
				{UserID: "u1", TenantID: 1, Role: types.TenantRoleAdmin, Status: types.TenantMemberStatusActive, JoinedAt: now},
				{UserID: "u2", TenantID: 1, Role: types.TenantRoleAdmin, Status: types.TenantMemberStatusActive, JoinedAt: now},
				{UserID: "u3", TenantID: 1, Role: types.TenantRoleViewer, Status: types.TenantMemberStatusActive, JoinedAt: now},
			}, nil
		},
	}
	batchCalls := 0
	singleCalls := 0
	us := &stubMemberUserService{
		getByIDs: func(_ context.Context, ids []string) (map[string]*types.User, error) {
			batchCalls++
			out := map[string]*types.User{}
			for _, id := range ids {
				out[id] = &types.User{ID: id, EmployeeID: "EMP-" + id, Username: id, Email: id + "@x.com"}
			}
			return out, nil
		},
		getByID: func(_ context.Context, _ string) (*types.User, error) {
			singleCalls++
			return nil, apprepo.ErrUserNotFound
		},
	}
	h := NewTenantMemberHandler(ms, us)
	w := doJSON(t, memberTestRouter(h), http.MethodGet, "/tenants/1/members", nil, "u-admin")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if batchCalls != 1 {
		t.Fatalf("GetUsersByIDs should be called exactly once, got %d", batchCalls)
	}
	if singleCalls != 0 {
		t.Fatalf("the handler must not fall back to per-row GetUserByID, got %d calls", singleCalls)
	}
}
