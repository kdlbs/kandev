package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
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
	if err := mgr.RebindWorkspaceForSession(context.Background(), execution.SessionID, "/new-workspace", []string{"/attached"}); err == nil {
		t.Fatal("RebindWorkspaceForSession unexpectedly succeeded")
	}
	if got := server.reboundPaths(); !sameStrings(got, []string{"/new-workspace", "/old-workspace"}) {
		t.Fatalf("rebound paths = %v, want new then old", got)
	}
	if execution.WorkspacePath != "/old-workspace" || !sameStrings(execution.WorkspaceSourceRoots, []string{"/old"}) {
		t.Fatalf("execution after rollback = path %q roots %v, want old workspace and roots", execution.WorkspacePath, execution.WorkspaceSourceRoots)
	}
	if loads := server.loads(); len(loads) != 1 || loads[0] != "acp-existing" {
		t.Fatalf("rollback loaded ACP sessions = %v, want [acp-existing]", loads)
	}
}

type workspaceRebindAgentctlServer struct {
	*httptest.Server
	mu              sync.Mutex
	startCount      int
	neverReady      bool
	firstStatus     bool
	statusCallCount int
	paths           []string
	loadedSessions  []string
	actionLog       []string
	connections     []*websocket.Conn
}

func newWorkspaceRebindAgentctlServer(t *testing.T, neverReady bool) *workspaceRebindAgentctlServer {
	t.Helper()
	server := &workspaceRebindAgentctlServer{neverReady: neverReady}
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/api/v1/stop", workspaceRebindSuccess)
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
		server.firstStatus = true
		server.mu.Unlock()
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
			if request.Action != "agent.session.load" {
				continue
			}
			var load struct {
				SessionID string `json:"session_id"`
			}
			_ = request.ParsePayload(&load)
			server.mu.Lock()
			server.loadedSessions = append(server.loadedSessions, load.SessionID)
			server.mu.Unlock()
			response, _ := ws.NewResponse(request.ID, request.Action, map[string]bool{"success": true})
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
