package sandbox

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenSandboxTemplateCatalogSingleEntry(t *testing.T) {
	mock := newOpenSandboxMockServer(t)
	client := newTestOpenSandboxRemoteClient(t, mock)

	templates, err := client.ListTemplates(context.Background())
	require.NoError(t, err)
	require.Len(t, templates, 1)
	entry := templates[0]
	require.Equal(t, "registry.example.com/weknora-sandbox:main", entry.ID)
	require.Equal(t, entry.ID, entry.Name)
	require.Equal(t, "ready", entry.Status)
	require.Equal(t, entry.ID, entry.Image)
	require.False(t, entry.Standard, "a custom registry image is not the standard template")
}

func TestOpenSandboxTemplateCatalogDefaultsToStandardImage(t *testing.T) {
	mock := newOpenSandboxMockServer(t)
	cfg := openSandboxTestConfig(t, mock)
	cfg.OpenSandboxTemplate = "" // empty → defaults to the standard image
	client, err := NewOpenSandboxRemoteClient(cfg)
	require.NoError(t, err)

	templates, err := client.ListTemplates(context.Background())
	require.NoError(t, err)
	require.Len(t, templates, 1)
	entry := templates[0]
	require.Equal(t, DefaultOpenSandboxTemplateImage, entry.ID)
	require.True(t, entry.Standard, "the default image must be recognised as the standard template")
}

func TestOpenSandboxTemplateCatalogSnapshotEntry(t *testing.T) {
	mock := newOpenSandboxMockServer(t)
	cfg := openSandboxTestConfig(t, mock)
	cfg.OpenSandboxTemplate = "snap-abc123"
	client, err := NewOpenSandboxRemoteClient(cfg)
	require.NoError(t, err)

	templates, err := client.ListTemplates(context.Background())
	require.NoError(t, err)
	require.Len(t, templates, 1)
	entry := templates[0]
	require.Equal(t, "snap-abc123", entry.ID)
	require.False(t, entry.Standard, "a snapshot ID is never the standard template")
}

func TestOpenSandboxEnsureAndReplaceReturnEntry(t *testing.T) {
	mock := newOpenSandboxMockServer(t)
	client := newTestOpenSandboxRemoteClient(t, mock)

	ensure, err := client.EnsureStandardTemplate(context.Background())
	require.NoError(t, err)
	require.Equal(t, "registry.example.com/weknora-sandbox:main", ensure.ID)

	replace, err := client.ReplaceStandardTemplate(context.Background())
	require.NoError(t, err)
	require.Equal(t, ensure.ID, replace.ID)

	require.NoError(t, client.DeleteSupersededStandardTemplates(context.Background(), ensure.ID))
	require.Equal(t, int32(0), mock.deleteCount.Load(),
		"OpenSandbox keeps no cluster-side template objects; delete is a no-op")
}