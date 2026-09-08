// Package sandbox: OpenSandbox adapter for the provider-neutral
// RemoteSandboxClient.
//
// OpenSandboxRemoteClient implements RemoteSandboxClient on top of the
// official Go SDK's two low-level clients: LifecycleClient for the control
// plane and ExecdClient for the in-sandbox exec/filesystem daemon. Callers
// speak RemoteSandboxClient; OpenSandbox-specific types, HTTP status codes,
// and workarounds never leak past this file — every return value is either a
// neutral DTO or a RemoteError with a stable Kind.
//
// Two protocol properties shape this adapter:
//
//   - One origin. execd is reached through the lifecycle server's endpoint
//     proxy (GetEndpoint with useServerProxy=true), so control and data
//     traffic share the guarded control transport and the gateway split in
//     gateway_transport.go is explicitly disabled for this backend.
//   - Absolute expiry. OpenSandbox reaps a sandbox at expiresAt regardless of
//     activity. Create requests a TTL and every Connect renews it, which
//     turns the absolute clock into the idle timeout WeKnora's session
//     lifecycle expects.
package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	opensandbox "github.com/alibaba/OpenSandbox/sdks/sandbox/go"
)

// Tuning knobs for the adapter's bounded waits. The server has its own
// counterparts (sandbox_create_timeout_seconds, snapshot_create_timeout_
// seconds); these bound only the client side of the same waits.
const (
	// openSandboxMinTimeoutSeconds is the server-enforced floor on a
	// sandbox's absolute TTL (max_sandbox_timeout_seconds may additionally
	// cap it). Values below are clamped up rather than rejected so a short
	// configured TTL still produces a spawnable sandbox.
	openSandboxMinTimeoutSeconds = 60

	// openSandboxReadyTimeout bounds how long Create waits for the sandbox
	// to reach Running AND answer an execd ping. The server itself waits
	// ~60s for readiness before returning, so this mostly covers the Docker
	// runtime's asynchronous start plus execd bootstrap.
	openSandboxReadyTimeout = 60 * time.Second

	openSandboxReadyPollInterval = 2 * time.Second

	// openSandboxSnapshotReadyTimeout bounds the client-side wait for a
	// snapshot to leave Creating. The Kubernetes runtime commits the
	// filesystem in a job the server already waits for (default 900s), so
	// this poll is a safety net that usually exits on the first check.
	openSandboxSnapshotReadyTimeout = 15 * time.Minute
	openSandboxSnapshotPollInterval = 3 * time.Second
	openSandboxSnapshotListPageSize = 100
	openSandboxSandboxListPageSize  = 100
	openSandboxListPageCap          = 100
)

// OpenSandboxRemoteClient implements RemoteSandboxClient on top of the
// OpenSandbox lifecycle API and its per-sandbox execd daemon.
type OpenSandboxRemoteClient struct {
	config    *Config
	lifecycle *opensandbox.LifecycleClient
	// httpClient is shared by the lifecycle client and every execd client so
	// all traffic rides the pool's guarded transport. Its Timeout is
	// deliberately 0: command streams share the client and a global timeout
	// kills them mid-flight (see callCtx).
	httpClient  *http.Client
	ttl         time.Duration
	httpTimeout time.Duration
}

// NewOpenSandboxRemoteClient constructs an OpenSandbox-backed client using a
// self-owned transport pool. Suitable for throwaway connectivity probes; the
// session lifecycle uses the shared process pool.
func NewOpenSandboxRemoteClient(config *Config) (*OpenSandboxRemoteClient, error) {
	return NewOpenSandboxRemoteClientWithPool(config, nil)
}

// NewOpenSandboxRemoteClientWithPool builds a client whose connections come
// from a caller-owned pool. Named configs construct a client per request, so
// the pool is what keeps connections alive across requests. A nil pool
// installs one guarded by this config's outbound policy.
func NewOpenSandboxRemoteClientWithPool(
	config *Config,
	pool *SandboxGatewayTransportPool,
) (*OpenSandboxRemoteClient, error) {
	if config == nil {
		return nil, errors.New("opensandbox remote client config is required")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(config.OpenSandboxAPIURL), "/")
	if baseURL == "" {
		return nil, openSandboxInvalidRequest("New", "api url is required", nil)
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, openSandboxInvalidRequest(
			"New", fmt.Sprintf("invalid opensandbox api url %q", baseURL), err,
		)
	}

	ttl := config.OpenSandboxSandboxTTL
	if ttl <= 0 {
		ttl = DefaultOpenSandboxSandboxTTL
	}
	httpTimeout := config.OpenSandboxHTTPTimeout
	if httpTimeout <= 0 {
		httpTimeout = DefaultOpenSandboxHTTPTimeout
	}

	if pool == nil {
		pool = NewSandboxGatewayTransportPoolWithPolicy(nil, OutboundURLPolicy{
			AllowPrivate: config.AllowPrivateEndpoints,
		})
	}
	// OpenSandbox has no gateway split: execd rides the lifecycle server's
	// own proxy, so everything lands on the control transport.
	routingConfig := *config
	routingConfig.Type = SandboxTypeOpenSandbox
	httpClient := &http.Client{Transport: pool.RoundTripperFor(&routingConfig)}

	return &OpenSandboxRemoteClient{
		config:      config,
		lifecycle:   opensandbox.NewLifecycleClient(baseURL, config.OpenSandboxAPIKey, opensandbox.WithHTTPClient(httpClient)),
		httpClient:  httpClient,
		ttl:         ttl,
		httpTimeout: httpTimeout,
	}, nil
}

// openSandboxRemoteHandle is the RemoteSandboxHandle OpenSandbox returns. The
// execd client is resolved lazily and cached because Connect must succeed
// while execd is still bootstrapping; the mutex guards that lazy resolution
// against concurrent Exec/filesystem calls.
type openSandboxRemoteHandle struct {
	sandboxID string
	metadata  map[string]string

	mu    sync.Mutex
	execd *opensandbox.ExecdClient
}

func (h *openSandboxRemoteHandle) ID() string {
	if h == nil {
		return ""
	}
	return h.sandboxID
}

func (h *openSandboxRemoteHandle) Provider() RemoteProvider { return SandboxTypeOpenSandbox }

func (h *openSandboxRemoteHandle) Metadata() map[string]string {
	if h == nil {
		return nil
	}
	return cloneMetadata(h.metadata)
}

// --- RemoteSandboxClient ------------------------------------------------------

func (c *OpenSandboxRemoteClient) Provider() RemoteProvider { return SandboxTypeOpenSandbox }

func (c *OpenSandboxRemoteClient) Capabilities() RemoteSandboxCapabilities {
	return RemoteSandboxCapabilities{
		SupportsReconnect:             true,
		SupportsMetadata:              true,
		SupportsListSandboxes:         true,
		SupportsPauseResume:           true,
		SupportsTimeoutRefresh:        true,
		SupportsFilesystemEnumeration: true,
		// A snapshot ID can be handed straight back as
		// CreateSandboxRequest.SnapshotID, which is what makes the skill-image
		// chain work. The Kubernetes runtime's snapshot path is verified;
		// the Docker runtime's is not (a failure surfaces as a visible
		// skill-install error, never silently).
		SupportsSnapshots: true,
		// The SDK's Volume model (host/pvc/ossfs) is cluster-specific with no
		// neutral mapping; skills ride on snapshots instead.
		SupportsVolumes: false,
	}
}

func (c *OpenSandboxRemoteClient) Health(ctx context.Context) error {
	// The lifecycle API has no unauthenticated /health route we can use —
	// the cheapest request that proves both reachability and the API key is
	// a one-item sandbox page.
	callCtx, cancel := c.callCtx(ctx)
	defer cancel()
	if _, err := c.lifecycle.ListSandboxes(callCtx, opensandbox.ListOptions{
		Page: 1, PageSize: 1,
	}); err != nil {
		logger.Errorf(ctx, "opensandbox remote client health check failed: %v", err)
		return normalizeOpenSandboxError("Health", err)
	}
	return nil
}

// callCtx bounds one ordinary HTTP call. Command streams deliberately avoid
// it: they run under their own execution timeout, and the shared
// http.Client's Timeout must stay 0 or long streams die mid-flight.
func (c *OpenSandboxRemoteClient) callCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.httpTimeout <= 0 {
		return ctx, func() {}
	}
	if _, bounded := ctx.Deadline(); bounded {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, c.httpTimeout)
}

// snapshotCtx bounds a snapshot create/wait: the Kubernetes runtime commits
// the filesystem server-side and its own wait defaults to 900s, so the
// ordinary HTTP timeout would abort a legitimate create.
func (c *OpenSandboxRemoteClient) snapshotCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, bounded := ctx.Deadline(); bounded {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, openSandboxSnapshotReadyTimeout)
}

func (c *OpenSandboxRemoteClient) Create(
	ctx context.Context,
	request RemoteCreateRequest,
) (RemoteSandboxHandle, error) {
	template := strings.TrimSpace(request.TemplateID)
	if template == "" {
		return nil, openSandboxInvalidRequest("Create", "template ID is required", nil)
	}
	if len(request.VolumeMounts) > 0 {
		return nil, NewRemoteError(
			SandboxTypeOpenSandbox, "Create", RemoteErrorKindUnsupported,
			"the OpenSandbox adapter does not map volume mounts", nil,
		)
	}
	if !request.Network.IsZero() {
		// OpenSandbox egress policy needs a server-side egress sidecar
		// ([egress] image) and rejects networkPolicy otherwise, so a
		// caller-supplied policy would either 400 or silently miss rules.
		// Fail loudly instead; the cluster's default egress applies.
		return nil, NewRemoteError(
			SandboxTypeOpenSandbox, "Create", RemoteErrorKindUnsupported,
			"the OpenSandbox adapter does not map network policy; configure cluster egress server-side",
			nil,
		)
	}

	timeoutSeconds := int(c.ttl / time.Second)
	switch request.Timeout.Mode {
	case "", RemoteTimeoutServerDefault:
		// Still send an explicit timeout: the server default (600s) is
		// shorter than the idle semantics Connect's renew implements.
	case RemoteTimeoutExplicit:
		if request.Timeout.Value > 0 {
			timeoutSeconds = int(request.Timeout.Value / time.Second)
		}
	default:
		return nil, openSandboxInvalidRequest(
			"Create", fmt.Sprintf("unsupported timeout mode %q", request.Timeout.Mode), nil,
		)
	}
	action := request.Timeout.Action
	if action == "" {
		action = RemoteOnTimeoutKill
	}
	// OpenSandbox only ever terminates at expiry; pause-on-timeout has no
	// server-side counterpart to map onto.
	if action != RemoteOnTimeoutKill {
		return nil, openSandboxInvalidRequest(
			"Create",
			fmt.Sprintf("unsupported timeout action %q: OpenSandbox reaps sandboxes at expiry", action),
			nil,
		)
	}
	if timeoutSeconds < openSandboxMinTimeoutSeconds {
		timeoutSeconds = openSandboxMinTimeoutSeconds
	}

	req := opensandbox.CreateSandboxRequest{
		// ResourceLimits has no omitempty; an explicit empty map keeps the
		// JSON "resourceLimits":{} instead of null.
		ResourceLimits: opensandbox.ResourceLimits{},
		Timeout:        &timeoutSeconds,
		Env:            cloneMetadata(request.EnvVars),
		Metadata:       cloneMetadata(request.Metadata),
	}
	if strings.ContainsAny(template, ":/") {
		// An image URI always carries a tag/digest colon or a registry/repo
		// slash; snapshot IDs are opaque tokens with neither.
		req.Image = &opensandbox.ImageSpec{URI: template}
		// The lifecycle API rejects an image without an explicit entrypoint
		// (422 "Entrypoint is required when image is provided"). PID 1 only
		// needs to keep the sandbox alive: execd traffic rides the lifecycle
		// server's own endpoint proxy, and idle policy is enforced through
		// RenewExpiration on Connect, not the image's CMD. The SDK's stock
		// keep-alive is therefore the right neutral default.
		req.Entrypoint = append([]string(nil), opensandbox.DefaultEntrypoint...)
	} else {
		req.SnapshotID = template
	}

	callCtx, cancel := c.callCtx(ctx)
	defer cancel()
	info, err := c.lifecycle.CreateSandbox(callCtx, req)
	if err != nil {
		return nil, normalizeOpenSandboxError("Create", err)
	}
	if info == nil || strings.TrimSpace(info.ID) == "" {
		return nil, NewRemoteError(
			SandboxTypeOpenSandbox, "Create", RemoteErrorKindInternal,
			"opensandbox api: create sandbox: empty sandboxID", nil,
		)
	}

	if err := c.waitReady(ctx, info.ID); err != nil {
		// A sandbox that never became ready must not leak: reap it so a
		// warmup failure does not hold cluster capacity until TTL.
		delCtx, delCancel := c.callCtx(context.WithoutCancel(ctx))
		if derr := c.lifecycle.DeleteSandbox(delCtx, info.ID); derr != nil {
			logger.Warnf(ctx, "[OpenSandboxRemote] cleanup of unready sandbox %s failed: %v", info.ID, derr)
		}
		delCancel()
		return nil, err
	}

	handle := &openSandboxRemoteHandle{
		sandboxID: info.ID,
		metadata:  cloneMetadata(request.Metadata),
	}
	if _, err := c.resolveHandleExecd(ctx, handle); err != nil {
		// waitReady already pinged execd, so this is unexpected but not
		// fatal: Exec re-resolves on demand.
		logger.Warnf(ctx, "[OpenSandboxRemote] execd resolve after create failed for %s: %v", info.ID, err)
	}
	logOpenSandboxSandboxCreated(ctx, c, info.ID, template)
	return handle, nil
}

// waitReady polls until the sandbox reports Running and execd answers a ping.
// Image pulls on a cold node routinely consume most of the budget; a sandbox
// that enters Failed/Terminated fails immediately with its server-side reason.
func (c *OpenSandboxRemoteClient) waitReady(ctx context.Context, sandboxID string) error {
	deadline := time.Now().Add(openSandboxReadyTimeout)
	if ctxDeadline, bounded := ctx.Deadline(); bounded && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	var lastErr error = errors.New("sandbox not ready")
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("opensandbox waitReady: %w", err)
		}
		callCtx, cancel := c.callCtx(ctx)
		info, err := c.lifecycle.GetSandbox(callCtx, sandboxID)
		cancel()
		if err != nil {
			normalized := normalizeOpenSandboxError("Create", err)
			lastErr = normalized
			// Auth/config problems never fix themselves; only transient
			// lookup failures are worth the remaining budget.
			if remote := remoteErrorOf(normalized); remote != nil {
				switch remote.Kind {
				case RemoteErrorKindAuthentication, RemoteErrorKindInvalidRequest,
					RemoteErrorKindUnsupported, RemoteErrorKindInternal:
					return normalized
				}
			}
		} else if info != nil {
			state := strings.TrimSpace(string(info.Status.State))
			switch normalizeOpenSandboxState(state) {
			case RemoteStateRunning:
				if pingErr := c.pingExecd(ctx, sandboxID); pingErr == nil {
					return nil
				} else {
					lastErr = fmt.Errorf("execd not answering yet: %w", pingErr)
				}
			case RemoteStateTerminal:
				return NewRemoteError(
					SandboxTypeOpenSandbox, "Create", RemoteErrorKindUnavailable,
					fmt.Sprintf("sandbox entered %s before becoming ready (%s)",
						state, openSandboxStatusDetail(info.Status.Reason, info.Status.Message)),
					nil,
				)
			default:
				lastErr = fmt.Errorf("sandbox state %s", state)
			}
		}
		if !time.Now().Before(deadline) {
			if remote := remoteErrorOf(lastErr); remote != nil {
				remote.Kind = RemoteErrorKindTimeout
				return remote
			}
			return NewRemoteError(
				SandboxTypeOpenSandbox, "Create", RemoteErrorKindTimeout,
				fmt.Sprintf("sandbox %s not ready within %s: %v",
					sandboxID, openSandboxReadyTimeout, lastErr),
				nil,
			)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("opensandbox waitReady: %w", ctx.Err())
		case <-time.After(openSandboxReadyPollInterval):
		}
	}
}

// pingExecd verifies the data plane is up. It builds a throwaway client
// rather than caching: the caller (waitReady or Connect) decides whether a
// failure is fatal.
func (c *OpenSandboxRemoteClient) pingExecd(ctx context.Context, sandboxID string) error {
	execd, err := c.buildExecdClient(ctx, sandboxID)
	if err != nil {
		return err
	}
	pingCtx, cancel := c.callCtx(ctx)
	defer cancel()
	return execd.Ping(pingCtx)
}

// buildExecdClient resolves the execd endpoint through the lifecycle server's
// proxy and returns an ExecdClient authenticated with the lifecycle API key.
func (c *OpenSandboxRemoteClient) buildExecdClient(
	ctx context.Context, sandboxID string,
) (*opensandbox.ExecdClient, error) {
	callCtx, cancel := c.callCtx(ctx)
	defer cancel()
	useProxy := true
	endpoint, err := c.lifecycle.GetEndpoint(callCtx, sandboxID, OpenSandboxExecdPort, &useProxy)
	if err != nil {
		return nil, normalizeOpenSandboxError("GetEndpoint", err)
	}
	if endpoint == nil || strings.TrimSpace(endpoint.Endpoint) == "" {
		return nil, NewRemoteError(
			SandboxTypeOpenSandbox, "GetEndpoint", RemoteErrorKindInternal,
			"opensandbox returned an empty execd endpoint", nil,
		)
	}
	execdURL := endpoint.Endpoint
	if !strings.HasPrefix(execdURL, "http") {
		execdURL = "https://" + execdURL
	}
	opts := []opensandbox.Option{opensandbox.WithHTTPClient(c.httpClient)}
	if len(endpoint.Headers) > 0 {
		opts = append(opts, opensandbox.WithHeaders(endpoint.Headers))
	}
	return opensandbox.NewExecdClient(execdURL, c.config.OpenSandboxAPIKey, opts...), nil
}

// resolveHandleExecd returns the handle's cached execd client, resolving and
// caching one on first use.
func (c *OpenSandboxRemoteClient) resolveHandleExecd(
	ctx context.Context, handle *openSandboxRemoteHandle,
) (*opensandbox.ExecdClient, error) {
	if handle == nil || strings.TrimSpace(handle.sandboxID) == "" {
		return nil, openSandboxInvalidRequest("Exec", "handle was not issued by OpenSandbox", nil)
	}
	handle.mu.Lock()
	execd := handle.execd
	handle.mu.Unlock()
	if execd != nil {
		return execd, nil
	}
	execd, err := c.buildExecdClient(ctx, handle.sandboxID)
	if err != nil {
		return nil, err
	}
	handle.mu.Lock()
	handle.execd = execd
	handle.mu.Unlock()
	return execd, nil
}

func (c *OpenSandboxRemoteClient) Connect(
	ctx context.Context,
	sandboxID string,
) (RemoteSandboxHandle, error) {
	sandboxID = strings.TrimSpace(sandboxID)
	if sandboxID == "" {
		return nil, openSandboxInvalidRequest("Connect", "sandbox ID is required", nil)
	}
	info, err := c.getSandboxInfo(ctx, "Connect", sandboxID)
	if err != nil {
		return nil, err
	}

	if normalizeOpenSandboxState(string(info.Status.State)) == RemoteStatePaused {
		callCtx, cancel := c.callCtx(ctx)
		resumeErr := c.lifecycle.ResumeSandbox(callCtx, sandboxID)
		cancel()
		if resumeErr != nil {
			return nil, normalizeOpenSandboxError("Resume", resumeErr)
		}
		if err := c.waitReady(ctx, sandboxID); err != nil {
			return nil, err
		}
	}

	// Renew the absolute expiry on every reconnect: OpenSandbox reaps at
	// expiresAt regardless of activity, so without this a long-lived session
	// dies mid-conversation. Best-effort — a capped or missing renew leaves
	// the previous expiry in place and Get's Terminal state drives rebind.
	renewCtx, renewCancel := c.callCtx(ctx)
	if _, renewErr := c.lifecycle.RenewExpiration(
		renewCtx, sandboxID, time.Now().Add(c.ttl),
	); renewErr != nil {
		logger.Warnf(ctx,
			"[OpenSandboxRemote] renew on connect failed for %s: %v (previous expiry stands)",
			sandboxID, renewErr)
	}
	renewCancel()

	handle := &openSandboxRemoteHandle{
		sandboxID: sandboxID,
		metadata:  cloneMetadata(info.Metadata),
	}
	if _, err := c.resolveHandleExecd(ctx, handle); err != nil {
		logger.Warnf(ctx,
			"[OpenSandboxRemote] execd resolve on connect failed for %s: %v (first exec will retry)",
			sandboxID, err)
	}
	return handle, nil
}

func (c *OpenSandboxRemoteClient) Get(
	ctx context.Context,
	sandboxID string,
) (*RemoteSandboxSummary, error) {
	sandboxID = strings.TrimSpace(sandboxID)
	if sandboxID == "" {
		return nil, openSandboxInvalidRequest("Get", "sandbox ID is required", nil)
	}
	info, err := c.getSandboxInfo(ctx, "Get", sandboxID)
	if err != nil {
		return nil, err
	}
	return openSandboxSummary(*info), nil
}

func (c *OpenSandboxRemoteClient) getSandboxInfo(
	ctx context.Context, op, sandboxID string,
) (*opensandbox.SandboxInfo, error) {
	callCtx, cancel := c.callCtx(ctx)
	defer cancel()
	info, err := c.lifecycle.GetSandbox(callCtx, sandboxID)
	if err != nil {
		if isRemoteStatus(err, http.StatusNotFound) {
			return nil, NewRemoteError(
				SandboxTypeOpenSandbox, op, RemoteErrorKindNotFound,
				"sandbox not found", nil,
			)
		}
		return nil, normalizeOpenSandboxError(op, err)
	}
	if info == nil {
		return nil, NewRemoteError(
			SandboxTypeOpenSandbox, op, RemoteErrorKindNotFound,
			"sandbox not found", nil,
		)
	}
	return info, nil
}

func (c *OpenSandboxRemoteClient) List(
	ctx context.Context,
	filter RemoteListFilter,
) ([]RemoteSandboxSummary, error) {
	result := make([]RemoteSandboxSummary, 0)
	opts := opensandbox.ListOptions{
		Page:     1,
		PageSize: openSandboxSandboxListPageSize,
		Metadata: cloneMetadata(filter.Metadata),
	}
	for pages := 0; ; pages++ {
		if pages >= openSandboxListPageCap {
			return nil, NewRemoteError(
				SandboxTypeOpenSandbox, "List", RemoteErrorKindInvalidRequest,
				"provider pagination did not terminate", nil,
			)
		}
		callCtx, cancel := c.callCtx(ctx)
		resp, err := c.lifecycle.ListSandboxes(callCtx, opts)
		cancel()
		if err != nil {
			return nil, normalizeOpenSandboxError("List", err)
		}
		if resp == nil {
			return result, nil
		}
		for i := range resp.Items {
			summary := openSandboxSummary(resp.Items[i])
			// Metadata is filtered server-side (AND semantics); the
			// client-side re-check mirrors Cube's defensive posture against a
			// server that ignores the query param.
			if !metadataMatches(summary.Metadata, filter.Metadata) ||
				!StateMatches(summary.State, filter.States) {
				continue
			}
			result = append(result, *summary)
		}
		if !resp.Pagination.HasNextPage {
			return result, nil
		}
		opts.Page++
	}
}

func (c *OpenSandboxRemoteClient) Delete(ctx context.Context, sandboxID string) error {
	sandboxID = strings.TrimSpace(sandboxID)
	if sandboxID == "" {
		return openSandboxInvalidRequest("Delete", "sandbox ID is required", nil)
	}
	callCtx, cancel := c.callCtx(ctx)
	defer cancel()
	if err := c.lifecycle.DeleteSandbox(callCtx, sandboxID); err != nil {
		return normalizeOpenSandboxError("Delete", err)
	}
	return nil
}

// --- execution ----------------------------------------------------------------

func (c *OpenSandboxRemoteClient) Exec(
	ctx context.Context,
	handle RemoteSandboxHandle,
	request RemoteExecRequest,
) (*RemoteExecResult, error) {
	openHandle, err := openSandboxHandle("Exec", handle)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(request.Command) == "" {
		return nil, openSandboxInvalidRequest("Exec", "command is required", nil)
	}
	if request.Shell && len(request.Args) != 0 {
		return nil, openSandboxInvalidRequest(
			"Exec", "shell execution cannot include argv arguments", nil,
		)
	}
	if request.Timeout < 0 {
		return nil, openSandboxInvalidRequest("Exec", "execution timeout cannot be negative", nil)
	}
	execd, err := c.resolveHandleExecd(ctx, openHandle)
	if err != nil {
		return nil, err
	}
	user := request.User
	if user == "" {
		user = DefaultSandboxExecUser
	}

	execCtx := ctx
	cancel := func() {}
	if request.Timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, request.Timeout)
	}
	defer cancel()

	line := request.Command
	if !request.Shell {
		line = buildShellLine(request.Command, request.Args)
	}
	if request.Stdin != "" {
		line = wrapWithStdin(line, request.Stdin)
	}

	req := opensandbox.RunCommandRequest{
		Command: line,
		Cwd:     request.WorkDir,
		Envs:    cloneMetadata(request.Env),
	}
	if request.Timeout > 0 {
		// Server-side kill races the ctx deadline; whichever fires first,
		// the command is dead and the Killed result below reports it.
		req.Timeout = int64(request.Timeout / time.Millisecond)
	}
	// User comes from the neutral request rather than being hardcoded:
	// running everything as root silently defeats file-mode protections on
	// shared volumes.
	req.UID, req.GID = openSandboxExecUser(user)

	logger.Infof(ctx, "[OpenSandboxRemote] data-plane exec sandbox=%s exec_user=%s cmd=%q",
		openHandle.sandboxID, strings.TrimSpace(user), line)

	stream := &openSandboxCommandStream{}
	startedAt := time.Now()
	execErr := execd.RunCommand(execCtx, req, stream.handleEvent)
	duration := time.Since(startedAt)

	if execErr != nil {
		if request.Timeout > 0 && errors.Is(execCtx.Err(), context.DeadlineExceeded) {
			return &RemoteExecResult{
				Stdout:   stream.stdoutText(),
				Stderr:   stream.stderrText(),
				Duration: duration,
				Killed:   true,
				ExitCode: -1,
			}, nil
		}
		normalized := normalizeOpenSandboxError("Exec", execErr)
		logger.Warnf(ctx,
			"[OpenSandboxRemote] data-plane exec failed sandbox=%s detail=%s",
			openHandle.sandboxID, RemoteErrorDiagnostics(normalized),
		)
		return nil, normalized
	}

	exitCode := stream.recordedExitCode()
	if exitCode == nil {
		exitCode = c.fetchExitCode(ctx, execd, stream.commandID)
	}
	if exitCode == nil {
		fallback := 0
		if stream.sawError {
			fallback = 1
		}
		exitCode = &fallback
	}
	return &RemoteExecResult{
		Stdout:   stream.stdoutText(),
		Stderr:   stream.stderrText(),
		ExitCode: *exitCode,
		Duration: duration,
	}, nil
}

// fetchExitCode is the authoritative exit-code fallback. The command stream
// can end without one (the SDK's own accumulator defaults it to 0), so one
// status GET recovers the real code. The parent ctx may carry the finished
// exec's deadline, so the fetch runs detached from cancellation.
func (c *OpenSandboxRemoteClient) fetchExitCode(
	ctx context.Context,
	execd *opensandbox.ExecdClient,
	commandID string,
) *int {
	if strings.TrimSpace(commandID) == "" {
		return nil
	}
	statusCtx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx), c.httpTimeout)
	defer cancel()
	status, err := execd.GetCommandStatus(statusCtx, commandID)
	if err != nil || status == nil || status.ExitCode == nil {
		return nil
	}
	code := int(*status.ExitCode)
	return &code
}

// openSandboxCommandStream accumulates one execd command stream. The SDK
// ships an Execution accumulator, but it drops the wire's exit_code field
// (defaulting to 0 on success and guessing from the error text otherwise), so
// the adapter parses events itself.
type openSandboxCommandStream struct {
	commandID string
	stdout    []string
	stderr    []string
	sawError  bool
	errorText string
	exitCode  *int
}

// openSandboxStreamEvent mirrors the execd wire event: {"type": "...",
// "text": "...", "exit_code": n, "ename": "...", "evalue": "..."} as NDJSON
// or SSE data.
type openSandboxStreamEvent struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	ExitCode *int   `json:"exit_code"`
	EName    string `json:"ename"`
	EValue   string `json:"evalue"`
}

func (s *openSandboxCommandStream) handleEvent(event opensandbox.StreamEvent) error {
	data := strings.TrimSpace(event.Data)
	if data == "" {
		return nil
	}
	var parsed openSandboxStreamEvent
	if err := json.Unmarshal([]byte(data), &parsed); err != nil {
		// Non-JSON payloads are raw stdout (the SDK's own fallback).
		s.stdout = append(s.stdout, event.Data)
		return nil
	}
	eventType := parsed.Type
	if eventType == "" {
		eventType = strings.TrimSpace(event.Event)
	}
	switch eventType {
	case "init":
		s.commandID = parsed.Text
	case "stdout":
		s.stdout = append(s.stdout, parsed.Text)
	case "stderr":
		s.stderr = append(s.stderr, parsed.Text)
	case "error":
		s.sawError = true
		s.errorText = strings.TrimSpace(parsed.EName + ": " + parsed.EValue)
		// execd reports process failures as errno-style evalue ("1" for a
		// non-zero exit) — the SDK parses it the same way.
		if code, convErr := strconv.Atoi(strings.TrimSpace(parsed.EValue)); convErr == nil {
			s.exitCode = &code
		}
		if parsed.ExitCode != nil {
			s.exitCode = parsed.ExitCode
		}
	case "execution_complete":
		if parsed.ExitCode != nil {
			s.exitCode = parsed.ExitCode
		}
	case "ping":
		// keep-alive
	default:
		if parsed.Text != "" {
			s.stdout = append(s.stdout, parsed.Text)
		}
	}
	return nil
}

// stdoutText/stderrText join per-event messages the way the SDK's Text()
// does: execd emits one message per line without trailing newlines.
func (s *openSandboxCommandStream) stdoutText() string {
	return strings.Join(s.stdout, "\n")
}

func (s *openSandboxCommandStream) stderrText() string {
	if len(s.stderr) == 0 {
		return s.errorText
	}
	joined := strings.Join(s.stderr, "\n")
	if s.errorText != "" {
		return joined + "\n" + s.errorText
	}
	return joined
}

func (s *openSandboxCommandStream) recordedExitCode() *int { return s.exitCode }

// openSandboxExecUser maps the neutral exec user onto execd's numeric
// uid/gid. execd has no user database — the image contract (uid 1000 with a
// writable /workspace) is what makes "user" work — and any other name is
// left to the server's default rather than guessed at.
func openSandboxExecUser(user string) (uid, gid *int32) {
	switch strings.TrimSpace(user) {
	case DefaultSandboxExecUser:
		id := int32(1000)
		return &id, &id
	case "root":
		id := int32(0)
		return &id, &id
	default:
		return nil, nil
	}
}

// --- filesystem ---------------------------------------------------------------

func (c *OpenSandboxRemoteClient) WriteFile(
	ctx context.Context,
	handle RemoteSandboxHandle,
	path string,
	content []byte,
) error {
	execd, err := c.execdFor(ctx, "WriteFile", handle)
	if err != nil {
		return err
	}
	if strings.TrimSpace(path) == "" {
		return openSandboxInvalidRequest("WriteFile", "path is required", nil)
	}
	callCtx, cancel := c.callCtx(ctx)
	defer cancel()
	err = execd.UploadFile(callCtx, bytes.NewReader(content), opensandbox.UploadFileOptions{
		Metadata: opensandbox.FileMetadata{
			Path: path,
			// 0644 keeps the file readable by the uid-1000 session user even
			// though the upload itself authenticates as the server default.
			Mode: opensandbox.OctalMode(0o644),
		},
	})
	if err != nil {
		return normalizeOpenSandboxError("WriteFile", err)
	}
	return nil
}

func (c *OpenSandboxRemoteClient) ReadFile(
	ctx context.Context,
	handle RemoteSandboxHandle,
	path string,
) ([]byte, error) {
	execd, err := c.execdFor(ctx, "ReadFile", handle)
	if err != nil {
		return nil, err
	}
	callCtx, cancel := c.callCtx(ctx)
	defer cancel()
	reader, err := execd.DownloadFile(callCtx, path, "")
	if err != nil {
		return nil, normalizeOpenSandboxError("ReadFile", err)
	}
	defer reader.Close()
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, normalizeOpenSandboxError("ReadFile", err)
	}
	return content, nil
}

func (c *OpenSandboxRemoteClient) ListDir(
	ctx context.Context,
	handle RemoteSandboxHandle,
	path string,
) ([]RemoteDirEntry, error) {
	execd, err := c.execdFor(ctx, "ListDir", handle)
	if err != nil {
		return nil, err
	}
	if path == "" {
		path = "/"
	}
	callCtx, cancel := c.callCtx(ctx)
	defer cancel()
	entries, err := execd.ListDirectory(callCtx, path)
	if err != nil {
		return nil, normalizeOpenSandboxError("ListDir", err)
	}
	result := make([]RemoteDirEntry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, RemoteDirEntry{
			Name:    openSandboxBaseName(entry.Path),
			Path:    entry.Path,
			Type:    openSandboxEntryType(entry.Type),
			Size:    entry.Size,
			ModTime: entry.ModifiedAt,
		})
	}
	return result, nil
}

func (c *OpenSandboxRemoteClient) MakeDir(
	ctx context.Context,
	handle RemoteSandboxHandle,
	dir string,
) error {
	execd, err := c.execdFor(ctx, "MakeDir", handle)
	if err != nil {
		return err
	}
	if strings.TrimSpace(dir) == "" {
		return openSandboxInvalidRequest("MakeDir", "directory is required", nil)
	}
	callCtx, cancel := c.callCtx(ctx)
	defer cancel()
	// execd's create-directory is already mkdir -p; ignoreExistingDir only
	// guards against a server that reports EEXIST for an existing path.
	if err := execd.CreateDirectory(callCtx, dir, opensandbox.OctalMode(0o755)); err != nil {
		return ignoreExistingDir(normalizeOpenSandboxError("MakeDir", err))
	}
	return nil
}

// Remove dispatches by kind: execd's delete-files endpoint refuses
// directories and delete-directory is recursive, so one stat decides.
func (c *OpenSandboxRemoteClient) Remove(
	ctx context.Context,
	handle RemoteSandboxHandle,
	path string,
) error {
	execd, err := c.execdFor(ctx, "Remove", handle)
	if err != nil {
		return err
	}
	entry, err := c.statFile(ctx, execd, "Remove", path)
	if err != nil {
		return err
	}
	callCtx, cancel := c.callCtx(ctx)
	defer cancel()
	if openSandboxEntryType(entry.Type) == RemoteEntryDir {
		err = execd.DeleteDirectory(callCtx, entry.Path)
	} else {
		err = execd.DeleteFiles(callCtx, []string{entry.Path})
	}
	if err != nil {
		return normalizeOpenSandboxError("Remove", err)
	}
	return nil
}

func (c *OpenSandboxRemoteClient) Stat(
	ctx context.Context,
	handle RemoteSandboxHandle,
	path string,
) (*RemoteStatEntry, error) {
	execd, err := c.execdFor(ctx, "Stat", handle)
	if err != nil {
		return nil, err
	}
	entry, err := c.statFile(ctx, execd, "Stat", path)
	if err != nil {
		return nil, err
	}
	return &RemoteStatEntry{
		Path:    entry.Path,
		Type:    openSandboxEntryType(entry.Type),
		Size:    entry.Size,
		ModTime: entry.ModifiedAt,
	}, nil
}

// statFile wraps GetFileInfo, which returns a path-keyed map, and normalizes
// a missing path to RemoteErrorKindNotFound.
func (c *OpenSandboxRemoteClient) statFile(
	ctx context.Context,
	execd *opensandbox.ExecdClient,
	op, path string,
) (*opensandbox.FileInfo, error) {
	if strings.TrimSpace(path) == "" {
		return nil, openSandboxInvalidRequest(op, "path is required", nil)
	}
	callCtx, cancel := c.callCtx(ctx)
	defer cancel()
	files, err := execd.GetFileInfo(callCtx, path)
	if err != nil {
		if isRemoteStatus(err, http.StatusNotFound) {
			return nil, NewRemoteError(
				SandboxTypeOpenSandbox, op, RemoteErrorKindNotFound,
				"path not found", nil,
			)
		}
		return nil, normalizeOpenSandboxError(op, err)
	}
	if entry, ok := files[path]; ok {
		return &entry, nil
	}
	// Some servers key the map by their own normalized absolute path.
	if len(files) == 1 {
		for _, entry := range files {
			probe := entry
			return &probe, nil
		}
	}
	return nil, NewRemoteError(
		SandboxTypeOpenSandbox, op, RemoteErrorKindNotFound,
		"path not found", nil,
	)
}

func (c *OpenSandboxRemoteClient) execdFor(
	ctx context.Context,
	op string,
	handle RemoteSandboxHandle,
) (*opensandbox.ExecdClient, error) {
	openHandle, err := openSandboxHandle(op, handle)
	if err != nil {
		return nil, err
	}
	return c.resolveHandleExecd(ctx, openHandle)
}

// --- snapshots ----------------------------------------------------------------

// CreateSnapshot snapshots a running sandbox and waits until the snapshot is
// spawnable: a Creating snapshot cannot back CreateSandboxRequest.SnapshotID,
// and handing one back would make the skill-image chain fail at spawn time.
func (c *OpenSandboxRemoteClient) CreateSnapshot(
	ctx context.Context, sandboxID string, name string,
) (RemoteSnapshotRef, error) {
	sandboxID = strings.TrimSpace(sandboxID)
	if sandboxID == "" {
		return RemoteSnapshotRef{}, openSandboxInvalidRequest("CreateSnapshot", "sandbox ID is required", nil)
	}
	snapCtx, snapCancel := c.snapshotCtx(ctx)
	defer snapCancel()
	info, err := c.lifecycle.CreateSnapshot(snapCtx, sandboxID, opensandbox.CreateSnapshotRequest{
		Name: strings.TrimSpace(name),
	})
	if err != nil {
		return RemoteSnapshotRef{}, normalizeOpenSandboxError("CreateSnapshot", err)
	}
	if info == nil || strings.TrimSpace(info.ID) == "" {
		return RemoteSnapshotRef{}, openSandboxInvalidRequest(
			"CreateSnapshot", "provider returned an empty snapshot ID", nil)
	}
	info, err = c.waitSnapshotReady(snapCtx, info)
	if err != nil {
		return RemoteSnapshotRef{}, err
	}
	var names []string
	if trimmed := strings.TrimSpace(info.Name); trimmed != "" {
		names = []string{trimmed}
	}
	return RemoteSnapshotRef{ID: info.ID, Names: names}, nil
}

func (c *OpenSandboxRemoteClient) waitSnapshotReady(
	ctx context.Context, info *opensandbox.SnapshotInfo,
) (*opensandbox.SnapshotInfo, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("opensandbox waitSnapshotReady: %w", err)
		}
		switch strings.TrimSpace(string(info.Status.State)) {
		case string(opensandbox.SnapshotStateReady):
			return info, nil
		case string(opensandbox.SnapshotStateFailed):
			return nil, NewRemoteError(
				SandboxTypeOpenSandbox, "CreateSnapshot", RemoteErrorKindUnavailable,
				fmt.Sprintf("snapshot %s failed (%s)", info.ID,
					openSandboxStatusDetail(info.Status.Reason, info.Status.Message)),
				nil,
			)
		}
		callCtx, cancel := c.callCtx(ctx)
		latest, err := c.lifecycle.GetSnapshot(callCtx, info.ID)
		cancel()
		if err != nil {
			if isRemoteStatus(err, http.StatusNotFound) {
				return nil, NewRemoteError(
					SandboxTypeOpenSandbox, "CreateSnapshot", RemoteErrorKindNotFound,
					fmt.Sprintf("snapshot %s disappeared while creating", info.ID), nil,
				)
			}
			return nil, normalizeOpenSandboxError("CreateSnapshot", err)
		}
		if latest != nil {
			info = latest
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("opensandbox waitSnapshotReady: %w", ctx.Err())
		case <-time.After(openSandboxSnapshotPollInterval):
		}
	}
}

// DeleteSnapshot removes a snapshot. A missing snapshot is success: delete is
// idempotent by contract.
func (c *OpenSandboxRemoteClient) DeleteSnapshot(ctx context.Context, snapshotID string) error {
	snapshotID = strings.TrimSpace(snapshotID)
	if snapshotID == "" {
		return openSandboxInvalidRequest("DeleteSnapshot", "snapshot ID is required", nil)
	}
	callCtx, cancel := c.callCtx(ctx)
	defer cancel()
	if err := c.lifecycle.DeleteSnapshot(callCtx, snapshotID); err != nil {
		normalized := normalizeOpenSandboxError("DeleteSnapshot", err)
		if IsRemoteNotFound(normalized) {
			return nil
		}
		return normalized
	}
	return nil
}

func (c *OpenSandboxRemoteClient) ListSnapshots(
	ctx context.Context, sandboxID string,
) ([]RemoteSnapshotRef, error) {
	var out []RemoteSnapshotRef
	opts := opensandbox.ListSnapshotsOptions{
		SandboxID: strings.TrimSpace(sandboxID),
		Page:      1,
		PageSize:  openSandboxSnapshotListPageSize,
	}
	for pages := 0; ; pages++ {
		if pages >= openSandboxListPageCap {
			return nil, NewRemoteError(
				SandboxTypeOpenSandbox, "ListSnapshots", RemoteErrorKindInvalidRequest,
				"provider pagination did not terminate", nil,
			)
		}
		callCtx, cancel := c.callCtx(ctx)
		resp, err := c.lifecycle.ListSnapshots(callCtx, opts)
		cancel()
		if err != nil {
			return nil, normalizeOpenSandboxError("ListSnapshots", err)
		}
		if resp == nil {
			return out, nil
		}
		for _, item := range resp.Items {
			if strings.TrimSpace(item.ID) == "" {
				continue
			}
			var names []string
			if trimmed := strings.TrimSpace(item.Name); trimmed != "" {
				names = []string{trimmed}
			}
			out = append(out, RemoteSnapshotRef{ID: item.ID, Names: names})
		}
		if !resp.Pagination.HasNextPage {
			return out, nil
		}
		opts.Page++
	}
}

// --- template catalog ---------------------------------------------------------

// ListTemplates reports the configured image (or snapshot ID) as the
// catalog's single entry, mirroring the Docker backend's minimal catalog:
// OpenSandbox has no server-side template object to administer — any image
// URI is spawnable as-is, and skill snapshots are referenced directly by ID
// rather than through the catalog.
func (c *OpenSandboxRemoteClient) ListTemplates(context.Context) ([]RemoteTemplate, error) {
	return []RemoteTemplate{c.catalogEntry()}, nil
}

// EnsureStandardTemplate returns the configured entry. There is nothing to
// build: existence is validated at first spawn, and a bad image surfaces
// there as a visible create failure.
func (c *OpenSandboxRemoteClient) EnsureStandardTemplate(context.Context) (*RemoteTemplate, error) {
	entry := c.catalogEntry()
	return &entry, nil
}

// ReplaceStandardTemplate is the same single entry: applying "the current
// spec" means whatever image the config now names.
func (c *OpenSandboxRemoteClient) ReplaceStandardTemplate(context.Context) (*RemoteTemplate, error) {
	entry := c.catalogEntry()
	return &entry, nil
}

// DeleteSupersededStandardTemplates is a no-op: OpenSandbox keeps no
// cluster-side template objects, and snapshots belong to the skill-image
// chain rather than the standard catalog.
func (c *OpenSandboxRemoteClient) DeleteSupersededStandardTemplates(context.Context, string) error {
	return nil
}

func (c *OpenSandboxRemoteClient) catalogEntry() RemoteTemplate {
	template := strings.TrimSpace(c.config.OpenSandboxTemplate)
	if template == "" {
		template = DefaultOpenSandboxTemplateImage
	}
	return RemoteTemplate{
		ID:       template,
		Name:     template,
		Status:   "ready",
		Image:    template,
		Standard: isStandardTemplateImage(template),
	}
}

// --- helpers ------------------------------------------------------------------

func openSandboxInvalidRequest(op, message string, cause error) error {
	return NewRemoteError(
		SandboxTypeOpenSandbox, op, RemoteErrorKindInvalidRequest, message, cause,
	)
}

// openSandboxHandle extracts the concrete handle from an opaque
// RemoteSandboxHandle. Returns an error when the handle is nil, not
// OpenSandbox's, or has an empty sandbox ID.
func openSandboxHandle(op string, handle RemoteSandboxHandle) (*openSandboxRemoteHandle, error) {
	openHandle, ok := handle.(*openSandboxRemoteHandle)
	if !ok || openHandle == nil || strings.TrimSpace(openHandle.sandboxID) == "" {
		return nil, openSandboxInvalidRequest(op, "handle was not issued by OpenSandbox", nil)
	}
	return openHandle, nil
}

// openSandboxSummary converts the SDK's SandboxInfo into the neutral DTO.
// TemplateID prefers the image URI and falls back to the snapshot ID so a
// rebound sandbox still names a spawnable template.
func openSandboxSummary(info opensandbox.SandboxInfo) *RemoteSandboxSummary {
	template := ""
	if info.Image != nil {
		template = info.Image.URI
	}
	if template == "" {
		template = info.SnapshotID
	}
	return &RemoteSandboxSummary{
		ID:         info.ID,
		TemplateID: template,
		State:      normalizeOpenSandboxState(string(info.Status.State)),
		RawState:   string(info.Status.State),
		Metadata:   cloneMetadata(info.Metadata),
		StartedAt:  info.CreatedAt,
	}
}

func normalizeOpenSandboxState(state string) RemoteSandboxState {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "running":
		return RemoteStateRunning
	case "paused":
		return RemoteStatePaused
	case "pending", "creating", "provisioning", "starting", "pausing", "resuming", "stopping":
		return RemoteStateTransitioning
	case "terminated", "stopped", "deleted", "failed", "error":
		return RemoteStateTerminal
	default:
		return RemoteStateUnknown
	}
}

func openSandboxEntryType(entryType string) RemoteDirEntryType {
	switch strings.ToLower(strings.TrimSpace(entryType)) {
	case "file":
		return RemoteEntryFile
	case "directory", "dir":
		return RemoteEntryDir
	default:
		return RemoteEntryOther
	}
}

func openSandboxBaseName(path string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(path), "/")
	if trimmed == "" {
		return "/"
	}
	if idx := strings.LastIndex(trimmed, "/"); idx >= 0 {
		return trimmed[idx+1:]
	}
	return trimmed
}

// openSandboxStatusDetail renders the reason/message pair both the sandbox and
// snapshot status structs carry, for create/wait failure messages.
func openSandboxStatusDetail(reason, message string) string {
	reason = strings.TrimSpace(reason)
	message = strings.TrimSpace(message)
	switch {
	case reason != "" && message != "":
		return reason + ": " + message
	case reason != "":
		return reason
	case message != "":
		return message
	default:
		return "no reason reported"
	}
}

// remoteErrorOf returns *RemoteError when err is one.
func remoteErrorOf(err error) *RemoteError {
	var remote *RemoteError
	if errors.As(err, &remote) {
		return remote
	}
	return nil
}

// isRemoteStatus reports whether err is an SDK APIError with the given HTTP
// status, without importing the wire types at call sites.
func isRemoteStatus(err error, status int) bool {
	var apiErr *opensandbox.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == status
}

// normalizeOpenSandboxError projects an OpenSandbox-native error (SDK APIError,
// net.Error, context cancellation) onto a RemoteError with a stable Kind. The
// original error is preserved via errors.Unwrap for diagnostics.
func normalizeOpenSandboxError(op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("opensandbox %s: %w", op, err)
	}

	kind := RemoteErrorKindInternal
	status := 0
	var apiErr *opensandbox.APIError
	var netErr net.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		kind = RemoteErrorKindTimeout
	case errors.As(err, &apiErr):
		kind = httpErrorKind(op, apiErr.StatusCode)
		status = apiErr.StatusCode
	case errors.As(err, &netErr) && netErr.Timeout():
		kind = RemoteErrorKindTimeout
	case errors.As(err, &netErr):
		kind = RemoteErrorKindUnavailable
	}
	kind = snapshotDeleteKind(op, kind, err.Error())
	remoteErr := NewRemoteError(SandboxTypeOpenSandbox, op, kind, err.Error(), err)
	remoteErr.StatusCode = status
	return remoteErr
}

// logOpenSandboxSandboxCreated records create-time fields operators need when
// exec fails with a generic auth error. Credential values are never logged.
func logOpenSandboxSandboxCreated(
	ctx context.Context,
	client *OpenSandboxRemoteClient,
	sandboxID, template string,
) {
	if client == nil {
		return
	}
	logger.Infof(ctx,
		"[OpenSandboxRemote] sandbox created id=%s template=%s api_url=%s api_key=%s",
		sandboxID,
		template,
		client.config.OpenSandboxAPIURL,
		cubeCredentialPresence(client.config.OpenSandboxAPIKey),
	)
}

var (
	_ RemoteSandboxClient   = (*OpenSandboxRemoteClient)(nil)
	_ RemoteSnapshotManager = (*OpenSandboxRemoteClient)(nil)
	_ RemoteTemplateCatalog = (*OpenSandboxRemoteClient)(nil)
	_ RemoteSandboxHandle   = (*openSandboxRemoteHandle)(nil)
)
