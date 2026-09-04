package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/models/asr"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/models/embedding"
	"github.com/Tencent/WeKnora/internal/models/rerank"
	"github.com/Tencent/WeKnora/internal/models/vlm"
	"github.com/Tencent/WeKnora/internal/types"
)

// ModelService defines the model service interface
type ModelService interface {
	// CreateModel creates a model
	CreateModel(ctx context.Context, model *types.Model) error
	// GetModelByID gets a model by ID
	GetModelByID(ctx context.Context, id string) (*types.Model, error)
	// ListModels lists all models
	ListModels(ctx context.Context) ([]*types.Model, error)
	// UpdateModel updates a model
	UpdateModel(ctx context.Context, model *types.Model) error
	// DeleteModel deletes a model
	DeleteModel(ctx context.Context, id string) error

	// UpdateModelCredentials writes one or more credential fields on the
	// model's Parameters. Nil pointer means "do not touch this field";
	// empty string is treated as no-op (use ClearModelCredential to remove).
	// Returns the updated model.
	UpdateModelCredentials(ctx context.Context, id string, apiKey, appSecret *string) (*types.Model, error)
	// ClearModelCredential removes a single credential field. field must be
	// "api_key" or "app_secret". Clearing an already-empty field is a no-op.
	ClearModelCredential(ctx context.Context, id, field string) error

	// ---- System-admin platform catalog (models are platform-managed since
	// // the 000094 enterprise model governance rework; these methods must
	// not read the tenant from the request context because a system admin
	// may have no workspace binding).

	// ListAllModels lists the whole platform catalog with optional filters.
	ListAllModels(ctx context.Context, modelType types.ModelType, nameQuery string) ([]*types.Model, error)
	// GetPlatformModel fetches a model regardless of workspace visibility,
	// with the same status gating as GetModelByID.
	GetPlatformModel(ctx context.Context, id string) (*types.Model, error)
	// CreatePlatformModel creates a model owned by the platform
	// (tenant_id = 0); local models still download in the background.
	CreatePlatformModel(ctx context.Context, model *types.Model) error
	// UpdatePlatformModel updates a model without a workspace scope;
	// builtin rows keep their shared semantics.
	UpdatePlatformModel(ctx context.Context, model *types.Model) error
	// UpdatePlatformModelCredentials / ClearPlatformModelCredential are the
	// workspace-scope-free variants of the credential subresource.
	UpdatePlatformModelCredentials(ctx context.Context, id string, apiKey, appSecret *string) (*types.Model, error)
	ClearPlatformModelCredential(ctx context.Context, id, field string) error
	// DeletePlatformModel deletes a model after checking references across
	// every workspace (KBs, agents, tenant memory pins).
	DeletePlatformModel(ctx context.Context, id string) error

	// ---- Workspace model assignments.

	// ListTenantAssignedModels returns the models assigned to a workspace
	// (builtin models excluded — they are visible to everyone anyway).
	ListTenantAssignedModels(ctx context.Context, tenantID uint64) ([]*types.Model, error)
	// SetTenantModelAssignments replaces the full assignment list of a
	// workspace. Removing a model that the workspace still references
	// (KB / agent / memory config) fails with ErrModelInUse.
	SetTenantModelAssignments(ctx context.Context, tenantID uint64, modelIDs []string, assignedBy string) error
	// GetEmbeddingModel gets an embedding model
	GetEmbeddingModel(ctx context.Context, modelId string) (embedding.Embedder, error)
	// GetEmbeddingModelForTenant gets an embedding model for a specific tenant (for cross-tenant sharing)
	GetEmbeddingModelForTenant(ctx context.Context, modelId string, tenantID uint64) (embedding.Embedder, error)
	// GetRerankModel gets a rerank model
	GetRerankModel(ctx context.Context, modelId string) (rerank.Reranker, error)
	// GetChatModel gets a chat model
	GetChatModel(ctx context.Context, modelId string) (chat.Chat, error)
	// GetVLMModel gets a vision language model
	GetVLMModel(ctx context.Context, modelId string) (vlm.VLM, error)
	// GetASRModel gets an automatic speech recognition model
	GetASRModel(ctx context.Context, modelId string) (asr.ASR, error)
}

// ModelRepository defines the model repository interface
type ModelRepository interface {
	// Create creates a model
	Create(ctx context.Context, model *types.Model) error
	// GetByID gets a model by ID
	GetByID(ctx context.Context, tenantID uint64, id string) (*types.Model, error)
	// List lists all models
	List(
		ctx context.Context,
		tenantID uint64,
		modelType types.ModelType,
		source types.ModelSource,
	) ([]*types.Model, error)
	// Update updates a model
	Update(ctx context.Context, model *types.Model) error
	// Delete deletes a model
	Delete(ctx context.Context, tenantID uint64, id string) error
	// ClearDefaultByType clears the default flag for all models of a specific type
	// optionally excluding a specific model ID.
	ClearDefaultByType(ctx context.Context, tenantID uint, modelType types.ModelType, excludeID string) error

	// ---- Platform catalog / assignment surface (system admin console).
	// GetByIDAnyTenant / ListAll / DeleteAnyTenant bypass workspace
	// visibility on purpose and are only reachable through SystemAdmin-guarded
	// handlers.
	GetByIDAnyTenant(ctx context.Context, id string) (*types.Model, error)
	ListAll(ctx context.Context, modelType types.ModelType, source types.ModelSource, nameQuery string) ([]*types.Model, error)
	DeleteAnyTenant(ctx context.Context, id string) error
	// CountModelUsages counts references to a model. tenantID > 0 scopes to
	// one workspace; 0 counts the whole platform.
	CountModelUsages(ctx context.Context, tenantID uint64, modelID string) (kbCount, agentCount, memoryCount int64, err error)
	// ListAssignedModelIDs returns the model IDs assigned to a workspace.
	ListAssignedModelIDs(ctx context.Context, tenantID uint64) ([]string, error)
	// ReplaceAssignments atomically replaces the assignment list of a workspace.
	ReplaceAssignments(ctx context.Context, tenantID uint64, modelIDs []string, assignedBy string) error
	// DeleteAssignmentsByModelID removes every assignment of one model.
	DeleteAssignmentsByModelID(ctx context.Context, modelID string) error
}
