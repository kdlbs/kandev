package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

func TestAppendAndListRouteAttempts_RoundTrip(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	seedRun(t, repo, "run-1", "agent-1", now)

	exitCode := 137
	finished := now.Add(time.Minute)
	first := &models.RouteAttempt{
		RunID:           "run-1",
		Seq:             1,
		ProviderID:      "claude-acp",
		Model:           "sonnet",
		Tier:            "balanced",
		Outcome:         "failed_provider_unavailable",
		ErrorCode:       "quota_limited",
		ErrorConfidence: "high",
		AdapterPhase:    "session_init",
		ClassifierRule:  "claude.stderr.quota.v1",
		ExitCode:        &exitCode,
		RawExcerpt:      "anthropic_quota_exceeded",
		StartedAt:       now,
		FinishedAt:      &finished,
	}
	if err := repo.AppendRouteAttempt(ctx, first); err != nil {
		t.Fatalf("append first: %v", err)
	}

	second := &models.RouteAttempt{
		RunID:      "run-1",
		Seq:        2,
		ProviderID: "codex-acp",
		Model:      "gpt-5.4",
		Tier:       "balanced",
		Outcome:    "launched",
		StartedAt:  now.Add(time.Minute),
	}
	if err := repo.AppendRouteAttempt(ctx, second); err != nil {
		t.Fatalf("append second: %v", err)
	}

	attempts, err := repo.ListRouteAttempts(ctx, "run-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(attempts) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(attempts))
	}
	if attempts[0].Seq != 1 || attempts[1].Seq != 2 {
		t.Errorf("attempts not ordered by seq: %v", attempts)
	}
	if attempts[0].ErrorCode != "quota_limited" {
		t.Errorf("error code lost: %q", attempts[0].ErrorCode)
	}
	if attempts[0].ExitCode == nil || *attempts[0].ExitCode != 137 {
		t.Errorf("exit code lost: %v", attempts[0].ExitCode)
	}
	if attempts[1].Outcome != "launched" {
		t.Errorf("outcome = %q", attempts[1].Outcome)
	}
}

func TestListRouteAttempts_EmptyReturnsEmptySlice(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	got, err := repo.ListRouteAttempts(ctx, "missing")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if got == nil {
		t.Fatal("expected empty slice, got nil")
	}
	if len(got) != 0 {
		t.Errorf("expected 0 attempts, got %d", len(got))
	}
}

func TestAppendRouteAttempt_DuplicateSeqRejected(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	seedRun(t, repo, "run-1", "agent-1", now)

	a := &models.RouteAttempt{
		RunID: "run-1", Seq: 1, ProviderID: "claude-acp", Model: "opus",
		Tier: "frontier", Outcome: "launched", StartedAt: now,
	}
	if err := repo.AppendRouteAttempt(ctx, a); err != nil {
		t.Fatalf("first append: %v", err)
	}
	if err := repo.AppendRouteAttempt(ctx, a); err == nil {
		t.Fatal("expected PK conflict on duplicate (run_id, seq)")
	}
}

func TestAppendRouteAttempt_FKCascadeOnRunDelete(t *testing.T) {
	// Foreign-key cascade is only enforced when PRAGMA foreign_keys=ON.
	// The shared in-memory helper does not set the pragma, so this test
	// builds its own DB with the pragma enabled — same pattern as
	// internal/workflow/repository/sqlite_test.go.
	repo, db := newRouteAttemptsRepoWithFK(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	seedRun(t, repo, "run-cascade", "agent-c", now)

	a := &models.RouteAttempt{
		RunID: "run-cascade", Seq: 1, ProviderID: "claude-acp",
		Model: "opus", Tier: "frontier", Outcome: "launched", StartedAt: now,
	}
	if err := repo.AppendRouteAttempt(ctx, a); err != nil {
		t.Fatalf("append: %v", err)
	}

	if _, err := db.Exec(`DELETE FROM runs WHERE id = ?`, "run-cascade"); err != nil {
		t.Fatalf("delete run: %v", err)
	}

	got, err := repo.ListRouteAttempts(ctx, "run-cascade")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected cascade to remove attempts, got %d", len(got))
	}
}

// newRouteAttemptsRepoWithFK builds an in-memory repo with PRAGMA
// foreign_keys=ON enabled, so the FK cascade on office_run_route_attempts
// is actually enforced. Mirrors the workflow store's test helper.
func newRouteAttemptsRepoWithFK(t *testing.T) (*sqlite.Repository, *sqlx.DB) {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatalf("pragma fk: %v", err)
	}
	if _, _, err := settingsstore.Provide(db, db); err != nil {
		t.Fatalf("settings store: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	return repo, db
}

// seedRun inserts a minimal runs row so AppendRouteAttempt's FK has
// a parent to reference. Mirrors workspace_deletion_test.go's helper.
func seedRun(t *testing.T, repo *sqlite.Repository, runID, agentID string, now time.Time) {
	t.Helper()
	if _, err := repo.ExecRaw(context.Background(),
		`INSERT INTO runs (id, agent_profile_id, reason, requested_at) VALUES (?, ?, 'test', ?)`,
		runID, agentID, now); err != nil {
		t.Fatalf("seed run: %v", err)
	}
}
