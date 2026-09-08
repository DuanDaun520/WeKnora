// Handler tests for the platform skill library surface (000098). The services
// are the real ones over in-memory stores and a shared-cache sqlite tenant
// skill repository, so the register/materialize/push paths under test are the
// production ones; the audit service is nil throughout, exercising the no-op
// path.
package handler

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// --- stores ---

// systemSkillRows mirrors the platform skill repository, stamping updated_at
// on the store side like the GORM implementation does — that stamp is what the
// assignment drift markers are computed from. Categories live in a separate
// registry (000105), created first, that skills reference by name.
type systemSkillRows struct {
	rows       []*types.PlatformSkillEntity
	categories []*types.PlatformSkillCategoryEntity
}

func (r *systemSkillRows) Create(_ context.Context, e *types.PlatformSkillEntity) error {
	cp := *e
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = time.Now()
	}
	if cp.UpdatedAt.IsZero() {
		cp.UpdatedAt = cp.CreatedAt
	}
	r.rows = append(r.rows, &cp)
	return nil
}

func (r *systemSkillRows) GetByID(_ context.Context, id string) (*types.PlatformSkillEntity, error) {
	for _, row := range r.rows {
		if row.ID == id {
			cp := *row
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *systemSkillRows) GetByName(_ context.Context, name string) (*types.PlatformSkillEntity, error) {
	for _, row := range r.rows {
		if row.Name == name {
			cp := *row
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *systemSkillRows) List(context.Context) ([]*types.PlatformSkillEntity, error) {
	return r.rows, nil
}

func (r *systemSkillRows) Update(_ context.Context, e *types.PlatformSkillEntity) error {
	for i, row := range r.rows {
		if row.ID == e.ID {
			cp := *e
			cp.UpdatedAt = time.Now()
			r.rows[i] = &cp
			return nil
		}
	}
	return nil
}

func (r *systemSkillRows) UpdateMeta(
	_ context.Context, id, category, author, zhName, zhDescription, helpURL string,
) error {
	for i, row := range r.rows {
		if row.ID == id {
			cp := *row
			cp.Category, cp.Author = category, author
			cp.ZhName, cp.ZhDescription = zhName, zhDescription
			cp.HelpURL = helpURL
			cp.UpdatedAt = time.Now()
			r.rows[i] = &cp
			return nil
		}
	}
	return nil
}

func (r *systemSkillRows) CreateCategory(
	_ context.Context, e *types.PlatformSkillCategoryEntity,
) error {
	cp := *e
	r.categories = append(r.categories, &cp)
	return nil
}

func (r *systemSkillRows) GetCategoryByName(
	_ context.Context, name string,
) (*types.PlatformSkillCategoryEntity, error) {
	for _, c := range r.categories {
		if c.Name == name {
			cp := *c
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *systemSkillRows) ListCategories(
	context.Context,
) ([]repository.SkillCategoryCount, error) {
	counts := map[string]int64{}
	for _, row := range r.rows {
		if row.Category != "" {
			counts[row.Category]++
		}
	}
	names := make([]string, 0, len(r.categories))
	for _, c := range r.categories {
		names = append(names, c.Name)
	}
	sort.Strings(names)
	out := make([]repository.SkillCategoryCount, 0, len(names))
	for _, name := range names {
		out = append(out, repository.SkillCategoryCount{
			Name: name, Count: counts[name],
		})
	}
	return out, nil
}

func (r *systemSkillRows) RenameCategory(
	_ context.Context, from, to string,
) (int64, error) {
	var moved int64
	for _, c := range r.categories {
		if c.Name == from {
			c.Name = to
		}
	}
	for i, row := range r.rows {
		if row.Category != from {
			continue
		}
		cp := *row
		cp.Category = to
		cp.UpdatedAt = time.Now()
		r.rows[i] = &cp
		moved++
	}
	return moved, nil
}

func (r *systemSkillRows) ClearCategory(_ context.Context, name string) (int64, error) {
	var cleared int64
	kept := r.categories[:0]
	for _, c := range r.categories {
		if c.Name != name {
			kept = append(kept, c)
		}
	}
	r.categories = kept
	for i, row := range r.rows {
		if row.Category != name {
			continue
		}
		cp := *row
		cp.Category = ""
		cp.UpdatedAt = time.Now()
		r.rows[i] = &cp
		cleared++
	}
	return cleared, nil
}

func (r *systemSkillRows) SoftDelete(_ context.Context, id string) error {
	kept := r.rows[:0]
	for _, row := range r.rows {
		if row.ID != id {
			kept = append(kept, row)
		}
	}
	r.rows = kept
	return nil
}

type systemSkillAssignmentRows struct {
	rows []*types.PlatformSkillAssignmentEntity
}

func (r *systemSkillAssignmentRows) Create(
	_ context.Context, e *types.PlatformSkillAssignmentEntity,
) error {
	cp := *e
	r.rows = append(r.rows, &cp)
	return nil
}

func (r *systemSkillAssignmentRows) GetBySkillAndTenant(
	_ context.Context, skillID string, tenantID uint64,
) (*types.PlatformSkillAssignmentEntity, error) {
	for _, row := range r.rows {
		if row.SkillID == skillID && row.TenantID == tenantID {
			cp := *row
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *systemSkillAssignmentRows) ListBySkill(
	_ context.Context, skillID string,
) ([]*types.PlatformSkillAssignmentEntity, error) {
	var out []*types.PlatformSkillAssignmentEntity
	for _, row := range r.rows {
		if row.SkillID == skillID {
			cp := *row
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (r *systemSkillAssignmentRows) Update(
	_ context.Context, e *types.PlatformSkillAssignmentEntity,
) error {
	for i, row := range r.rows {
		if row.ID == e.ID {
			cp := *e
			r.rows[i] = &cp
			return nil
		}
	}
	return nil
}

func (r *systemSkillAssignmentRows) SoftDelete(_ context.Context, id string) error {
	kept := r.rows[:0]
	for _, row := range r.rows {
		if row.ID != id {
			kept = append(kept, row)
		}
	}
	r.rows = kept
	return nil
}

// systemSkillFileStore records which tenant namespace each save landed in.
type systemSkillFileStore struct {
	mu      sync.Mutex
	next    int
	stored  map[string][]byte
	saves   []uint64 // tenant ids, in save order
	deleted []string
}

func (s *systemSkillFileStore) CheckConnectivity(context.Context) error { return nil }

func (s *systemSkillFileStore) SaveFile(
	context.Context, *multipart.FileHeader, uint64, string,
) (string, error) {
	return "", nil
}

func (s *systemSkillFileStore) SaveBytes(
	_ context.Context, data []byte, tenantID uint64, _ string, _ bool,
) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	ref := fmt.Sprintf("file://system-skill-%d.zip", s.next)
	if s.stored == nil {
		s.stored = map[string][]byte{}
	}
	copied := make([]byte, len(data))
	copy(copied, data)
	s.stored[ref] = copied
	s.saves = append(s.saves, tenantID)
	return ref, nil
}

func (s *systemSkillFileStore) GetFile(_ context.Context, ref string) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.stored[ref]
	if !ok {
		return nil, fmt.Errorf("bundle %s not found", ref)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (s *systemSkillFileStore) GetFileURL(context.Context, string) (string, error) {
	return "", nil
}

func (s *systemSkillFileStore) CopyFile(
	_ context.Context, srcPath string, _ uint64, _ string,
) (string, error) {
	return srcPath, nil
}

func (s *systemSkillFileStore) DeleteFile(_ context.Context, ref string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.stored, ref)
	s.deleted = append(s.deleted, ref)
	return nil
}

func (s *systemSkillFileStore) saveTenants() []uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]uint64(nil), s.saves...)
}

type systemSkillResolver struct{ store *systemSkillFileStore }

func (r *systemSkillResolver) ResolveFileService(
	context.Context, *types.Tenant, string, string, string,
) (interfaces.FileService, string, error) {
	return r.store, "", nil
}

func (r *systemSkillResolver) ResolveBackend(
	context.Context, *types.Tenant, string, string,
) (*types.StorageBackend, error) {
	return nil, nil
}

// --- fixture ---

type systemSkillHandlerFixture struct {
	handler      *SystemSkillHandler
	svc          *service.PlatformSkillService
	skills       *systemSkillRows
	assigns      *systemSkillAssignmentRows
	tenantSkills repository.TenantSkillRepository
	files        *systemSkillFileStore
	tenants      *sandboxConnTenantService
	engine       *gin.Engine
}

// compile-time: the stores satisfy what the services need.
var (
	_ repository.PlatformSkillRepository           = (*systemSkillRows)(nil)
	_ repository.PlatformSkillAssignmentRepository = (*systemSkillAssignmentRows)(nil)
	_ interfaces.FileService                       = (*systemSkillFileStore)(nil)
	_ interfaces.StorageBackendResolver            = (*systemSkillResolver)(nil)
)

func newSystemSkillHandlerFixture(t *testing.T) *systemSkillHandlerFixture {
	t.Helper()
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&types.TenantSkillEntity{}, &types.TenantSkillCatalogEntity{},
		&types.TenantSkillSnapshotEntity{}, &types.TenantUserEnvVar{},
	))
	fx := &systemSkillHandlerFixture{
		skills:       &systemSkillRows{},
		assigns:      &systemSkillAssignmentRows{},
		tenantSkills: repository.NewTenantSkillRepository(db),
		files:        &systemSkillFileStore{},
		tenants:      &sandboxConnTenantService{known: map[uint64]string{7: "alpha", 8: "beta"}},
	}
	resolver := &systemSkillResolver{store: fx.files}
	catalogSvc := service.NewTenantSkillService(
		fx.tenantSkills, nil, resolver, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	fx.svc = service.NewPlatformSkillService(
		fx.skills, fx.assigns, fx.tenantSkills, catalogSvc, fx.tenants, resolver)
	fx.handler = NewSystemSkillHandler(
		fx.svc,
		fx.tenants,
		nil, // audit service is optional; nil exercises the no-op path
	)
	gin.SetMode(gin.TestMode)
	fx.engine = gin.New()
	fx.engine.Use(middleware.ErrorHandler())
	admin := fx.engine.Group("/api/v1/system/admin/skills")
	{
		admin.GET("", fx.handler.ListSkills)
		admin.POST("", fx.handler.CreateSkill)
		admin.GET("/categories", fx.handler.ListSkillCategories)
		admin.POST("/categories", fx.handler.CreateSkillCategory)
		admin.PUT("/categories/rename", fx.handler.RenameSkillCategory)
		admin.PUT("/categories/remove", fx.handler.RemoveSkillCategory)
		admin.GET("/:id", fx.handler.GetSkill)
		admin.PUT("/:id", fx.handler.UpdateSkill)
		admin.DELETE("/:id", fx.handler.DeleteSkill)
		admin.PUT("/:id/meta", fx.handler.UpdateSkillMeta)
		admin.GET("/:id/files", fx.handler.ListSkillFiles)
		admin.GET("/:id/files/content", fx.handler.GetSkillFile)
		admin.GET("/:id/tenant-assignments", fx.handler.ListTenantAssignments)
		admin.PUT("/:id/tenant-assignments", fx.handler.UpdateTenantAssignments)
		admin.POST("/:id/push", fx.handler.PushSkill)
	}
	return fx
}

// --- helpers ---

func systemSkillMD(name, body string) string {
	return "---\nname: " + name + "\ndescription: system handler test skill\n---\n\n" + body + "\n"
}

func systemSkillZip(t *testing.T, name, body string) []byte {
	t.Helper()
	return systemSkillZipWithManifest(t, systemSkillMD(name, body))
}

// systemSkillZipWithManifest zips an arbitrary SKILL.md so tests can exercise
// frontmatter metadata (version/author/category) end to end.
func systemSkillZipWithManifest(t *testing.T, manifest string) []byte {
	t.Helper()
	buf := &bytes.Buffer{}
	w := zip.NewWriter(buf)
	for path, content := range map[string]string{
		"SKILL.md":       manifest,
		"scripts/run.py": "print('run')\n",
	} {
		f, err := w.Create(path)
		require.NoError(t, err)
		_, err = f.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	return buf.Bytes()
}

func systemSkillUploadRequest(
	t *testing.T, method, path string, archive []byte,
) *http.Request {
	t.Helper()
	return systemSkillUploadRequestWithFields(t, method, path, archive, nil)
}

// systemSkillUploadRequestWithFields rides extra form fields next to the file
// field — the category/author of a registration.
func systemSkillUploadRequestWithFields(
	t *testing.T, method, path string, archive []byte, fields map[string]string,
) *http.Request {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "skill.zip")
	require.NoError(t, err)
	_, err = part.Write(archive)
	require.NoError(t, err)
	for key, value := range fields {
		require.NoError(t, writer.WriteField(key, value))
	}
	require.NoError(t, writer.Close())
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func (fx *systemSkillHandlerFixture) do(
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

// createSkill registers a platform skill through the real multipart route and
// returns its id.
func (fx *systemSkillHandlerFixture) createSkill(t *testing.T, name, body string) string {
	t.Helper()
	w := httptest.NewRecorder()
	fx.engine.ServeHTTP(w, systemSkillUploadRequest(t, http.MethodPost,
		"/api/v1/system/admin/skills", systemSkillZip(t, name, body)))
	require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())
	out := map[string]any{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	data := out["data"].(map[string]any)
	require.Equal(t, name, data["name"])
	return data["id"].(string)
}

func (fx *systemSkillHandlerFixture) updateSkill(t *testing.T, id, name, body string) {
	t.Helper()
	w := httptest.NewRecorder()
	fx.engine.ServeHTTP(w, systemSkillUploadRequest(t, http.MethodPut,
		"/api/v1/system/admin/skills/"+id, systemSkillZip(t, name, body)))
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
}

// registerCategory pre-registers a category through the real POST route and
// asserts 201 — skills may only reference categories that exist first (000105).
func (fx *systemSkillHandlerFixture) registerCategory(t *testing.T, name string) {
	t.Helper()
	w, out := fx.do(t, http.MethodPost, "/api/v1/system/admin/skills/categories",
		map[string]any{"name": name})
	require.Equal(t, http.StatusCreated, w.Code, "body: %v", out)
}

func (fx *systemSkillHandlerFixture) assign(
	t *testing.T, id string, tenantIDs ...uint64,
) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	rows := make([]map[string]any, 0, len(tenantIDs))
	for _, tenantID := range tenantIDs {
		rows = append(rows, map[string]any{"tenant_id": tenantID})
	}
	return fx.do(t, http.MethodPut,
		"/api/v1/system/admin/skills/"+id+"/tenant-assignments",
		map[string]any{"assignments": rows})
}

func systemSkillErrorCode(out map[string]any) string {
	errObj, ok := out["error"].(map[string]any)
	if !ok {
		return ""
	}
	code, _ := errObj["code"].(string)
	return code
}

func systemSkillResults(t *testing.T, out map[string]any) []map[string]any {
	t.Helper()
	raw, ok := out["results"].([]any)
	require.True(t, ok, "body has no results array: %v", out)
	rows := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		rows = append(rows, item.(map[string]any))
	}
	return rows
}

func systemSkillResultByTenant(
	rows []map[string]any, tenantID uint64,
) map[string]any {
	for _, row := range rows {
		if uint64(row["tenant_id"].(float64)) == tenantID {
			return row
		}
	}
	return nil
}

// --- create / get / list / files ---

func TestSystemSkillCreateViaMultipartThenGetAndList(t *testing.T) {
	fx := newSystemSkillHandlerFixture(t)
	id := fx.createSkill(t, "pdf-tools", "body v1")

	// The platform zip landed in exactly one namespace-scoped save.
	require.Equal(t, []uint64{1 << 62}, fx.files.saveTenants(),
		"the platform zip belongs to the sentinel namespace, not a workspace")

	w, out := fx.do(t, http.MethodGet, "/api/v1/system/admin/skills/"+id, nil)
	require.Equal(t, http.StatusOK, w.Code)
	data := out["data"].(map[string]any)
	require.Equal(t, "pdf-tools", data["name"])
	require.NotEmpty(t, data["bundle_sha256"])
	assignments, ok := data["assignments"].([]any)
	require.True(t, ok, "assignments must never be null")
	require.Empty(t, assignments)

	w, out = fx.do(t, http.MethodGet, "/api/v1/system/admin/skills", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Len(t, out["data"].([]any), 1)

	w, _ = fx.do(t, http.MethodGet, "/api/v1/system/admin/skills/missing", nil)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestSystemSkillCreateDuplicateNameIs409(t *testing.T) {
	fx := newSystemSkillHandlerFixture(t)
	fx.createSkill(t, "pdf-tools", "body v1")

	w := httptest.NewRecorder()
	fx.engine.ServeHTTP(w, systemSkillUploadRequest(t, http.MethodPost,
		"/api/v1/system/admin/skills", systemSkillZip(t, "pdf-tools", "body v2")))
	require.Equal(t, http.StatusConflict, w.Code)
	out := map[string]any{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.Equal(t, "name_conflict", systemSkillErrorCode(out))
}

// ftp:// fails source validation before any network I/O, so the JSON branch's
// 400 shape is exercised without a fetch.
func TestSystemSkillCreateFromInvalidSourceIs400(t *testing.T) {
	fx := newSystemSkillHandlerFixture(t)

	w, out := fx.do(t, http.MethodPost, "/api/v1/system/admin/skills",
		map[string]any{"source": "ftp://example.com/skill"})
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, out["error"].(map[string]any)["message"], "http(s)")

	w, _ = fx.do(t, http.MethodPost, "/api/v1/system/admin/skills",
		map[string]any{"source": ""})
	require.Equal(t, http.StatusBadRequest, w.Code, "an empty source is a binding 400")
}

func TestSystemSkillUpdateRefusesRenameWith400(t *testing.T) {
	fx := newSystemSkillHandlerFixture(t)
	id := fx.createSkill(t, "pdf-tools", "body v1")

	w := httptest.NewRecorder()
	fx.engine.ServeHTTP(w, systemSkillUploadRequest(t, http.MethodPut,
		"/api/v1/system/admin/skills/"+id, systemSkillZip(t, "other-name", "body v2")))
	require.Equal(t, http.StatusBadRequest, w.Code)
	out := map[string]any{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.Equal(t, "name_immutable", systemSkillErrorCode(out))
}

func TestSystemSkillFilesEndpoints(t *testing.T) {
	fx := newSystemSkillHandlerFixture(t)
	id := fx.createSkill(t, "pdf-tools", "body v1")

	w, out := fx.do(t, http.MethodGet, "/api/v1/system/admin/skills/"+id+"/files", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Len(t, out["data"].([]any), 2)

	w, out = fx.do(t, http.MethodGet,
		"/api/v1/system/admin/skills/"+id+"/files/content?path=SKILL.md", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, out["data"].(map[string]any)["content"], "body v1")

	w, _ = fx.do(t, http.MethodGet,
		"/api/v1/system/admin/skills/"+id+"/files/content?path=missing.txt", nil)
	require.Equal(t, http.StatusNotFound, w.Code)
}

// --- assignments ---

func TestSystemSkillAssignmentValidation(t *testing.T) {
	fx := newSystemSkillHandlerFixture(t)
	id := fx.createSkill(t, "pdf-tools", "body v1")

	w, _ := fx.do(t, http.MethodPut, "/api/v1/system/admin/skills/missing/tenant-assignments",
		map[string]any{"assignments": []map[string]any{{"tenant_id": 7}}})
	require.Equal(t, http.StatusNotFound, w.Code, "an unknown skill is a 404")

	w, _ = fx.do(t, http.MethodPut, "/api/v1/system/admin/skills/"+id+"/tenant-assignments",
		map[string]any{"assignments": []map[string]any{{"tenant_id": 7}, {"tenant_id": 7}}})
	require.Equal(t, http.StatusBadRequest, w.Code, "a duplicate workspace is a 400")

	w, _ = fx.do(t, http.MethodPut, "/api/v1/system/admin/skills/"+id+"/tenant-assignments",
		map[string]any{"assignments": []map[string]any{{"tenant_id": 0}}})
	require.Equal(t, http.StatusBadRequest, w.Code, "a zero workspace id is a 400")

	w, _ = fx.do(t, http.MethodPut, "/api/v1/system/admin/skills/"+id+"/tenant-assignments",
		map[string]any{"assignments": []map[string]any{{"tenant_id": 99}}})
	require.Equal(t, http.StatusNotFound, w.Code, "an unknown workspace is a 404")
}

func TestSystemSkillAssignMaterializesWithProvenanceAndNoDrift(t *testing.T) {
	fx := newSystemSkillHandlerFixture(t)
	id := fx.createSkill(t, "pdf-tools", "body v1")

	w, out := fx.assign(t, id, 7, 8)
	require.Equal(t, http.StatusOK, w.Code)
	for _, row := range systemSkillResults(t, out) {
		require.Equal(t, "assigned", row["status"])
		require.NotEmpty(t, row["catalog_id"])
	}
	require.Equal(t, "beta", systemSkillResultByTenant(systemSkillResults(t, out), 8)["tenant_name"])
	require.Len(t, out["assignments"].([]any), 2)

	ctx := context.Background()
	for _, tenantID := range []uint64{7, 8} {
		cat, err := fx.tenantSkills.GetCatalogByName(ctx, tenantID, "pdf-tools")
		require.NoError(t, err)
		require.NotNil(t, cat)
		require.Equal(t, id, cat.SourcePlatformSkillID,
			"the materialized row carries the platform provenance")
	}

	w, out = fx.do(t, http.MethodGet,
		"/api/v1/system/admin/skills/"+id+"/tenant-assignments", nil)
	require.Equal(t, http.StatusOK, w.Code)
	for _, raw := range out["data"].([]any) {
		require.False(t, raw.(map[string]any)["drift"].(bool),
			"a fresh assignment must not read as drifting")
	}
}

// --- delete guard ---

func TestSystemSkillDeleteBlockedWhileAssigned(t *testing.T) {
	fx := newSystemSkillHandlerFixture(t)
	id := fx.createSkill(t, "pdf-tools", "body v1")
	fx.assign(t, id, 7)

	w, out := fx.do(t, http.MethodDelete, "/api/v1/system/admin/skills/"+id, nil)
	require.Equal(t, http.StatusConflict, w.Code)
	require.Equal(t, "assignments_exist", systemSkillErrorCode(out))
	errObj := out["error"].(map[string]any)
	require.Len(t, errObj["data"].(map[string]any)["assignments"].([]any), 1)

	w, out = fx.assign(t, id)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "unassigned",
		systemSkillResultByTenant(systemSkillResults(t, out), 7)["status"])

	w, _ = fx.do(t, http.MethodDelete, "/api/v1/system/admin/skills/"+id, nil)
	require.Equal(t, http.StatusOK, w.Code)
	w, out = fx.do(t, http.MethodGet, "/api/v1/system/admin/skills", nil)
	require.Empty(t, out["data"].([]any))
}

// --- push ---

// One workspace took the name over with a self-built row after deleting its
// materialized one: push reports it as blocked while the other workspace
// updates — partial success shape.
func TestSystemSkillPushPartialSuccessShape(t *testing.T) {
	fx := newSystemSkillHandlerFixture(t)
	id := fx.createSkill(t, "pdf-tools", "body v1")
	fx.assign(t, id, 7, 8)

	ctx := context.Background()
	cat8, err := fx.tenantSkills.GetCatalogByName(ctx, 8, "pdf-tools")
	require.NoError(t, err)
	require.NoError(t, fx.tenantSkills.DeleteCatalog(ctx, 8, cat8.ID))
	require.NoError(t, fx.tenantSkills.CreateCatalog(ctx,
		&types.TenantSkillCatalogEntity{ID: "cat-self", TenantID: 8, Name: "pdf-tools"}))

	fx.updateSkill(t, id, "pdf-tools", "body v2")

	w, out := fx.do(t, http.MethodPost, "/api/v1/system/admin/skills/"+id+"/push", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.EqualValues(t, 1, out["updated"])
	require.EqualValues(t, 0, out["healed"])
	require.EqualValues(t, 1, out["blocked"])
	results := systemSkillResults(t, out)
	require.Equal(t, "updated", systemSkillResultByTenant(results, 7)["status"])
	blocked := systemSkillResultByTenant(results, 8)
	require.Equal(t, "blocked", blocked["status"])
	require.Equal(t, "name_conflict", blocked["code"])

	cat7, err := fx.tenantSkills.GetCatalogByName(ctx, 7, "pdf-tools")
	require.NoError(t, err)
	require.NotNil(t, cat7)
	require.Equal(t, id, cat7.SourcePlatformSkillID, "a push keeps the provenance")
}

func TestSystemSkillPushHealsLocallyDeletedRow(t *testing.T) {
	fx := newSystemSkillHandlerFixture(t)
	id := fx.createSkill(t, "pdf-tools", "body v1")
	fx.assign(t, id, 7)

	ctx := context.Background()
	cat7, err := fx.tenantSkills.GetCatalogByName(ctx, 7, "pdf-tools")
	require.NoError(t, err)
	require.NoError(t, fx.tenantSkills.DeleteCatalog(ctx, 7, cat7.ID))

	fx.updateSkill(t, id, "pdf-tools", "body v2")

	w, out := fx.do(t, http.MethodPost, "/api/v1/system/admin/skills/"+id+"/push", nil)
	require.Equal(t, http.StatusOK, w.Code)
	// "updated" counts updated+healed; healed is broken out separately.
	require.EqualValues(t, 1, out["updated"])
	require.EqualValues(t, 1, out["healed"])
	require.Equal(t, "healed", systemSkillResults(t, out)[0]["status"])

	healed, err := fx.tenantSkills.GetCatalogByName(ctx, 7, "pdf-tools")
	require.NoError(t, err)
	require.NotNil(t, healed, "the push re-materialized the deleted row")
	require.Equal(t, id, healed.SourcePlatformSkillID)
}

// A workspace that installed the skill keeps its install while the platform
// pushes a newer bundle: unassign must report the guard instead of silently
// dropping a live sandbox's definition.
func TestSystemSkillUnassignBlockedWhileSkillsInstalled(t *testing.T) {
	fx := newSystemSkillHandlerFixture(t)
	id := fx.createSkill(t, "pdf-tools", "body v1")
	fx.assign(t, id, 7)

	ctx := context.Background()
	cat7, err := fx.tenantSkills.GetCatalogByName(ctx, 7, "pdf-tools")
	require.NoError(t, err)
	require.NoError(t, fx.tenantSkills.CreateSkill(ctx, &types.TenantSkillEntity{
		ID: "sk-1", TenantID: 7, SandboxConfigID: "cfg-1", CatalogID: cat7.ID,
		Name: "pdf-tools", Status: types.SkillStatusReady, Enabled: true,
	}))

	w, out := fx.assign(t, id)
	require.Equal(t, http.StatusOK, w.Code)
	blocked := systemSkillResultByTenant(systemSkillResults(t, out), 7)
	require.Equal(t, "blocked", blocked["status"])
	require.Equal(t, "skills_installed", blocked["code"])
	require.Equal(t, []any{"pdf-tools"}, blocked["skill_names"])

	stillThere, err := fx.tenantSkills.GetCatalogByName(ctx, 7, "pdf-tools")
	require.NoError(t, err)
	require.NotNil(t, stillThere, "the platform surface has no force cascade")
}

// --- definition metadata (000104): category / author ---

const systemSkillFrontmatterMeta = `
author: Frontmatter Author
category: frontmatter-cat
`

// Empty form fields fall back to the SKILL.md frontmatter values; non-empty
// form fields override them. Both branches end in the response projection.
func TestSystemSkillCreateReadsCategoryAndAuthor(t *testing.T) {
	fx := newSystemSkillHandlerFixture(t)
	// Categories must exist before skills reference them (000105): the
	// frontmatter-only fallback and the form override below both name
	// registered categories.
	fx.registerCategory(t, "frontmatter-cat")
	fx.registerCategory(t, "form-cat")

	register := func(name string, fields map[string]string) map[string]any {
		t.Helper()
		manifest := "---\nname: " + name +
			"\ndescription: system handler test skill" + systemSkillFrontmatterMeta + "---\n\nbody v1\n"
		w := httptest.NewRecorder()
		fx.engine.ServeHTTP(w, systemSkillUploadRequestWithFields(t, http.MethodPost,
			"/api/v1/system/admin/skills", systemSkillZipWithManifest(t, manifest), fields))
		require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())
		out := map[string]any{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
		return out["data"].(map[string]any)
	}

	// No form fields: the frontmatter seeds the metadata.
	data := register("pdf-tools", nil)
	require.Equal(t, "frontmatter-cat", data["category"])
	require.Equal(t, "Frontmatter Author", data["author"])

	// Form fields win over whatever the frontmatter says.
	data = register("doc-tools", map[string]string{
		"category": "form-cat", "author": "Form Author",
	})
	require.Equal(t, "form-cat", data["category"])
	require.Equal(t, "Form Author", data["author"])
}

// Create-first (000105): a form category that is not registered is a 400, and a
// SKILL.md frontmatter category nobody registered is silently dropped — the
// skill lands uncategorized rather than minting a new one.
func TestSystemSkillCreateCategoryMustBeRegistered(t *testing.T) {
	fx := newSystemSkillHandlerFixture(t)

	w := httptest.NewRecorder()
	fx.engine.ServeHTTP(w, systemSkillUploadRequestWithFields(t, http.MethodPost,
		"/api/v1/system/admin/skills", systemSkillZip(t, "pdf-tools", "body v1"),
		map[string]string{"category": "ghost"}))
	require.Equal(t, http.StatusBadRequest, w.Code, "body: %s", w.Body.String())
	out := map[string]any{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.Equal(t, "category_not_found",
		out["error"].(map[string]any)["code"])

	// Frontmatter names an unregistered category: registration still succeeds
	// and the skill is simply uncategorized.
	manifest := "---\nname: doc-tools\ndescription: system handler test skill" +
		"\ncategory: ghost\n---\n\nbody v1\n"
	w = httptest.NewRecorder()
	fx.engine.ServeHTTP(w, systemSkillUploadRequestWithFields(t, http.MethodPost,
		"/api/v1/system/admin/skills", systemSkillZipWithManifest(t, manifest), nil))
	require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())
	out = map[string]any{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.Empty(t, out["data"].(map[string]any)["category"])
}

// POST /skills/categories is the registry's create surface: duplicates are a
// 409 and a fresh unused category lists with count 0.
func TestSystemSkillCategoryCreateEndpoint(t *testing.T) {
	fx := newSystemSkillHandlerFixture(t)

	w, out := fx.do(t, http.MethodPost, "/api/v1/system/admin/skills/categories",
		map[string]any{"name": "docs"})
	require.Equal(t, http.StatusCreated, w.Code, "body: %v", out)
	require.Equal(t, "docs", out["data"].(map[string]any)["name"])

	w, out = fx.do(t, http.MethodPost, "/api/v1/system/admin/skills/categories",
		map[string]any{"name": "docs"})
	require.Equal(t, http.StatusConflict, w.Code, "body: %v", out)
	require.Equal(t, "category_exists", out["error"].(map[string]any)["code"])

	w, _ = fx.do(t, http.MethodPost, "/api/v1/system/admin/skills/categories",
		map[string]any{"name": "   "})
	require.Equal(t, http.StatusBadRequest, w.Code)

	w, out = fx.do(t, http.MethodGet, "/api/v1/system/admin/skills/categories", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, []any{map[string]any{"name": "docs", "count": float64(0)}},
		out["data"], "a freshly created, unused category still lists")
}

// The length cap is enforced on both content types before any fetch: the JSON
// branch 400s without network I/O.
func TestSystemSkillCreateMetaTooLongIs400(t *testing.T) {
	fx := newSystemSkillHandlerFixture(t)
	long := strings.Repeat("x", 256)

	w := httptest.NewRecorder()
	fx.engine.ServeHTTP(w, systemSkillUploadRequestWithFields(t, http.MethodPost,
		"/api/v1/system/admin/skills", systemSkillZip(t, "pdf-tools", "body v1"),
		map[string]string{"category": long}))
	require.Equal(t, http.StatusBadRequest, w.Code)

	w, out := fx.do(t, http.MethodPost, "/api/v1/system/admin/skills",
		map[string]any{"source": "https://example.com/skill.zip", "author": long})
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, out["error"].(map[string]any)["message"], "too long")
}

func TestSystemSkillMetaPresenceSemantics(t *testing.T) {
	fx := newSystemSkillHandlerFixture(t)
	fx.registerCategory(t, "docs")
	fx.registerCategory(t, "office")
	id := fx.createSkill(t, "pdf-tools", "body v1")

	meta := func(t *testing.T, body map[string]any) map[string]any {
		w, out := fx.do(t, http.MethodPut, "/api/v1/system/admin/skills/"+id+"/meta", body)
		require.Equal(t, http.StatusOK, w.Code, "body: %v", out)
		return out["data"].(map[string]any)
	}

	// Baseline: a fresh registration is uncategorized with no author.
	w, out := fx.do(t, http.MethodGet, "/api/v1/system/admin/skills/"+id, nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Empty(t, out["data"].(map[string]any)["category"])

	// Set both.
	data := meta(t, map[string]any{"category": "docs", "author": "Alice"})
	require.Equal(t, "docs", data["category"])
	require.Equal(t, "Alice", data["author"])

	// An absent field keeps its value; a present one replaces it.
	data = meta(t, map[string]any{"category": "office"})
	require.Equal(t, "office", data["category"])
	require.Equal(t, "Alice", data["author"])
	data = meta(t, map[string]any{"author": "Bob"})
	require.Equal(t, "office", data["category"])
	require.Equal(t, "Bob", data["author"])

	// An explicit empty string clears back to uncategorized / unknown.
	data = meta(t, map[string]any{"category": "", "author": ""})
	require.Empty(t, data["category"])
	require.Empty(t, data["author"])

	// A non-empty category must be registered (000105).
	w, out = fx.do(t, http.MethodPut, "/api/v1/system/admin/skills/"+id+"/meta",
		map[string]any{"category": "ghost"})
	require.Equal(t, http.StatusBadRequest, w.Code, "body: %v", out)
	require.Equal(t, "category_not_found", out["error"].(map[string]any)["code"])

	w, _ = fx.do(t, http.MethodPut, "/api/v1/system/admin/skills/missing/meta",
		map[string]any{"category": "docs"})
	require.Equal(t, http.StatusNotFound, w.Code)

	w, _ = fx.do(t, http.MethodPut, "/api/v1/system/admin/skills/"+id+"/meta",
		map[string]any{"category": strings.Repeat("x", 256)})
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSystemSkillCategoryDirectoryAndBulkOps(t *testing.T) {
	fx := newSystemSkillHandlerFixture(t)
	fx.registerCategory(t, "docs")
	fx.registerCategory(t, "office")
	createCategorized := func(name, category string) string {
		t.Helper()
		fields := map[string]string{}
		if category != "" {
			fields["category"] = category
		}
		w := httptest.NewRecorder()
		fx.engine.ServeHTTP(w, systemSkillUploadRequestWithFields(t, http.MethodPost,
			"/api/v1/system/admin/skills", systemSkillZip(t, name, "body v1"), fields))
		require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())
		out := map[string]any{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
		return out["data"].(map[string]any)["id"].(string)
	}
	idDocsA := createCategorized("pdf-tools", "docs")
	createCategorized("doc-tools", "docs")
	createCategorized("sheet-tools", "office")
	createCategorized("misc-tools", "")

	w, out := fx.do(t, http.MethodGet, "/api/v1/system/admin/skills/categories", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, []any{
		map[string]any{"name": "docs", "count": float64(2)},
		map[string]any{"name": "office", "count": float64(1)},
	}, out["data"], "sorted, counted, and uncategorized rows stay out")

	// A rename moves every skill of the category and lights drift on their
	// assignments — the metadata reaches workspaces through push.
	fx.assign(t, idDocsA, 7)
	time.Sleep(2 * time.Millisecond) // separate the meta stamp from PushedAt
	w, out = fx.do(t, http.MethodPut, "/api/v1/system/admin/skills/categories/rename",
		map[string]any{"from": "docs", "to": "文档"})
	require.Equal(t, http.StatusOK, w.Code)
	require.EqualValues(t, 2, out["renamed"])
	w, out = fx.do(t, http.MethodGet, "/api/v1/system/admin/skills/"+idDocsA, nil)
	require.Equal(t, "文档", out["data"].(map[string]any)["category"])
	w, out = fx.do(t, http.MethodGet,
		"/api/v1/system/admin/skills/"+idDocsA+"/tenant-assignments", nil)
	require.True(t, out["data"].([]any)[0].(map[string]any)["drift"].(bool),
		"a category rename is a drift driver")

	w, out = fx.do(t, http.MethodPut, "/api/v1/system/admin/skills/categories/remove",
		map[string]any{"name": "文档"})
	require.Equal(t, http.StatusOK, w.Code)
	require.EqualValues(t, 2, out["cleared"])
	w, out = fx.do(t, http.MethodGet, "/api/v1/system/admin/skills/categories", nil)
	require.Equal(t, []any{
		map[string]any{"name": "office", "count": float64(1)},
	}, out["data"])

	w, _ = fx.do(t, http.MethodPut, "/api/v1/system/admin/skills/categories/rename",
		map[string]any{"from": "office", "to": "office"})
	require.Equal(t, http.StatusBadRequest, w.Code, "a rename onto itself is a 400")
	w, _ = fx.do(t, http.MethodPut, "/api/v1/system/admin/skills/categories/remove",
		map[string]any{"name": "   "})
	require.Equal(t, http.StatusBadRequest, w.Code)
}

// The registration metadata flows into the materialized workspace catalog row.
func TestSystemSkillAssignmentMaterializesMetadata(t *testing.T) {
	fx := newSystemSkillHandlerFixture(t)
	fx.registerCategory(t, "docs")
	w := httptest.NewRecorder()
	fx.engine.ServeHTTP(w, systemSkillUploadRequestWithFields(t, http.MethodPost,
		"/api/v1/system/admin/skills", systemSkillZip(t, "pdf-tools", "body v1"),
		map[string]string{"category": "docs", "author": "Alice"}))
	require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())
	out := map[string]any{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	id := out["data"].(map[string]any)["id"].(string)

	fx.assign(t, id, 7)

	cat, err := fx.tenantSkills.GetCatalogByName(context.Background(), 7, "pdf-tools")
	require.NoError(t, err)
	require.NotNil(t, cat)
	require.Equal(t, "docs", cat.Category, "the assignment carries the category")
	require.Equal(t, "Alice", cat.Author, "the assignment carries the author")
}

// A bundle re-register replaces the bundle facts but never the admin-managed
// metadata — that has its own endpoint and its own drift.
func TestSystemSkillBundleReregisterKeepsMeta(t *testing.T) {
	fx := newSystemSkillHandlerFixture(t)
	fx.registerCategory(t, "docs")
	w := httptest.NewRecorder()
	fx.engine.ServeHTTP(w, systemSkillUploadRequestWithFields(t, http.MethodPost,
		"/api/v1/system/admin/skills", systemSkillZip(t, "pdf-tools", "body v1"),
		map[string]string{"category": "docs", "author": "Alice"}))
	require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())
	out := map[string]any{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	id := out["data"].(map[string]any)["id"].(string)

	// The v2 bundle ships different frontmatter author/category on purpose:
	// neither may clobber the admin-managed values.
	manifest := "---\nname: pdf-tools\ndescription: system handler test skill" +
		systemSkillFrontmatterMeta + "---\n\nbody v2\n"
	w = httptest.NewRecorder()
	fx.engine.ServeHTTP(w, systemSkillUploadRequest(t, http.MethodPut,
		"/api/v1/system/admin/skills/"+id, systemSkillZipWithManifest(t, manifest)))
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	w, out = fx.do(t, http.MethodGet, "/api/v1/system/admin/skills/"+id, nil)
	require.Equal(t, http.StatusOK, w.Code)
	data := out["data"].(map[string]any)
	require.Equal(t, "docs", data["category"])
	require.Equal(t, "Alice", data["author"])
}
