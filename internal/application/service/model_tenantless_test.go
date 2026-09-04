package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

// System-admin console paths (model debug, parser credential status) run
// without a tenant in the context. The model getters must resolve
// platform-wide instead of panicking on the missing TenantIDContextKey —
// this is exactly the regression behind the console debug 500s.
func TestGetModelByID_TenantlessContextResolvesPlatformWide(t *testing.T) {
	modelID := "platform-model"
	svc := NewModelService(
		&stubModelRepoForDelete{model: &types.Model{
			ID: modelID, TenantID: 0, Status: types.ModelStatusActive,
		}},
		&stubKBRepoForModelDelete{},
		&stubAgentRepoForModelDelete{},
		nil, nil, nil,
	)

	model, err := svc.GetModelByID(context.Background(), modelID)
	require.NoError(t, err)
	require.NotNil(t, model)
	require.Equal(t, modelID, model.ID)
}
