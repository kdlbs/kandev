package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/registry"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestRescanWorkspaceForSessionRefreshesSourceRoots(t *testing.T) {
	var got []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/workspace/rescan" {
			t.Errorf("path = %s", r.URL.Path)
		}
		var request struct {
			WorkspaceSourceRoots []string `json:"workspace_source_roots"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		got = request.WorkspaceSourceRoots
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mgr, execution := workspaceSourceTestManager(t, server.URL, []string{"/old"})
	roots := []string{"/attached/folder", "/attached/repo"}
	if err := mgr.RescanWorkspaceForSession(context.Background(), execution.SessionID, "", roots); err != nil {
		t.Fatalf("RescanWorkspaceForSession: %v", err)
	}
	if !sameStrings(got, roots) || !sameStrings(execution.WorkspaceSourceRoots, roots) {
		t.Fatalf("roots forwarded=%v stored=%v, want %v", got, execution.WorkspaceSourceRoots, roots)
	}
}

func TestRescanWorkspaceForSessionRestoresRootsOnFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))
	defer server.Close()

	mgr, execution := workspaceSourceTestManager(t, server.URL, []string{"/old"})
	if err := mgr.RescanWorkspaceForSession(context.Background(), execution.SessionID, "", []string{"/new"}); err == nil {
		t.Fatal("RescanWorkspaceForSession unexpectedly succeeded")
	}
	if !sameStrings(execution.WorkspaceSourceRoots, []string{"/old"}) {
		t.Fatalf("roots after failed rescan = %v, want old roots", execution.WorkspaceSourceRoots)
	}
}

func TestRebindWorkspaceForSessionRestoresRootsAfterRebindFailure(t *testing.T) {
	var roots [][]string
	stops := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/stop":
			stops++
			if stops == 2 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
		case "/api/v1/workspace/rebind":
			var request struct {
				WorkspaceSourceRoots []string `json:"workspace_source_roots"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			roots = append(roots, request.WorkspaceSourceRoots)
			w.WriteHeader(http.StatusUnprocessableEntity)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	mgr, execution := workspaceSourceTestManager(t, server.URL, []string{"/old"})
	execution.Status = v1.AgentStatusReady
	execution.ACPSessionID = "acp"
	if err := mgr.RebindWorkspaceForSession(context.Background(), execution.SessionID, "/new-workspace", []string{"/attached"}); err == nil {
		t.Fatal("RebindWorkspaceForSession unexpectedly succeeded")
	}
	if len(roots) != 1 || !sameStrings(roots[0], []string{"/attached"}) {
		t.Fatalf("rebind roots = %v, want attached roots", roots)
	}
	if !sameStrings(execution.WorkspaceSourceRoots, []string{"/old"}) {
		t.Fatalf("roots after failed rebind = %v, want old roots", execution.WorkspaceSourceRoots)
	}
}

func TestRebindWorkspaceForSessionWaitsForRestartedAdapterBeforeLoadingSession(t *testing.T) {
	server := newWorkspaceRebindAgentctlServer(t, false)

	mgr, execution := workspaceSourceTestManager(t, server.URL, []string{"/old"})
	t.Cleanup(server.Close)
	t.Cleanup(server.closeConnections)
	execution.Status = v1.AgentStatusReady
	execution.ACPSessionID = "acp-existing"
	if err := mgr.RebindWorkspaceForSession(context.Background(), execution.SessionID, "/new-workspace", []string{"/attached"}); err != nil {
		t.Fatalf("RebindWorkspaceForSession: %v", err)
	}
	if loads := server.loads(); len(loads) != 1 || loads[0] != "acp-existing" {
		t.Fatalf("loaded ACP sessions = %v, want [acp-existing]", loads)
	}
	if execution.ACPSessionID != "acp-existing" {
		t.Fatalf("ACP session ID = %q, want existing ID", execution.ACPSessionID)
	}
	if server.statusCalls() < 2 {
		t.Fatalf("status calls = %d, want readiness polling before load", server.statusCalls())
	}
	if actions := server.actions(); !sameStrings(actions, []string{"agent.initialize", "agent.session.load"}) {
		t.Fatalf("ACP actions = %v, want initialize before load", actions)
	}
}

func TestRebindWorkspaceForSessionCreatesNewSessionWhenProviderCannotChangeResumeCWD(t *testing.T) {
	server := newWorkspaceRebindAgentctlServer(t, false)

	mgr, execution := workspaceSourceTestManager(t, server.URL, []string{"/old"})
	t.Cleanup(server.Close)
	t.Cleanup(server.closeConnections)
	mgr.registry = registry.NewRegistry(newTestLogger())
	mgr.registry.LoadDefaults()
	history, err := NewSessionHistoryManager(t.TempDir(), "", newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	mgr.historyManager = history
	execution.AgentID = "opencode-acp"
	execution.Status = v1.AgentStatusReady
	execution.ACPSessionID = "acp-existing"
	execution.historyEnabled = true
	if err := history.AppendUserMessage(execution.SessionID, "earlier request"); err != nil {
		t.Fatal(err)
	}

	if err := mgr.RebindWorkspaceForSession(context.Background(), execution.SessionID, "/new-workspace", []string{"/attached"}); err != nil {
		t.Fatalf("RebindWorkspaceForSession: %v", err)
	}
	if loads := server.loads(); len(loads) != 0 {
		t.Fatalf("loaded ACP sessions = %v, want none", loads)
	}
	if execution.ACPSessionID != "acp-new" {
		t.Fatalf("ACP session ID = %q, want acp-new", execution.ACPSessionID)
	}
	if !execution.needsResumeContext {
		t.Fatal("fresh workspace session should inject recorded context on the next prompt")
	}
	if actions := server.actions(); !sameStrings(actions, []string{"agent.initialize", "agent.session.new"}) {
		t.Fatalf("ACP actions = %v, want initialize then new session", actions)
	}
}

func TestRebindWorkspaceForSessionCreatesNewSessionWhenAdditionalDirectoriesChange(t *testing.T) {
	server := newWorkspaceRebindAgentctlServer(t, false)
	t.Cleanup(server.Close)
	t.Cleanup(server.closeConnections)
	mgr, execution := workspaceSourceTestManager(t, server.URL, []string{"/old-workspace"})
	mgr.registry = registry.NewRegistry(newTestLogger())
	mgr.registry.LoadDefaults()
	execution.AgentID = "codex-acp"
	execution.Status = v1.AgentStatusReady
	execution.ACPSessionID = "acp-existing"

	newRoots := []string{"/new-workspace", "/new-workspace/attached-outside-cwd"}
	if err := mgr.RebindWorkspaceForSession(context.Background(), execution.SessionID, "/new-workspace", newRoots); err != nil {
		t.Fatalf("RebindWorkspaceForSession: %v", err)
	}
	if loads := server.loads(); len(loads) != 0 {
		t.Fatalf("loaded ACP sessions = %v, want none after directory grants changed", loads)
	}
	if execution.ACPSessionID != "acp-new" {
		t.Fatalf("ACP session ID = %q, want a new grant-bearing session", execution.ACPSessionID)
	}
	if !sameStrings(execution.WorkspaceSourceRoots, newRoots) {
		t.Fatalf("workspace roots = %v, want %v", execution.WorkspaceSourceRoots, newRoots)
	}
	if actions := server.actions(); !sameStrings(actions, []string{"agent.initialize", "agent.session.new"}) {
		t.Fatalf("ACP actions = %v, want initialize then new session", actions)
	}
}

func TestRebindWorkspaceForSessionReadinessTimeoutRollsBack(t *testing.T) {
	server := newWorkspaceRebindAgentctlServer(t, true)
	mgr, execution := workspaceSourceTestManager(t, server.URL, []string{"/old"})
	t.Cleanup(server.Close)
	t.Cleanup(server.closeConnections)

	previousTimeout, previousPoll := workspaceRebindReadyTimeout, workspaceRebindReadyPoll
	workspaceRebindReadyTimeout, workspaceRebindReadyPoll = 20*time.Millisecond, time.Millisecond
	t.Cleanup(func() {
		workspaceRebindReadyTimeout, workspaceRebindReadyPoll = previousTimeout, previousPoll
	})

	execution.Status = v1.AgentStatusReady
	execution.WorkspacePath = "/old-workspace"
	execution.ACPSessionID = "acp-existing"
	execution.RuntimeName = executor.NameLocal
	mgr.executorRegistry = NewExecutorRegistry(newTestRegistryLogger())
	mgr.executorRegistry.Register(&gitMetadataAttestingExecutor{MockExecutor: MockExecutor{name: executor.NameLocal}})
	oldProjection := newLinkedGitMetadataProjection(t)
	newProjection := newLinkedGitMetadataProjection(t)
	execution.GitMetadataProjections = []*worktree.GitMetadataProjection{oldProjection}
	if err := mgr.RebindWorkspaceWithGitMetadata(context.Background(), execution.SessionID, "/new-workspace", []*worktree.GitMetadataProjection{newProjection}, []string{"/attached"}); err == nil {
		t.Fatal("RebindWorkspaceForSession unexpectedly succeeded")
	}
	if got := server.reboundPaths(); !sameStrings(got, []string{"/new-workspace", "/old-workspace"}) {
		t.Fatalf("rebound paths = %v, want new then old", got)
	}
	if execution.WorkspacePath != "/old-workspace" || !sameStrings(execution.WorkspaceSourceRoots, []string{"/old"}) {
		t.Fatalf("execution after rollback = path %q roots %v, want old workspace and roots", execution.WorkspacePath, execution.WorkspaceSourceRoots)
	}
	if len(execution.GitMetadataProjections) != 1 || execution.GitMetadataProjections[0] != oldProjection {
		t.Fatalf("projections after rollback = %#v, want old projection", execution.GitMetadataProjections)
	}
	if loads := server.loads(); len(loads) != 1 || loads[0] != "acp-existing" {
		t.Fatalf("rollback loaded ACP sessions = %v, want [acp-existing]", loads)
	}
}

func TestRebindWorkspaceWithGitMetadataRejectsInvalidReplacementBeforeStopping(t *testing.T) {
	mgr := &Manager{executionStore: NewExecutionStore()}
	oldProjection := &worktree.GitMetadataProjection{CheckoutPath: "/old-workspace", Hash: "old"}
	execution := &AgentExecution{
		ID:                     "execution",
		SessionID:              "session",
		Status:                 v1.AgentStatusReady,
		ACPSessionID:           "acp-existing",
		GitMetadataProjections: []*worktree.GitMetadataProjection{oldProjection},
	}
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatal(err)
	}

	err := mgr.RebindWorkspaceWithGitMetadata(context.Background(), execution.SessionID, "/new-workspace", []*worktree.GitMetadataProjection{{CheckoutPath: "/missing-checkout"}}, []string{"/attached"})
	if err == nil || !strings.Contains(err.Error(), gitMetadataProjectionInvalid) {
		t.Fatalf("RebindWorkspaceWithGitMetadata error = %v, want %q", err, gitMetadataProjectionInvalid)
	}
	if len(execution.GitMetadataProjections) != 1 || execution.GitMetadataProjections[0] != oldProjection {
		t.Fatalf("projections after rejected replacement = %#v, want old projection", execution.GitMetadataProjections)
	}
}

func TestRebindWorkspaceWithGitMetadataFailsClosedWithoutRefreshCapability(t *testing.T) {
	mgr := &Manager{executionStore: NewExecutionStore()}
	execution := &AgentExecution{ID: "execution", SessionID: "session", Status: v1.AgentStatusReady, ACPSessionID: "acp", RuntimeName: executor.NameStandalone}
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatal(err)
	}

	err := mgr.RebindWorkspaceWithGitMetadata(context.Background(), execution.SessionID, "/new-workspace", []*worktree.GitMetadataProjection{newLinkedGitMetadataProjection(t)}, []string{"/attached"})
	if err == nil || !strings.Contains(err.Error(), gitMetadataProjectionUnsupported) {
		t.Fatalf("RebindWorkspaceWithGitMetadata error = %v, want %q", err, gitMetadataProjectionUnsupported)
	}
}

func TestRebindWorkspaceWithGitMetadataRejectsRemoteChildBeforeStopping(t *testing.T) {
	server := newWorkspaceRebindAgentctlServer(t, false)
	mgr, execution := workspaceSourceTestManager(t, server.URL, []string{"/old"})
	t.Cleanup(server.Close)
	t.Cleanup(server.closeConnections)

	oldProjection := newLinkedGitMetadataProjection(t)
	execution.Status = v1.AgentStatusReady
	execution.ACPSessionID = "acp-existing"
	execution.RuntimeName = executor.NameSSH
	execution.GitMetadataProjections = []*worktree.GitMetadataProjection{oldProjection}
	mgr.executorRegistry = NewExecutorRegistry(newTestRegistryLogger())
	mgr.executorRegistry.Register(&SSHExecutor{})

	err := mgr.RebindWorkspaceWithGitMetadata(context.Background(), execution.SessionID, "/new-workspace", []*worktree.GitMetadataProjection{newLinkedGitMetadataProjection(t)}, []string{"/attached"})
	if err == nil || !strings.Contains(err.Error(), gitMetadataProjectionUnsupported) {
		t.Fatalf("RebindWorkspaceWithGitMetadata error = %v, want %q", err, gitMetadataProjectionUnsupported)
	}
	if got := server.reboundPaths(); len(got) != 0 {
		t.Fatalf("remote child was rebound before policy refresh: %v", got)
	}
	if execution.Status != v1.AgentStatusReady || execution.WorkspacePath != "" || !sameStrings(execution.WorkspaceSourceRoots, []string{"/old"}) {
		t.Fatalf("execution changed after rejected remote rebind: status=%q path=%q roots=%v", execution.Status, execution.WorkspacePath, execution.WorkspaceSourceRoots)
	}
	if len(execution.GitMetadataProjections) != 1 || execution.GitMetadataProjections[0] != oldProjection {
		t.Fatalf("projection changed after rejected remote rebind: %#v", execution.GitMetadataProjections)
	}
}

func TestRebindWorkspaceWithGitMetadataRejectsDockerBeforeStopping(t *testing.T) {
	server := newWorkspaceRebindAgentctlServer(t, false)
	mgr, execution := workspaceSourceTestManager(t, server.URL, []string{"/old"})
	t.Cleanup(server.Close)
	t.Cleanup(server.closeConnections)

	oldProjection := newLinkedGitMetadataProjection(t)
	execution.Status = v1.AgentStatusReady
	execution.ACPSessionID = "acp-existing"
	execution.RuntimeName = executor.NameDocker
	execution.GitMetadataProjections = []*worktree.GitMetadataProjection{oldProjection}
	mgr.executorRegistry = NewExecutorRegistry(newTestRegistryLogger())
	mgr.executorRegistry.Register(&MockExecutor{name: executor.NameDocker})

	err := mgr.RebindWorkspaceWithGitMetadata(context.Background(), execution.SessionID, "/new-workspace", []*worktree.GitMetadataProjection{newLinkedGitMetadataProjection(t)}, []string{"/attached"})
	if err == nil || !strings.Contains(err.Error(), gitMetadataProjectionUnsupported) {
		t.Fatalf("RebindWorkspaceWithGitMetadata error = %v, want %q", err, gitMetadataProjectionUnsupported)
	}
	if !strings.Contains(err.Error(), "atomically replace") {
		t.Fatalf("RebindWorkspaceWithGitMetadata error = %v, want recovery guidance", err)
	}
	if got := server.reboundPaths(); len(got) != 0 {
		t.Fatalf("Docker child was rebound before mount refresh: %v", got)
	}
	if execution.Status != v1.AgentStatusReady || execution.WorkspacePath != "" || !sameStrings(execution.WorkspaceSourceRoots, []string{"/old"}) {
		t.Fatalf("execution changed after rejected Docker rebind: status=%q path=%q roots=%v", execution.Status, execution.WorkspacePath, execution.WorkspaceSourceRoots)
	}
	if len(execution.GitMetadataProjections) != 1 || execution.GitMetadataProjections[0] != oldProjection {
		t.Fatalf("projection changed after rejected Docker rebind: %#v", execution.GitMetadataProjections)
	}
}

type workspaceRebindAgentctlServer struct {
	*httptest.Server
	mu                sync.Mutex
	startCount        int
	stopCount         int
	childRunning      bool
	neverReady        bool
	firstStatus       bool
	statusCallCount   int
	paths             []string
	workspaceRoots    [][]string
	configured        []map[string]string
	attestations      int
	attestedRoots     [][]string
	attestationErr    bool
	failAttestAt      int
	failStartAt       int
	failConfigureAt   int
	failConfigure     bool
	failMaterialize   bool
	failMaterializeAt int
	failRemove        bool
	failLoadAt        int
	failNewAt         int
	materialized      map[string]bool
	operations        []string
	materializeCount  int
	loadCount         int
	newSessionCount   int
	loadedSessions    []string
	actionLog         []string
	connections       []*websocket.Conn
}

func newWorkspaceRebindAgentctlServer(t *testing.T, neverReady bool) *workspaceRebindAgentctlServer {
	t.Helper()
	server := &workspaceRebindAgentctlServer{neverReady: neverReady}
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/api/v1/stop", func(w http.ResponseWriter, _ *http.Request) {
		server.mu.Lock()
		server.stopCount++
		server.childRunning = false
		server.operations = append(server.operations, "stop")
		server.mu.Unlock()
		workspaceRebindSuccess(w, nil)
	})
	mux.HandleFunc("/api/v1/workspace/materialize-repository", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Destination string `json:"destination"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		server.mu.Lock()
		server.materializeCount++
		if server.failMaterialize || server.failMaterializeAt == server.materializeCount {
			server.mu.Unlock()
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "materialization rejected"})
			return
		}
		if server.materialized == nil {
			server.materialized = make(map[string]bool)
		}
		server.materialized[request.Destination] = true
		server.operations = append(server.operations, "materialize")
		server.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"destination": "added-main", "git_metadata_attested": true})
	})
	mux.HandleFunc("/api/v1/workspace/materialize-repository/remove", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Destination string `json:"destination"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		server.mu.Lock()
		if server.failRemove {
			server.mu.Unlock()
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "removal rejected"})
			return
		}
		delete(server.materialized, request.Destination)
		server.operations = append(server.operations, "remove")
		server.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]bool{"removed": true})
	})
	mux.HandleFunc("/api/v1/workspace/rescan", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			WorkspaceSourceRoots []string `json:"workspace_source_roots"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		server.mu.Lock()
		server.workspaceRoots = append(server.workspaceRoots, append([]string(nil), request.WorkspaceSourceRoots...))
		server.operations = append(server.operations, "rescan")
		server.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/v1/workspace/reconcile", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			WorkspaceSourceRoots []string `json:"workspace_source_roots"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		server.mu.Lock()
		server.workspaceRoots = append(server.workspaceRoots, append([]string(nil), request.WorkspaceSourceRoots...))
		server.operations = append(server.operations, "reconcile")
		server.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/v1/workspace/attest-git-metadata", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			CheckoutRoots []string `json:"checkout_roots"`
		}
		if r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}
		server.mu.Lock()
		server.attestations++
		server.operations = append(server.operations, "attest")
		failed := server.attestationErr || server.failAttestAt == server.attestations
		roots := append([]string(nil), server.workspaceRoots[len(server.workspaceRoots)-1]...)
		if request.CheckoutRoots != nil {
			roots = append([]string(nil), request.CheckoutRoots...)
		}
		server.attestedRoots = append(server.attestedRoots, append([]string(nil), roots...))
		for _, root := range roots[1:] {
			if !server.materialized[filepath.Base(root)] {
				failed = true
			}
		}
		server.mu.Unlock()
		if failed {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "attestation rejected"})
			return
		}
		checkouts := make([]map[string]string, 0, len(roots))
		for _, root := range roots {
			checkouts = append(checkouts, map[string]string{"checkout_path": root, "git_dir": root + "/.git"})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"attested": true, "checkouts": checkouts})
	})
	mux.HandleFunc("/api/v1/agent/configure", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Env map[string]string `json:"env"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		server.mu.Lock()
		server.configured = append(server.configured, request.Env)
		server.operations = append(server.operations, "configure")
		failed := server.failConfigure || server.failConfigureAt == len(server.configured)
		server.mu.Unlock()
		if failed {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "configuration rejected"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	})
	mux.HandleFunc("/api/v1/workspace/rebind", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			WorkDir string `json:"work_dir"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		server.mu.Lock()
		server.paths = append(server.paths, request.WorkDir)
		server.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/v1/start", func(w http.ResponseWriter, _ *http.Request) {
		server.mu.Lock()
		server.startCount++
		server.operations = append(server.operations, "start")
		failed := server.failStartAt == server.startCount
		server.firstStatus = true
		if !failed {
			server.childRunning = true
		}
		server.mu.Unlock()
		if failed {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "restart rejected"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
	})
	mux.HandleFunc("/api/v1/status", func(w http.ResponseWriter, _ *http.Request) {
		server.mu.Lock()
		server.statusCallCount++
		status := "running"
		if server.startCount == 1 && server.neverReady || server.firstStatus {
			status = "starting"
			server.firstStatus = false
		}
		server.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]string{"agent_status": status})
	})
	mux.HandleFunc("/api/v1/agent/stream", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		server.mu.Lock()
		server.connections = append(server.connections, conn)
		server.mu.Unlock()
		defer func() { _ = conn.Close() }()
		for {
			_, payload, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var request ws.Message
			if json.Unmarshal(payload, &request) != nil || request.Type != ws.MessageTypeRequest {
				continue
			}
			server.mu.Lock()
			server.actionLog = append(server.actionLog, request.Action)
			server.mu.Unlock()
			if request.Action == "agent.initialize" {
				response, _ := ws.NewResponse(request.ID, request.Action, map[string]any{"success": true})
				data, _ := json.Marshal(response)
				if conn.WriteMessage(websocket.TextMessage, data) != nil {
					return
				}
				continue
			}
			if request.Action == "agent.session.new" {
				server.mu.Lock()
				server.newSessionCount++
				failed := server.failNewAt == server.newSessionCount
				server.mu.Unlock()
				response, _ := ws.NewResponse(request.ID, request.Action, map[string]any{
					"success":    !failed,
					"session_id": "acp-new",
				})
				data, _ := json.Marshal(response)
				if conn.WriteMessage(websocket.TextMessage, data) != nil {
					return
				}
				continue
			}
			if request.Action != "agent.session.load" {
				continue
			}
			var load struct {
				SessionID string `json:"session_id"`
			}
			_ = request.ParsePayload(&load)
			server.mu.Lock()
			server.loadedSessions = append(server.loadedSessions, load.SessionID)
			server.loadCount++
			failed := server.failLoadAt == server.loadCount
			server.mu.Unlock()
			response, _ := ws.NewResponse(request.ID, request.Action, map[string]bool{"success": !failed})
			data, _ := json.Marshal(response)
			if conn.WriteMessage(websocket.TextMessage, data) != nil {
				return
			}
		}
	})
	server.Server = httptest.NewServer(mux)
	return server
}

func workspaceRebindSuccess(w http.ResponseWriter, _ *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (s *workspaceRebindAgentctlServer) loads() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.loadedSessions...)
}

func (s *workspaceRebindAgentctlServer) reboundPaths() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.paths...)
}

func (s *workspaceRebindAgentctlServer) statusCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusCallCount
}

func (s *workspaceRebindAgentctlServer) actions() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.actionLog...)
}

func (s *workspaceRebindAgentctlServer) configuredEnvs() []map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	configs := make([]map[string]string, len(s.configured))
	for index, env := range s.configured {
		configs[index] = cloneStringMap(env)
	}
	return configs
}

func (s *workspaceRebindAgentctlServer) attestationCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.attestations
}

func (s *workspaceRebindAgentctlServer) attestationRoots() [][]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	roots := make([][]string, len(s.attestedRoots))
	for index := range s.attestedRoots {
		roots[index] = append([]string(nil), s.attestedRoots[index]...)
	}
	return roots
}

func (s *workspaceRebindAgentctlServer) hasMaterialized(destination string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.materialized[destination]
}

func (s *workspaceRebindAgentctlServer) operationLog() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.operations...)
}

func (s *workspaceRebindAgentctlServer) running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.childRunning
}

func (s *workspaceRebindAgentctlServer) closeConnections() {
	s.mu.Lock()
	connections := append([]*websocket.Conn(nil), s.connections...)
	s.mu.Unlock()
	for _, connection := range connections {
		_ = connection.Close()
	}
}

func workspaceSourceTestManager(t *testing.T, rawURL string, roots []string) (*Manager, *AgentExecution) {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatal(err)
	}
	execution := &AgentExecution{ID: "execution", SessionID: "session", WorkspaceSourceRoots: roots, agentctl: agentctl.NewClient(parsed.Hostname(), port, newTestLogger())}
	store := NewExecutionStore()
	if err := store.Add(execution); err != nil {
		t.Fatal(err)
	}
	streamManager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	t.Cleanup(streamManager.Wait)
	return &Manager{executionStore: store, logger: newTestLogger(), streamManager: streamManager}, execution
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
