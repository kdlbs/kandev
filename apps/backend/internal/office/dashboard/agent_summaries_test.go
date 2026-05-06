package dashboard_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
)

// stubAgentList satisfies shared.AgentReader and returns a fixed list of
// agents regardless of workspace. Tests assemble the slice they want and
// hand the stub to the dashboard service.
type stubAgentList struct {
	agents []*models.AgentInstance
}

func (s *stubAgentList) GetAgentInstance(_ context.Context, id string) (*models.AgentInstance, error) {
	for _, a := range s.agents {
		if a.ID == id {
			return a, nil
		}
	}
	return nil, nil
}

func (s *stubAgentList) ListAgentInstances(_ context.Context, _ string) ([]*models.AgentInstance, error) {
	return s.agents, nil
}

func (s *stubAgentList) ListAgentInstancesByIDs(_ context.Context, ids []string) ([]*models.AgentInstance, error) {
	want := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		want[id] = struct{}{}
	}
	out := make([]*models.AgentInstance, 0, len(ids))
	for _, a := range s.agents {
		if _, ok := want[a.ID]; ok {
			out = append(out, a)
		}
	}
	return out, nil
}

// sessionsTableDDL extends run_activity_test's barebones schema with the
// columns ListRecentSessionsByAgentBatch reads (agent_profile_id) and
// the task_session_messages table CountToolCallMessagesBySession queries.
const sessionsTableDDL = `
CREATE TABLE IF NOT EXISTS task_sessions (
	id TEXT PRIMARY KEY,
	task_id TEXT NOT NULL,
	agent_execution_id TEXT NOT NULL DEFAULT '',
	agent_profile_id TEXT,
	state TEXT NOT NULL DEFAULT 'CREATED',
	started_at TIMESTAMP NOT NULL,
	completed_at TIMESTAMP,
	updated_at TIMESTAMP NOT NULL
);
CREATE TABLE IF NOT EXISTS task_session_messages (
	id TEXT PRIMARY KEY,
	task_session_id TEXT NOT NULL,
	type TEXT NOT NULL DEFAULT 'message',
	content TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMP NOT NULL
);
`

func ensureAgentSummariesSchema(t *testing.T, db *sqlx.DB) {
	t.Helper()
	if _, err := db.Exec(sessionsTableDDL); err != nil {
		t.Fatalf("ensureAgentSummariesSchema: %v", err)
	}
}

// fetchAgentSummaries hits the new GET endpoint and returns the parsed
// response, failing the test if the call doesn't 200.
func fetchAgentSummaries(t *testing.T, deps *testDeps, wsID string) dashboard.AgentSummariesResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/"+wsID+"/agent-summaries", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("agent-summaries: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp dashboard.AgentSummariesResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp
}

// withAgents replaces deps.svc + deps.router with a fresh service whose
// AgentReader returns `agents`. The default newTestDeps stubAgentReader
// always returns nil; this helper swaps it for the test's expected list.
func withAgents(t *testing.T, deps *testDeps, agents []*models.AgentInstance) {
	t.Helper()
	log := logger.Default()
	activity := shared.NewActivityLogger(deps.repo, log)
	stubAgents := &stubAgentList{agents: agents}
	svc := dashboard.NewDashboardService(deps.repo, log, activity, stubAgents, &stubCostChecker{})

	router := gin.New()
	group := router.Group("/api/v1/office")
	dashboard.RegisterRoutes(group, svc, deps.repo, nil, log)

	deps.svc = svc
	deps.router = router
}

func insertSession(t *testing.T, db *sqlx.DB, id, taskID, agentInstanceID, state string, startedAt time.Time, completedAt *time.Time) {
	t.Helper()
	if completedAt != nil {
		_, err := db.Exec(`
			INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, completed_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, id, taskID, agentInstanceID, state, startedAt.UTC().Format(time.RFC3339), completedAt.UTC().Format(time.RFC3339), startedAt.UTC().Format(time.RFC3339))
		if err != nil {
			t.Fatalf("insert session %s: %v", id, err)
		}
		return
	}
	_, err := db.Exec(`
		INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, id, taskID, agentInstanceID, state, startedAt.UTC().Format(time.RFC3339), startedAt.UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert session %s: %v", id, err)
	}
}

func insertToolCall(t *testing.T, db *sqlx.DB, id, sessionID string) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO task_session_messages (id, task_session_id, type, created_at)
		VALUES (?, ?, 'tool_call', ?)
	`, id, sessionID, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert tool_call %s: %v", id, err)
	}
}

func TestGetAgentSummaries_EmptyWorkspace(t *testing.T) {
	deps := newTestDeps(t)
	ensureAgentSummariesSchema(t, deps.db)

	resp := fetchAgentSummaries(t, deps, "ws-empty")
	if len(resp.Agents) != 0 {
		t.Fatalf("agents = %d, want 0", len(resp.Agents))
	}
	if resp.Agents == nil {
		t.Fatalf("agents slice is nil; expected empty []")
	}
}

func TestGetAgentSummaries_AgentsNoSessions(t *testing.T) {
	deps := newTestDeps(t)
	ensureAgentSummariesSchema(t, deps.db)

	withAgents(t, deps, []*models.AgentInstance{
		{ID: "a1", Name: "CEO", Role: "ceo"},
		{ID: "a2", Name: "Worker", Role: "worker"},
		{ID: "a3", Name: "Reviewer", Role: "worker"},
	})

	resp := fetchAgentSummaries(t, deps, "ws-no-sessions")
	if len(resp.Agents) != 3 {
		t.Fatalf("agents = %d, want 3", len(resp.Agents))
	}
	for _, a := range resp.Agents {
		if a.Status != "never_run" {
			t.Errorf("agent %s status = %q, want never_run", a.AgentID, a.Status)
		}
		if a.LiveSession != nil {
			t.Errorf("agent %s has unexpected live_session", a.AgentID)
		}
		if a.LastSession != nil {
			t.Errorf("agent %s has unexpected last_session", a.AgentID)
		}
		if len(a.RecentSessions) != 0 {
			t.Errorf("agent %s recent_sessions = %d, want 0", a.AgentID, len(a.RecentSessions))
		}
	}
}

func TestGetAgentSummaries_MixedStatuses(t *testing.T) {
	deps := newTestDeps(t)
	ensureAgentSummariesSchema(t, deps.db)

	withAgents(t, deps, []*models.AgentInstance{
		{ID: "live-agent", Name: "Alice", Role: "ceo"},
		{ID: "finished-agent", Name: "Bob", Role: "worker"},
		{ID: "idle-agent", Name: "Carol", Role: "worker"},
	})

	insertTestTask(t, deps.db, "t-live", "ws-mixed", "Active task", "in_progress", 2)
	insertTestTask(t, deps.db, "t-done", "ws-mixed", "Done task", "todo", 2)

	now := time.Now().UTC()
	insertSession(t, deps.db, "s-live", "t-live", "live-agent", "RUNNING", now.Add(-1*time.Minute), nil)

	completed := now.Add(-30 * time.Second)
	insertSession(t, deps.db, "s-done", "t-done", "finished-agent", "COMPLETED", now.Add(-5*time.Minute), &completed)
	insertToolCall(t, deps.db, "msg-1", "s-done")
	insertToolCall(t, deps.db, "msg-2", "s-done")

	resp := fetchAgentSummaries(t, deps, "ws-mixed")
	if len(resp.Agents) != 3 {
		t.Fatalf("agents = %d, want 3", len(resp.Agents))
	}

	byID := map[string]dashboard.AgentSummary{}
	for _, a := range resp.Agents {
		byID[a.AgentID] = a
	}

	if got := byID["live-agent"].Status; got != "live" {
		t.Errorf("live-agent status = %q, want live", got)
	}
	if byID["live-agent"].LiveSession == nil {
		t.Error("live-agent missing live_session")
	}
	if got := byID["finished-agent"].Status; got != "finished" {
		t.Errorf("finished-agent status = %q, want finished", got)
	}
	if byID["finished-agent"].LiveSession != nil {
		t.Error("finished-agent should not have live_session")
	}
	if byID["finished-agent"].LastSession == nil {
		t.Error("finished-agent missing last_session")
	}
	if got := byID["finished-agent"].LastSession.CommandCount; got != 2 {
		t.Errorf("finished-agent command_count = %d, want 2", got)
	}
	if got := byID["idle-agent"].Status; got != "never_run" {
		t.Errorf("idle-agent status = %q, want never_run", got)
	}

	// Sort: live agent first.
	if resp.Agents[0].AgentID != "live-agent" {
		t.Errorf("first agent = %q, want live-agent", resp.Agents[0].AgentID)
	}
}

// TestGetAgentSummaries_HandlerEmptyResponseShape pins the JSON shape of an
// empty workspace response — frontend depends on `agents` being an array,
// never null.
func TestGetAgentSummaries_HandlerEmptyResponseShape(t *testing.T) {
	deps := newTestDeps(t)
	ensureAgentSummariesSchema(t, deps.db)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/ws-empty-shape/agent-summaries", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(raw["agents"]) != "[]" {
		t.Errorf("agents JSON = %s, want []", string(raw["agents"]))
	}
}

func TestGetAgentSummaries_RecentSessionsCappedAt5(t *testing.T) {
	deps := newTestDeps(t)
	ensureAgentSummariesSchema(t, deps.db)

	withAgents(t, deps, []*models.AgentInstance{{ID: "busy", Name: "Busy", Role: "worker"}})

	insertTestTask(t, deps.db, "t-busy", "ws-cap", "Busy task", "todo", 2)

	now := time.Now().UTC()
	for i := 0; i < 8; i++ {
		started := now.Add(-time.Duration(i+1) * time.Minute)
		completed := started.Add(10 * time.Second)
		insertSession(t, deps.db, "s-busy-"+string(rune('a'+i)), "t-busy", "busy", "COMPLETED", started, &completed)
	}

	resp := fetchAgentSummaries(t, deps, "ws-cap")
	if len(resp.Agents) != 1 {
		t.Fatalf("agents = %d, want 1", len(resp.Agents))
	}
	got := len(resp.Agents[0].RecentSessions)
	if got != 5 {
		t.Errorf("recent_sessions = %d, want 5 (cap)", got)
	}
}
