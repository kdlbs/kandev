package sqlite_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/routing"
	"github.com/kandev/kandev/internal/testutil"
)

func TestListSeatSteps(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	exerciseSeatSteps(t, newAuthorProviderRepo(t, db))
}

// TestPostgresListSeatSteps is the PostgreSQL twin. Skips unless
// KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresListSeatSteps(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	exerciseSeatSteps(t, newAuthorProviderRepo(t, db))
}

func exerciseSeatSteps(t *testing.T, repo *sqlite.Repository) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	for _, wf := range []struct {
		id, ws, name string
		hidden       int // workflows.hidden is an integer column on both databases.
	}{
		{"wf-office", "ws-a", "Office Default", 1},
		{"wf-custom", "ws-a", "Custom", 0},
		{"wf-other", "ws-b", "Elsewhere", 0},
	} {
		if _, err := repo.ExecRaw(ctx,
			`INSERT INTO workflows (id, workspace_id, name, hidden, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			wf.id, wf.ws, wf.name, wf.hidden, now, now); err != nil {
			t.Fatalf("insert workflow %s: %v", wf.id, err)
		}
	}
	seat := func(role string, exclude bool) string {
		if exclude {
			return `{"type":"ensure_participant_seat","config":{"role":"` + role + `","exclude_author_provider":true}}`
		}
		return `{"type":"ensure_participant_seat","config":{"role":"` + role + `"}}`
	}
	for _, step := range []struct {
		id, wf, name string
		position     int
		events       string
	}{
		{"office-work", "wf-office", "Work", 0, `{"on_enter":[{"type":"auto_start_agent"}]}`},
		{"office-approval", "wf-office", "Approval", 2, `{"on_enter":[` + seat("approver", false) + `]}`},
		{"office-review", "wf-office", "Review", 1, `{"on_enter":[{"type":"clear_decisions"},` + seat("reviewer", true) + `]}`},
		{"custom-review", "wf-custom", "Check", 0, `{"on_enter":[` + seat("reviewer", false) + `]}`},
		{"other-review", "wf-other", "Review", 0, `{"on_enter":[` + seat("reviewer", true) + `]}`},
	} {
		if _, err := repo.ExecRaw(ctx,
			`INSERT INTO workflow_steps (id, workflow_id, name, position, stage_type, events) VALUES (?, ?, ?, ?, 'review', ?)`,
			step.id, step.wf, step.name, step.position, step.events); err != nil {
			t.Fatalf("insert step %s: %v", step.id, err)
		}
	}

	got, err := repo.ListSeatSteps(ctx, "ws-a")
	if err != nil {
		t.Fatalf("ListSeatSteps: %v", err)
	}
	want := []routing.SeatStep{
		{WorkflowID: "wf-custom", WorkflowName: "Custom", StepID: "custom-review", StepName: "Check", Role: "reviewer"},
		{WorkflowID: "wf-office", WorkflowName: "Office Default", StepID: "office-review", StepName: "Review", Role: "reviewer", ExcludeAuthorProvider: true},
		{WorkflowID: "wf-office", WorkflowName: "Office Default", StepID: "office-approval", StepName: "Approval", Role: "approver"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("seat steps = %+v\nwant %+v", got, want)
	}
	if none, err := repo.ListSeatSteps(ctx, "ws-empty"); err != nil || len(none) != 0 {
		t.Fatalf("empty workspace seat steps = %+v, %v", none, err)
	}
}
