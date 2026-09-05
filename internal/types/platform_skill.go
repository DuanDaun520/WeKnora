// Package types: platform-level skill library entities (000098).
//
// A skill is registered once at platform level (bundle metadata + provenance
// source) and materialized into workspaces through explicit assignments: each
// assignment copies the platform zip into the workspace's own object storage
// and upserts an ordinary tenant_skill_catalog row tagged with
// source_platform_skill_id. The platform zip itself lives in the PLATFORM
// storage namespace (sentinel tenant id), so it follows process-wide storage
// and never a workspace-configured backend.
package types

import (
	"time"

	"gorm.io/gorm"
)

// PlatformSkillEntity is one platform-level skill definition.
type PlatformSkillEntity struct {
	ID   string `gorm:"type:varchar(36);primaryKey"`
	Name string `gorm:"type:varchar(255);not null"`
	// Version/Description/Instructions are parsed from the bundle's SKILL.md.
	Version      string `gorm:"type:varchar(64)"`
	Description  string `gorm:"type:text"`
	Instructions string `gorm:"type:text"`
	// BundleRef locates the stored zip in the platform storage namespace.
	BundleRef    string `gorm:"type:varchar(1024)"`
	BundleSHA256 string `gorm:"type:varchar(64)"`
	// Source is provenance only ("@owner/slug", a URL, or "upload"): shown in
	// the console, never re-fetched automatically.
	Source string `gorm:"type:varchar(1024)"`

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

// TableName pins the table so GORM's pluralizer cannot drift.
func (PlatformSkillEntity) TableName() string { return "platform_skills" }

// PlatformSkillAssignmentEntity links one platform skill to one workspace. It
// survives the workspace deleting its materialized catalog row (an empty
// shell the push action heals); the live-row facts of the assignment itself
// are this table's, unlike 000097 where the materialized row IS the
// assignment.
type PlatformSkillAssignmentEntity struct {
	ID       string `gorm:"type:varchar(36);primaryKey"`
	TenantID uint64
	SkillID  string `gorm:"type:varchar(36);not null"`
	// MaterializedCatalogID is the tenant_skill_catalog.id of the last
	// materialization; may be stale (workspace deleted the row) — push heals.
	MaterializedCatalogID string `gorm:"type:varchar(36);not null default ''"`
	// PushedAt is the platform skill's updated_at snapshot at the last
	// successful push/assignment; older than the skill's updated_at = drift.
	PushedAt  time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

// TableName pins the table so GORM's pluralizer cannot drift.
func (PlatformSkillAssignmentEntity) TableName() string {
	return "platform_skill_assignments"
}
