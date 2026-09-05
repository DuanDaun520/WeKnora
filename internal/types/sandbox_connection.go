// Package types: platform-level sandbox connection entity (000097).
//
// A sandbox connection is configured once at platform level (provider
// endpoints, credentials, runtime defaults) and materialized into workspaces
// through explicit assignments. The credential-bearing payload lives in
// Config, reusing TenantSandboxConfig's encrypted Value/Scan hooks.
//
// SkillImage and VolumeMount are always nil here by construction: a skill
// snapshot is stamped onto each materialized config row by the skill
// installer, and volume names are per-tenant, so carrying either on the
// shared connection would leak one workspace's state into another.
package types

import (
	"time"

	"gorm.io/gorm"
)

// SandboxConnectionEntity is one platform-level sandbox connection template.
type SandboxConnectionEntity struct {
	ID          string `gorm:"type:varchar(36);primaryKey"`
	Name        string `gorm:"type:varchar(255);not null"`
	Description string `gorm:"type:text"`

	// SandboxType is promoted out of Config so listing needs no decryption,
	// mirroring TenantSandboxConfigEntity.
	SandboxType string               `gorm:"type:varchar(32);not null"`
	Config      *TenantSandboxConfig `gorm:"type:jsonb"`

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

// TableName pins the table so GORM's pluralizer cannot drift.
func (SandboxConnectionEntity) TableName() string {
	return "sandbox_connections"
}
