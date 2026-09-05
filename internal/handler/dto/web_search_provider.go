package dto

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// WebSearchProviderResponse mirrors types.WebSearchProviderEntity for
// response bodies, with the APIKey field removed by construction. Credential
// presence is exposed via the /credentials subresource.
type WebSearchProviderResponse struct {
	ID          string                         `json:"id"`
	TenantID    uint64                         `json:"tenant_id"`
	Name        string                         `json:"name"`
	Provider    types.WebSearchProviderType    `json:"provider"`
	Description string                         `json:"description"`
	Parameters  WebSearchProviderParametersDTO `json:"parameters"`
	IsDefault   bool                           `json:"is_default"`
	CreatedAt   time.Time                      `json:"created_at"`
	UpdatedAt   time.Time                      `json:"updated_at"`
	// Per-field "configured?" map. See MCPServiceResponse.Credentials.
	Credentials map[string]CredentialFieldMetadata `json:"credentials,omitempty"`
}

// WebSearchProviderParametersDTO holds every parameter field except APIKey.
// EngineID, BaseURL, ProxyURL and ExtraConfig are not secrets (they describe
// where to send the request, not how to authenticate) and remain visible.
type WebSearchProviderParametersDTO struct {
	EngineID    string            `json:"engine_id,omitempty"`
	BaseURL     string            `json:"base_url,omitempty"`
	ProxyURL    string            `json:"proxy_url,omitempty"`
	ExtraConfig map[string]string `json:"extra_config,omitempty"`
}

// NewWebSearchProviderResponse converts a stored entity into its response shape.
func NewWebSearchProviderResponse(ctx context.Context, e *types.WebSearchProviderEntity) *WebSearchProviderResponse {
	if e == nil {
		return nil
	}
	params := WebSearchProviderParametersDTO{
		EngineID:    e.Parameters.EngineID,
		BaseURL:     e.Parameters.BaseURL,
		ProxyURL:    e.Parameters.ProxyURL,
		ExtraConfig: e.Parameters.ExtraConfig,
	}
	if !CanViewIntegrationSecrets(ctx) {
		params.ProxyURL = ""
		params.ExtraConfig = nil
	}
	return &WebSearchProviderResponse{
		ID:          e.ID,
		TenantID:    e.TenantID,
		Name:        e.Name,
		Provider:    e.Provider,
		Description: e.Description,
		Parameters:  params,
		IsDefault:   e.IsDefault,
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
		Credentials: map[string]CredentialFieldMetadata{
			"api_key": {Configured: e.Parameters.APIKey != ""},
		},
	}
}

func NewWebSearchProviderResponses(ctx context.Context, es []*types.WebSearchProviderEntity) []*WebSearchProviderResponse {
	out := make([]*WebSearchProviderResponse, 0, len(es))
	for _, e := range es {
		out = append(out, NewWebSearchProviderResponse(ctx, e))
	}
	return out
}

// WebSearchProviderAssignmentDTO mirrors
// types.WebSearchProviderAssignmentInfo — the admin-console view of one
// "this workspace uses this service" row.
type WebSearchProviderAssignmentDTO struct {
	TenantID   uint64    `json:"tenant_id"`
	TenantName string    `json:"tenant_name"`
	IsDefault  bool      `json:"is_default"`
	AssignedAt time.Time `json:"assigned_at"`
}

// SystemWebSearchProviderResponse is the platform-catalog shape served by
// /system/admin/web-search-providers: the base provider plus its workspace
// assignments. IsDefault is deliberately absent — a default is a
// per-workspace property carried by the assignment rows, not by the
// platform row.
type SystemWebSearchProviderResponse struct {
	ID          string                         `json:"id"`
	TenantID    uint64                         `json:"tenant_id"`
	Name        string                         `json:"name"`
	Provider    types.WebSearchProviderType    `json:"provider"`
	Description string                         `json:"description"`
	Parameters  WebSearchProviderParametersDTO `json:"parameters"`
	CreatedAt   time.Time                      `json:"created_at"`
	UpdatedAt   time.Time                      `json:"updated_at"`
	// Per-field "configured?" map. See MCPServiceResponse.Credentials.
	Credentials  map[string]CredentialFieldMetadata `json:"credentials,omitempty"`
	Assignments  []WebSearchProviderAssignmentDTO   `json:"assignments"`
}

// NewSystemWebSearchProviderResponse converts a platform entity plus its
// assignment rows into the admin-console response shape.
func NewSystemWebSearchProviderResponse(
	ctx context.Context,
	e *types.WebSearchProviderEntity,
	assignments []types.WebSearchProviderAssignmentInfo,
) *SystemWebSearchProviderResponse {
	if e == nil {
		return nil
	}
	params := WebSearchProviderParametersDTO{
		EngineID:    e.Parameters.EngineID,
		BaseURL:     e.Parameters.BaseURL,
		ProxyURL:    e.Parameters.ProxyURL,
		ExtraConfig: e.Parameters.ExtraConfig,
	}
	if !CanViewIntegrationSecrets(ctx) {
		params.ProxyURL = ""
		params.ExtraConfig = nil
	}
	dtos := make([]WebSearchProviderAssignmentDTO, 0, len(assignments))
	for _, a := range assignments {
		dtos = append(dtos, WebSearchProviderAssignmentDTO{
			TenantID:   a.TenantID,
			TenantName: a.TenantName,
			IsDefault:  a.IsDefault,
			AssignedAt: a.AssignedAt,
		})
	}
	return &SystemWebSearchProviderResponse{
		ID:          e.ID,
		TenantID:    e.TenantID,
		Name:        e.Name,
		Provider:    e.Provider,
		Description: e.Description,
		Parameters:  params,
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
		Credentials: map[string]CredentialFieldMetadata{
			"api_key": {Configured: e.Parameters.APIKey != ""},
		},
		Assignments: dtos,
	}
}
