package dashboard_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/shared"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
)

// stubPermissionLister satisfies shared.PermissionLister for testing.
type stubPermissionLister struct {
	items []shared.PendingPermission
}

func (s *stubPermissionLister) ListPendingPermissions() []shared.PendingPermission {
	return s.items
}

// stubSettingsProvider satisfies dashboard.SettingsProvider for testing.
type stubSettingsProvider struct {
	settings *dashboard.WorkspaceSettings
	lastMode string
}

func (s *stubSettingsProvider) GetSettings(_ string) (*dashboard.WorkspaceSettings, error) {
	if s.settings == nil {
		return &dashboard.WorkspaceSettings{Name: "default", PermissionHandlingMode: "human"}, nil
	}
	return s.settings, nil
}

func (s *stubSettingsProvider) UpdatePermissionHandlingMode(_ string, mode string) error {
	s.lastMode = mode
	return nil
}

func (s *stubSettingsProvider) UpdateRecoveryLookbackHours(_ string, _ int) error {
	return nil
}

// testDeps wires a real in-memory dashboard service for testing.
type testDeps struct {
	db     *sqlx.DB
	repo   *sqlite.Repository
	svc    *dashboard.DashboardService
	router *gin.Engine
	agents *stubAgentReader
}

func newTestDeps(t *testing.T) *testDeps {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// In-memory SQLite gives each connection its own database, so any
	// goroutine that grabs a fresh connection from the pool sees an
	// empty schema. The dashboard endpoint now fans its sub-queries
	// out via errgroup; pin the pool to a single connection so all
	// goroutines see the same in-memory tables.
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store: %v", err)
	}

	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	// Create the tasks table (owned by task service in production).
	// workflow_step_id is required so the per-task participant lookup
	// (now backed by workflow_step_participants) can resolve a step.
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
			identifier TEXT DEFAULT '',
			is_ephemeral INTEGER DEFAULT 0,
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

	// workflows is referenced by FKs on workflow_steps (workflow store
	// schema). Create it before NewWithDB so the workflow repo's
	// initSchema can succeed.
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS workflows (
		id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL DEFAULT '',
		workflow_template_id TEXT DEFAULT '', name TEXT NOT NULL,
		description TEXT DEFAULT '', created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL
	)`)
	if err != nil {
		t.Fatalf("create workflows: %v", err)
	}
	// task_sessions is referenced by FK on the workflow store's
	// session_step_history table. Mirror the columns the dashboard
	// tests insert against so later CREATE-IF-NOT-EXISTS calls in
	// individual tests are no-ops on a compatible schema.
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS task_sessions (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL DEFAULT '',
		agent_execution_id TEXT NOT NULL DEFAULT '',
		agent_profile_id TEXT,
		state TEXT NOT NULL DEFAULT 'CREATED',
		started_at TIMESTAMP,
		completed_at TIMESTAMP,
		updated_at TIMESTAMP
	)`)
	if err != nil {
		t.Fatalf("create task_sessions: %v", err)
	}
	// Build the workflow repo against the same DB so workflow_step_participants
	// and workflow_step_decisions exist with their canonical schema.
	wfRepo, err := workflowrepo.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("workflow repo: %v", err)
	}

	// Create label tables used by the issues endpoints.
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS office_labels (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			name TEXT NOT NULL,
			color TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(workspace_id, name)
		)
	`)
	if err != nil {
		t.Fatalf("create office_labels table: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS office_task_labels (
			task_id TEXT NOT NULL,
			label_id TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (task_id, label_id)
		)
	`)
	if err != nil {
		t.Fatalf("create office_task_labels table: %v", err)
	}

	log := logger.Default()
	activity := shared.NewActivityLogger(repo, log)
	agentSvc := &stubAgentReader{}
	costSvc := &stubCostChecker{}
	svc := dashboard.NewDashboardService(repo, log, activity, agentSvc, costSvc)
	svc.SetDecisionStore(wfRepo)

	router := gin.New()
	group := router.Group("/api/v1/office")
	dashboard.RegisterRoutes(group, svc, repo, nil, log)

	return &testDeps{db: db, repo: repo, svc: svc, router: router, agents: agentSvc}
}

// stubAgentReader returns nil/nil by default; tests that need agent
// resolution (e.g. pending_approvers name lookup) populate `names`.
type stubAgentReader struct{ names map[string]string }

func (s *stubAgentReader) GetAgentInstance(_ context.Context, id string) (*models.AgentInstance, error) {
	if name, ok := s.names[id]; ok {
		return &models.AgentInstance{ID: id, Name: name}, nil
	}
	return nil, nil
}

func (s *stubAgentReader) ListAgentInstances(_ context.Context, _ string) ([]*models.AgentInstance, error) {
	return nil, nil
}

func (s *stubAgentReader) ListAgentInstancesByIDs(_ context.Context, ids []string) ([]*models.AgentInstance, error) {
	out := make([]*models.AgentInstance, 0, len(ids))
	for _, id := range ids {
		if name, ok := s.names[id]; ok {
			out = append(out, &models.AgentInstance{ID: id, Name: name})
		}
	}
	return out, nil
}

type stubCostChecker struct{}

func (s *stubCostChecker) GetCostSummary(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

// insertTestTask inserts a minimal task row for testing. The legacy
// integer priority is mapped to the matching TEXT priority label since
// the priority column is now TEXT after the office migration.
func insertTestTask(t *testing.T, db *sqlx.DB, id, wsID, title, state string, priority int) {
	t.Helper()
	// Give every test task a deterministic workflow_step_id so the office
	// participant lookup (which now resolves through the task's step) has
	// a stable target.
	_, err := db.Exec(`
		INSERT INTO tasks (id, workspace_id, title, state, priority, identifier, workflow_step_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
	`, id, wsID, title, state, intPriorityLabel(priority), id, "step-"+id)
	if err != nil {
		t.Fatalf("insert task %s: %v", id, err)
	}
}

// intPriorityLabel maps a legacy integer priority to its TEXT label.
func intPriorityLabel(p int) string {
	switch p {
	case 4:
		return "critical"
	case 3:
		return "high"
	case 1:
		return "low"
	default:
		return "medium"
	}
}

func TestCreateComment_PublishesOfficeCommentCreated(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "task-comment-event", "ws-1", "Comment Event", "todo", 1)

	eb := bus.NewMemoryEventBus(logger.Default())
	deps.svc.SetEventBus(eb)

	var got map[string]string
	if _, err := eb.Subscribe(events.OfficeCommentCreated, func(_ context.Context, event *bus.Event) error {
		raw, ok := event.Data.(map[string]string)
		if !ok {
			t.Fatalf("event data type = %T, want map[string]string", event.Data)
		}
		got = raw
		return nil
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/office/tasks/task-comment-event/comments", strings.NewReader(`{"body":"Please revise"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	deps.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got["task_id"] != "task-comment-event" || got["comment_id"] == "" || got["author_type"] != "user" {
		t.Fatalf("event data = %#v, want task/comment/user fields", got)
	}
}

func insertTestChildTask(
	t *testing.T,
	db *sqlx.DB,
	id, wsID, parentID, title, state string,
	priority int,
) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO tasks (
			id, workspace_id, parent_id, title, state, priority, identifier, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
	`, id, wsID, parentID, title, state, intPriorityLabel(priority), id)
	if err != nil {
		t.Fatalf("insert child task %s: %v", id, err)
	}
}

func TestListTasks_ReturnsIssuesForWorkspace(t *testing.T) {
	deps := newTestDeps(t)
	db := deps.db

	insertTestTask(t, db, "t1", "ws1", "Alpha", "in_progress", 3)
	insertTestTask(t, db, "t2", "ws1", "Beta", "todo", 1)
	insertTestTask(t, db, "t3", "ws2", "Other", "done", 0)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/ws1/tasks", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp dashboard.TaskListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Tasks) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(resp.Tasks))
	}
	for _, iss := range resp.Tasks {
		if iss.WorkspaceID != "ws1" {
			t.Errorf("unexpected workspace id %q", iss.WorkspaceID)
		}
	}
}

func TestListTasks_PriorityMappedToString(t *testing.T) {
	deps := newTestDeps(t)
	db := deps.db

	insertTestTask(t, db, "p4", "wsp", "Critical task", "todo", 4)
	insertTestTask(t, db, "p3", "wsp", "High task", "todo", 3)
	insertTestTask(t, db, "p0", "wsp", "No priority", "todo", 0)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/wsp/tasks", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp dashboard.TaskListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	byID := make(map[string]string)
	for _, iss := range resp.Tasks {
		byID[iss.ID] = iss.Priority
	}

	cases := map[string]string{"p4": "critical", "p3": "high", "p0": "medium"}
	for id, want := range cases {
		if got := byID[id]; got != want {
			t.Errorf("priority for %s: got %q, want %q", id, got, want)
		}
	}
}

func TestGetTask_ReturnsIssue(t *testing.T) {
	deps := newTestDeps(t)
	db := deps.db

	insertTestTask(t, db, "gi1", "ws-get", "Get Issue", "done", 2)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/tasks/gi1", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp dashboard.TaskResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Task == nil {
		t.Fatal("expected issue in response, got nil")
	}
	if resp.Task.ID != "gi1" {
		t.Errorf("expected id gi1, got %q", resp.Task.ID)
	}
	if resp.Task.Priority != "medium" {
		t.Errorf("expected priority 'medium' (2), got %q", resp.Task.Priority)
	}
}

func TestGetTask_ReturnsChildBlockers(t *testing.T) {
	deps := newTestDeps(t)
	db := deps.db
	ctx := context.Background()

	insertTestTask(t, db, "parent", "ws-chain", "Parent", "todo", 2)
	insertTestChildTask(t, db, "child-1", "ws-chain", "parent", "Design", "done", 2)
	insertTestChildTask(t, db, "child-2", "ws-chain", "parent", "Build", "todo", 2)
	if err := deps.repo.CreateTaskBlocker(ctx, &models.TaskBlocker{
		TaskID:        "child-2",
		BlockerTaskID: "child-1",
	}); err != nil {
		t.Fatalf("CreateTaskBlocker: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/tasks/parent", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp dashboard.TaskResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Task.Children) != 2 {
		t.Fatalf("children = %d, want 2", len(resp.Task.Children))
	}
	blockedBy := map[string][]string{}
	for _, child := range resp.Task.Children {
		blockedBy[child.ID] = child.BlockedBy
	}
	if len(blockedBy["child-2"]) != 1 || blockedBy["child-2"][0] != "child-1" {
		t.Fatalf("child-2 blockers = %v, want [child-1]", blockedBy["child-2"])
	}
	if len(blockedBy["child-1"]) != 0 {
		t.Fatalf("child-1 blockers = %v, want none", blockedBy["child-1"])
	}
}

func TestGetTask_Returns404ForMissing(t *testing.T) {
	deps := newTestDeps(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/tasks/nope", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestListComments_ReturnsEmptyList(t *testing.T) {
	deps := newTestDeps(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/tasks/taskX/comments", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp dashboard.CommentListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Comments == nil {
		t.Error("expected non-nil comments slice")
	}
	if len(resp.Comments) != 0 {
		t.Errorf("expected 0 comments, got %d", len(resp.Comments))
	}
}

func TestCreateComment_CreatesAndReturnsComment(t *testing.T) {
	deps := newTestDeps(t)

	body := `{"body":"this is a comment"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/office/tasks/taskY/comments",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp dashboard.CommentResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Comment == nil {
		t.Fatal("expected comment in response")
	}
	if resp.Comment.Body != "this is a comment" {
		t.Errorf("expected body 'this is a comment', got %q", resp.Comment.Body)
	}
	if resp.Comment.TaskID != "taskY" {
		t.Errorf("expected task_id taskY, got %q", resp.Comment.TaskID)
	}
	if resp.Comment.ID == "" {
		t.Error("expected non-empty comment id")
	}
}

func TestCreateComment_RequiresBody(t *testing.T) {
	deps := newTestDeps(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/office/tasks/taskZ/comments",
		strings.NewReader(`{"body":""}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateWorkspaceSettings_ReturnsOK(t *testing.T) {
	deps := newTestDeps(t)

	body := `{"name":"updated-ws","description":"new desc"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/office/workspaces/ws1/settings",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ok, _ := resp["ok"].(bool); !ok {
		t.Errorf("expected ok:true in response, got %v", resp)
	}
}

// Verify the comment list is ordered (oldest first) after multiple creates.
func TestListComments_OrderedByCreatedAt(t *testing.T) {
	deps := newTestDeps(t)
	ctx := context.Background()

	for _, body := range []string{"first", "second", "third"} {
		cm := &models.TaskComment{
			TaskID:     "ordered-task",
			AuthorType: "user",
			AuthorID:   "u1",
			Body:       body,
			Source:     "user",
			CreatedAt:  time.Now(),
		}
		if err := deps.repo.CreateTaskComment(ctx, cm); err != nil {
			t.Fatalf("create comment: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/tasks/ordered-task/comments", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp dashboard.CommentListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Comments) != 3 {
		t.Fatalf("expected 3 comments, got %d", len(resp.Comments))
	}
	expected := []string{"first", "second", "third"}
	for i, c := range resp.Comments {
		if c.Body != expected[i] {
			t.Errorf("comment[%d].Body = %q, want %q", i, c.Body, expected[i])
		}
	}
}

// TestListComments_PopulatesRunState pins that the comments list
// response carries the {runId, runStatus, runError} fields for any
// comment whose `task_comment:<id>` idempotency_key matches a row in
// office_runs. Comments without a run row come back without those
// fields (omitempty keeps the JSON clean), and run state never bleeds
// across comments.
func TestListComments_PopulatesRunState(t *testing.T) {
	deps := newTestDeps(t)
	ctx := context.Background()

	// Two user comments on the same task. The first triggered a
	// run that finished cleanly; the second triggered a run that
	// failed with a message; the third has no run.
	for _, body := range []string{"first", "second", "third"} {
		cm := &models.TaskComment{
			ID:         "cm-" + body,
			TaskID:     "rs-task",
			AuthorType: "user",
			AuthorID:   "user",
			Body:       body,
			Source:     "user",
			CreatedAt:  time.Now(),
		}
		if err := deps.repo.CreateTaskComment(ctx, cm); err != nil {
			t.Fatalf("create comment %s: %v", body, err)
		}
	}

	finishedRun := &models.Run{
		ID:             "run-finished",
		AgentProfileID: "a1",
		Reason:         "task_comment",
		Payload:        `{"task_id":"rs-task","comment_id":"cm-first"}`,
		Status:         "finished",
		CoalescedCount: 1,
		IdempotencyKey: ptr("task_comment:cm-first"),
	}
	if err := deps.repo.CreateRun(ctx, finishedRun); err != nil {
		t.Fatalf("create finished run: %v", err)
	}
	failedRun := &models.Run{
		ID:             "run-failed",
		AgentProfileID: "a1",
		Reason:         "task_comment",
		Payload:        `{"task_id":"rs-task","comment_id":"cm-second"}`,
		Status:         "failed",
		CoalescedCount: 1,
		IdempotencyKey: ptr("task_comment:cm-second"),
	}
	if err := deps.repo.CreateRun(ctx, failedRun); err != nil {
		t.Fatalf("create failed run: %v", err)
	}
	if err := deps.repo.SetRunErrorMessageForTest(ctx, failedRun.ID, "kaboom"); err != nil {
		t.Fatalf("set error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/tasks/rs-task/comments", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var resp dashboard.CommentListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Comments) != 3 {
		t.Fatalf("expected 3 comments, got %d", len(resp.Comments))
	}
	byID := map[string]*dashboard.CommentDTO{}
	for _, c := range resp.Comments {
		byID[c.ID] = c
	}
	if got := byID["cm-first"]; got == nil || got.RunID != "run-finished" || got.RunStatus != "finished" {
		t.Errorf("cm-first = %+v, want run-finished/finished", got)
	}
	if got := byID["cm-first"]; got != nil && got.RunError != "" {
		t.Errorf("cm-first.RunError = %q, want empty", got.RunError)
	}
	if got := byID["cm-second"]; got == nil || got.RunID != "run-failed" || got.RunStatus != "failed" || got.RunError != "kaboom" {
		t.Errorf("cm-second = %+v, want run-failed/failed/kaboom", got)
	}
	if got := byID["cm-third"]; got == nil || got.RunID != "" || got.RunStatus != "" {
		t.Errorf("cm-third should have no run fields, got %+v", got)
	}
}

func ptr(s string) *string { return &s }

// insertTestLabel inserts a label and attaches it to a task.
func insertTestLabel(t *testing.T, db *sqlx.DB, labelID, wsID, name, color, taskID string) {
	t.Helper()
	_, err := db.Exec(`
		INSERT OR IGNORE INTO office_labels (id, workspace_id, name, color, created_at, updated_at)
		VALUES (?, ?, ?, ?, datetime('now'), datetime('now'))
	`, labelID, wsID, name, color)
	if err != nil {
		t.Fatalf("insert label %s: %v", name, err)
	}
	_, err = db.Exec(`
		INSERT OR IGNORE INTO office_task_labels (task_id, label_id, created_at)
		VALUES (?, ?, datetime('now'))
	`, taskID, labelID)
	if err != nil {
		t.Fatalf("attach label %s to task %s: %v", name, taskID, err)
	}
}

func TestListTasks_LabelsFromJunctionTable(t *testing.T) {
	deps := newTestDeps(t)
	db := deps.db

	insertTestTask(t, db, "lbl-task", "ws-lbl", "Label Task", "todo", 0)
	insertTestLabel(t, db, "lbl-1", "ws-lbl", "bug", "#ef4444", "lbl-task")
	insertTestLabel(t, db, "lbl-2", "ws-lbl", "feature", "#3b82f6", "lbl-task")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/ws-lbl/tasks", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp dashboard.TaskListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Tasks) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(resp.Tasks))
	}

	issue := resp.Tasks[0]
	if len(issue.Labels) != 2 {
		t.Fatalf("expected 2 labels, got %d: %v", len(issue.Labels), issue.Labels)
	}

	labelNames := map[string]string{}
	for _, l := range issue.Labels {
		labelNames[l.Name] = l.Color
	}
	if labelNames["bug"] != "#ef4444" {
		t.Errorf("expected bug label color #ef4444, got %q", labelNames["bug"])
	}
	if labelNames["feature"] != "#3b82f6" {
		t.Errorf("expected feature label color #3b82f6, got %q", labelNames["feature"])
	}
}

func TestGetTask_LabelsFromJunctionTable(t *testing.T) {
	deps := newTestDeps(t)
	db := deps.db

	insertTestTask(t, db, "single-lbl", "ws-single", "Single Label Task", "todo", 0)
	insertTestLabel(t, db, "lbl-s1", "ws-single", "urgent", "#f59e0b", "single-lbl")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/tasks/single-lbl", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp dashboard.TaskResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Task == nil {
		t.Fatal("expected issue in response, got nil")
	}
	if len(resp.Task.Labels) != 1 {
		t.Fatalf("expected 1 label, got %d: %v", len(resp.Task.Labels), resp.Task.Labels)
	}
	if resp.Task.Labels[0].Name != "urgent" {
		t.Errorf("expected label name 'urgent', got %q", resp.Task.Labels[0].Name)
	}
	if resp.Task.Labels[0].Color != "#f59e0b" {
		t.Errorf("expected label color '#f59e0b', got %q", resp.Task.Labels[0].Color)
	}
}

func TestListTasks_EmptyLabelsWhenNoneAttached(t *testing.T) {
	deps := newTestDeps(t)
	db := deps.db

	insertTestTask(t, db, "no-lbl", "ws-nolbl", "No Label Task", "todo", 0)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/ws-nolbl/tasks", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp dashboard.TaskListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Tasks) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(resp.Tasks))
	}
	if resp.Tasks[0].Labels == nil {
		t.Error("expected non-nil labels slice, got nil")
	}
	if len(resp.Tasks[0].Labels) != 0 {
		t.Errorf("expected 0 labels, got %d", len(resp.Tasks[0].Labels))
	}
}

// -- Permission request inbox tests --

// -- Workspace settings tests --

func TestGetWorkspaceSettings_ReturnsDefaultsWhenNoProvider(t *testing.T) {
	deps := newTestDeps(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/ws1/settings", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp dashboard.WorkspaceSettingsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Settings == nil {
		t.Fatal("expected non-nil settings")
	}
	if resp.Settings.PermissionHandlingMode != "human" {
		t.Errorf("expected default mode 'human', got %q", resp.Settings.PermissionHandlingMode)
	}
}

func TestGetWorkspaceSettings_UsesProvider(t *testing.T) {
	deps := newTestDeps(t)
	provider := &stubSettingsProvider{
		settings: &dashboard.WorkspaceSettings{
			Name:                   "default",
			PermissionHandlingMode: "auto_approve",
		},
	}
	deps.svc.SetSettingsProvider(provider)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/ws1/settings", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp dashboard.WorkspaceSettingsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Settings == nil {
		t.Fatal("expected non-nil settings")
	}
	if resp.Settings.PermissionHandlingMode != "auto_approve" {
		t.Errorf("expected 'auto_approve', got %q", resp.Settings.PermissionHandlingMode)
	}
}

func TestUpdateWorkspaceSettings_UpdatesPermissionHandlingMode(t *testing.T) {
	deps := newTestDeps(t)
	provider := &stubSettingsProvider{}
	deps.svc.SetSettingsProvider(provider)

	body := `{"permission_handling_mode":"auto_approve"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/office/workspaces/ws1/settings",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if provider.lastMode != "auto_approve" {
		t.Errorf("expected mode 'auto_approve' to be persisted, got %q", provider.lastMode)
	}
}

func TestUpdateWorkspaceSettings_RejectsInvalidMode(t *testing.T) {
	deps := newTestDeps(t)
	provider := &stubSettingsProvider{}
	deps.svc.SetSettingsProvider(provider)

	body := `{"permission_handling_mode":"invalid_mode"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/office/workspaces/ws1/settings",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid mode, got %d: %s", w.Code, w.Body.String())
	}
}

// -- Timeline tests --

func TestGetTask_ReturnsTimelineField(t *testing.T) {
	deps := newTestDeps(t)
	db := deps.db

	insertTestTask(t, db, "timeline-task", "ws-tl", "Timeline Task", "todo", 0)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/tasks/timeline-task", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp dashboard.TaskResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Task == nil {
		t.Fatal("expected issue in response")
	}
	// Timeline must always be present (never nil) even when no events exist.
	if resp.Timeline == nil {
		t.Error("expected non-nil timeline field in issue response")
	}
}
