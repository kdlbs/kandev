package sqlite

// Internal (white-box) test file: the eval*Gate methods are unexported,
// so exercising their context-cancellation handling directly needs
// package-level access, unlike every other test in this directory
// (package sqlite_test).

import (
	"context"
	"expvar"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
)

// launchCheckFailedCount reads the current office_launch_check_failed_total
// value for gate, so a test can assert on the delta a single evaluation
// produces without depending on whether office_gate_failure_state's own
// write also happens to fail under the same canceled context (it does,
// which makes the persisted-row count alone an unreliable signal here).
func launchCheckFailedCount(gate string) int64 {
	v := shared.LaunchCheckFailedTotal.Get(shared.LaunchSafetyLabel("gate", gate))
	iv, ok := v.(*expvar.Int)
	if !ok {
		return 0
	}
	return iv.Value()
}

// gateFailureStateSchema is office_gate_failure_state's DDL, duplicated
// here for the same reason commitClaimTestSchema duplicates runs/
// office_launch_ledger: this package cannot import
// internal/office/repository/sqlite (import cycle).
const gateFailureStateSchema = `
CREATE TABLE office_gate_failure_state (
	workspace_id         TEXT      NOT NULL,
	gate                 TEXT      NOT NULL,
	consecutive_failures INTEGER   NOT NULL DEFAULT 0,
	last_escalation_at   TIMESTAMP,
	updated_at           TIMESTAMP NOT NULL,
	PRIMARY KEY (workspace_id, gate)
)`

// newShutdownContextTestRepo builds a runs repository over
// commitClaimTestSchema plus office_gate_failure_state, with a real
// agent_profiles table from settingsstore (the same table
// agentCeiling's query reads), and one seeded agent profile.
func newShutdownContextTestRepo(t *testing.T) *Repository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "shutdown-context.db")
	writerRaw, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	writer := sqlx.NewDb(writerRaw, "sqlite3")
	t.Cleanup(func() { _ = writer.Close() })
	if _, err := writer.Exec(commitClaimTestSchema); err != nil {
		t.Fatalf("create commit-claim schema: %v", err)
	}
	if _, err := writer.Exec(gateFailureStateSchema); err != nil {
		t.Fatalf("create gate-failure-state schema: %v", err)
	}
	if _, _, err := settingsstore.Provide(writer, writer, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}

	now := time.Now().UTC()
	if _, err := writer.Exec(
		`INSERT INTO agents (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		"test-agent", "test-agent", now, now,
	); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	if _, err := writer.Exec(
		`INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, created_at, updated_at, workspace_id, max_concurrent_sessions)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"agent-1", "test-agent", "agent-1", "agent-1", now, now, "ws-1", 5,
	); err != nil {
		t.Fatalf("seed agent profile: %v", err)
	}

	return NewWithDB(writer, writer)
}

// TestGateEvaluators_ContextCanceledDefersWithoutRecordingFailure pins
// AC-OFFICE-LAUNCH-SAFETY-001.8's last sentence: "A shutdown-cancelled
// evaluation shall defer the run without recording a gate failure, so a
// restart is not mistaken for a gate that is failing closed." Every one
// of the 5 deferral gates must, on a context-canceled read, report
// blocked (defer) without writing an office_gate_failure_state row.
func TestGateEvaluators_ContextCanceledDefersWithoutRecordingFailure(t *testing.T) {
	limits := ClaimSafetyLimits{
		MaxConcurrentWorkspace: 5,
		MaxConcurrentInstance:  5,
		RoutineBudgetPerHour:   5,
		WorkspaceBudgetPerHour: 5,
		PromotionAge:           time.Hour,
	}
	budgetWindowStart := time.Now().UTC().Add(-time.Hour)

	cases := []struct {
		name string
		gate string
		eval func(repo *Repository, ctx context.Context, tx *sqlx.Tx, candidate *models.Run) bool
	}{
		{"agent_ceiling", gateAgentCeiling, func(r *Repository, ctx context.Context, tx *sqlx.Tx, c *models.Run) bool {
			return r.evalAgentCeilingGate(ctx, tx, c, limits)
		}},
		{"workspace_ceiling", gateWorkspaceCeiling, func(r *Repository, ctx context.Context, tx *sqlx.Tx, c *models.Run) bool {
			return r.evalWorkspaceCeilingGate(ctx, tx, c, limits)
		}},
		{"instance_ceiling", gateInstanceCeiling, func(r *Repository, ctx context.Context, tx *sqlx.Tx, c *models.Run) bool {
			return r.evalInstanceCeilingGate(ctx, tx, c, limits)
		}},
		{"routine_budget", gateRoutineBudget, func(r *Repository, ctx context.Context, tx *sqlx.Tx, c *models.Run) bool {
			return r.evalRoutineBudgetGate(ctx, tx, c, limits, budgetWindowStart)
		}},
		{"workspace_budget", gateWorkspaceBudget, func(r *Repository, ctx context.Context, tx *sqlx.Tx, c *models.Run) bool {
			return r.evalWorkspaceBudgetGate(ctx, tx, c, limits, budgetWindowStart)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newShutdownContextTestRepo(t)
			ctx := context.Background()
			candidate := &models.Run{
				ID:             "candidate-" + tc.name,
				AgentProfileID: "agent-1",
				WorkspaceID:    "ws-1",
				RoutineID:      "routine-1",
				Reason:         "task_assigned",
				Status:         "queued",
			}

			tx, err := repo.db.BeginTxx(ctx, nil)
			if err != nil {
				t.Fatalf("begin tx: %v", err)
			}
			t.Cleanup(func() { _ = tx.Rollback() })

			canceledCtx, cancel := context.WithCancel(context.Background())
			cancel()

			before := launchCheckFailedCount(tc.gate)
			blocked := tc.eval(repo, canceledCtx, tx, candidate)
			if !blocked {
				t.Errorf("%s: blocked = false, want true (a context-canceled read must defer)", tc.name)
			}
			after := launchCheckFailedCount(tc.gate)
			if after != before {
				t.Errorf("%s: office_launch_check_failed_total went %d -> %d, want unchanged (a shutdown cancellation must not count as a gate failure)", tc.name, before, after)
			}
		})
	}
}
