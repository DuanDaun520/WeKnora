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
	// Category/Author are admin-managed definition metadata (000104): seeded
	// from SKILL.md frontmatter or the register form on create, edited only
	// through the meta endpoint, never touched by a bundle re-register. Edits
	// bump updated_at so assignment drift routes them through push.
	Category string `gorm:"type:varchar(255);not null;default:''"`
	Author   string `gorm:"type:varchar(255);not null;default:''"`
	// ZhName/ZhDescription are Chinese display metadata (000106), also
	// admin-managed like category/author above. The console card titles the
	// skill with zh_name and summarizes it with the first 40 chars of
	// zh_description, each falling back to the SKILL.md value when empty.
	// SKILL.md has no counterpart to seed them, so empty stays empty.
	ZhName        string `gorm:"type:varchar(255);not null;default:''"`
	ZhDescription string `gorm:"type:text;not null;default:''"`
	// HelpURL is the optional "介绍与帮助网址" (000109), also admin-managed:
	// the user-side skill detail dialog renders it as an external link (new
	// tab) when set. Must be an http(s) URL; empty = none.
	HelpURL string `gorm:"type:varchar(1024);not null;default:''"`

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

// PlatformSkillCategoryEntity is one row of the category registry (000105).
// Categories are created FIRST in the console's「分类管理」dialog and skills then
// only pick from the registry (the register drawer is not creatable anymore), so
// this table — not platform_skills.category — is the vocabulary source. A skill
// references a category by NAME (platform_skills.category remains a plain
// string, empty = uncategorized): there is deliberately no foreign key, because
// deleting a category clears that string on its skills rather than cascading.
type PlatformSkillCategoryEntity struct {
	ID   string `gorm:"type:varchar(36);primaryKey"`
	Name string `gorm:"type:varchar(255);not null"`
	// Unique among live rows only — enforced by the partial unique index in
	// migration 000105 (soft-deleted rows keep the name claimable again).
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

// TableName pins the table so GORM's pluralizer cannot drift.
func (PlatformSkillCategoryEntity) TableName() string { return "platform_skill_categories" }
