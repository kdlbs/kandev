package reachability

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
)

func newStoreTestRepo(t *testing.T) *sqlite.Repository {
	t.Helper()
	path := filepath.Join(t.TempDir(), "reachability-store.db")
	dbConn, err := db.OpenSQLite(path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repo, err := sqlite.NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		_ = sqlxDB.Close()
		t.Fatalf("new repository: %v", err)
	}
	t.Cleanup(func() { _ = sqlxDB.Close() })
	return repo
}

func newStoreTestExecutor(t *testing.T, repo *sqlite.Repository) *models.Executor {
	t.Helper()
	executor := &models.Executor{
		ID:     "exec-" + t.Name(),
		Name:   "ssh-host",
		Type:   models.ExecutorTypeSSH,
		Status: models.ExecutorStatusActive,
		Config: map[string]string{"ssh_host": "10.0.0.1"},
	}
	if err := repo.CreateExecutor(context.Background(), executor); err != nil {
		t.Fatalf("create executor: %v", err)
	}
	return executor
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-001.9, AC-EXECUTORS-SSH-REACHABILITY-001.10
//
// Drives a success/fail/fail/success sequence and asserts the exact probe
// index on which each direction flips. A naive flip-on-first-result
// implementation would report unreachable after the very first failure
// (index 2) instead of the second (index 3), and would report a below-
// threshold failure as a state change even though the state stands still.
func TestStoreObserveHysteresisSequence(t *testing.T) {
	repo := newStoreTestRepo(t)
	executor := newStoreTestExecutor(t, repo)
	s := &store{repo: repo, log: logger.Default()}
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	type step struct {
		label        string
		outcome      lifecycle.SSHProbeOutcome
		wantState    models.ExecutorReachabilityState
		wantChanged  bool
		wantFailures int
	}
	steps := []step{
		{
			label:        "1: first success establishes reachable",
			outcome:      lifecycle.SSHProbeOutcome{Success: true, Host: "10.0.0.1"},
			wantState:    models.ExecutorReachabilityStateReachable,
			wantChanged:  true,
			wantFailures: 0,
		},
		{
			label: "2: below-threshold failure leaves state reachable",
			outcome: lifecycle.SSHProbeOutcome{
				Host: "10.0.0.1", Reason: lifecycle.SSHReachabilityReasonNetwork, Message: "connection refused",
			},
			wantState:    models.ExecutorReachabilityStateReachable,
			wantChanged:  false,
			wantFailures: 1,
		},
		{
			label: "3: failure reaching failureThreshold flips to unreachable",
			outcome: lifecycle.SSHProbeOutcome{
				Host: "10.0.0.1", Reason: lifecycle.SSHReachabilityReasonNetwork, Message: "connection refused",
			},
			wantState:    models.ExecutorReachabilityStateUnreachable,
			wantChanged:  true,
			wantFailures: 2,
		},
		{
			label:        "4: a single success flips back to reachable with a zero counter",
			outcome:      lifecycle.SSHProbeOutcome{Success: true, Host: "10.0.0.1"},
			wantState:    models.ExecutorReachabilityStateReachable,
			wantChanged:  true,
			wantFailures: 0,
		},
	}

	for i, st := range steps {
		checkedAt := base.Add(time.Duration(i) * time.Second)
		executor.UpdatedAt = mustGetUpdatedAt(t, repo, executor.ID)
		result := s.Observe(ctx, executor, st.outcome, checkedAt)
		if !result.Applied {
			t.Fatalf("%s: write was refused, want applied", st.label)
		}
		if result.Current != st.wantState {
			t.Fatalf("%s: state = %q, want %q", st.label, result.Current, st.wantState)
		}
		if result.StateChanged != st.wantChanged {
			t.Fatalf("%s: changed = %v, want %v", st.label, result.StateChanged, st.wantChanged)
		}
		got, err := repo.GetExecutorReachability(ctx, executor.ID)
		if err != nil {
			t.Fatalf("%s: get: %v", st.label, err)
		}
		if got.ConsecutiveFailures != st.wantFailures {
			t.Fatalf("%s: consecutive_failures = %d, want %d", st.label, got.ConsecutiveFailures, st.wantFailures)
		}
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-001.9
//
// A single config or host_key failure writes unreachable on that first
// probe, with the counter still below failureThreshold — sticky reasons
// bypass the threshold entirely.
func TestStoreObserveStickyFailureIsUnreachableOnFirstProbe(t *testing.T) {
	tests := []struct {
		name   string
		reason lifecycle.SSHReachabilityReason
	}{
		{"config", lifecycle.SSHReachabilityReasonConfig},
		{"host_key", lifecycle.SSHReachabilityReasonHostKey},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newStoreTestRepo(t)
			executor := newStoreTestExecutor(t, repo)
			s := &store{repo: repo, log: logger.Default()}
			ctx := context.Background()

			result := s.Observe(ctx, executor, lifecycle.SSHProbeOutcome{
				Host: "10.0.0.1", Reason: tt.reason, Message: "boom",
			}, time.Now().UTC())

			if !result.Applied || result.Current != models.ExecutorReachabilityStateUnreachable {
				t.Fatalf("result = %+v, want applied unreachable on the first probe", result)
			}
			got, err := repo.GetExecutorReachability(ctx, executor.ID)
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			if got.ConsecutiveFailures >= failureThreshold {
				t.Fatalf("consecutive_failures = %d, want below failureThreshold (%d) — sticky must not need it", got.ConsecutiveFailures, failureThreshold)
			}
		})
	}
}

func mustGetUpdatedAt(t *testing.T, repo *sqlite.Repository, id string) time.Time {
	t.Helper()
	executor, err := repo.GetExecutor(context.Background(), id)
	if err != nil {
		t.Fatalf("get executor: %v", err)
	}
	return executor.UpdatedAt
}
