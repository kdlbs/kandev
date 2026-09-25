package scheduler

import (
	"context"
	"expvar"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/logger"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
)

// newSplitConnTestRepo builds an office repository backed by two distinct
// SQLite connections to the same file-backed database — a writer and a
// reader, as production wiring uses. Unlike newReactivityTestRepo (one
// shared in-memory connection for both), this lets a test sever the
// reader alone and force a genuine read error, while the writer keeps
// working. WAL mode is required for two live connections against one
// SQLite file.
func newSplitConnTestRepo(t *testing.T) (repo *officesqlite.Repository, reader *sqlx.DB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db") + "?_journal_mode=WAL"

	writer, err := sqlx.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	t.Cleanup(func() { _ = writer.Close() })

	reader, err = sqlx.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open reader: %v", err)
	}

	log := mustTestLogger(t)
	repo, err = officesqlite.NewWithDB(writer, reader, log)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	return repo, reader
}

func mustTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return log
}

// assignmentRateLimitCounterValue reads one key out of the
// office_assignment_rate_limit_total expvar map, mirroring
// internal/runs/service's own counterValue test helper (unexported there,
// so duplicated here rather than exported for one cross-package test).
func assignmentRateLimitCounterValue(t *testing.T, key string) int64 {
	t.Helper()
	v := expvar.Get("office_assignment_rate_limit_total")
	if v == nil {
		t.Fatalf("expvar map %q not registered", "office_assignment_rate_limit_total")
	}
	m, ok := v.(*expvar.Map)
	if !ok {
		t.Fatalf("expvar %q is not a *expvar.Map", "office_assignment_rate_limit_total")
	}
	var got int64
	m.Do(func(kv expvar.KeyValue) {
		if kv.Key != key {
			return
		}
		if iv, ok := kv.Value.(*expvar.Int); ok {
			got = iv.Value()
		}
	})
	return got
}

// TestCheckAssignmentWakeAllowance_RealCountReadFailureAdmits covers
// AC-OFFICE-ASSIGN-RATE-002.2 end to end through the production call
// path: with the reader connection actually severed (not a simulated
// error passed to the reporting helper directly), CountAgentInitiated
// AssignmentWakes returns a genuine error, and checkAssignmentWakeAllowance
// must still admit (return refused=false) and move the count_read_failed
// counter exactly once.
func TestCheckAssignmentWakeAllowance_RealCountReadFailureAdmits(t *testing.T) {
	repo, reader := newSplitConnTestRepo(t)
	ss := NewSchedulerService(repo, mustTestLogger(t), nil)
	ctx := context.Background()

	before := assignmentRateLimitCounterValue(t, "reason=count_read_failed")

	if err := reader.Close(); err != nil {
		t.Fatalf("close reader: %v", err)
	}

	refused := ss.checkAssignmentWakeAllowance(ctx, "agent-1", RunReasonTaskAssigned,
		`{"task_id":"read-failure-task","actor_type":"agent"}`)
	if refused {
		t.Fatal("a genuine count-read failure must fail open (admit), not refuse")
	}

	after := assignmentRateLimitCounterValue(t, "reason=count_read_failed")
	if after != before+1 {
		t.Fatalf("count_read_failed counter = %d, want %d", after, before+1)
	}
}
