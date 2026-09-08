package sandbox

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// newTestOpenSandboxRemoteClient wires a real OpenSandboxRemoteClient at the
// two-plane mock. Tests exercise the adapter through its public
// RemoteSandboxClient / RemoteSnapshotManager / RemoteTemplateCatalog surface
// only.
func newTestOpenSandboxRemoteClient(t *testing.T, mock *openSandboxMockServer) *OpenSandboxRemoteClient {
	t.Helper()
	client, err := NewOpenSandboxRemoteClient(openSandboxTestConfig(t, mock))
	require.NoError(t, err)
	return client
}

func TestOpenSandboxRemoteClientProviderAndCapabilities(t *testing.T) {
	client := newTestOpenSandboxRemoteClient(t, newOpenSandboxMockServer(t))

	require.Equal(t, SandboxTypeOpenSandbox, client.Provider())
	require.Equal(t, RemoteSandboxCapabilities{
		SupportsReconnect:             true,
		SupportsMetadata:              true,
		SupportsListSandboxes:         true,
		SupportsPauseResume:           true,
		SupportsTimeoutRefresh:        true,
		SupportsFilesystemEnumeration: true,
		SupportsSnapshots:             true,
		SupportsVolumes:               false,
	}, client.Capabilities())
}

func TestOpenSandboxRemoteClientConstructorValidatesAPIURL(t *testing.T) {
	mock := newOpenSandboxMockServer(t)
	for _, apiURL := range []string{"", "   ", "not-a-url", "ftp://host:8080/v1"} {
		cfg := openSandboxTestConfig(t, mock)
		cfg.OpenSandboxAPIURL = apiURL
		_, err := NewOpenSandboxRemoteClient(cfg)
		require.Error(t, err, "api url %q must be rejected", apiURL)
		require.True(t, IsRemoteInvalidRequest(err))
	}
	_, err := NewOpenSandboxRemoteClient(nil)
	require.Error(t, err)
}

func TestOpenSandboxHealth(t *testing.T) {
	client := newTestOpenSandboxRemoteClient(t, newOpenSandboxMockServer(t))

	require.NoError(t, client.Health(context.Background()))
}

func TestOpenSandboxHealthSurfacesAuthFailures(t *testing.T) {
	mock := newOpenSandboxMockServer(t)
	mock.failLifecycleStatus = 401
	client := newTestOpenSandboxRemoteClient(t, mock)

	err := client.Health(context.Background())

	require.Error(t, err)
	require.Equal(t, RemoteErrorKindAuthentication, remoteKind(err))
}

func TestOpenSandboxCreateSendsImageAndDefaults(t *testing.T) {
	mock := newOpenSandboxMockServer(t)
	client := newTestOpenSandboxRemoteClient(t, mock)
	ctx := context.Background()

	handle, err := client.Create(ctx, RemoteCreateRequest{
		TemplateID: "registry.example.com/weknora-sandbox:main",
		Metadata:   map[string]string{"weknora_session": "sess-1"},
		EnvVars:    map[string]string{"WEKNORA_TENANT": "t1"},
	})
	require.NoError(t, err)
	require.Equal(t, "osb-1", handle.ID())
	require.Equal(t, SandboxTypeOpenSandbox, handle.Provider())

	mock.mu.Lock()
	body := mock.createBody
	mock.mu.Unlock()
	image, ok := body["image"].(map[string]any)
	require.True(t, ok, "image template must be sent as image.uri, got %v", body)
	require.Equal(t, "registry.example.com/weknora-sandbox:main", image["uri"])
	require.NotContains(t, body, "snapshotId")
	// The configured TTL is the initial absolute expiry (30min default).
	require.Equal(t, float64(1800), body["timeout"])
	// ResourceLimits has no omitempty in the SDK; the adapter sends {}.
	_, isLimits := body["resourceLimits"].(map[string]any)
	require.True(t, isLimits, "create must send an explicit empty resourceLimits object")
	meta, _ := body["metadata"].(map[string]any)
	require.Equal(t, "sess-1", meta["weknora_session"])
	env, _ := body["env"].(map[string]any)
	require.Equal(t, "t1", env["WEKNORA_TENANT"])
}

func TestOpenSandboxCreateSendsAPIKeyOnBothPlanes(t *testing.T) {
	mock := newOpenSandboxMockServer(t)
	client := newTestOpenSandboxRemoteClient(t, mock)
	ctx := context.Background()

	handle, err := client.Create(ctx, RemoteCreateRequest{TemplateID: "img:latest"})
	require.NoError(t, err)
	mock.SetExecutor(func(string, osbMockCommandRequest) osbMockExecOutcome {
		return osbMockExecOutcome{exit: 0, delivery: "stream"}
	})
	_, err = client.Exec(ctx, handle, RemoteExecRequest{Command: "true", Shell: true})
	require.NoError(t, err)

	mock.mu.Lock()
	lifecycleAuth := append([]string(nil), mock.lifecycleAuth...)
	execdAuth := append([]string(nil), mock.execdAuth...)
	mock.mu.Unlock()
	require.NotEmpty(t, lifecycleAuth)
	require.NotEmpty(t, execdAuth)
	for _, v := range lifecycleAuth {
		require.Equal(t, "test-key", v)
	}
	for _, v := range execdAuth {
		require.Equal(t, "test-key", v)
	}
}

func TestOpenSandboxCreateFromSnapshotID(t *testing.T) {
	mock := newOpenSandboxMockServer(t)
	client := newTestOpenSandboxRemoteClient(t, mock)

	_, err := client.Create(context.Background(), RemoteCreateRequest{TemplateID: "snap-abc123"})

	require.NoError(t, err)
	mock.mu.Lock()
	body := mock.createBody
	mock.mu.Unlock()
	require.Equal(t, "snap-abc123", body["snapshotId"])
	require.NotContains(t, body, "image")
}

func TestOpenSandboxCreateClampsTTLToServerFloor(t *testing.T) {
	mock := newOpenSandboxMockServer(t)
	cfg := openSandboxTestConfig(t, mock)
	cfg.OpenSandboxSandboxTTL = 10 * time.Second
	client, err := NewOpenSandboxRemoteClient(cfg)
	require.NoError(t, err)

	_, err = client.Create(context.Background(), RemoteCreateRequest{TemplateID: "img:latest"})

	require.NoError(t, err)
	mock.mu.Lock()
	body := mock.createBody
	mock.mu.Unlock()
	require.Equal(t, float64(60), body["timeout"])
}

func TestOpenSandboxCreateUsesExplicitTimeout(t *testing.T) {
	mock := newOpenSandboxMockServer(t)
	client := newTestOpenSandboxRemoteClient(t, mock)

	_, err := client.Create(context.Background(), RemoteCreateRequest{
		TemplateID: "img:latest",
		Timeout:    RemoteTimeoutPolicy{Mode: RemoteTimeoutExplicit, Value: 5 * time.Minute},
	})

	require.NoError(t, err)
	mock.mu.Lock()
	body := mock.createBody
	mock.mu.Unlock()
	require.Equal(t, float64(300), body["timeout"])
}

func TestOpenSandboxCreateRejectsInvalidRequests(t *testing.T) {
	client := newTestOpenSandboxRemoteClient(t, newOpenSandboxMockServer(t))
	ctx := context.Background()

	cases := []struct {
		name    string
		request RemoteCreateRequest
		kind    RemoteErrorKind
	}{
		{"empty template", RemoteCreateRequest{TemplateID: "  "}, RemoteErrorKindInvalidRequest},
		{"volume mounts", RemoteCreateRequest{
			TemplateID:   "img:latest",
			VolumeMounts: []RemoteVolumeMount{{Name: "v", Path: "/data"}},
		}, RemoteErrorKindUnsupported},
		{"network policy", RemoteCreateRequest{
			TemplateID: "img:latest",
			Network:    RemoteNetworkPolicy{AllowInternetAccess: boolPtr(false)},
		}, RemoteErrorKindUnsupported},
		{"bad timeout mode", RemoteCreateRequest{
			TemplateID: "img:latest",
			Timeout:    RemoteTimeoutPolicy{Mode: "bogus"},
		}, RemoteErrorKindInvalidRequest},
		{"pause action", RemoteCreateRequest{
			TemplateID: "img:latest",
			Timeout:    RemoteTimeoutPolicy{Action: RemoteOnTimeoutPause},
		}, RemoteErrorKindInvalidRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := client.Create(ctx, tc.request)
			require.Error(t, err)
			require.Equal(t, tc.kind, remoteKind(err))
		})
	}
}

func TestOpenSandboxCreateWaitsForRunning(t *testing.T) {
	mock := newOpenSandboxMockServer(t)
	// The create response and the first poll report Pending; only the second
	// poll sees Running, so Create must survive one waitReady cycle.
	mock.pendingCreateStates = []string{"Pending", "Pending", "Running"}
	client := newTestOpenSandboxRemoteClient(t, mock)

	handle, err := client.Create(context.Background(), RemoteCreateRequest{TemplateID: "img:latest"})

	require.NoError(t, err)
	require.Equal(t, "osb-1", handle.ID())
	require.Equal(t, int32(1), mock.createCount.Load(), "the poll loop must not re-create")
	require.GreaterOrEqual(t, mock.endpointHits.Load(), int32(1), "readiness must ping execd")
}

func TestOpenSandboxCreateReapsFailedSandbox(t *testing.T) {
	mock := newOpenSandboxMockServer(t)
	mock.pendingCreateStates = []string{"Failed"}
	client := newTestOpenSandboxRemoteClient(t, mock)

	_, err := client.Create(context.Background(), RemoteCreateRequest{TemplateID: "img:latest"})

	require.Error(t, err)
	require.Equal(t, RemoteErrorKindUnavailable, remoteKind(err))
	require.Equal(t, int32(1), mock.deleteCount.Load(),
		"a sandbox that never became ready must be reaped")
	require.Equal(t, 0, mock.SandboxCount())
}

func TestOpenSandboxLifecycleRoundtrip(t *testing.T) {
	mock := newOpenSandboxMockServer(t)
	client := newTestOpenSandboxRemoteClient(t, mock)
	ctx := context.Background()

	handle, err := client.Create(ctx, RemoteCreateRequest{
		TemplateID: "img:latest",
		Metadata:   map[string]string{"weknora_session": "sess-1", "tenant": "t1"},
	})
	require.NoError(t, err)

	summary, err := client.Get(ctx, handle.ID())
	require.NoError(t, err)
	require.Equal(t, handle.ID(), summary.ID)
	require.Equal(t, RemoteStateRunning, summary.State)
	require.Equal(t, "Running", summary.RawState)
	require.Equal(t, "img:latest", summary.TemplateID)
	require.Equal(t, "sess-1", summary.Metadata["weknora_session"])

	// A second sandbox with different metadata proves the server-side filter
	// drives List, plus the client-side re-check.
	_, err = client.Create(ctx, RemoteCreateRequest{
		TemplateID: "img:latest",
		Metadata:   map[string]string{"weknora_session": "sess-2"},
	})
	require.NoError(t, err)

	matched, err := client.List(ctx, RemoteListFilter{Metadata: map[string]string{"weknora_session": "sess-1"}})
	require.NoError(t, err)
	require.Len(t, matched, 1)
	require.Equal(t, handle.ID(), matched[0].ID)

	all, err := client.List(ctx, RemoteListFilter{})
	require.NoError(t, err)
	require.Len(t, all, 2)

	running, err := client.List(ctx, RemoteListFilter{States: []RemoteSandboxState{RemoteStatePaused}})
	require.NoError(t, err)
	require.Empty(t, running)

	require.NoError(t, client.Delete(ctx, handle.ID()))
	_, err = client.Get(ctx, handle.ID())
	require.Error(t, err)
	require.True(t, IsRemoteNotFound(err))

	err = client.Delete(ctx, "osb-missing")
	require.Error(t, err)
	require.True(t, IsRemoteNotFound(err))
}

func TestOpenSandboxConnectResumesPausedSandbox(t *testing.T) {
	mock := newOpenSandboxMockServer(t)
	client := newTestOpenSandboxRemoteClient(t, mock)
	ctx := context.Background()

	handle, err := client.Create(ctx, RemoteCreateRequest{TemplateID: "img:latest"})
	require.NoError(t, err)
	mock.mu.Lock()
	mock.sandboxes[handle.ID()].states = []string{"Paused"}
	mock.mu.Unlock()

	rebound, err := client.Connect(ctx, handle.ID())

	require.NoError(t, err)
	require.Equal(t, handle.ID(), rebound.ID())
	require.Equal(t, int32(1), mock.resumeCount.Load())
	require.Equal(t, int32(0), mock.pauseCount.Load())
}

func TestOpenSandboxConnectRenewsExpiry(t *testing.T) {
	mock := newOpenSandboxMockServer(t)
	client := newTestOpenSandboxRemoteClient(t, mock)
	ctx := context.Background()

	handle, err := client.Create(ctx, RemoteCreateRequest{TemplateID: "img:latest"})
	require.NoError(t, err)

	before := time.Now()
	_, err = client.Connect(ctx, handle.ID())
	require.NoError(t, err)

	renews := mock.RenewExpiries()
	require.Len(t, renews, 1, "every Connect must renew the absolute expiry")
	require.WithinDuration(t, before.Add(30*time.Minute), renews[0], 5*time.Second)
}

func TestOpenSandboxExecShellDefaultsToSessionUser(t *testing.T) {
	mock := newOpenSandboxMockServer(t)
	mock.SetExecutor(func(_ string, req osbMockCommandRequest) osbMockExecOutcome {
		return osbMockExecOutcome{
			stdout: []string{"line-1", "line-2"}, exit: 3, delivery: "stream",
		}
	})
	client := newTestOpenSandboxRemoteClient(t, mock)
	ctx := context.Background()
	handle, err := client.Create(ctx, RemoteCreateRequest{TemplateID: "img:latest"})
	require.NoError(t, err)

	result, err := client.Exec(ctx, handle, RemoteExecRequest{
		Command: "echo line-1 && echo line-2; exit 3",
		Shell:   true,
		WorkDir: "/workspace",
		Env:     map[string]string{"FLAG": "1"},
		Timeout: 30 * time.Second,
	})

	require.NoError(t, err)
	require.Equal(t, "line-1\nline-2", result.Stdout)
	require.Equal(t, 3, result.ExitCode)
	require.False(t, result.Killed)

	cmd := mock.LastCommand()
	require.Equal(t, "echo line-1 && echo line-2; exit 3", cmd.command)
	require.Equal(t, "/workspace", cmd.cwd)
	require.Equal(t, "1", cmd.envs["FLAG"])
	require.Equal(t, int64(30000), cmd.timeout)
	require.Equal(t, int32(1000), *cmd.uid, "user must map to uid 1000")
	require.Equal(t, int32(1000), *cmd.gid)
}

func TestOpenSandboxExecArgvAndUserMapping(t *testing.T) {
	mock := newOpenSandboxMockServer(t)
	mock.SetExecutor(func(string, osbMockCommandRequest) osbMockExecOutcome {
		return osbMockExecOutcome{exit: 0, delivery: "stream"}
	})
	client := newTestOpenSandboxRemoteClient(t, mock)
	ctx := context.Background()
	handle, err := client.Create(ctx, RemoteCreateRequest{TemplateID: "img:latest"})
	require.NoError(t, err)

	_, err = client.Exec(ctx, handle, RemoteExecRequest{
		Command: "python3", Args: []string{"-c", "print(1)"},
	})
	require.NoError(t, err)
	require.Equal(t, buildShellLine("python3", []string{"-c", "print(1)"}), mock.LastCommand().command)
	require.Zero(t, mock.LastCommand().timeout, "no exec timeout must leave the wire timeout unset")
	require.Equal(t, int32(1000), *mock.LastCommand().uid)

	_, err = client.Exec(ctx, handle, RemoteExecRequest{Command: "id", Shell: true, User: "root"})
	require.NoError(t, err)
	require.Equal(t, int32(0), *mock.LastCommand().uid)
	require.Equal(t, int32(0), *mock.LastCommand().gid)

	_, err = client.Exec(ctx, handle, RemoteExecRequest{Command: "id", Shell: true, User: "nobody"})
	require.NoError(t, err)
	require.Nil(t, mock.LastCommand().uid, "unknown users fall back to the server default")
	require.Nil(t, mock.LastCommand().gid)
}

func TestOpenSandboxExecWrapsStdin(t *testing.T) {
	mock := newOpenSandboxMockServer(t)
	mock.SetExecutor(func(string, osbMockCommandRequest) osbMockExecOutcome {
		return osbMockExecOutcome{exit: 0, delivery: "stream"}
	})
	client := newTestOpenSandboxRemoteClient(t, mock)
	ctx := context.Background()
	handle, err := client.Create(ctx, RemoteCreateRequest{TemplateID: "img:latest"})
	require.NoError(t, err)

	_, err = client.Exec(ctx, handle, RemoteExecRequest{
		Command: "wc -l", Shell: true, Stdin: "hello\nworld\n",
	})

	require.NoError(t, err)
	cmd := mock.LastCommand().command
	require.Contains(t, cmd, "cat <<'WEKNORA_STDIN_EOF'")
	require.Contains(t, cmd, "hello\nworld\n")
}

func TestOpenSandboxExecExitCodePaths(t *testing.T) {
	cases := []struct {
		name     string
		outcome  osbMockExecOutcome
		exitCode int
		stderr   string
	}{
		{
			name:     "stream execution_complete",
			outcome:  osbMockExecOutcome{exit: 3, delivery: "stream"},
			exitCode: 3,
		},
		{
			name:     "status fallback when stream omits it",
			outcome:  osbMockExecOutcome{exit: 7, delivery: "status"},
			exitCode: 7,
		},
		{
			name:     "error event carries numeric evalue",
			outcome:  osbMockExecOutcome{exit: 2, delivery: "error"},
			exitCode: 2,
			stderr:   "exit: 2", // an error event with no stderr lines surfaces as Stderr
		},
		{
			name:     "defensive in-stream exit_code wins over evalue",
			outcome:  osbMockExecOutcome{exit: 9, delivery: "error_field"},
			exitCode: 9,
			stderr:   "exit: 1",
		},
		{
			name:     "no code anywhere falls back to zero",
			outcome:  osbMockExecOutcome{exit: 5, delivery: "none"},
			exitCode: 0,
		},
		{
			name: "stderr lines are separated from stdout",
			outcome: osbMockExecOutcome{
				stdout: []string{"out"}, stderr: []string{"err-1", "err-2"},
				exit: 1, delivery: "stream",
			},
			exitCode: 1,
			stderr:   "err-1\nerr-2",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := newOpenSandboxMockServer(t)
			outcome := tc.outcome
			mock.SetExecutor(func(string, osbMockCommandRequest) osbMockExecOutcome {
				return outcome
			})
			client := newTestOpenSandboxRemoteClient(t, mock)
			ctx := context.Background()
			handle, err := client.Create(ctx, RemoteCreateRequest{TemplateID: "img:latest"})
			require.NoError(t, err)

			result, err := client.Exec(ctx, handle, RemoteExecRequest{Command: "x", Shell: true})

			require.NoError(t, err)
			require.Equal(t, tc.exitCode, result.ExitCode)
			require.Equal(t, tc.stderr, result.Stderr)
			require.False(t, result.Killed)
		})
	}
}

func TestOpenSandboxExecTimeoutKills(t *testing.T) {
	mock := newOpenSandboxMockServer(t)
	mock.SetExecutor(func(string, osbMockCommandRequest) osbMockExecOutcome {
		return osbMockExecOutcome{
			stdout: []string{"partial"}, delay: 800 * time.Millisecond,
			exit: 0, delivery: "stream",
		}
	})
	client := newTestOpenSandboxRemoteClient(t, mock)
	ctx := context.Background()
	handle, err := client.Create(ctx, RemoteCreateRequest{TemplateID: "img:latest"})
	require.NoError(t, err)

	result, err := client.Exec(ctx, handle, RemoteExecRequest{
		Command: "sleep 800", Shell: true, Timeout: 200 * time.Millisecond,
	})

	require.NoError(t, err)
	require.True(t, result.Killed)
	require.Equal(t, -1, result.ExitCode)
	require.Equal(t, "partial", result.Stdout)
}

func TestOpenSandboxCommandStreamOutlivesHTTPTimeout(t *testing.T) {
	mock := newOpenSandboxMockServer(t)
	mock.SetExecutor(func(string, osbMockCommandRequest) osbMockExecOutcome {
		return osbMockExecOutcome{
			stdout: []string{"finished"}, delay: 300 * time.Millisecond,
			exit: 0, delivery: "stream",
		}
	})
	cfg := openSandboxTestConfig(t, mock)
	cfg.OpenSandboxHTTPTimeout = 100 * time.Millisecond
	client, err := NewOpenSandboxRemoteClient(cfg)
	require.NoError(t, err)
	handle, err := client.Create(context.Background(), RemoteCreateRequest{TemplateID: "img:latest"})
	require.NoError(t, err)

	result, err := client.Exec(context.Background(), handle, RemoteExecRequest{
		Command: "slow", Shell: true, Timeout: 5 * time.Second,
	})

	require.NoError(t, err, "command streams must not share the HTTP call timeout")
	require.Equal(t, "finished", result.Stdout)
	require.Equal(t, 0, result.ExitCode)
}

func TestOpenSandboxExecRejectsInvalidRequests(t *testing.T) {
	mock := newOpenSandboxMockServer(t)
	client := newTestOpenSandboxRemoteClient(t, mock)
	ctx := context.Background()
	handle, err := client.Create(ctx, RemoteCreateRequest{TemplateID: "img:latest"})
	require.NoError(t, err)

	t.Run("foreign handle", func(t *testing.T) {
		_, err := client.Exec(ctx, &fakeRemoteHandle{id: "x", provider: SandboxTypeCube}, RemoteExecRequest{
			Command: "true", Shell: true,
		})
		require.Error(t, err)
		require.True(t, IsRemoteInvalidRequest(err))
	})
	t.Run("empty command", func(t *testing.T) {
		_, err := client.Exec(ctx, handle, RemoteExecRequest{Command: "  ", Shell: true})
		require.Error(t, err)
		require.True(t, IsRemoteInvalidRequest(err))
	})
	t.Run("shell with argv", func(t *testing.T) {
		_, err := client.Exec(ctx, handle, RemoteExecRequest{
			Command: "ls", Shell: true, Args: []string{"-l"},
		})
		require.Error(t, err)
		require.True(t, IsRemoteInvalidRequest(err))
	})
	t.Run("negative timeout", func(t *testing.T) {
		_, err := client.Exec(ctx, handle, RemoteExecRequest{
			Command: "ls", Shell: true, Timeout: -time.Second,
		})
		require.Error(t, err)
		require.True(t, IsRemoteInvalidRequest(err))
	})
}

func TestOpenSandboxFileWriteReadRoundtrip(t *testing.T) {
	mock := newOpenSandboxMockServer(t)
	client := newTestOpenSandboxRemoteClient(t, mock)
	ctx := context.Background()
	handle, err := client.Create(ctx, RemoteCreateRequest{TemplateID: "img:latest"})
	require.NoError(t, err)

	content := []byte("artifact payload\nline two\n")
	require.NoError(t, client.WriteFile(ctx, handle, "/workspace/output/result.txt", content))

	read, err := client.ReadFile(ctx, handle, "/workspace/output/result.txt")
	require.NoError(t, err)
	require.Equal(t, content, read)

	_, err = client.ReadFile(ctx, handle, "/workspace/output/missing.txt")
	require.Error(t, err)
	require.True(t, IsRemoteNotFound(err))

	err = client.WriteFile(ctx, handle, "  ", content)
	require.Error(t, err)
	require.True(t, IsRemoteInvalidRequest(err))
}

func TestOpenSandboxListDirAndStat(t *testing.T) {
	mock := newOpenSandboxMockServer(t)
	client := newTestOpenSandboxRemoteClient(t, mock)
	ctx := context.Background()
	handle, err := client.Create(ctx, RemoteCreateRequest{TemplateID: "img:latest"})
	require.NoError(t, err)
	require.NoError(t, client.WriteFile(ctx, handle, "/workspace/input/a.txt", []byte("aaa")))
	require.NoError(t, client.WriteFile(ctx, handle, "/workspace/input/b.txt", []byte("bbbb")))
	require.NoError(t, client.MakeDir(ctx, handle, "/workspace/input/sub"))

	entries, err := client.ListDir(ctx, handle, "/workspace/input")
	require.NoError(t, err)
	names := map[string]RemoteDirEntryType{}
	for _, entry := range entries {
		names[entry.Name] = entry.Type
		require.True(t, entry.Path == "/workspace/input/"+entry.Name)
	}
	require.Equal(t, RemoteEntryFile, names["a.txt"])
	require.Equal(t, RemoteEntryFile, names["b.txt"])
	require.Equal(t, RemoteEntryDir, names["sub"])

	stat, err := client.Stat(ctx, handle, "/workspace/input/a.txt")
	require.NoError(t, err)
	require.Equal(t, RemoteEntryFile, stat.Type)
	require.Equal(t, int64(3), stat.Size)

	stat, err = client.Stat(ctx, handle, "/workspace/input/sub")
	require.NoError(t, err)
	require.Equal(t, RemoteEntryDir, stat.Type)

	_, err = client.Stat(ctx, handle, "/workspace/input/nope")
	require.Error(t, err)
	require.True(t, IsRemoteNotFound(err))

	// Empty path lists the root.
	entries, err = client.ListDir(ctx, handle, "")
	require.NoError(t, err)
	require.NotEmpty(t, entries)
}

func TestOpenSandboxMakeDirIsIdempotent(t *testing.T) {
	mock := newOpenSandboxMockServer(t)
	client := newTestOpenSandboxRemoteClient(t, mock)
	ctx := context.Background()
	handle, err := client.Create(ctx, RemoteCreateRequest{TemplateID: "img:latest"})
	require.NoError(t, err)

	require.NoError(t, client.MakeDir(ctx, handle, "/workspace/output"))
	require.NoError(t, client.MakeDir(ctx, handle, "/workspace/output"))

	// A server that rejects mkdir of an existing path with a 500 + "already
	// exists" body must still be treated as success (ignoreExistingDir).
	mock.mu.Lock()
	mock.directoryExistsStatus = 500
	mock.mu.Unlock()
	require.NoError(t, client.MakeDir(ctx, handle, "/workspace/output"))

	err = client.MakeDir(ctx, handle, "  ")
	require.Error(t, err)
	require.True(t, IsRemoteInvalidRequest(err))
}

func TestOpenSandboxRemoveDispatchesByKind(t *testing.T) {
	mock := newOpenSandboxMockServer(t)
	client := newTestOpenSandboxRemoteClient(t, mock)
	ctx := context.Background()
	handle, err := client.Create(ctx, RemoteCreateRequest{TemplateID: "img:latest"})
	require.NoError(t, err)
	require.NoError(t, client.MakeDir(ctx, handle, "/workspace/input/dir"))
	require.NoError(t, client.WriteFile(ctx, handle, "/workspace/input/dir/inner.txt", []byte("x")))
	require.NoError(t, client.WriteFile(ctx, handle, "/workspace/input/file.txt", []byte("y")))

	// Directory removal is recursive.
	require.NoError(t, client.Remove(ctx, handle, "/workspace/input/dir"))
	_, err = client.Stat(ctx, handle, "/workspace/input/dir/inner.txt")
	require.Error(t, err)
	require.True(t, IsRemoteNotFound(err))

	// File removal must go through the file endpoint, not the directory one.
	require.NoError(t, client.Remove(ctx, handle, "/workspace/input/file.txt"))
	_, err = client.Stat(ctx, handle, "/workspace/input/file.txt")
	require.Error(t, err)
	require.True(t, IsRemoteNotFound(err))

	err = client.Remove(ctx, handle, "/workspace/input/nope")
	require.Error(t, err)
	require.True(t, IsRemoteNotFound(err))
}

func TestOpenSandboxSnapshotTrio(t *testing.T) {
	mock := newOpenSandboxMockServer(t)
	client := newTestOpenSandboxRemoteClient(t, mock)
	ctx := context.Background()
	handle, err := client.Create(ctx, RemoteCreateRequest{TemplateID: "img:latest"})
	require.NoError(t, err)

	ref, err := client.CreateSnapshot(ctx, handle.ID(), "weknora-sk-cfg1-g1")
	require.NoError(t, err)
	require.Equal(t, "snap-1", ref.ID)
	require.Equal(t, []string{"weknora-sk-cfg1-g1"}, ref.Names)
	require.NotEmpty(t, ref.ID, "snapshot must be Ready before CreateSnapshot returns")

	refs, err := client.ListSnapshots(ctx, handle.ID())
	require.NoError(t, err)
	require.Len(t, refs, 1)
	require.Equal(t, "snap-1", refs[0].ID)
	require.Equal(t, []string{"weknora-sk-cfg1-g1"}, refs[0].Names)

	require.NoError(t, client.DeleteSnapshot(ctx, ref.ID))
	refs, err = client.ListSnapshots(ctx, handle.ID())
	require.NoError(t, err)
	require.Empty(t, refs)

	// Delete is idempotent: a missing snapshot is success.
	require.NoError(t, client.DeleteSnapshot(ctx, ref.ID))
}

func TestOpenSandboxSnapshotEdgeCases(t *testing.T) {
	mock := newOpenSandboxMockServer(t)
	client := newTestOpenSandboxRemoteClient(t, mock)
	ctx := context.Background()
	handle, err := client.Create(ctx, RemoteCreateRequest{TemplateID: "img:latest"})
	require.NoError(t, err)

	_, err = client.CreateSnapshot(ctx, "  ", "n")
	require.Error(t, err)
	require.True(t, IsRemoteInvalidRequest(err))

	_, err = client.CreateSnapshot(ctx, "osb-missing", "n")
	require.Error(t, err)
	require.True(t, IsRemoteNotFound(err))

	err = client.DeleteSnapshot(ctx, "  ")
	require.Error(t, err)
	require.True(t, IsRemoteInvalidRequest(err))

	refs, err := client.ListSnapshots(ctx, handle.ID())
	require.NoError(t, err)
	require.Empty(t, refs)
}

func TestOpenSandboxListRejectsStuckPagination(t *testing.T) {
	mock := newOpenSandboxMockServer(t)
	mock.stuckPagination = true
	client := newTestOpenSandboxRemoteClient(t, mock)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := client.List(ctx, RemoteListFilter{})

	require.Error(t, err)
	require.True(t, IsRemoteInvalidRequest(err))
}

func TestOpenSandboxListSnapshotsRejectsStuckPagination(t *testing.T) {
	mock := newOpenSandboxMockServer(t)
	mock.stuckPagination = true
	client := newTestOpenSandboxRemoteClient(t, mock)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := client.ListSnapshots(ctx, "osb-1")

	require.Error(t, err)
	require.True(t, IsRemoteInvalidRequest(err))
}

func TestOpenSandboxStateNormalization(t *testing.T) {
	cases := map[string]RemoteSandboxState{
		"Running":      RemoteStateRunning,
		"running":      RemoteStateRunning,
		"Paused":       RemoteStatePaused,
		"Pending":      RemoteStateTransitioning,
		"Creating":     RemoteStateTransitioning,
		"Provisioning": RemoteStateTransitioning,
		"Resuming":     RemoteStateTransitioning,
		"Terminated":   RemoteStateTerminal,
		"Stopped":      RemoteStateTerminal,
		"Failed":       RemoteStateTerminal,
		"error":        RemoteStateTerminal,
		"Weird":        RemoteStateUnknown,
		"":             RemoteStateUnknown,
	}
	for raw, want := range cases {
		require.Equal(t, want, normalizeOpenSandboxState(raw), "state %q", raw)
	}
}

func TestOpenSandboxErrorNormalization(t *testing.T) {
	cases := []struct {
		status int
		kind   RemoteErrorKind
	}{
		{400, RemoteErrorKindInvalidRequest},
		{401, RemoteErrorKindAuthentication},
		{403, RemoteErrorKindAuthentication},
		{404, RemoteErrorKindNotFound},
		{409, RemoteErrorKindConflict},
		{410, RemoteErrorKindTerminal},
		{429, RemoteErrorKindCapacity},
		{500, RemoteErrorKindUnavailable},
		{503, RemoteErrorKindUnavailable},
	}
	for _, tc := range cases {
		t.Run(strconv.Itoa(tc.status), func(t *testing.T) {
			mock := newOpenSandboxMockServer(t)
			mock.failLifecycleStatus = tc.status
			client := newTestOpenSandboxRemoteClient(t, mock)

			err := client.Health(context.Background())

			require.Error(t, err)
			require.Equal(t, tc.kind, remoteKind(err))
		})
	}
}

func TestOpenSandboxCreateNotFoundIsInvalidTemplate(t *testing.T) {
	mock := newOpenSandboxMockServer(t)
	mock.failLifecycleStatus = 404
	client := newTestOpenSandboxRemoteClient(t, mock)

	_, err := client.Create(context.Background(), RemoteCreateRequest{TemplateID: "img:latest"})

	require.Error(t, err)
	require.Equal(t, RemoteErrorKindInvalidRequest, remoteKind(err),
		"a 404 on create means the template is bad, not that a sandbox vanished")
}

func TestOpenSandboxExecdErrorNormalization(t *testing.T) {
	mock := newOpenSandboxMockServer(t)
	client := newTestOpenSandboxRemoteClient(t, mock)
	ctx := context.Background()
	handle, err := client.Create(ctx, RemoteCreateRequest{TemplateID: "img:latest"})
	require.NoError(t, err)

	mock.mu.Lock()
	mock.failExecdStatus = 400
	mock.mu.Unlock()
	_, err = client.Exec(ctx, handle, RemoteExecRequest{Command: "x", Shell: true})
	require.Error(t, err)
	require.Equal(t, RemoteErrorKindInvalidRequest, remoteKind(err))

	mock.mu.Lock()
	mock.failExecdStatus = 503
	mock.mu.Unlock()
	_, err = client.ReadFile(ctx, handle, "/workspace/whatever")
	require.Error(t, err)
	require.Equal(t, RemoteErrorKindUnavailable, remoteKind(err))
}

func boolPtr(v bool) *bool { return &v }
