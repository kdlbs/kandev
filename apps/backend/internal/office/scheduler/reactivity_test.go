package scheduler

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
)

// newReactivityTestRepo builds an in-memory office repository plus the
// minimal tasks/workflow_step_participants tables cascadeReviewRequested
// reads from. Those tables belong to the task/workflow schema, not
// office's own initSchema, so tests that exercise participant fan-out
// create them ad hoc — mirroring
// internal/office/repository/sqlite/participants_test.go.
func newReactivityTestRepo(t *testing.T) *officesqlite.Repository {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store: %v", err)
	}
	repo, err := officesqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	ctx := context.Background()
	if _, err := repo.ExecRaw(ctx, `
		CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			workspace_id TEXT DEFAULT '',
			workflow_step_id TEXT DEFAULT '',
			parent_id TEXT DEFAULT '',
			state TEXT DEFAULT '',
			assignee_user_id TEXT NOT NULL DEFAULT ''
		)
	`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `
		CREATE TABLE IF NOT EXISTS workflow_step_participants (
			id TEXT PRIMARY KEY,
			step_id TEXT NOT NULL,
			task_id TEXT DEFAULT '',
			role TEXT NOT NULL,
			agent_profile_id TEXT DEFAULT '',
			decision_required INTEGER DEFAULT 0,
			position INTEGER DEFAULT 0,
			created_at TIMESTAMP NOT NULL DEFAULT '1970-01-01 00:00:00'
		)
	`); err != nil {
		t.Fatalf("create workflow_step_participants table: %v", err)
	}
	return repo
}

func newReactivityTestScheduler(t *testing.T, repo *officesqlite.Repository) *SchedulerService {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return NewSchedulerService(repo, log, nil)
}

// TestCascadeReviewRequested_FiltersToReviewerAndApprover is the
// regression test for the review fan-out defect: cascadeReviewRequested
// must wake only reviewer and approver participants, per its own doc
// comment. Before the fix it iterated every participant filtered only on
// an empty agent ID, so watcher/collaborator/runner rows were woken too.
func TestCascadeReviewRequested_FiltersToReviewerAndApprover(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ctx := context.Background()

	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, workspace_id, workflow_step_id)
		VALUES ('task-1', 'ws-1', 'step-1')
	`); err != nil {
		t.Fatalf("insert task: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES
			('p-reviewer', 'step-1', 'task-1', 'reviewer', 'agent-reviewer', 1, 0),
			('p-approver', 'step-1', 'task-1', 'approver', 'agent-approver', 1, 1),
			('p-watcher', 'step-1', 'task-1', 'watcher', 'agent-watcher', 0, 2),
			('p-collaborator', 'step-1', 'task-1', 'collaborator', 'agent-collaborator', 0, 3),
			('p-runner', 'step-1', 'task-1', 'runner', 'agent-runner', 0, 4)
	`); err != nil {
		t.Fatalf("insert participants: %v", err)
	}

	ss := newReactivityTestScheduler(t, repo)
	task := &TaskSnapshot{ID: "task-1", WorkspaceID: "ws-1"}

	var woken []string
	queue := func(agentID string, c RunContext) {
		woken = append(woken, agentID)
	}

	ss.cascadeReviewRequested(ctx, task, TaskMutation{}, queue)

	want := map[string]bool{"agent-reviewer": true, "agent-approver": true}
	if len(woken) != len(want) {
		t.Fatalf("woken = %v, want exactly reviewer+approver", woken)
	}
	for _, agentID := range woken {
		if !want[agentID] {
			t.Fatalf("cascadeReviewRequested woke %q, want only reviewer/approver", agentID)
		}
	}
}

// recordedQueueCall captures one queue(agentID, RunContext) invocation from
// reactToComment for assertion.
type recordedQueueCall struct {
	agentID string
	ctx     RunContext
}

// TestReactToComment_SelfCommentDoesNotWakeAssignee is the card's required
// regression: a comment authored by the task's own assignee (author_type
// "agent", author_id == the assignee's agent profile id) must NOT queue a
// task_comment run for that assignee. This is the guard in reactToComment
// (reactivity.go); it only works when the comment's author_id was
// persisted as the acting agent's real profile id (dashboard's
// createComment handler is the thing that used to break this by hardcoding
// author_id="user").
func TestReactToComment_SelfCommentDoesNotWakeAssignee(t *testing.T) {
	ss := &SchedulerService{logger: logger.Default()}
	task := &TaskSnapshot{
		ID:                     "task-1",
		WorkspaceID:            "ws-1",
		State:                  "IN_PROGRESS",
		AssigneeAgentProfileID: "agent-a",
	}
	comment := &MutationComment{
		ID:         "comment-1",
		Body:       "Landscape scan complete, no action needed.",
		AuthorType: "agent",
		AuthorID:   "agent-a",
	}

	var calls []recordedQueueCall
	queue := func(agentID string, c RunContext) {
		calls = append(calls, recordedQueueCall{agentID: agentID, ctx: c})
	}

	ss.reactToComment(context.Background(), task, comment, queue)

	for _, c := range calls {
		if c.agentID == "agent-a" && c.ctx.Reason == RunReasonTaskComment {
			t.Fatalf("agent-a was woken by its own comment: %+v", c)
		}
	}
}

// TestReactToComment_OtherAuthorWakesAssignee is the positive counterpart:
// a comment from a different agent (or the user) on a task assigned to
// agent-b DOES queue a task_comment run for agent-b. Preserving this is
// explicitly required by the card — the fix must not suppress cross-agent
// wakes while closing the self-wake hole.
func TestReactToComment_OtherAuthorWakesAssignee(t *testing.T) {
	ss := &SchedulerService{logger: logger.Default()}
	task := &TaskSnapshot{
		ID:                     "task-2",
		WorkspaceID:            "ws-1",
		State:                  "IN_PROGRESS",
		AssigneeAgentProfileID: "agent-b",
	}
	comment := &MutationComment{
		ID:         "comment-2",
		Body:       "Please take a look at this.",
		AuthorType: "agent",
		AuthorID:   "agent-a",
	}

	var calls []recordedQueueCall
	queue := func(agentID string, c RunContext) {
		calls = append(calls, recordedQueueCall{agentID: agentID, ctx: c})
	}

	ss.reactToComment(context.Background(), task, comment, queue)

	found := false
	for _, c := range calls {
		if c.agentID == "agent-b" && c.ctx.Reason == RunReasonTaskComment {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected agent-b to be queued a task_comment run, got calls: %+v", calls)
	}
}
