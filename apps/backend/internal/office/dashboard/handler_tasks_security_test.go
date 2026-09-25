package dashboard_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/agents"
	"github.com/kandev/kandev/internal/office/dashboard"
	officemodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/shared"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
)

// tasksSecurityFixture wires the authenticated dashboard stack for the
// GET /workspaces/:wsId/tasks route. An agent JWT must not reach this
// route: it carries no list_tasks capability check and no audit event,
// unlike GET /runtime/tasks, so an agent caller here would bypass both.
type tasksSecurityFixture struct {
	router    *gin.Engine
	agentsSvc *agents.AgentService
	repo      *sqlite.Repository
}

func newTasksSecurityFixture(t *testing.T) *tasksSecurityFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store: %v", err)
	}

	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL DEFAULT '',
			description TEXT DEFAULT '',
			state TEXT DEFAULT 'todo',
			priority TEXT NOT NULL DEFAULT 'medium' CHECK (priority IN ('critical','high','medium','low')),
			position INTEGER DEFAULT 0,
			parent_id TEXT DEFAULT '',
			project_id TEXT DEFAULT '',
			assignee_agent_profile_id TEXT DEFAULT '',
			assignee_user_id TEXT NOT NULL DEFAULT '',
			assignment_generation INTEGER NOT NULL DEFAULT 0,
			labels TEXT DEFAULT '[]',
			metadata TEXT DEFAULT '{}',
			identifier TEXT DEFAULT '',
			is_ephemeral INTEGER DEFAULT 0,
			origin TEXT DEFAULT 'manual',
			execution_policy TEXT DEFAULT '',
			execution_state TEXT DEFAULT '',
			workflow_id TEXT NOT NULL DEFAULT '',
			workflow_step_id TEXT DEFAULT '',
			archived_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS workspaces (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL DEFAULT '',
		description TEXT DEFAULT '',
		owner_id TEXT DEFAULT '',
		default_executor_id TEXT DEFAULT '',
		default_environment_id TEXT DEFAULT '',
		default_agent_profile_id TEXT DEFAULT '',
		default_config_agent_profile_id TEXT DEFAULT '',
		office_workflow_id TEXT DEFAULT '',
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		t.Fatalf("create workspaces table: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS workflows (
		id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL DEFAULT '',
		workflow_template_id TEXT DEFAULT '', name TEXT NOT NULL,
		description TEXT DEFAULT '', created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL
	)`)
	if err != nil {
		t.Fatalf("create workflows: %v", err)
	}
	if _, err := workflowrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("workflow repo: %v", err)
	}

	log := logger.Default()
	activity := shared.NewActivityLogger(repo, log)
	agentsSvc := agents.NewAgentService(repo, log, nil)
	agentsSvc.SetAuth(agents.NewAgentAuth("test-key"))
	svc := dashboard.NewDashboardService(repo, log, activity, agentsSvc, &stubCostChecker{})

	r := gin.New()
	r.Use(agents.AgentAuthMiddleware(agentsSvc))
	group := r.Group("/api/v1/office")
	dashboard.RegisterRoutes(group, svc, repo, nil, nil, nil, log)

	return &tasksSecurityFixture{router: r, agentsSvc: agentsSvc, repo: repo}
}

func seedTasksWorkspace(t *testing.T, repo *sqlite.Repository, id string) {
	t.Helper()
	_, err := repo.ExecRaw(context.Background(),
		`INSERT OR IGNORE INTO workspaces (id, name) VALUES (?, ?)`, id, id,
	)
	if err != nil {
		t.Fatalf("seed workspace %q: %v", id, err)
	}
}

func seedTasksAgent(t *testing.T, svc *agents.AgentService, repo *sqlite.Repository, id, workspaceID string) *officemodels.AgentInstance {
	t.Helper()
	seedTasksWorkspace(t, repo, workspaceID)
	a := &officemodels.AgentInstance{
		ID:          id,
		WorkspaceID: workspaceID,
		Name:        id,
		Role:        officemodels.AgentRoleWorker,
		Status:      officemodels.AgentStatusIdle,
		Permissions: shared.DefaultPermissions(shared.AgentRoleWorker),
	}
	if err := svc.CreateAgentInstance(context.Background(), a); err != nil {
		t.Fatalf("create agent %q: %v", id, err)
	}
	return a
}

func listTasksReq(wsID, token string) *http.Request {
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/office/workspaces/"+wsID+"/tasks",
		nil,
	)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

func searchTasksReq(wsID, token string) *http.Request {
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/office/workspaces/"+wsID+"/tasks/search",
		nil,
	)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

// TestListTasks_AgentCallerMustUseRuntimeEndpoint proves the fix for the
// bypass an agent JWT had of the list_tasks capability gate: GET
// /workspaces/:wsId/tasks sits under the same Office route group as GET
// /runtime/tasks and inherits AgentAuthMiddleware, which only validates the
// token — it does not check any runtime capability. Before this fix, an
// agent whose capability snapshot lacked list_tasks (or a taskless run with
// no board-read grant at all) could still reach the whole workspace's task
// list through this route, ungated and unaudited.
func TestListTasks_AgentCallerMustUseRuntimeEndpoint(t *testing.T) {
	f := newTasksSecurityFixture(t)
	agent := seedTasksAgent(t, f.agentsSvc, f.repo, "agent-a", "ws-1")

	// A run token with an explicit empty capabilities snapshot ({}), which
	// denies every capability including list_tasks.
	token, err := f.agentsSvc.MintRuntimeJWT(agent.ID, "", agent.WorkspaceID, "run-1", "sess-1", "{}")
	if err != nil {
		t.Fatalf("mint jwt: %v", err)
	}

	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, listTasksReq("ws-1", token))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
}

// TestListTasks_UICallerWithoutTokenSucceeds proves the fix is scoped to
// agent callers only: a browser/UI request with no bearer token keeps
// working exactly as before.
func TestListTasks_UICallerWithoutTokenSucceeds(t *testing.T) {
	f := newTasksSecurityFixture(t)
	seedTasksWorkspace(t, f.repo, "ws-1")

	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, listTasksReq("ws-1", ""))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}

// TestSearchTasks_AgentCallerMustUseRuntimeEndpoint proves the same fix for
// the sibling GET /workspaces/:wsId/tasks/search route: an empty query
// invokes the same unbounded ListTasks read as the plain list route, and a
// nonempty query searches the same workspace, so this route needs the
// identical agent-caller guard.
func TestSearchTasks_AgentCallerMustUseRuntimeEndpoint(t *testing.T) {
	f := newTasksSecurityFixture(t)
	agent := seedTasksAgent(t, f.agentsSvc, f.repo, "agent-a", "ws-1")

	token, err := f.agentsSvc.MintRuntimeJWT(agent.ID, "", agent.WorkspaceID, "run-1", "sess-1", "{}")
	if err != nil {
		t.Fatalf("mint jwt: %v", err)
	}

	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, searchTasksReq("ws-1", token))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
}

// TestSearchTasks_UICallerWithoutTokenSucceeds proves the fix is scoped to
// agent callers only: a browser/UI request with no bearer token keeps
// working exactly as before.
func TestSearchTasks_UICallerWithoutTokenSucceeds(t *testing.T) {
	f := newTasksSecurityFixture(t)
	seedTasksWorkspace(t, f.repo, "ws-1")

	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, searchTasksReq("ws-1", ""))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}
