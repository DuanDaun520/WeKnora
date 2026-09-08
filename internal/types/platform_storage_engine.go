package types

import (
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/storageallowlist"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	PlatformStorageEngineStatusActive   = "active"
	PlatformStorageEngineStatusDisabled = "disabled"
)

// PlatformStorageEngine is a platform-level storage engine configuration.
// Admins create and manage these engines, then assign them to workspaces.
// Each workspace can be assigned at most one platform storage engine.
type PlatformStorageEngine struct {
	ID        string               `json:"id" gorm:"type:varchar(36);primaryKey"`
	Name      string               `json:"name" gorm:"type:varchar(255);uniqueIndex;not null"`
	Provider  string               `json:"provider" gorm:"type:varchar(32);not null;index"`
	Config    StorageBackendConfig `json:"config" gorm:"type:json"`
	Status    string               `json:"status" gorm:"type:varchar(16);not null;default:'active';index"`
	CreatedAt time.Time            `json:"created_at"`
	UpdatedAt time.Time            `json:"updated_at"`
	DeletedAt gorm.DeletedAt       `json:"deleted_at" gorm:"index"`
}

func (PlatformStorageEngine) TableName() string { return "platform_storage_engines" }

func (e *PlatformStorageEngine) BeforeCreate(_ *gorm.DB) error {
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	if e.Status == "" {
		e.Status = PlatformStorageEngineStatusActive
	}
	return nil
}

func (e *PlatformStorageEngine) Validate() error {
	e.Name = strings.TrimSpace(e.Name)
	if e.Name == "" {
		return errors.NewValidationError("name is required")
	}
	e.Provider = strings.ToLower(strings.TrimSpace(e.Provider))
	if !storageallowlist.IsAllowed(e.Provider) {
		return errors.NewValidationError(fmt.Sprintf("storage provider %q is not allowed", e.Provider))
	}
	if !isSupportedStorageBackendProvider(e.Provider) {
		return errors.NewValidationError(fmt.Sprintf("unsupported storage provider: %s", e.Provider))
	}
	if e.Status != "" && e.Status != PlatformStorageEngineStatusActive && e.Status != PlatformStorageEngineStatusDisabled {
		return errors.NewValidationError("status must be active or disabled")
	}
	return e.Config.ValidateForProvider(e.Provider)
}

// ToStorageEngineConfig adapts the platform engine to the existing provider implementations.
func (e PlatformStorageEngine) ToStorageEngineConfig() *StorageEngineConfig {
	// Reuse the same logic from StorageBackend
	b := StorageBackend{Provider: e.Provider, Config: e.Config}
	return b.ToStorageEngineConfig()
}

// NewPlatformStorageEngineResponse masks sensitive fields for API responses.
func NewPlatformStorageEngineResponse(engine *PlatformStorageEngine) PlatformStorageEngine {
	out := *engine
	out.Config = engine.Config.MaskSensitiveFields()
	return out
}

// TenantStorageEngineAssignment tracks which workspace is assigned which platform engine.
// This is stored in the tenants table via platform_storage_engine_id column.
type TenantStorageEngineAssignment struct {
	TenantID                 uint64 `json:"tenant_id" gorm:"primaryKey"`
	PlatformStorageEngineID  string `json:"platform_storage_engine_id" gorm:"type:varchar(36);not null"`
	AssignedAt               time.Time `json:"assigned_at"`
	AssignedBy               uint64 `json:"assigned_by"` // user ID of admin who made the assignment
}

func (TenantStorageEngineAssignment) TableName() string { return "tenant_storage_engine_assignments" }