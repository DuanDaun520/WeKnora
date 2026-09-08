package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	filesvc "github.com/Tencent/WeKnora/internal/application/service/file"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PlatformStorageEngineService struct {
	repo interfaces.PlatformStorageEngineRepository
	db   *gorm.DB
}

func NewPlatformStorageEngineService(
	repo interfaces.PlatformStorageEngineRepository,
	db *gorm.DB,
) *PlatformStorageEngineService {
	return &PlatformStorageEngineService{repo: repo, db: db}
}

func (s *PlatformStorageEngineService) Create(ctx context.Context, engine *types.PlatformStorageEngine) error {
	if err := engine.Validate(); err != nil {
		return err
	}
	if err := validatePlatformStorageEngineEndpoint(engine); err != nil {
		return err
	}
	if err := s.Test(ctx, engine); err != nil {
		return apperrors.NewBadRequestError("storage connection test failed").WithDetails(secutils.SanitizeStorageConnectivityError(err))
	}
	// Check for duplicate name
	existing, err := s.repo.GetByName(ctx, engine.Name)
	if err != nil {
		return err
	}
	if existing != nil {
		return apperrors.NewConflictError("a storage engine with this name already exists")
	}
	engine.CreatedAt, engine.UpdatedAt = time.Now(), time.Now()
	return s.repo.Create(ctx, engine)
}

func (s *PlatformStorageEngineService) Update(ctx context.Context, incoming *types.PlatformStorageEngine) error {
	existing, err := s.repo.GetByID(ctx, incoming.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return apperrors.NewNotFoundError("storage engine not found")
	}
	// Merge secrets from existing config
	incoming.Config = incoming.Config.MergeSecrets(existing.Config)
	// Check if location key changed
	if incoming.Config.LocationKey(existing.Provider) != existing.Config.LocationKey(existing.Provider) {
		return apperrors.NewBadRequestError("endpoint, region, bucket and path prefix are immutable; create a new engine instead")
	}
	if incoming.Status == "" {
		incoming.Status = existing.Status
	}
	if incoming.Status == types.PlatformStorageEngineStatusDisabled && existing.Status != types.PlatformStorageEngineStatusDisabled {
		// Check if any workspace is using this engine
		var count int64
		if err := s.db.WithContext(ctx).Model(&types.Tenant{}).Where("platform_storage_engine_id = ?", incoming.ID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return apperrors.NewBadRequestError(fmt.Sprintf("cannot disable: %d workspace(s) are using this engine", count))
		}
	}
	if err := incoming.Validate(); err != nil {
		return err
	}
	if err := validatePlatformStorageEngineEndpoint(incoming); err != nil {
		return err
	}
	if err := s.Test(ctx, incoming); err != nil {
		return apperrors.NewBadRequestError("storage connection test failed").WithDetails(secutils.SanitizeStorageConnectivityError(err))
	}
	incoming.UpdatedAt = time.Now()
	return s.repo.Update(ctx, incoming)
}

func (s *PlatformStorageEngineService) Delete(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var engine types.PlatformStorageEngine
		query := tx.Where("id = ?", id)
		if tx.Dialector.Name() == "postgres" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := query.First(&engine).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperrors.NewNotFoundError("storage engine not found")
			}
			return err
		}
		// Check if any workspace is using this engine
		var count int64
		if err := tx.Model(&types.Tenant{}).Where("platform_storage_engine_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return apperrors.NewBadRequestError(fmt.Sprintf("cannot delete: %d workspace(s) are using this engine", count))
		}
		return tx.Delete(&engine).Error
	})
}

func (s *PlatformStorageEngineService) Test(ctx context.Context, engine *types.PlatformStorageEngine) error {
	if err := engine.Validate(); err != nil {
		return err
	}
	if err := validatePlatformStorageEngineEndpoint(engine); err != nil {
		return err
	}
	if engine.Provider == "local" {
		baseDir := strings.TrimSpace(os.Getenv("LOCAL_STORAGE_BASE_DIR"))
		if baseDir == "" {
			baseDir = "/data/files"
		}
		candidate := filepath.Join(baseDir, strings.Trim(strings.TrimSpace(engine.Config.PathPrefix), "/\\"))
		safeDir, err := secutils.SafePathUnderBase(baseDir, candidate)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(safeDir, 0o755); err != nil {
			return fmt.Errorf("create local storage directory: %w", err)
		}
	}
	c := engine.Config
	switch engine.Provider {
	case "local":
		fileService, _, err := filesvc.NewFileServiceFromStorageConfig("local", engine.ToStorageEngineConfig(), "")
		if err != nil {
			return err
		}
		return fileService.CheckConnectivity(ctx)
	case "minio":
		if c.Mode == "docker" {
			c.Endpoint = os.Getenv("MINIO_ENDPOINT")
			c.AccessKeyID = os.Getenv("MINIO_ACCESS_KEY_ID")
			c.SecretAccessKey = os.Getenv("MINIO_SECRET_ACCESS_KEY")
			if c.BucketName == "" {
				c.BucketName = os.Getenv("MINIO_BUCKET_NAME")
			}
		}
		return filesvc.CheckMinioConnectivity(ctx, c.Endpoint, c.AccessKeyID, c.SecretAccessKey, c.BucketName, c.UseSSL)
	case "cos":
		return filesvc.CheckCosConnectivity(ctx, c.BucketName, c.Region, c.AccessKeyID, c.SecretAccessKey)
	case "tos":
		return filesvc.CheckTosConnectivity(ctx, c.Endpoint, c.Region, c.AccessKeyID, c.SecretAccessKey, c.BucketName)
	case "s3":
		return filesvc.CheckS3ConnectivityWithOptions(ctx, c.Endpoint, c.AccessKeyID, c.SecretAccessKey, c.BucketName, c.Region, c.ForcePathStyle)
	case "oss":
		return filesvc.CheckOssConnectivity(ctx, c.Endpoint, c.Region, c.AccessKeyID, c.SecretAccessKey, c.BucketName)
	case "ks3":
		return filesvc.CheckKS3Connectivity(ctx, c.Endpoint, c.Region, c.AccessKeyID, c.SecretAccessKey, c.BucketName)
	case "obs":
		return filesvc.CheckObsConnectivity(ctx, c.Endpoint, c.Region, c.AccessKeyID, c.SecretAccessKey, c.BucketName)
	default:
		return fmt.Errorf("unsupported storage provider: %s", engine.Provider)
	}
}

func (s *PlatformStorageEngineService) AssignToTenant(ctx context.Context, tenantID uint64, engineID string, assignedBy uint64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Verify engine exists and is active
		var engine types.PlatformStorageEngine
		if err := tx.Where("id = ?", engineID).First(&engine).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperrors.NewNotFoundError("storage engine not found")
			}
			return err
		}
		if engine.Status != types.PlatformStorageEngineStatusActive {
			return apperrors.NewBadRequestError("only an active storage engine can be assigned")
		}
		// Verify tenant exists
		var tenant types.Tenant
		if err := tx.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperrors.NewNotFoundError("workspace not found")
			}
			return err
		}
		// Assign engine
		return tx.Model(&types.Tenant{}).Where("id = ?", tenantID).
			Update("platform_storage_engine_id", engineID).Error
	})
}

func (s *PlatformStorageEngineService) UnassignFromTenant(ctx context.Context, tenantID uint64) error {
	return s.db.WithContext(ctx).Model(&types.Tenant{}).Where("id = ?", tenantID).
		Update("platform_storage_engine_id", nil).Error
}

func (s *PlatformStorageEngineService) GetByID(ctx context.Context, id string) (*types.PlatformStorageEngine, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *PlatformStorageEngineService) List(ctx context.Context) ([]*types.PlatformStorageEngine, error) {
	return s.repo.List(ctx)
}

func (s *PlatformStorageEngineService) GetTenantAssignment(ctx context.Context, tenantID uint64) (*types.PlatformStorageEngine, error) {
	var tenant types.Tenant
	if err := s.db.WithContext(ctx).Select("id", "platform_storage_engine_id").
		Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return nil, err
	}
	if tenant.PlatformStorageEngineID == nil {
		return nil, nil
	}
	return s.repo.GetByID(ctx, *tenant.PlatformStorageEngineID)
}

func (s *PlatformStorageEngineService) CountTenants(ctx context.Context, engineID string) (int64, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&types.Tenant{}).Where("platform_storage_engine_id = ?", engineID).Count(&count).Error
	return count, err
}

func validatePlatformStorageEngineEndpoint(engine *types.PlatformStorageEngine) error {
	if engine.Provider == "local" || (engine.Provider == "minio" && engine.Config.Mode == "docker") {
		return nil
	}
	endpoint := strings.TrimSpace(engine.Config.Endpoint)
	if engine.Provider == "cos" || endpoint == "" {
		return nil
	}
	if !strings.Contains(endpoint, "://") {
		scheme := "https://"
		if engine.Provider == "minio" && !engine.Config.UseSSL {
			scheme = "http://"
		}
		endpoint = scheme + endpoint
	}
	if err := secutils.ValidateURLForSSRF(endpoint); err != nil {
		return apperrors.NewBadRequestError("storage endpoint failed SSRF validation").WithDetails(err.Error())
	}
	return nil
}