//go:build opensandbox_integration

package sandbox

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestOpenSandboxIntegrationLifecycle exercises the adapter against a real
// OpenSandbox lifecycle server. Credentials are supplied via environment
// variables and must never be committed to the repository.
//
//	OPENSANDBOX_API_URL   lifecycle base URL (must include /v1; default http://127.0.0.1:8080/v1)
//	OPENSANDBOX_API_KEY   server api_key (required)
//	OPENSANDBOX_IMAGE     image for new sandboxes (default wechatopenai/weknora-sandbox:main)
//	OPENSANDBOX_SNAPSHOTS set to 1 to also exercise the snapshot roundtrip
func TestOpenSandboxIntegrationLifecycle(t *testing.T) {
	apiURL := os.Getenv("OPENSANDBOX_API_URL")
	if apiURL == "" {
		apiURL = "http://127.0.0.1:8080/v1"
	}
	apiKey := os.Getenv("OPENSANDBOX_API_KEY")
	if apiKey == "" {
		t.Skip("set OPENSANDBOX_API_KEY to run the OpenSandbox integration test")
	}
	image := os.Getenv("OPENSANDBOX_IMAGE")
	if image == "" {
		image = DefaultOpenSandboxTemplateImage
	}

	client, err := NewOpenSandboxRemoteClient(&Config{
		Type:                  SandboxTypeOpenSandbox,
		AllowPrivateEndpoints: true,
		OpenSandboxAPIURL:     apiURL,
		OpenSandboxAPIKey:     apiKey,
		OpenSandboxTemplate:   image,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	if err := client.Health(ctx); err != nil {
		t.Skipf("opensandbox server not reachable at %s: %v", apiURL, err)
	}

	handle, err := client.Create(ctx, RemoteCreateRequest{
		TemplateID: image,
		Metadata:   map[string]string{"weknora_integration": "go-test"},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Delete(context.Background(), handle.ID()) })

	t.Run("exec marker", func(t *testing.T) {
		res, err := client.Exec(ctx, handle, RemoteExecRequest{
			Command: "printf integration-ok", Shell: true,
		})
		require.NoError(t, err)
		require.Zero(t, res.ExitCode)
		require.Contains(t, res.Stdout, "integration-ok")
	})

	t.Run("reconnect preserves session", func(t *testing.T) {
		rebound, err := client.Connect(ctx, handle.ID())
		require.NoError(t, err)

		res, err := client.Exec(ctx, rebound, RemoteExecRequest{
			Command: "echo still-alive", Shell: true,
		})
		require.NoError(t, err)
		require.Contains(t, res.Stdout, "still-alive")
	})

	t.Run("files roundtrip", func(t *testing.T) {
		payload := []byte("artifact payload\nline two\n")
		require.NoError(t, client.WriteFile(ctx, handle, "/workspace/output/marker.txt", payload))

		read, err := client.ReadFile(ctx, handle, "/workspace/output/marker.txt")
		require.NoError(t, err)
		require.Equal(t, payload, read)
	})

	t.Run("exec timeout kills", func(t *testing.T) {
		res, err := client.Exec(ctx, handle, RemoteExecRequest{
			Command: "sleep 30", Shell: true, Timeout: 2 * time.Second,
		})
		require.NoError(t, err)
		require.True(t, res.Killed, "a command exceeding its timeout must be killed")
		require.Equal(t, -1, res.ExitCode)
	})

	if os.Getenv("OPENSANDBOX_SNAPSHOTS") == "1" {
		t.Run("snapshot roundtrip", func(t *testing.T) {
			name := "weknora-integration-" + time.Now().Format("150405")
			ref, err := client.CreateSnapshot(ctx, handle.ID(), name)
			require.NoError(t, err)
			t.Cleanup(func() { _ = client.DeleteSnapshot(context.Background(), ref.ID) })

			refs, err := client.ListSnapshots(ctx, handle.ID())
			require.NoError(t, err)
			ids := make([]string, len(refs))
			for i, r := range refs {
				ids[i] = r.ID
			}
			require.Contains(t, ids, ref.ID)
		})
	}

	t.Run("summary", func(t *testing.T) {
		summary, err := client.Get(ctx, handle.ID())
		require.NoError(t, err)
		require.Equal(t, RemoteStateRunning, summary.State)
		require.Equal(t, "go-test", summary.Metadata["weknora_integration"])
	})
}