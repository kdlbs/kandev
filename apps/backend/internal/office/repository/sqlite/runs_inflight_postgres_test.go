package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresHasInFlightRunForTask is the PostgreSQL twin of
// TestHasInFlightRunForTask. HasInFlightRunForTask reads task_id out of the
// runs payload, and a literal json_extract(...) there is a syntax error on
// Postgres (payload->>'task_id' is the Postgres form) — the same trap
// TestPostgresHasPriorTasklessFailedRun was written for. Running the predicate
// against a real Postgres backend makes a dialect regression fail loudly
// instead of only in production, where its failure mode is a detector that
// errors on every tick and surfaces nothing.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresHasInFlightRunForTask(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	ctx := context.Background()

	// runs is created by the task repository's schema init, mirroring
	// production boot order (see failure_postgres_test.go).
	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}

	now := time.Now().UTC()
	cases := []struct {
		status string
		want   bool
	}{
		{"queued", true},
		{"claimed", true},
		{"finished", false},
		{"failed", false},
		{"cancelled", false},
	}
	for _, tc := range cases {
		taskID := "pg-inflight-" + tc.status
		seedPostgresRun(t, ctx, repo, "pg-run-"+tc.status, "agent-a", tc.status, now,
			`{"task_id":"`+taskID+`"}`)

		got, err := repo.HasInFlightRunForTask(ctx, taskID)
		if err != nil {
			t.Fatalf("HasInFlightRunForTask(%s): %v", tc.status, err)
		}
		if got != tc.want {
			t.Errorf("HasInFlightRunForTask(status=%s) = %v, want %v", tc.status, got, tc.want)
		}
	}

	// Scoping: the in-flight rows above must not answer for a different task.
	got, err := repo.HasInFlightRunForTask(ctx, "pg-inflight-absent")
	if err != nil {
		t.Fatalf("HasInFlightRunForTask (absent task): %v", err)
	}
	if got {
		t.Error("got true, want false — no run carries this task_id")
	}
}
