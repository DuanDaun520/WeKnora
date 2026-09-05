package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newWebSearchProviderPlatformDB builds an in-memory sqlite DB mirroring the
// 000095 schema bits the repository relies on: the assignment table with its
// UNIQUE (tenant_id, provider_id) constraint (the ReplaceProviderAssignments
// upsert targets it) and a minimal tenants table for the ListAssignmentInfos
// LEFT JOIN.
func newWebSearchProviderPlatformDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&types.WebSearchProviderEntity{}, &types.TenantWebSearchProviderAssignment{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS uq_tenant_web_search_provider_assignments ON tenant_web_search_provider_assignments(tenant_id, provider_id)").Error; err != nil {
		t.Fatalf("unique index: %v", err)
	}
	if err := db.Exec("CREATE TABLE IF NOT EXISTS tenants (id INTEGER PRIMARY KEY, name TEXT)").Error; err != nil {
		t.Fatalf("tenants table: %v", err)
	}
	return db
}

func seedPlatformProviders(t *testing.T, db *gorm.DB, ids ...string) {
	t.Helper()
	for _, id := range ids {
		if err := db.Create(&types.WebSearchProviderEntity{
			ID:       id,
			TenantID: 0,
			Name:     id,
			Provider: types.WebSearchProviderTypeDuckDuckGo,
		}).Error; err != nil {
			t.Fatalf("seed provider %s: %v", id, err)
		}
	}
}

func assignmentFlag(t *testing.T, db *gorm.DB, tenantID uint64, providerID string) (found, isDefault bool) {
	t.Helper()
	var row types.TenantWebSearchProviderAssignment
	err := db.Where("tenant_id = ? AND provider_id = ?", tenantID, providerID).First(&row).Error
	if err == gorm.ErrRecordNotFound {
		return false, false
	}
	if err != nil {
		t.Fatalf("load assignment %d/%s: %v", tenantID, providerID, err)
	}
	return true, row.IsDefault
}

// A workspace flagged default on one provider must lose the flag on every
// other provider — the invariant is catalog-wide, not per-provider.
func TestReplaceProviderAssignmentsEnforcesSingleDefaultPerWorkspace(t *testing.T) {
	db := newWebSearchProviderPlatformDB(t)
	seedPlatformProviders(t, db, "p1", "p2")
	repo := NewWebSearchProviderRepository(db)
	ctx := context.Background()

	if err := repo.ReplaceProviderAssignments(ctx, "p1", []types.TenantWebSearchProviderAssignment{
		{TenantID: 7, IsDefault: true},
		{TenantID: 8, IsDefault: false},
	}, "admin"); err != nil {
		t.Fatalf("replace p1: %v", err)
	}
	// Workspace 7 now defaults to p2: p1's default flag must be cleared.
	if err := repo.ReplaceProviderAssignments(ctx, "p2", []types.TenantWebSearchProviderAssignment{
		{TenantID: 7, IsDefault: true},
	}, "admin"); err != nil {
		t.Fatalf("replace p2: %v", err)
	}

	if found, def := assignmentFlag(t, db, 7, "p1"); !found || def {
		t.Fatalf("workspace 7 / p1: found=%v default=%v, want found with default cleared", found, def)
	}
	if found, def := assignmentFlag(t, db, 7, "p2"); !found || !def {
		t.Fatalf("workspace 7 / p2: found=%v default=%v, want flagged default", found, def)
	}
	if found, _ := assignmentFlag(t, db, 8, "p1"); !found {
		t.Fatal("workspace 8 / p1 assignment missing")
	}

	got, err := repo.GetDefault(ctx, 7)
	if err != nil {
		t.Fatalf("get default: %v", err)
	}
	if got == nil || got.ID != "p2" {
		t.Fatalf("workspace 7 default = %+v, want p2", got)
	}
}

// Replace is replace-all: workspaces dropped from the list lose their rows,
// and an empty list unassigns the provider everywhere.
func TestReplaceProviderAssignmentsRemovesDroppedWorkspaces(t *testing.T) {
	db := newWebSearchProviderPlatformDB(t)
	seedPlatformProviders(t, db, "p1")
	repo := NewWebSearchProviderRepository(db)
	ctx := context.Background()

	if err := repo.ReplaceProviderAssignments(ctx, "p1", []types.TenantWebSearchProviderAssignment{
		{TenantID: 7}, {TenantID: 8},
	}, "admin"); err != nil {
		t.Fatalf("initial replace: %v", err)
	}
	if err := repo.ReplaceProviderAssignments(ctx, "p1", []types.TenantWebSearchProviderAssignment{
		{TenantID: 8, IsDefault: true},
	}, "admin"); err != nil {
		t.Fatalf("shrinking replace: %v", err)
	}

	if found, _ := assignmentFlag(t, db, 7, "p1"); found {
		t.Fatal("workspace 7 assignment should have been removed by the replace")
	}
	if found, def := assignmentFlag(t, db, 8, "p1"); !found || !def {
		t.Fatalf("workspace 8 / p1: found=%v default=%v, want kept and defaulted", found, def)
	}

	if err := repo.ReplaceProviderAssignments(ctx, "p1", nil, "admin"); err != nil {
		t.Fatalf("empty replace: %v", err)
	}
	if found, _ := assignmentFlag(t, db, 8, "p1"); found {
		t.Fatal("empty replace should have unassigned every workspace")
	}
}

// Without any flagged default, the earliest assignment acts as the workspace
// default; flagging one later wins over assignment order.
func TestGetDefaultFallsBackToEarliestAssignment(t *testing.T) {
	db := newWebSearchProviderPlatformDB(t)
	seedPlatformProviders(t, db, "p1", "p2")
	repo := NewWebSearchProviderRepository(db)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	earlier := base
	later := base.Add(2 * time.Hour)
	for _, row := range []types.TenantWebSearchProviderAssignment{
		{TenantID: 7, ProviderID: "p2", AssignedAt: later},  // assigned second
		{TenantID: 7, ProviderID: "p1", AssignedAt: earlier}, // assigned first
	} {
		if err := db.Create(&row).Error; err != nil {
			t.Fatalf("seed assignment: %v", err)
		}
	}

	got, err := repo.GetDefault(ctx, 7)
	if err != nil {
		t.Fatalf("get default (unflagged): %v", err)
	}
	if got == nil || got.ID != "p1" {
		t.Fatalf("workspace 7 effective default = %+v, want earliest assignment p1", got)
	}

	if err := db.Model(&types.TenantWebSearchProviderAssignment{}).
		Where("tenant_id = ? AND provider_id = ?", 7, "p2").
		Update("is_default", true).Error; err != nil {
		t.Fatalf("flag p2 default: %v", err)
	}
	got, err = repo.GetDefault(ctx, 7)
	if err != nil {
		t.Fatalf("get default (flagged): %v", err)
	}
	if got == nil || got.ID != "p2" {
		t.Fatalf("workspace 7 effective default = %+v, want flagged p2", got)
	}

	if got, err = repo.GetDefault(ctx, 9); err != nil || got != nil {
		t.Fatalf("unassigned workspace default = (%+v, %v), want (nil, nil)", got, err)
	}
}

// Visibility is the assignment: List/GetByID expose only assigned providers,
// and IsDefault markers follow the same effective-default rule as GetDefault.
func TestListAndGetByIDFollowAssignments(t *testing.T) {
	db := newWebSearchProviderPlatformDB(t)
	seedPlatformProviders(t, db, "p1", "p2", "p3")
	repo := NewWebSearchProviderRepository(db)
	ctx := context.Background()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, row := range []types.TenantWebSearchProviderAssignment{
		{TenantID: 7, ProviderID: "p1", AssignedAt: base},
		{TenantID: 7, ProviderID: "p2", AssignedAt: base.Add(time.Hour), IsDefault: true},
	} {
		if err := db.Create(&row).Error; err != nil {
			t.Fatalf("seed assignment: %v", err)
		}
	}

	list, err := repo.List(ctx, 7)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list returned %d providers, want 2 (p3 unassigned)", len(list))
	}
	for _, provider := range list {
		switch provider.ID {
		case "p2":
			if !provider.IsDefault {
				t.Fatal("flagged p2 should carry IsDefault")
			}
		case "p1":
			if provider.IsDefault {
				t.Fatal("p1 should not be marked default while p2 is flagged")
			}
		default:
			t.Fatalf("unexpected provider %s in workspace list", provider.ID)
		}
	}

	got, err := repo.GetByID(ctx, 7, "p1")
	if err != nil || got == nil {
		t.Fatalf("GetByID assigned = (%+v, %v), want non-nil", got, err)
	}
	if got.IsDefault {
		t.Fatal("GetByID p1 should mirror the effective-default marker (false)")
	}
	if got, err = repo.GetByID(ctx, 7, "p3"); err != nil || got != nil {
		t.Fatalf("GetByID unassigned = (%+v, %v), want (nil, nil)", got, err)
	}
}

// ListAssignmentInfos joins workspace names for the admin drawer; purging on
// delete leaves nothing behind.
func TestListAssignmentInfosAndDeleteCascade(t *testing.T) {
	db := newWebSearchProviderPlatformDB(t)
	seedPlatformProviders(t, db, "p1")
	if err := db.Exec("INSERT INTO tenants (id, name) VALUES (7, 'Alpha'), (8, 'Beta')").Error; err != nil {
		t.Fatalf("seed tenants: %v", err)
	}
	repo := NewWebSearchProviderRepository(db)
	ctx := context.Background()

	if err := repo.ReplaceProviderAssignments(ctx, "p1", []types.TenantWebSearchProviderAssignment{
		{TenantID: 7, IsDefault: true},
		{TenantID: 8},
	}, "admin"); err != nil {
		t.Fatalf("replace: %v", err)
	}

	infos, err := repo.ListAssignmentInfos(ctx, "p1")
	if err != nil {
		t.Fatalf("list assignment infos: %v", err)
	}
	if len(infos) != 2 {
		t.Fatalf("assignment infos = %d rows, want 2", len(infos))
	}
	byTenant := map[uint64]types.WebSearchProviderAssignmentInfo{}
	for _, info := range infos {
		byTenant[info.TenantID] = info
	}
	if info := byTenant[7]; info.TenantName != "Alpha" || !info.IsDefault {
		t.Fatalf("workspace 7 info = %+v, want name Alpha flagged default", info)
	}
	if info := byTenant[8]; info.TenantName != "Beta" || info.IsDefault {
		t.Fatalf("workspace 8 info = %+v, want name Beta not default", info)
	}

	if err := repo.DeleteAssignmentsByProviderID(ctx, "p1"); err != nil {
		t.Fatalf("purge assignments: %v", err)
	}
	infos, err = repo.ListAssignmentInfos(ctx, "p1")
	if err != nil {
		t.Fatalf("list after purge: %v", err)
	}
	if len(infos) != 0 {
		t.Fatalf("assignments after purge = %d rows, want 0", len(infos))
	}
}
