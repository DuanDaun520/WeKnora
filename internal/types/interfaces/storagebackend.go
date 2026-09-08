package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

type StorageBackendRepository interface {
	Create(ctx context.Context, backend *types.StorageBackend) error
	GetByID(ctx context.Context, tenantID uint64, id string) (*types.StorageBackend, error)
	List(ctx context.Context, tenantID uint64) ([]*types.StorageBackend, error)
	Update(ctx context.Context, backend *types.StorageBackend) error
	Delete(ctx context.Context, tenantID uint64, id string) error
	FindLegacyAlias(ctx context.Context, tenantID uint64, provider string) (*types.StorageBackend, error)
}

type StorageBackendService interface {
	Create(ctx context.Context, backend *types.StorageBackend) error
	Update(ctx context.Context, backend *types.StorageBackend) error
	Delete(ctx context.Context, tenantID uint64, id string) error
	SetDefault(ctx context.Context, tenantID uint64, id string) error
	Test(ctx context.Context, backend *types.StorageBackend) error
}

// StorageBackendResolver is the single runtime entry point for resolving one
// concrete storage instance. backendID wins; provider is a legacy fallback.
type StorageBackendResolver interface {
	ResolveFileService(ctx context.Context, tenant *types.Tenant, backendID, provider, localBaseDir string) (FileService, string, error)
	ResolveBackend(ctx context.Context, tenant *types.Tenant, backendID, provider string) (*types.StorageBackend, error)
}

// PlatformStorageEngineRepository manages platform-level storage engines.
// These are created by platform admins and assigned to workspaces.
type PlatformStorageEngineRepository interface {
	Create(ctx context.Context, engine *types.PlatformStorageEngine) error
	GetByID(ctx context.Context, id string) (*types.PlatformStorageEngine, error)
	GetByName(ctx context.Context, name string) (*types.PlatformStorageEngine, error)
	List(ctx context.Context) ([]*types.PlatformStorageEngine, error)
	Update(ctx context.Context, engine *types.PlatformStorageEngine) error
	Delete(ctx context.Context, id string) error
}
