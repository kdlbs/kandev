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
	officemodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/shared"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
)

// commentSecurityFixture wires the authenticated dashboard stack. The route
// accepts UI requests without a token, but agent tokens must use the runtime
// endpoint so capability and task-scope checks cannot be bypassed.
type commentSecurityFixture struct {
	router    *gin.Engine
	agentsSvc *agents.AgentService
	repo      *sqlite.Repository
	handoff   *taskservice.HandoffService
}

// commentWindowReaderAdapter bridges the office repo's Office-model comment
// window read to the task service's neutral CommentReader contract — the
// same conversion internal/backendapp's officeCommentReaderAdapter performs
// in production, duplicated here because that adapter is unexported outside
// its package.
type commentWindowReaderAdapter struct {
	repo *sqlite.Repository
}

func (a *commentWindowReaderAdapter) ListTaskCommentsWindow(
	ctx context.Context, taskID string, limit int,
) ([]taskservice.CommentRecord, int, error) {
	rows, total, err := a.repo.ListTaskCommentsWindow(ctx, taskID, limit)
	if err != nil {
		return nil, 0, err
	}
	records := make([]taskservice.CommentRecord, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		records = append(records, taskservice.CommentRecord{
			ID:         row.ID,
			TaskID:     row.TaskID,
			AuthorType: row.AuthorType,
			AuthorID:   row.AuthorID,
			Source:     row.Source,
			Body:       row.Body,
			CreatedAt:  row.CreatedAt,
		})
	}
	return records, total, nil
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
			position INTEGER DEFAULT 0,
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

	taskRepo, err := tasksqlite.NewWithDB(db, db, log)
	if err != nil {
		t.Fatalf("new task repo: %v", err)
	}
	// The task repo's migrations leave PRAGMA foreign_keys=ON on this shared
	// connection. Restore the fixture's pre-existing FK-disabled behavior —
	// tests here seed agents/tasks minimally and don't register the CLI
	// tool rows agent_profiles.agent_id's FK points at.
	if _, err := db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		t.Fatalf("disable foreign_keys: %v", err)
	}
	handoffDocSvc := taskservice.NewDocumentService(taskRepo, log)
	handoff := taskservice.NewHandoffService(taskRepo, taskRepo, handoffDocSvc, repo, repo, log)
	handoff.SetCommentReader(&commentWindowReaderAdapter{repo: repo})

	r := gin.New()
	r.Use(agents.AgentAuthMiddleware(agentsSvc))
	group := r.Group("/api/v1/office")
	dashboard.RegisterRoutes(group, svc, repo, nil, handoff, log)

	return &commentSecurityFixture{router: r, agentsSvc: agentsSvc, repo: repo, handoff: handoff}
}

// seedCommentWorkspace inserts a minimal workspace row. Required because the
// fixture shares its sqlite connection with a task/repository/sqlite
// instance, whose migrations leave PRAGMA foreign_keys=ON on that
// connection — agents and tasks referencing an unseeded workspace_id now
// fail their FK constraint instead of silently succeeding.
func seedCommentWorkspace(t *testing.T, repo *sqlite.Repository, id string) {
	t.Helper()
	_, err := repo.ExecRaw(context.Background(),
		`INSERT OR IGNORE INTO workspaces (id, name) VALUES (?, ?)`, id, id,
	)
	if err != nil {
		t.Fatalf("seed workspace %q: %v", id, err)
	}
}

func seedCommentAgent(t *testing.T, svc *agents.AgentService, repo *sqlite.Repository, id, workspaceID string) *officemodels.AgentInstance {
	t.Helper()
	seedCommentWorkspace(t, repo, workspaceID)
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
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/office/tasks/"+taskID+"/comments",
		bytes.NewBufferString(body),
	)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

func TestCreateComment_AgentCallerMustUseRuntimeEndpoint(t *testing.T) {
	f := newCommentSecurityFixture(t)
	agent := seedCommentAgent(t, f.agentsSvc, f.repo, "agent-a", "ws-1")
	seedCommentTask(t, f.repo, "task-1", "ws-1", agent.ID)
	seedCommentTask(t, f.repo, "task-2", "ws-1", "")

	// The token has no post_comment capability and is scoped to task-1. A
	// dashboard request for task-2 must still be rejected before persistence.
	token, err := f.agentsSvc.MintRuntimeJWT(agent.ID, "task-1", agent.WorkspaceID, "run-1", "sess-1", "")
	if err != nil {
		t.Fatalf("mint jwt: %v", err)
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, postCommentReq("task-2", token, `{"body":"hello"}`))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	comments, err := f.repo.ListTaskComments(context.Background(), "task-2")
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	if len(comments) != 0 {
		t.Fatalf("comments = %+v, want none persisted", comments)
	}
}

func TestCreateComment_RejectsAgentAuthorTypeWithoutJWT(t *testing.T) {
	f := newCommentSecurityFixture(t)
	seedCommentTask(t, f.repo, "task-1", "ws-1", "")

	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, postCommentReq("task-1", "", `{"body":"hello","author_type":"agent"}`))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
}
