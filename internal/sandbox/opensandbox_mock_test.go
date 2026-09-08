package sandbox

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// openSandboxMockServer emulates both OpenSandbox planes on one httptest
// server, mirroring the single-origin deployment the adapter targets: the
// lifecycle REST API under /v1/... and the execd data plane at a per-sandbox
// endpoint path (/execd/{sandboxID}/...) that stands in for the server's
// endpoint proxy. OpenSandboxRemoteClient tests exercise the adapter through
// its public RemoteSandboxClient surface without requiring a real cluster.
//
// Wire fidelity notes (verified against the official Go SDK v1.0.5):
//   - states are CAPITALIZED strings ("Pending"/"Running"/"Paused"/
//     "Terminated"/"Failed"; snapshots "Creating"/"Ready"/"Failed");
//   - the command stream is NDJSON: one JSON blob per line, events separated
//     by blank lines (the SDK's SSE parser dispatches on the blank line and
//     errors on a stream with zero events);
//   - error bodies are {"code": ..., "message": ...}.
type openSandboxMockServer struct {
	server *httptest.Server
	mu     sync.Mutex

	// apiKey, when non-empty, both planes require (OPEN-SANDBOX-API-KEY on
	// lifecycle, X-EXECD-ACCESS-TOKEN on execd) and mismatches answer 401.
	apiKey string

	// failLifecycleStatus, when non-zero, makes every /v1 route fail with
	// the status (after auth) — for error-normalization tests.
	failLifecycleStatus int
	// failExecdStatus, when non-zero, makes every execd route fail likewise.
	failExecdStatus int

	createBody   map[string]any
	createCount  atomic.Int32
	deleteCount  atomic.Int32
	resumeCount  atomic.Int32
	pauseCount   atomic.Int32
	endpointHits atomic.Int32
	renewCount   atomic.Int32
	nextID       atomic.Int64

	sandboxes   map[string]*osbMockSandbox
	snapshots   map[string]*osbMockSnapshot
	snapshotSeq atomic.Int64

	// renewExpiries records the parsed expiresAt of every renew-expiration.
	renewExpiries []time.Time

	// files is the per-sandbox filesystem tree (files + directories).
	files map[string]map[string]*osbMockNode

	// commands is commandID → record; statusFallback holds the exit code the
	// /command/status endpoint reports when the stream omitted one.
	commands   map[string]*osbMockCommand
	commandSeq atomic.Int64
	cmdHistory []osbMockCommandRequest

	// executor drives POST /command. nil → immediate exit 0 with no output.
	executor func(sandboxID string, req osbMockCommandRequest) osbMockExecOutcome

	// pendingCreateStates is served by the sandbox created next before it
	// settles on Running — exercising Create's waitReady poll. The final
	// element may be a terminal state ("Failed") to exercise the reap path.
	pendingCreateStates []string

	// stuckPagination makes sandbox/snapshot lists always claim a next page.
	stuckPagination bool

	// directoryExistsStatus, when non-zero, makes POST /directories on an
	// existing directory fail with this status and an "already exists" body
	// (execd servers that reject mkdir of an existing path).
	directoryExistsStatus int

	lifecycleAuth []string // recorded OPEN-SANDBOX-API-KEY values
	execdAuth     []string // recorded X-EXECD-ACCESS-TOKEN values
}

type osbMockSandbox struct {
	id         string
	image      string
	snapshotID string
	reason     string
	message    string
	metadata   map[string]string
	env        map[string]string
	timeout    int
	createdAt  time.Time
	expiresAt  time.Time

	// states is the queue of states successive GETs serve; the last element
	// is the steady state.
	states    []string
	stateIdx  int
	pausedSet bool
}

type osbMockSnapshot struct {
	id        string
	sandboxID string
	name      string
	// states mirrors the sandbox state queue for snapshot readiness polls.
	states    []string
	stateIdx  int
	createdAt time.Time
}

type osbMockNode struct {
	isDir   bool
	content []byte
	mod     time.Time
}

type osbMockCommandRequest struct {
	command string
	cwd     string
	timeout int64
	uid     *int32
	gid     *int32
	envs    map[string]string
}

// osbMockExecOutcome models a command result and how its exit code reaches
// the adapter, mirroring the wire shapes execd actually produces.
type osbMockExecOutcome struct {
	stdout []string
	stderr []string
	exit   int32
	// delivery selects the exit-code channel:
	//   "stream" — execution_complete carries exit_code (fast path);
	//   "status" — the stream omits it; GET /command/{id}/status reports it;
	//   "error"  — an error event whose evalue is the numeric exit code;
	//   "error_field" — an error event with evalue "1" plus a defensive
	//                   in-stream exit_code that must win;
	//   "none"   — neither channel carries one (adapter falls back to 0/1).
	delivery string
	// delay holds the stream open before completion (timeout tests).
	delay time.Duration
}

type osbMockCommand struct {
	id      string
	sandbox string
	req     osbMockCommandRequest
	outcome osbMockExecOutcome
}

func newOpenSandboxMockServer(t *testing.T) *openSandboxMockServer {
	t.Helper()
	m := &openSandboxMockServer{
		apiKey:    "test-key",
		sandboxes: map[string]*osbMockSandbox{},
		snapshots: map[string]*osbMockSnapshot{},
		files:     map[string]map[string]*osbMockNode{},
		commands:  map[string]*osbMockCommand{},
	}
	m.server = httptest.NewServer(http.HandlerFunc(m.handle))
	t.Cleanup(m.server.Close)
	return m
}

// APIURL returns the lifecycle base URL, /v1 suffix included as the adapter
// contract requires.
func (m *openSandboxMockServer) APIURL() string { return m.server.URL + "/v1" }

// SetExecutor installs the callback invoked for every POST /command.
func (m *openSandboxMockServer) SetExecutor(
	f func(sandboxID string, req osbMockCommandRequest) osbMockExecOutcome,
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executor = f
}

// SandboxCount snapshots the live sandbox count.
func (m *openSandboxMockServer) SandboxCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sandboxes)
}

// LastCommand returns the most recent recorded command request.
func (m *openSandboxMockServer) LastCommand() osbMockCommandRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.cmdHistory) == 0 {
		return osbMockCommandRequest{}
	}
	return m.cmdHistory[len(m.cmdHistory)-1]
}

// RenewExpiries copies the recorded renew-expiration timestamps.
func (m *openSandboxMockServer) RenewExpiries() []time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]time.Time(nil), m.renewExpiries...)
}

func (m *openSandboxMockServer) handle(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/v1/") || r.URL.Path == "/v1" {
		m.handleLifecycle(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/execd/") {
		m.handleExecd(w, r)
		return
	}
	http.NotFound(w, r)
}

// --- lifecycle plane ----------------------------------------------------------

func (m *openSandboxMockServer) handleLifecycle(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	m.lifecycleAuth = append(m.lifecycleAuth, r.Header.Get("OPEN-SANDBOX-API-KEY"))
	fail := m.failLifecycleStatus
	apiKey := m.apiKey
	m.mu.Unlock()
	if apiKey != "" && r.Header.Get("OPEN-SANDBOX-API-KEY") != apiKey {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"code": "unauthorized", "message": "invalid api key",
		})
		return
	}
	if fail != 0 {
		writeJSON(w, fail, map[string]string{
			"code": "server_error", "message": "injected failure",
		})
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/v1")
	switch {
	case path == "/sandboxes" && r.Method == http.MethodPost:
		m.handleCreateSandbox(w, r)
	case path == "/sandboxes" && r.Method == http.MethodGet:
		m.handleListSandboxes(w, r)
	case sandboxAction(path, "/pause", http.MethodPost, r):
		m.handlePauseResume(w, r, "Paused")
	case sandboxAction(path, "/resume", http.MethodPost, r):
		m.handlePauseResume(w, r, "Running")
	case sandboxAction(path, "/renew-expiration", http.MethodPost, r):
		m.handleRenewExpiration(w, r)
	case sandboxAction(path, "/snapshots", http.MethodPost, r):
		m.handleCreateSnapshot(w, r)
	case strings.Contains(path, "/endpoints/") && r.Method == http.MethodGet:
		m.handleGetEndpoint(w, r)
	case strings.HasPrefix(path, "/sandboxes/") && r.Method == http.MethodGet:
		m.handleGetSandbox(w, r)
	case strings.HasPrefix(path, "/sandboxes/") && r.Method == http.MethodDelete:
		m.handleDeleteSandbox(w, r)
	case path == "/snapshots" && r.Method == http.MethodGet:
		m.handleListSnapshots(w, r)
	case strings.HasPrefix(path, "/snapshots/") && r.Method == http.MethodGet:
		m.handleGetSnapshot(w, r)
	case strings.HasPrefix(path, "/snapshots/") && r.Method == http.MethodDelete:
		m.handleDeleteSnapshot(w, r)
	default:
		http.NotFound(w, r)
	}
}

func sandboxAction(path, action string, method string, r *http.Request) bool {
	return r.Method == method && strings.HasSuffix(path, action)
}

func (m *openSandboxMockServer) handleCreateSandbox(w http.ResponseWriter, r *http.Request) {
	m.createCount.Add(1)
	body, _ := io.ReadAll(r.Body)
	var raw map[string]any
	_ = json.Unmarshal(body, &raw)

	m.mu.Lock()
	m.createBody = raw
	id := "osb-" + strconv.FormatInt(m.nextID.Add(1), 10)
	metadata := map[string]string{}
	if rawMeta, ok := raw["metadata"].(map[string]any); ok {
		for k, v := range rawMeta {
			metadata[k] = fmt.Sprint(v)
		}
	}
	env := map[string]string{}
	if rawEnv, ok := raw["env"].(map[string]any); ok {
		for k, v := range rawEnv {
			env[k] = fmt.Sprint(v)
		}
	}
	timeout := 0
	if rawTimeout, ok := raw["timeout"].(float64); ok {
		timeout = int(rawTimeout)
	}
	now := time.Now().UTC()
	states := append([]string(nil), m.pendingCreateStates...)
	m.pendingCreateStates = nil
	if len(states) == 0 {
		states = []string{"Running"}
	}
	sb := &osbMockSandbox{
		id:        id,
		metadata:  metadata,
		env:       env,
		timeout:   timeout,
		createdAt: now,
		expiresAt: now.Add(time.Duration(timeout) * time.Second),
		states:    states,
	}
	if image, ok := raw["image"].(map[string]any); ok {
		sb.image, _ = image["uri"].(string)
	}
	if snapshotID, ok := raw["snapshotId"].(string); ok {
		sb.snapshotID = snapshotID
	}
	m.sandboxes[id] = sb
	m.files[id] = map[string]*osbMockNode{"/": {isDir: true, mod: now}}
	m.mu.Unlock()

	writeJSON(w, http.StatusCreated, sb.wire())
}

func (m *openSandboxMockServer) handleGetSandbox(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/v1"), "/sandboxes/")
	m.mu.Lock()
	sb, ok := m.sandboxes[id]
	var body map[string]any
	if ok {
		body = sb.wire() // render under the lock: wire() advances the state queue
	}
	m.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"code": "not_found", "message": "sandbox not found",
		})
		return
	}
	writeJSON(w, http.StatusOK, body)
}

func (m *openSandboxMockServer) handleListSandboxes(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	filter := map[string]string{}
	if raw := query.Get("metadata"); raw != "" {
		if parsed, err := url.ParseQuery(raw); err == nil {
			for k, vals := range parsed {
				if len(vals) > 0 {
					filter[k] = vals[0]
				}
			}
		}
	}
	page := 1
	if v, err := strconv.Atoi(query.Get("page")); err == nil && v > 0 {
		page = v
	}
	pageSize := 100
	if v, err := strconv.Atoi(query.Get("pageSize")); err == nil && v > 0 {
		pageSize = v
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	filtered := make([]*osbMockSandbox, 0, len(m.sandboxes))
	for _, sb := range m.sandboxes {
		match := true
		for k, v := range filter {
			if sb.metadata[k] != v {
				match = false
				break
			}
		}
		if match {
			filtered = append(filtered, sb)
		}
	}
	total := len(filtered)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	items := make([]map[string]any, 0, end-start)
	for _, sb := range filtered[start:end] {
		items = append(items, sb.wire())
	}
	hasNext := end < total
	if m.stuckPagination {
		hasNext = true
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"pagination": map[string]any{
			"page": page, "pageSize": pageSize, "totalItems": total,
			"totalPages": (total + pageSize - 1) / pageSize, "hasNextPage": hasNext,
		},
	})
}

func (m *openSandboxMockServer) handleDeleteSandbox(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/v1"), "/sandboxes/")
	m.mu.Lock()
	_, ok := m.sandboxes[id]
	delete(m.sandboxes, id)
	delete(m.files, id)
	m.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"code": "not_found", "message": "sandbox not found",
		})
		return
	}
	m.deleteCount.Add(1)
	w.WriteHeader(http.StatusNoContent)
}

func (m *openSandboxMockServer) handlePauseResume(w http.ResponseWriter, r *http.Request, state string) {
	id := sandboxIDFromAction(r.URL.Path)
	m.mu.Lock()
	sb, ok := m.sandboxes[id]
	m.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"code": "not_found", "message": "sandbox not found",
		})
		return
	}
	if state == "Paused" {
		m.pauseCount.Add(1)
	} else {
		m.resumeCount.Add(1)
	}
	m.mu.Lock()
	sb.states = []string{state}
	sb.stateIdx = 0
	sb.pausedSet = state == "Paused"
	m.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (m *openSandboxMockServer) handleRenewExpiration(w http.ResponseWriter, r *http.Request) {
	id := sandboxIDFromAction(r.URL.Path)
	var body struct {
		ExpiresAt time.Time `json:"expiresAt"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	m.mu.Lock()
	sb, ok := m.sandboxes[id]
	if ok && !body.ExpiresAt.IsZero() {
		sb.expiresAt = body.ExpiresAt
	}
	m.renewExpiries = append(m.renewExpiries, body.ExpiresAt)
	m.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"code": "not_found", "message": "sandbox not found",
		})
		return
	}
	m.renewCount.Add(1)
	writeJSON(w, http.StatusOK, map[string]any{"expiresAt": body.ExpiresAt})
}

func (m *openSandboxMockServer) handleGetEndpoint(w http.ResponseWriter, r *http.Request) {
	m.endpointHits.Add(1)
	// /v1/sandboxes/{id}/endpoints/{port} — the ID precedes "/endpoints/".
	rest := strings.TrimPrefix(r.URL.Path, "/v1/sandboxes/")
	id := rest[:strings.Index(rest, "/endpoints/")]
	m.mu.Lock()
	_, ok := m.sandboxes[id]
	m.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"code": "not_found", "message": "sandbox not found",
		})
		return
	}
	// The real server proxies execd traffic itself; the mock stands in with
	// a per-sandbox path on the same origin.
	writeJSON(w, http.StatusOK, map[string]any{
		"endpoint": m.server.URL + "/execd/" + id,
		"headers":  map[string]string{},
	})
}

func (m *openSandboxMockServer) handleCreateSnapshot(w http.ResponseWriter, r *http.Request) {
	id := sandboxIDFromAction(r.URL.Path)
	var body struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sandboxes[id]; !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"code": "not_found", "message": "sandbox not found",
		})
		return
	}
	m.snapshotSeq.Add(1)
	snapshotID := "snap-" + strconv.FormatInt(m.snapshotSeq.Load(), 10)
	m.snapshots[snapshotID] = &osbMockSnapshot{
		id:        snapshotID,
		sandboxID: id,
		name:      strings.TrimSpace(body.Name),
		states:    []string{"Ready"},
		createdAt: time.Now().UTC(),
	}
	writeJSON(w, http.StatusCreated, snapshotWire(m.snapshots[snapshotID]))
}

func (m *openSandboxMockServer) handleGetSnapshot(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/v1"), "/snapshots/")
	m.mu.Lock()
	snap, ok := m.snapshots[id]
	var body map[string]any
	if ok {
		body = snapshotWire(snap) // render under the lock: it advances the state queue
	}
	m.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"code": "not_found", "message": "snapshot not found",
		})
		return
	}
	writeJSON(w, http.StatusOK, body)
}

func (m *openSandboxMockServer) handleDeleteSnapshot(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/v1"), "/snapshots/")
	m.mu.Lock()
	_, ok := m.snapshots[id]
	delete(m.snapshots, id)
	m.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"code": "not_found", "message": "snapshot not found",
		})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (m *openSandboxMockServer) handleListSnapshots(w http.ResponseWriter, r *http.Request) {
	sandboxID := r.URL.Query().Get("sandboxId")
	page := 1
	if v, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && v > 0 {
		page = v
	}
	pageSize := 100
	if v, err := strconv.Atoi(r.URL.Query().Get("pageSize")); err == nil && v > 0 {
		pageSize = v
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	filtered := make([]*osbMockSnapshot, 0, len(m.snapshots))
	for _, snap := range m.snapshots {
		if sandboxID != "" && snap.sandboxID != sandboxID {
			continue
		}
		filtered = append(filtered, snap)
	}
	total := len(filtered)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	items := make([]map[string]any, 0, end-start)
	for _, snap := range filtered[start:end] {
		items = append(items, snapshotWire(snap))
	}
	hasNext := end < total
	if m.stuckPagination {
		hasNext = true
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"pagination": map[string]any{
			"page": page, "pageSize": pageSize, "totalItems": total,
			"totalPages": (total + pageSize - 1) / pageSize, "hasNextPage": hasNext,
		},
	})
}

// --- execd plane --------------------------------------------------------------

// handleExecd routes /execd/{sandboxID}/... requests. The per-sandbox prefix
// stands in for the lifecycle server's endpoint proxy and gives every sandbox
// an isolated filesystem/command space.
func (m *openSandboxMockServer) handleExecd(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/execd/")
	slash := strings.Index(rest, "/")
	if slash <= 0 {
		http.NotFound(w, r)
		return
	}
	sandboxID, sub := rest[:slash], rest[slash:]

	m.mu.Lock()
	m.execdAuth = append(m.execdAuth, r.Header.Get("X-EXECD-ACCESS-TOKEN"))
	_, known := m.sandboxes[sandboxID]
	fail := m.failExecdStatus
	apiKey := m.apiKey
	m.mu.Unlock()
	if apiKey != "" && r.Header.Get("X-EXECD-ACCESS-TOKEN") != apiKey {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"code": "unauthorized", "message": "invalid access token",
		})
		return
	}
	if fail != 0 {
		writeJSON(w, fail, map[string]string{
			"code": "server_error", "message": "injected failure",
		})
		return
	}
	if !known {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"code": "not_found", "message": "sandbox not found",
		})
		return
	}

	switch {
	case sub == "/ping" && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	case sub == "/command" && r.Method == http.MethodPost:
		m.handleRunCommand(w, r, sandboxID)
	case strings.HasPrefix(sub, "/command/status/") && r.Method == http.MethodGet:
		m.handleCommandStatus(w, r, sandboxID)
	case sub == "/files/info" && r.Method == http.MethodGet:
		m.handleFileInfo(w, r, sandboxID)
	case sub == "/files/download" && r.Method == http.MethodGet:
		m.handleFileDownload(w, r, sandboxID)
	case sub == "/files/upload" && r.Method == http.MethodPost:
		m.handleFileUpload(w, r, sandboxID)
	case sub == "/files" && r.Method == http.MethodDelete:
		m.handleFileDelete(w, r, sandboxID)
	case sub == "/directories/list" && r.Method == http.MethodGet:
		m.handleDirectoryList(w, r, sandboxID)
	case sub == "/directories" && r.Method == http.MethodPost:
		m.handleDirectoryCreate(w, r, sandboxID)
	case sub == "/directories" && r.Method == http.MethodDelete:
		m.handleDirectoryDelete(w, r, sandboxID)
	default:
		http.NotFound(w, r)
	}
}

func (m *openSandboxMockServer) handleRunCommand(w http.ResponseWriter, r *http.Request, sandboxID string) {
	var raw map[string]any
	_ = json.NewDecoder(r.Body).Decode(&raw)
	req := osbMockCommandRequest{
		command: asString(raw["command"]),
		cwd:     asString(raw["cwd"]),
		timeout: asInt64(raw["timeout"]),
		envs:    asStringMap(raw["envs"]),
	}
	if v, ok := raw["uid"].(float64); ok {
		id := int32(v)
		req.uid = &id
	}
	if v, ok := raw["gid"].(float64); ok {
		id := int32(v)
		req.gid = &id
	}

	m.mu.Lock()
	m.cmdHistory = append(m.cmdHistory, req)
	executor := m.executor
	m.mu.Unlock()

	outcome := osbMockExecOutcome{exit: 0, delivery: "stream"}
	if executor != nil {
		outcome = executor(sandboxID, req)
	}

	m.mu.Lock()
	m.commandSeq.Add(1)
	commandID := "cmd-" + strconv.FormatInt(m.commandSeq.Load(), 10)
	m.commands[commandID] = &osbMockCommand{
		id: commandID, sandbox: sandboxID, req: req, outcome: outcome,
	}
	m.mu.Unlock()

	w.Header().Set("Content-Type", "text/event-stream")
	// NDJSON: one JSON blob per line, blank line between events. The SDK's
	// parser dispatches an event on the blank line and rejects a stream with
	// zero events, so the init event is always written. Output precedes the
	// delay so a client killed mid-delay still sees the partial stdout.
	writeOSBEvent(w, map[string]any{"type": "init", "text": commandID})
	for _, line := range outcome.stdout {
		writeOSBEvent(w, map[string]any{"type": "stdout", "text": line})
	}
	for _, line := range outcome.stderr {
		writeOSBEvent(w, map[string]any{"type": "stderr", "text": line})
	}
	if outcome.delay > 0 {
		time.Sleep(outcome.delay)
	}
	switch outcome.delivery {
	case "stream":
		writeOSBEvent(w, map[string]any{
			"type": "execution_complete", "exit_code": outcome.exit,
		})
	case "status":
		// exit code deliberately omitted from the stream; the status
		// endpoint below reports it.
		writeOSBEvent(w, map[string]any{"type": "execution_complete"})
	case "error":
		writeOSBEvent(w, map[string]any{
			"type": "error", "ename": "exit",
			"evalue": strconv.FormatInt(int64(outcome.exit), 10),
		})
	case "error_field":
		writeOSBEvent(w, map[string]any{
			"type": "error", "ename": "exit", "evalue": "1",
			"exit_code": outcome.exit,
		})
	default:
		writeOSBEvent(w, map[string]any{"type": "execution_complete"})
	}
}

// writeOSBEvent writes one NDJSON event followed by the blank separator line.
func writeOSBEvent(w http.ResponseWriter, payload map[string]any) {
	data, _ := json.Marshal(payload)
	_, _ = w.Write(append(data, '\n', '\n'))
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (m *openSandboxMockServer) handleCommandStatus(w http.ResponseWriter, r *http.Request, sandboxID string) {
	commandID := strings.TrimPrefix(r.URL.Path, "/execd/"+sandboxID+"/command/status/")
	m.mu.Lock()
	cmd, ok := m.commands[commandID]
	m.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"code": "not_found", "message": "command not found",
		})
		return
	}
	body := map[string]any{
		"id": commandID, "content": "", "running": false,
		"started_at":  time.Now().Add(-time.Second).UTC().Format(time.RFC3339Nano),
		"finished_at": time.Now().UTC().Format(time.RFC3339Nano),
	}
	// Only the "status" delivery channel wants a code here; "none" must
	// leave the adapter without any authoritative exit code.
	if cmd.outcome.delivery == "status" {
		body["exit_code"] = cmd.outcome.exit
	}
	writeJSON(w, http.StatusOK, body)
}

func (m *openSandboxMockServer) handleFileInfo(w http.ResponseWriter, r *http.Request, sandboxID string) {
	path := r.URL.Query().Get("path")
	m.mu.Lock()
	node := m.files[sandboxID][path]
	m.mu.Unlock()
	if node == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"code": "not_found", "message": "path not found",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{path: osbFileInfoWire(path, node)})
}

func (m *openSandboxMockServer) handleFileDownload(w http.ResponseWriter, r *http.Request, sandboxID string) {
	path := r.URL.Query().Get("path")
	m.mu.Lock()
	node := m.files[sandboxID][path]
	m.mu.Unlock()
	if node == nil || node.isDir {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"code": "not_found", "message": "path not found",
		})
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write(node.content)
}

func (m *openSandboxMockServer) handleFileUpload(w http.ResponseWriter, r *http.Request, sandboxID string) {
	reader, err := r.MultipartReader()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"code": "bad_request", "message": "expected multipart body",
		})
		return
	}
	var meta struct {
		Path string `json:"path"`
		Mode int    `json:"mode"`
	}
	var content []byte
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"code": "bad_request", "message": "malformed multipart body",
			})
			return
		}
		switch part.FormName() {
		case "metadata":
			_ = json.NewDecoder(part).Decode(&meta)
		case "file":
			content, _ = io.ReadAll(part)
		}
		part.Close()
	}
	if strings.TrimSpace(meta.Path) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"code": "bad_request", "message": "metadata path is required",
		})
		return
	}

	m.mu.Lock()
	tree := m.files[sandboxID]
	if tree == nil {
		tree = map[string]*osbMockNode{}
		m.files[sandboxID] = tree
	}
	now := time.Now().UTC()
	m.ensureDirsLocked(tree, parentDir(meta.Path), now)
	tree[meta.Path] = &osbMockNode{content: content, mod: now}
	m.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{})
}

func (m *openSandboxMockServer) handleFileDelete(w http.ResponseWriter, r *http.Request, sandboxID string) {
	m.mu.Lock()
	tree := m.files[sandboxID]
	for _, path := range r.URL.Query()["path"] {
		if node := tree[path]; node != nil && !node.isDir {
			delete(tree, path)
		}
	}
	m.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (m *openSandboxMockServer) handleDirectoryList(w http.ResponseWriter, r *http.Request, sandboxID string) {
	dir := r.URL.Query().Get("path")
	m.mu.Lock()
	tree := m.files[sandboxID]
	entries := make([]map[string]any, 0)
	for path, node := range tree {
		if parentDir(path) == dir && path != dir {
			entries = append(entries, osbFileInfoWire(path, node))
		}
	}
	m.mu.Unlock()
	writeJSON(w, http.StatusOK, entries)
}

func (m *openSandboxMockServer) handleDirectoryCreate(w http.ResponseWriter, r *http.Request, sandboxID string) {
	var body map[string]map[string]int
	_ = json.NewDecoder(r.Body).Decode(&body)

	m.mu.Lock()
	tree := m.files[sandboxID]
	if tree == nil {
		tree = map[string]*osbMockNode{}
		m.files[sandboxID] = tree
	}
	existsStatus := m.directoryExistsStatus
	now := time.Now().UTC()
	for path := range body {
		if node := tree[path]; node != nil && node.isDir && existsStatus != 0 {
			m.mu.Unlock()
			writeJSON(w, existsStatus, map[string]string{
				"code": "already_exists", "message": "directory already exists",
			})
			return
		}
		m.ensureDirsLocked(tree, path, now)
	}
	m.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{})
}

func (m *openSandboxMockServer) handleDirectoryDelete(w http.ResponseWriter, r *http.Request, sandboxID string) {
	dir := r.URL.Query().Get("path")
	m.mu.Lock()
	tree := m.files[sandboxID]
	delete(tree, dir)
	for path := range tree {
		if strings.HasPrefix(path, dir+"/") {
			delete(tree, path)
		}
	}
	m.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// ensureDirsLocked creates path and every missing parent as directories.
// Callers must hold m.mu.
func (m *openSandboxMockServer) ensureDirsLocked(
	tree map[string]*osbMockNode, path string, now time.Time,
) {
	if path == "" || path == "/" {
		return
	}
	if node := tree[path]; node != nil {
		return
	}
	m.ensureDirsLocked(tree, parentDir(path), now)
	tree[path] = &osbMockNode{isDir: true, mod: now}
}

// --- wire shapes ---------------------------------------------------------------

// wire renders the sandbox in SandboxInfo wire form; each render serves the
// current queue element and advances one step (bounded at the steady state),
// so a create response of "Pending" is followed by a first GET of "Running".
func (s *osbMockSandbox) wire() map[string]any {
	state := s.states[min(s.stateIdx, len(s.states)-1)]
	if s.stateIdx < len(s.states)-1 {
		s.stateIdx++
	}
	status := map[string]any{"state": state}
	if s.reason != "" {
		status["reason"] = s.reason
	}
	if s.message != "" {
		status["message"] = s.message
	}
	body := map[string]any{
		"id":        s.id,
		"status":    status,
		"createdAt": s.createdAt.Format(time.RFC3339Nano),
	}
	if len(s.metadata) > 0 {
		body["metadata"] = s.metadata
	}
	if !s.expiresAt.IsZero() {
		body["expiresAt"] = s.expiresAt.Format(time.RFC3339Nano)
	}
	if s.image != "" {
		body["image"] = map[string]any{"uri": s.image}
	}
	if s.snapshotID != "" {
		body["snapshotId"] = s.snapshotID
	}
	return body
}

func snapshotWire(snap *osbMockSnapshot) map[string]any {
	state := snap.states[min(snap.stateIdx, len(snap.states)-1)]
	if snap.stateIdx < len(snap.states)-1 {
		snap.stateIdx++
	}
	return map[string]any{
		"id":        snap.id,
		"sandboxId": snap.sandboxID,
		"name":      snap.name,
		"status":    map[string]any{"state": state},
		"createdAt": snap.createdAt.Format(time.RFC3339Nano),
	}
}

func osbFileInfoWire(path string, node *osbMockNode) map[string]any {
	entryType := "file"
	if node.isDir {
		entryType = "directory"
	}
	mode := 420 // 0644
	if node.isDir {
		mode = 493 // 0755
	}
	ts := node.mod.UTC().Format(time.RFC3339Nano)
	return map[string]any{
		"path": path, "type": entryType,
		"size": int64(len(node.content)), "mode": mode,
		"modified_at": ts, "created_at": ts,
		"owner": "user", "group": "user",
	}
}

// sandboxIDFromAction extracts the ID from /v1/sandboxes/{id}/{action}.
func sandboxIDFromAction(path string) string {
	rest := strings.TrimPrefix(strings.TrimPrefix(path, "/v1"), "/sandboxes/")
	if idx := strings.LastIndex(rest, "/"); idx > 0 {
		return rest[:idx]
	}
	return rest
}

func parentDir(path string) string {
	trimmed := strings.TrimRight(path, "/")
	if idx := strings.LastIndex(trimmed, "/"); idx > 0 {
		return trimmed[:idx]
	}
	return "/"
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asInt64(v any) int64 {
	f, _ := v.(float64)
	return int64(f)
}

func asStringMap(v any) map[string]string {
	raw, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, item := range raw {
		out[k] = fmt.Sprint(item)
	}
	return out
}

// openSandboxTestConfig builds a *Config wired at the mock server. Both
// planes are same-origin (the endpoint the mock hands out points back at it),
// and AllowPrivateEndpoints is required because httptest binds 127.0.0.1.
func openSandboxTestConfig(t *testing.T, mock *openSandboxMockServer) *Config {
	t.Helper()
	return &Config{
		Type:                   SandboxTypeOpenSandbox,
		AllowPrivateEndpoints:  true,
		OpenSandboxAPIURL:      mock.APIURL(),
		OpenSandboxAPIKey:      mock.apiKey,
		OpenSandboxTemplate:    "registry.example.com/weknora-sandbox:main",
		OpenSandboxSandboxTTL:  30 * time.Minute,
		OpenSandboxHTTPTimeout: 5 * time.Second,
		DefaultTimeout:         60 * time.Second,
	}
}
