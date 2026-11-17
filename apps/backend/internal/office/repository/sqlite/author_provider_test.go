package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/routing"
	runsmodels "github.com/kandev/kandev/internal/runs/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

func newAuthorProviderRepo(t *testing.T, db *sqlx.DB) *sqlite.Repository {
	t.Helper()
	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store: %v", err)
	}
	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}
	return repo
}

func TestAuthorProviderQueries(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	repo := newAuthorProviderRepo(t, db)
	exerciseAuthorProviderQueries(t, repo)
	exerciseExclusionDeclarations(t, repo)
}

// TestPostgresAuthorProviderQueries is the PostgreSQL twin: the author lookup
// reads task_id and workflow_step_id out of the runs payload with
// dialect.JSONExtract. Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresAuthorProviderQueries(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo := newAuthorProviderRepo(t, db)
	exerciseAuthorProviderQueries(t, repo)
	exerciseExclusionDeclarations(t, repo)
}

func exerciseAuthorProviderQueries(t *testing.T, repo *sqlite.Repository) {
	t.Helper()
	ctx := context.Background()
	for _, step := range []struct{ id, stage, events string }{
		{"work", "work", `{}`},
		{"review", "review", `{"on_enter":[{"type":"ensure_participant_seat","config":{"role":"reviewer","exclude_author_provider":true}}]}`},
	} {
		if _, err := repo.ExecRaw(ctx,
			`INSERT INTO workflow_steps (id, workflow_id, name, position, stage_type, events) VALUES (?, 'wf-1', ?, 0, ?, ?)`,
			step.id, step.id, step.stage, step.events); err != nil {
			t.Fatalf("insert step %s: %v", step.id, err)
		}
	}

	t.Run("step events decode", func(t *testing.T) {
		events, err := repo.GetWorkflowStepEvents(ctx, "review")
		if err != nil {
			t.Fatalf("GetWorkflowStepEvents: %v", err)
		}
		if len(events.OnEnter) != 1 || events.OnEnter[0].Config["exclude_author_provider"] != true {
			t.Fatalf("events = %+v", events)
		}
		missing, err := repo.GetWorkflowStepEvents(ctx, "absent")
		if err != nil || len(missing.OnEnter) != 0 {
			t.Fatalf("missing step = %+v, %v; want empty, nil", missing, err)
		}
	})

	t.Run("no author recorded", func(t *testing.T) {
		got, err := repo.GetTaskAuthorProvider(ctx, "task-1")
		if err != nil || got != "" {
			t.Fatalf("GetTaskAuthorProvider = %q, %v; want empty", got, err)
		}
	})

	base := time.Now().UTC().Add(-time.Hour)
	seedAuthorRun(t, repo, "run-work-a", `{"task_id":"task-1","workflow_step_id":"work"}`, "claude-acp", base)
	seedAuthorRun(t, repo, "run-review", `{"task_id":"task-1","workflow_step_id":"review"}`, "codex-acp", base.Add(10*time.Minute))
	seedAuthorRun(t, repo, "run-other-task", `{"task_id":"task-2","workflow_step_id":"work"}`, "opencode-acp", base.Add(20*time.Minute))
	seedAuthorRun(t, repo, "run-unlaunched", `{"task_id":"task-1","workflow_step_id":"work"}`, "", base.Add(30*time.Minute))

	t.Run("latest launched work-stage run", func(t *testing.T) {
		got, err := repo.GetTaskAuthorProvider(ctx, "task-1")
		if err != nil || got != "claude-acp" {
			t.Fatalf("GetTaskAuthorProvider = %q, %v; want claude-acp", got, err)
		}
	})

	t.Run("revisit uses the most recent work run", func(t *testing.T) {
		seedAuthorRun(t, repo, "run-work-b", `{"task_id":"task-1","workflow_step_id":"work"}`, "codex-acp", base.Add(40*time.Minute))
		got, err := repo.GetTaskAuthorProvider(ctx, "task-1")
		if err != nil || got != "codex-acp" {
			t.Fatalf("GetTaskAuthorProvider = %q, %v; want codex-acp", got, err)
		}
	})

	t.Run("queue-time step recognizes reactivity work runs", func(t *testing.T) {
		seedAuthorRun(t, repo, "run-work-queued", `{"task_id":"task-1","queued_workflow_step_id":"work"}`, "opencode-acp", base.Add(50*time.Minute))
		seedAuthorRun(t, repo, "run-review-queued", `{"task_id":"task-1","queued_workflow_step_id":"review"}`, "claude-acp", base.Add(55*time.Minute))
		got, err := repo.GetTaskAuthorProvider(ctx, "task-1")
		if err != nil || got != "opencode-acp" {
			t.Fatalf("GetTaskAuthorProvider = %q, %v; want opencode-acp", got, err)
		}
	})
}

func seedAuthorRun(t *testing.T, repo *sqlite.Repository, id, payload, provider string, claimedAt time.Time) {
	t.Helper()
	ctx := context.Background()
	run := &runsmodels.Run{
		ID: id, AgentProfileID: "agent-" + id, Reason: "task_assigned", Payload: payload,
		Status: "finished", CoalescedCount: 1, RequestedAt: claimedAt,
	}
	if err := repo.CreateRun(ctx, run); err != nil {
		t.Fatalf("create run %s: %v", id, err)
	}
	if _, err := repo.ExecRaw(ctx, `UPDATE runs SET claimed_at = ? WHERE id = ?`, claimedAt, id); err != nil {
		t.Fatalf("set claimed_at %s: %v", id, err)
	}
	if provider != "" {
		if err := repo.SetRunResolvedRoute(ctx, id, provider+"-profile", provider, provider+"-model"); err != nil {
			t.Fatalf("set resolved route %s: %v", id, err)
		}
	}
}

func exerciseExclusionDeclarations(t *testing.T, repo *sqlite.Repository) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	for _, wf := range []struct{ id, ws, name string }{
		{"wf-1", "ws-a", "Office Review"},
		{"wf-other", "ws-b", "Other Workspace"},
	} {
		if _, err := repo.ExecRaw(ctx,
			`INSERT INTO workflows (id, workspace_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			wf.id, wf.ws, wf.name, now, now); err != nil {
			t.Fatalf("insert workflow %s: %v", wf.id, err)
		}
	}
	if _, err := repo.ExecRaw(ctx,
		`INSERT INTO workflow_steps (id, workflow_id, name, position, stage_type, events) VALUES ('other-review', 'wf-other', 'Review', 0, 'review', ?)`,
		`{"on_enter":[{"type":"ensure_participant_seat","config":{"role":"reviewer","exclude_author_provider":true}}]}`); err != nil {
		t.Fatalf("insert other step: %v", err)
	}

	decls, err := repo.ListAuthorExclusionDeclarations(ctx, "ws-a")
	if err != nil {
		t.Fatalf("ListAuthorExclusionDeclarations: %v", err)
	}
	want := routing.ExclusionDeclaration{
		WorkflowID: "wf-1", WorkflowName: "Office Review", StepID: "review", StepName: "review", Role: "reviewer",
	}
	if len(decls) != 1 || decls[0] != want {
		t.Fatalf("declarations = %+v, want [%+v]", decls, want)
	}
	if none, err := repo.ListAuthorExclusionDeclarations(ctx, "ws-empty"); err != nil || len(none) != 0 {
		t.Fatalf("empty workspace declarations = %+v, %v", none, err)
	}
}
