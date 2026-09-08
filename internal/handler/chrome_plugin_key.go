package handler

import (
	"fmt"
	"net/http"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// ChromePluginKeyHandler handles user-level Chrome plugin API key operations.
// It allows regular users (including workspace admins) to generate a dedicated
// API key for the Chrome extension with predefined capabilities.
type ChromePluginKeyHandler struct {
	apiKeyService interfaces.TenantAPIKeyService
	kbService     interfaces.KnowledgeBaseService
	userService   interfaces.UserService
}

func NewChromePluginKeyHandler(
	apiKeyService interfaces.TenantAPIKeyService,
	kbService interfaces.KnowledgeBaseService,
	userService interfaces.UserService,
) *ChromePluginKeyHandler {
	return &ChromePluginKeyHandler{
		apiKeyService: apiKeyService,
		kbService:     kbService,
		userService:   userService,
	}
}

// ChromePluginKeyPrefix is the naming convention for Chrome plugin keys.
// Format: {EmployeeID}-{Username}-ApiKey
const ChromePluginKeyPrefix = "ApiKey"

// chromePluginCapabilities are the predefined capabilities for Chrome plugin keys.
// They include: retrieve (KB search), chat (conversation), read_agents (list agents),
// and ingest (write KB content).
var chromePluginCapabilities = []string{
	string(types.APIKeyCapabilityRetrieve),
	string(types.APIKeyCapabilityChat),
	string(types.APIKeyCapabilityReadAgents),
	string(types.APIKeyCapabilityIngest),
}

// GenerateKey creates a new Chrome plugin API key for the current user.
// The key is named "{EmployeeID}-{Username}-ApiKey" and has predefined capabilities.
// Knowledge base scope is automatically set to KBs with AllowMemberContribute=true.
func (h *ChromePluginKeyHandler) GenerateKey(c *gin.Context) {
	ctx := c.Request.Context()

	// Get current user info - UserID is stored as string in context
	userIDVal, exists := c.Get(types.UserIDContextKey.String())
	if !exists {
		c.Error(errors.NewUnauthorizedError("user not authenticated"))
		return
	}
	userID, ok := userIDVal.(string)
	if !ok || userID == "" {
		c.Error(errors.NewUnauthorizedError("user not authenticated"))
		return
	}

	user, err := h.userService.GetUserByID(ctx, userID)
	if err != nil || user == nil {
		c.Error(errors.NewNotFoundError("user not found"))
		return
	}

	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	if tenantID == 0 {
		c.Error(errors.NewBadRequestError("workspace context missing"))
		return
	}

	// Generate key name: {EmployeeID}-{Username}-ApiKey
	keyName := fmt.Sprintf("%s-%s-%s", user.EmployeeID, user.Username, ChromePluginKeyPrefix)

	// Check if key already exists
	existingKeys, err := h.apiKeyService.ListAPIKeys(ctx, tenantID)
	if err != nil {
		c.Error(errors.NewInternalServerError("failed to list API keys").WithDetails(err.Error()))
		return
	}

	for _, key := range existingKeys {
		if key.Name == keyName && !key.IsRevoked() {
			c.Error(errors.NewConflictError("Chrome plugin key already exists"))
			return
		}
	}

	// Get KBs with AllowMemberContribute=true
	kbs, err := h.kbService.ListKnowledgeBasesByTenantID(ctx, tenantID)
	if err != nil {
		c.Error(errors.NewInternalServerError("failed to list knowledge bases").WithDetails(err.Error()))
		return
	}

	var kbIDs []string
	for _, kb := range kbs {
		if kb.AllowMemberContribute {
			kbIDs = append(kbIDs, kb.ID)
		}
	}

	// Create the API key
	result, err := h.apiKeyService.CreateAPIKey(ctx, interfaces.TenantAPIKeyCreateRequest{
		TenantID:         tenantID,
		Name:             keyName,
		FullAccess:       false,
		KnowledgeBaseIDs: kbIDs,
		Capabilities:     chromePluginCapabilities,
	})
	if err != nil {
		c.Error(errors.NewInternalServerError("failed to create API key").WithDetails(err.Error()))
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data": gin.H{
			"id":                 result.APIKey.ID,
			"name":               result.APIKey.Name,
			"api_key":            result.Token,
			"knowledge_base_ids": result.APIKey.KnowledgeBaseIDs,
			"capabilities":       result.APIKey.Capabilities,
			"created_at":         result.APIKey.CreatedAt,
		},
	})
}

// GetKey retrieves the current user's Chrome plugin key (if exists).
// Returns the key info with masked API key value.
func (h *ChromePluginKeyHandler) GetKey(c *gin.Context) {
	ctx := c.Request.Context()

	// Get current user info - UserID is stored as string in context
	userIDVal, exists := c.Get(types.UserIDContextKey.String())
	if !exists {
		c.Error(errors.NewUnauthorizedError("user not authenticated"))
		return
	}
	userID, ok := userIDVal.(string)
	if !ok || userID == "" {
		c.Error(errors.NewUnauthorizedError("user not authenticated"))
		return
	}

	user, err := h.userService.GetUserByID(ctx, userID)
	if err != nil || user == nil {
		c.Error(errors.NewNotFoundError("user not found"))
		return
	}

	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	if tenantID == 0 {
		c.Error(errors.NewBadRequestError("workspace context missing"))
		return
	}

	keyName := fmt.Sprintf("%s-%s-%s", user.EmployeeID, user.Username, ChromePluginKeyPrefix)

	// Find the key
	keys, err := h.apiKeyService.ListAPIKeys(ctx, tenantID)
	if err != nil {
		c.Error(errors.NewInternalServerError("failed to list API keys").WithDetails(err.Error()))
		return
	}

	for _, key := range keys {
		if key.Name == keyName && !key.IsRevoked() {
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data": gin.H{
					"id":                 key.ID,
					"name":               key.Name,
					"api_key":            maskAPIKey(key.APIKey),
					"knowledge_base_ids": key.KnowledgeBaseIDs,
					"capabilities":       key.Capabilities,
					"created_at":         key.CreatedAt,
				},
			})
			return
		}
	}

	// No key found
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    nil,
	})
}

// DeleteKey revokes the current user's Chrome plugin key.
func (h *ChromePluginKeyHandler) DeleteKey(c *gin.Context) {
	ctx := c.Request.Context()

	// Get current user info - UserID is stored as string in context
	userIDVal, exists := c.Get(types.UserIDContextKey.String())
	if !exists {
		c.Error(errors.NewUnauthorizedError("user not authenticated"))
		return
	}
	userID, ok := userIDVal.(string)
	if !ok || userID == "" {
		c.Error(errors.NewUnauthorizedError("user not authenticated"))
		return
	}

	user, err := h.userService.GetUserByID(ctx, userID)
	if err != nil || user == nil {
		c.Error(errors.NewNotFoundError("user not found"))
		return
	}

	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	if tenantID == 0 {
		c.Error(errors.NewBadRequestError("workspace context missing"))
		return
	}

	keyName := fmt.Sprintf("%s-%s-%s", user.EmployeeID, user.Username, ChromePluginKeyPrefix)

	// Find the key
	keys, err := h.apiKeyService.ListAPIKeys(ctx, tenantID)
	if err != nil {
		c.Error(errors.NewInternalServerError("failed to list API keys").WithDetails(err.Error()))
		return
	}

	var targetKey *types.TenantAPIKey
	for _, key := range keys {
		if key.Name == keyName && !key.IsRevoked() {
			targetKey = key
			break
		}
	}

	if targetKey == nil {
		c.Error(errors.NewNotFoundError("Chrome plugin key not found"))
		return
	}

	// Revoke the key
	if err := h.apiKeyService.RevokeAPIKey(ctx, tenantID, targetKey.ID); err != nil {
		c.Error(errors.NewInternalServerError("failed to revoke API key").WithDetails(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
	})
}

// maskAPIKey masks the API key for display purposes.
// Shows first 8 and last 4 characters, with asterisks in between.
func maskAPIKey(key string) string {
	if len(key) <= 12 {
		return key
	}
	return key[:8] + "****" + key[len(key)-4:]
}