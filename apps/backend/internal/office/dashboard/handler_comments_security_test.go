package dashboard_test

import (
	"bytes"
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
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/shared"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
)

// commentSecurityFixture wires the minimal stack needed to exercise the
// comment handler's identity checks: a real agents service (to mint JWTs
// and answer CallerFromContext via AgentAuthMiddleware) sitting in front
// of a real dashboard service backed by the same in-memory SQLite. Unlike
// newTestDeps's router, this one is authenticated, so it can prove what
// happens when a caller's JWT identity and request-body author_id disagree.
type commentSecurityFixture struct {
	router    *gin.Engine
	agentsSvc *agents.AgentService
	repo      *sqlite.Repository
}

func newCommentSecurityFixture(t *testing.T) *commentSecurityFixture {
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
			parent_id TEXT DEFAULT '',
			project_id TEXT DEFAULT '',
			assignee_agent_profile_id TEXT DEFAULT '',
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
		office_workflow_id TEXT DEFAULT ''
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
	// GetTaskExecutionFields' assignee projection joins
	// workflow_step_participants (via RunnerProjection); build the
	// workflow repo against the same DB so that table exists, mirroring
	// newTestDeps in handler_test.go.
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
	dashboard.RegisterRoutes(group, svc, repo, nil, log)

	return &commentSecurityFixture{router: r, agentsSvc: agentsSvc, repo: repo}
}

// seedCommentAgent creates an agent_profiles row in the given workspace.
func seedCommentAgent(t *testing.T, svc *agents.AgentService, id, workspaceID string) *models.AgentInstance {
	t.Helper()
	a := &models.AgentInstance{
		ID:          id,
		WorkspaceID: workspaceID,
		Name:        id,
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
		Permissions: shared.DefaultPermissions(shared.AgentRoleWorker),
	}
	if err := svc.CreateAgentInstance(context.Background(), a); err != nil {
		t.Fatalf("create agent %q: %v", id, err)
	}
	return a
}

// seedCommentTask inserts a bare task row so GetTaskExecutionFields has
// something to resolve the workspace from.
func seedCommentTask(t *testing.T, repo *sqlite.Repository, id, workspaceID, assigneeAgentID string) {
	t.Helper()
	_, err := repo.ExecRaw(context.Background(),
		`INSERT INTO tasks (id, workspace_id, title, assignee_agent_profile_id) VALUES (?, ?, ?, ?)`,
		id, workspaceID, "task", assigneeAgentID,
	)
	if err != nil {
		t.Fatalf("seed task %q: %v", id, err)
	}
}

func postCommentReq(taskID, token, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/office/tasks/"+taskID+"/comments",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

// TestCreateComment_AgentAuthorPersistsJWTIdentity is the happy path: an
// agent's own JWT identity is what gets persisted as author_id.
func TestCreateComment_AgentAuthorPersistsJWTIdentity(t *testing.T) {
	f := newCommentSecurityFixture(t)
	agentA := seedCommentAgent(t, f.agentsSvc, "agent-a", "ws-1")
	seedCommentTask(t, f.repo, "task-1", "ws-1", agentA.ID)

	token, err := f.agentsSvc.MintRuntimeJWT(agentA.ID, "task-1", agentA.WorkspaceID, "", "sess-1", "")
	if err != nil {
		t.Fatalf("mint jwt: %v", err)
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, postCommentReq("task-1", token,
		`{"body":"hello","author_type":"agent"}`))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	comments, err := f.repo.ListTaskComments(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	if len(comments) != 1 || comments[0].AuthorID != agentA.ID {
		t.Fatalf("comments = %+v, want single comment authored by %q", comments, agentA.ID)
	}
}

// TestCreateComment_SpoofedAuthorIDRejectedWithForbidden is the core B1
// regression: an agent authenticated as A sending author_id: B must not
// persist B. Before the fix, the handler trusted req.AuthorID outright,
// letting any agent impersonate any other agent (including the task's own
// assignee, which would suppress that assignee's wake via the
// scheduler/reactivity.go self-comment guard).
func TestCreateComment_SpoofedAuthorIDRejectedWithForbidden(t *testing.T) {
	f := newCommentSecurityFixture(t)
	agentA := seedCommentAgent(t, f.agentsSvc, "agent-a", "ws-1")
	agentB := seedCommentAgent(t, f.agentsSvc, "agent-b", "ws-1")
	seedCommentTask(t, f.repo, "task-1", "ws-1", agentB.ID)

	token, err := f.agentsSvc.MintRuntimeJWT(agentA.ID, "task-1", agentA.WorkspaceID, "", "sess-1", "")
	if err != nil {
		t.Fatalf("mint jwt: %v", err)
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, postCommentReq("task-1", token,
		`{"body":"hello","author_type":"agent","author_id":"agent-b"}`))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (spoofed author_id rejected); body=%s", rec.Code, rec.Body.String())
	}
	comments, err := f.repo.ListTaskComments(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	for _, c := range comments {
		if c.AuthorID == agentB.ID {
			t.Fatalf("spoofed author_id %q from request body landed in a persisted comment", agentB.ID)
		}
	}
}

// TestCreateComment_CrossWorkspaceAgentForbidden pins workspace isolation:
// an agent in one workspace cannot comment as itself on a task that lives
// in another workspace (GetAgentInstance's name-based lookup can otherwise
// cross workspace boundaries).
func TestCreateComment_CrossWorkspaceAgentForbidden(t *testing.T) {
	f := newCommentSecurityFixture(t)
	agentA := seedCommentAgent(t, f.agentsSvc, "agent-a", "ws-a")
	seedCommentTask(t, f.repo, "task-1", "ws-b", "")

	token, err := f.agentsSvc.MintRuntimeJWT(agentA.ID, "task-1", agentA.WorkspaceID, "", "sess-1", "")
	if err != nil {
		t.Fatalf("mint jwt: %v", err)
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, postCommentReq("task-1", token,
		`{"body":"hello","author_type":"agent"}`))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (cross-workspace); body=%s", rec.Code, rec.Body.String())
	}
}

// TestCreateComment_UnauthenticatedAgentTypeForbidden pins that an
// agent-typed comment with no JWT at all is rejected outright, rather than
// falling back to trusting the request body.
func TestCreateComment_UnauthenticatedAgentTypeForbidden(t *testing.T) {
	f := newCommentSecurityFixture(t)
	agentA := seedCommentAgent(t, f.agentsSvc, "agent-a", "ws-1")
	seedCommentTask(t, f.repo, "task-1", "ws-1", agentA.ID)

	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, postCommentReq("task-1", "",
		`{"body":"hello","author_type":"agent","author_id":"agent-a"}`))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (no authenticated caller); body=%s", rec.Code, rec.Body.String())
	}
}

// TestCreateComment_AgentAuthorMatchingBodyIDSucceeds covers the live fast
// path the Kandev CLI actually uses: an authenticated agent sending its own
// correct author_id in the body. The mismatch/spoofing tests above only
// cover an absent or wrong body author_id, leaving this positive case
// unverified.
func TestCreateComment_AgentAuthorMatchingBodyIDSucceeds(t *testing.T) {
	f := newCommentSecurityFixture(t)
	agentA := seedCommentAgent(t, f.agentsSvc, "agent-a", "ws-1")
	seedCommentTask(t, f.repo, "task-1", "ws-1", agentA.ID)

	token, err := f.agentsSvc.MintRuntimeJWT(agentA.ID, "task-1", agentA.WorkspaceID, "", "sess-1", "")
	if err != nil {
		t.Fatalf("mint jwt: %v", err)
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, postCommentReq("task-1", token,
		`{"body":"hello","author_type":"agent","author_id":"agent-a"}`))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	comments, err := f.repo.ListTaskComments(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	if len(comments) != 1 || comments[0].AuthorID != agentA.ID {
		t.Fatalf("comments = %+v, want single comment authored by %q", comments, agentA.ID)
	}
}

// TestCreateComment_AuthenticatedAgentClaimingUserTypeForbidden is the P1
// regression from PR review: an authenticated agent that submits
// author_type: "user" must not be silently persisted as a user comment.
// Before the fix, the agent-attribution branch only ran when author_type
// was "agent", so a caller with a valid JWT but a "user" (or omitted) body
// type fell through to the user path — mislabeling the comment and letting
// it evade the scheduler's self-comment guard, which only suppresses a wake
// when author_type=="agent".
func TestCreateComment_AuthenticatedAgentClaimingUserTypeForbidden(t *testing.T) {
	f := newCommentSecurityFixture(t)
	agentA := seedCommentAgent(t, f.agentsSvc, "agent-a", "ws-1")
	seedCommentTask(t, f.repo, "task-1", "ws-1", agentA.ID)

	token, err := f.agentsSvc.MintRuntimeJWT(agentA.ID, "task-1", agentA.WorkspaceID, "", "sess-1", "")
	if err != nil {
		t.Fatalf("mint jwt: %v", err)
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, postCommentReq("task-1", token,
		`{"body":"hello","author_type":"user"}`))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (agent JWT claiming user author type); body=%s", rec.Code, rec.Body.String())
	}
	comments, err := f.repo.ListTaskComments(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	if len(comments) != 0 {
		t.Fatalf("comments = %+v, want none persisted", comments)
	}
}

// TestCreateComment_AuthenticatedAgentOmittedTypeDefaultsToAgent covers the
// other half of the same P1 fix: an authenticated agent that omits
// author_type entirely is attributed as an agent (derived from the JWT),
// not silently downgraded to the user sentinel.
func TestCreateComment_AuthenticatedAgentOmittedTypeDefaultsToAgent(t *testing.T) {
	f := newCommentSecurityFixture(t)
	agentA := seedCommentAgent(t, f.agentsSvc, "agent-a", "ws-1")
	seedCommentTask(t, f.repo, "task-1", "ws-1", agentA.ID)

	token, err := f.agentsSvc.MintRuntimeJWT(agentA.ID, "task-1", agentA.WorkspaceID, "", "sess-1", "")
	if err != nil {
		t.Fatalf("mint jwt: %v", err)
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, postCommentReq("task-1", token, `{"body":"hello"}`))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	comments, err := f.repo.ListTaskComments(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	if len(comments) != 1 || comments[0].AuthorType != "agent" || comments[0].AuthorID != agentA.ID {
		t.Fatalf("comments = %+v, want single agent comment authored by %q", comments, agentA.ID)
	}
}
