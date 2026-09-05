// Handler tests for the platform sandbox-connection surface (000097). The
// service is the real one over in-memory stores; provider-touching paths are
// avoided (no identity rotation), so nothing here reaches the network.
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

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/sandbox"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// --- stores ---

type sandboxConnHandlerRepos struct {
	connections []*types.SandboxConnectionEntity
	configRows  []*types.TenantSandboxConfigEntity
}

func (r *sandboxConnHandlerRepos) Create(
	_ context.Context, e *types.SandboxConnectionEntity,
) error {
	r.connections = append(r.connections, e)
	return nil
}

func (r *sandboxConnHandlerRepos) GetByID(
	_ context.Context, id string,
) (*types.SandboxConnectionEntity, error) {
	for _, e := range r.connections {
		if e.ID == id {
			return e, nil
		}
	}
	return nil, nil
}

func (r *sandboxConnHandlerRepos) List(
	context.Context,
) ([]*types.SandboxConnectionEntity, error) {
	return r.connections, nil
}

func (r *sandboxConnHandlerRepos) Update(
	_ context.Context, e *types.SandboxConnectionEntity,
) error {
	for i, existing := range r.connections {
		if existing.ID == e.ID {
			r.connections[i] = e
			return nil
		}
	}
	return nil
}

func (r *sandboxConnHandlerRepos) SoftDelete(_ context.Context, id string) error {
	kept := r.connections[:0]
	for _, e := range r.connections {
		if e.ID != id {
			kept = append(kept, e)
		}
	}
	r.connections = kept
	return nil
}

func (r *sandboxConnHandlerRepos) GetConfig(
	_ context.Context, tenantID uint64, id string,
) (*types.TenantSandboxConfigEntity, error) {
	for _, row := range r.configRows {
		if row.TenantID == tenantID && row.ID == id {
			return row, nil
		}
	}
	return nil, nil
}

func (r *sandboxConnHandlerRepos) ListConfigsBySource(
	_ context.Context, connectionID string,
) ([]*types.TenantSandboxConfigEntity, error) {
	var out []*types.TenantSandboxConfigEntity
	for _, row := range r.configRows {
		if row.SourceConnectionID == connectionID {
			out = append(out, row)
		}
	}
	return out, nil
}

type sandboxConnSkills struct{ byConfig map[string]int }

func (s *sandboxConnSkills) ListSkillsByConfig(
	_ context.Context, _ uint64, configID string,
) ([]*types.TenantSkillEntity, error) {
	count := s.byConfig[configID]
	if count == 0 {
		return nil, nil
	}
	out := make([]*types.TenantSkillEntity, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, &types.TenantSkillEntity{Name: "skill-" + strings.Repeat("a", i+1)})
	}
	return out, nil
}

// sandboxConnTenantService implements only what the platform surface calls;
// the nil-embedded interface panics loudly on anything else.
type sandboxConnTenantService struct {
	interfaces.TenantService
	known map[uint64]string
}

func (s *sandboxConnTenantService) GetTenantByID(
	_ context.Context, tenantID uint64,
) (*types.Tenant, error) {
	if name, ok := s.known[tenantID]; ok {
		return &types.Tenant{ID: tenantID, Name: name}, nil
	}
	return nil, nil
}

// --- fixture ---

type sandboxConnHandlerFixture struct {
	handler *SystemSandboxConnectionHandler
	repos   *sandboxConnHandlerRepos
	svc     *service.SandboxConnectionService
	skills  *sandboxConnSkills
	engine  *gin.Engine
}

// compile-time: the multi-store satisfies both repositories the service needs.
var (
	_ repository.SandboxConnectionRepository   = (*sandboxConnHandlerRepos)(nil)
	_ repository.TenantSandboxConfigRepository = (*configRepoAdapter)(nil)
	_ service.SandboxConnectionTenantReader    = (*sandboxConnTenantService)(nil)
)

func newSandboxConnHandlerFixture(t *testing.T) *sandboxConnHandlerFixture {
	t.Helper()
	t.Setenv("SYSTEM_AES_KEY", strings.Repeat("k", 32))
	fx := &sandboxConnHandlerFixture{
		repos:  &sandboxConnHandlerRepos{},
		skills: &sandboxConnSkills{byConfig: map[string]int{}},
	}
	configSvc := service.NewTenantSandboxConfigService(
		&configRepoAdapter{repos: fx.repos},
		stubSandboxAgents{},
		sandbox.DefaultConfig(), nil, nil)
	tenantSvc := &sandboxConnTenantService{known: map[uint64]string{7: "alpha", 8: "beta"}}
	fx.svc = service.NewSandboxConnectionService(
		fx.repos, &configRepoAdapter{repos: fx.repos}, configSvc,
		fx.skills, tenantSvc)
	fx.handler = NewSystemSandboxConnectionHandler(
		fx.svc,
		tenantSvc,
		nil, // audit service is optional; nil exercises the no-op path
	)
	gin.SetMode(gin.TestMode)
	fx.engine = gin.New()
	fx.engine.Use(middleware.ErrorHandler())
	admin := fx.engine.Group("/api/v1/system/admin/sandbox-connections")
	{
		admin.GET("", fx.handler.ListConnections)
		admin.POST("", fx.handler.CreateConnection)
		admin.POST("/check", fx.handler.CheckDraftConnection)
		admin.POST("/templates/query", fx.handler.QueryTemplates)
		admin.GET("/:id", fx.handler.GetConnection)
		admin.PUT("/:id", fx.handler.UpdateConnection)
		admin.DELETE("/:id", fx.handler.DeleteConnection)
		admin.POST("/:id/check", fx.handler.CheckConnection)
		admin.GET("/:id/tenant-assignments", fx.handler.ListTenantAssignments)
		admin.PUT("/:id/tenant-assignments", fx.handler.UpdateTenantAssignments)
		admin.POST("/:id/push", fx.handler.PushConnection)
	}
	return fx
}

// stubSandboxAgents satisfies the agent slice of the config service.
type stubSandboxAgents struct{}

func (stubSandboxAgents) ListNamesBySandboxConfigID(
	context.Context, uint64, string,
) ([]string, error) {
	return nil, nil
}

func (fx *sandboxConnHandlerFixture) do(
	t *testing.T, method, path string, body any,
) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(payload)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	fx.engine.ServeHTTP(w, req)
	out := map[string]any{}
	if w.Body.Len() > 0 {
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out),
			"body: %s", w.Body.String())
	}
	return w, out
}

func sandboxConnPayload(name string) map[string]any {
	return map[string]any{
		"name":        name,
		"description": "d",
		"config": map[string]any{
			"sandbox_type": "e2b",
			"e2b": map[string]any{
				"api_key":        "key-a",
				"api_url":        "https://api.e2b.app",
				"sandbox_domain": "e2b.app",
				"template_id":    "t1",
			},
		},
	}
}

func (fx *sandboxConnHandlerFixture) createConnection(
	t *testing.T, name string,
) map[string]any {
	t.Helper()
	w, body := fx.do(t, http.MethodPost, "/api/v1/system/admin/sandbox-connections",
		sandboxConnPayload(name))
	require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())
	return body["data"].(map[string]any)
}

// --- tests ---

func TestSandboxConnectionCreateBindsAndMasks(t *testing.T) {
	fx := newSandboxConnHandlerFixture(t)

	w, _ := fx.do(t, http.MethodPost, "/api/v1/system/admin/sandbox-connections",
		map[string]any{"description": "missing name"})
	require.Equal(t, http.StatusBadRequest, w.Code)

	data := fx.createConnection(t, "team-e2b")
	require.Equal(t, "team-e2b", data["name"])
	assignments, ok := data["assignments"].([]any)
	require.True(t, ok, "assignments must be present and non-null on create")
	require.Empty(t, assignments)

	cfg := data["config"].(map[string]any)
	e2b := cfg["e2b"].(map[string]any)
	require.NotEqual(t, "key-a", e2b["api_key"], "credentials must be masked")
}

func TestSandboxConnectionDuplicateNameConflicts(t *testing.T) {
	fx := newSandboxConnHandlerFixture(t)
	fx.createConnection(t, "team-e2b")

	w, body := fx.do(t, http.MethodPost, "/api/v1/system/admin/sandbox-connections",
		sandboxConnPayload("team-e2b"))
	require.Equal(t, http.StatusConflict, w.Code)
	errObj := body["error"].(map[string]any)
	require.Equal(t, "name_conflict", errObj["code"])
}

func TestSandboxConnectionUnknownIDIs404(t *testing.T) {
	fx := newSandboxConnHandlerFixture(t)

	w, _ := fx.do(t, http.MethodGet, "/api/v1/system/admin/sandbox-connections/nope", nil)
	require.Equal(t, http.StatusNotFound, w.Code)

	w, _ = fx.do(t, http.MethodPost, "/api/v1/system/admin/sandbox-connections/nope/check",
		map[string]any{"deep": false})
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestSandboxConnectionDeleteBlockedWhileAssigned(t *testing.T) {
	fx := newSandboxConnHandlerFixture(t)
	conn := fx.createConnection(t, "team-e2b")
	id := conn["id"].(string)
	w, _ := fx.do(t, http.MethodPut,
		"/api/v1/system/admin/sandbox-connections/"+id+"/tenant-assignments",
		map[string]any{"assignments": []map[string]any{{"tenant_id": 7}}})
	require.Equal(t, http.StatusOK, w.Code)

	w, body := fx.do(t, http.MethodDelete, "/api/v1/system/admin/sandbox-connections/"+id, nil)
	require.Equal(t, http.StatusConflict, w.Code)
	errObj := body["error"].(map[string]any)
	require.Equal(t, "assignments_exist", errObj["code"])
	data := errObj["data"].(map[string]any)
	require.Len(t, data["assignments"].([]any), 1)
}

func TestSandboxConnectionAssignmentsReplaceAll(t *testing.T) {
	fx := newSandboxConnHandlerFixture(t)
	conn := fx.createConnection(t, "team-e2b")
	path := "/api/v1/system/admin/sandbox-connections/" + conn["id"].(string) + "/tenant-assignments"

	// Zero and duplicate tenant IDs are bad input.
	w, _ := fx.do(t, http.MethodPut, path,
		map[string]any{"assignments": []map[string]any{{"tenant_id": 0}}})
	require.Equal(t, http.StatusBadRequest, w.Code)
	w, _ = fx.do(t, http.MethodPut, path,
		map[string]any{"assignments": []map[string]any{{"tenant_id": 7}, {"tenant_id": 7}}})
	require.Equal(t, http.StatusBadRequest, w.Code)

	// Unknown workspaces are 404 rather than silently dropped.
	w, _ = fx.do(t, http.MethodPut, path,
		map[string]any{"assignments": []map[string]any{{"tenant_id": 404}}})
	require.Equal(t, http.StatusNotFound, w.Code)

	w, body := fx.do(t, http.MethodPut, path,
		map[string]any{"assignments": []map[string]any{{"tenant_id": 7}, {"tenant_id": 8}}})
	require.Equal(t, http.StatusOK, w.Code)
	results := body["results"].([]any)
	require.Len(t, results, 2)
	for _, r := range results {
		require.Equal(t, "assigned", r.(map[string]any)["status"])
	}
	assignments := body["assignments"].([]any)
	require.Len(t, assignments, 2)
}

// A blocked removal is a per-workspace result, never a request failure: the
// other workspaces' outcomes must still be reported.
func TestSandboxConnectionUnassignBlockedBySkillsIsPerRowOutcome(t *testing.T) {
	fx := newSandboxConnHandlerFixture(t)
	conn := fx.createConnection(t, "team-e2b")
	id := conn["id"].(string)
	path := "/api/v1/system/admin/sandbox-connections/" + id + "/tenant-assignments"

	w, body := fx.do(t, http.MethodPut, path,
		map[string]any{"assignments": []map[string]any{{"tenant_id": 7}, {"tenant_id": 8}}})
	require.Equal(t, http.StatusOK, w.Code)
	rows, err := fx.repos.ListConfigsBySource(context.Background(), id)
	require.NoError(t, err)
	for _, row := range rows {
		if row.TenantID == 7 {
			fx.skills.byConfig[row.ID] = 2
		}
	}

	w, body = fx.do(t, http.MethodPut, path,
		map[string]any{"assignments": []map[string]any{{"tenant_id": 8}}})
	require.Equal(t, http.StatusOK, w.Code)
	results := body["results"].([]any)
	require.Len(t, results, 1)
	blocked := results[0].(map[string]any)
	require.Equal(t, "blocked", blocked["status"])
	require.Equal(t, "skills_installed", blocked["code"])
	require.NotEmpty(t, blocked["skill_names"])

	// The blocked workspace stays assigned.
	assignments := body["assignments"].([]any)
	require.Len(t, assignments, 2)
}

func TestSandboxConnectionPushReportsCounts(t *testing.T) {
	fx := newSandboxConnHandlerFixture(t)
	conn := fx.createConnection(t, "team-e2b")
	id := conn["id"].(string)
	w, _ := fx.do(t, http.MethodPut,
		"/api/v1/system/admin/sandbox-connections/"+id+"/tenant-assignments",
		map[string]any{"assignments": []map[string]any{{"tenant_id": 7}}})
	require.Equal(t, http.StatusOK, w.Code)

	w, body := fx.do(t, http.MethodPost,
		"/api/v1/system/admin/sandbox-connections/"+id+"/push", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, float64(1), body["updated"])
	require.Equal(t, float64(0), body["blocked"])
	results := body["results"].([]any)
	require.Equal(t, "updated", results[0].(map[string]any)["status"])
}

func TestSandboxConnectionDraftCheckRejectsIncompleteConfig(t *testing.T) {
	fx := newSandboxConnHandlerFixture(t)

	w, _ := fx.do(t, http.MethodPost, "/api/v1/system/admin/sandbox-connections/check",
		map[string]any{"config": map[string]any{"sandbox_type": "e2b"}})
	require.Equal(t, http.StatusBadRequest, w.Code,
		"an incomplete draft must fail validation before any provider call")
}

func TestSandboxConnectionTemplateQueryRequiresConnectionForReplace(t *testing.T) {
	fx := newSandboxConnHandlerFixture(t)

	w, _ := fx.do(t, http.MethodPost,
		"/api/v1/system/admin/sandbox-connections/templates/query",
		map[string]any{"replace_standard": true})
	require.Equal(t, http.StatusBadRequest, w.Code)
}

// The workspace DTO grew an additive source marker; existing callers that do
// not know it keep their byte-identical responses.
func TestSandboxConfigResponseCarriesSourceConnectionID(t *testing.T) {
	resp := toSandboxConfigResponse(&types.TenantSandboxConfigEntity{
		ID:                 "cfg-1",
		Name:               "team",
		SandboxType:        "e2b",
		Config:             &types.TenantSandboxConfig{SandboxType: "e2b"},
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
		SourceConnectionID: "conn-1",
	})
	require.Equal(t, "conn-1", resp.SourceConnectionID)

	plain := toSandboxConfigResponse(&types.TenantSandboxConfigEntity{
		ID: "cfg-2", Name: "self", SandboxType: "e2b",
		Config: &types.TenantSandboxConfig{SandboxType: "e2b"},
	})
	require.Empty(t, plain.SourceConnectionID)
}

// --- adapter ---

// configRepoAdapter exposes the multi-tenant config slice of the shared store
// as the repository interface the config service consumes.
type configRepoAdapter struct {
	repos *sandboxConnHandlerRepos
}

func (a *configRepoAdapter) Create(ctx context.Context, e *types.TenantSandboxConfigEntity) error {
	a.repos.configRows = append(a.repos.configRows, e)
	return nil
}

func (a *configRepoAdapter) GetByID(
	ctx context.Context, tenantID uint64, id string,
) (*types.TenantSandboxConfigEntity, error) {
	return a.repos.GetConfig(ctx, tenantID, id)
}

func (a *configRepoAdapter) ListByTenant(
	_ context.Context, tenantID uint64,
) ([]*types.TenantSandboxConfigEntity, error) {
	var out []*types.TenantSandboxConfigEntity
	for _, row := range a.repos.configRows {
		if row.TenantID == tenantID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (a *configRepoAdapter) ListAll(context.Context) ([]*types.TenantSandboxConfigEntity, error) {
	return a.repos.configRows, nil
}

func (a *configRepoAdapter) Update(
	_ context.Context, e *types.TenantSandboxConfigEntity,
) error {
	for i, row := range a.repos.configRows {
		if row.TenantID == e.TenantID && row.ID == e.ID {
			a.repos.configRows[i] = e
			return nil
		}
	}
	return nil
}

func (a *configRepoAdapter) SoftDelete(
	_ context.Context, tenantID uint64, id string,
) error {
	kept := a.repos.configRows[:0]
	for _, row := range a.repos.configRows {
		if row.TenantID != tenantID || row.ID != id {
			kept = append(kept, row)
		}
	}
	a.repos.configRows = kept
	return nil
}

func (a *configRepoAdapter) SetCordon(
	_ context.Context, tenantID uint64, id string, at time.Time,
) error {
	row, _ := a.GetByID(context.Background(), tenantID, id)
	if row != nil {
		row.CordonedAt = &at
	}
	return nil
}

func (a *configRepoAdapter) ClearCordon(
	_ context.Context, tenantID uint64, id string,
) error {
	row, _ := a.GetByID(context.Background(), tenantID, id)
	if row != nil {
		row.CordonedAt = nil
	}
	return nil
}

func (a *configRepoAdapter) ListBySourceConnection(
	ctx context.Context, connectionID string,
) ([]*types.TenantSandboxConfigEntity, error) {
	return a.repos.ListConfigsBySource(ctx, connectionID)
}

func (a *configRepoAdapter) MarkSourcePushed(
	_ context.Context, tenantID uint64, id, connectionID string, at time.Time,
) error {
	row, _ := a.GetByID(context.Background(), tenantID, id)
	if row != nil {
		row.SourceConnectionID = connectionID
		row.SourcePushedAt = &at
	}
	return nil
}
