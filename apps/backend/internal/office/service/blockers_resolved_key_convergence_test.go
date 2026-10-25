package service

import (
	"context"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/runs/dedupkeys"
	"github.com/kandev/kandev/internal/workflow/engine"
)

// newBlockersConvergenceTestRepo builds a real office repository (real
// migrations, so task_blockers is the production schema) plus the minimal
// hand-rolled tasks table resolveAndWakeIfUnblocked reads state from —
// tasks is owned by the task package's schema, which office's migrations
// only reference by foreign key.
func newBlockersConvergenceTestRepo(t *testing.T) *sqlite.Repository {
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
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	ctx := context.Background()
	if _, err := repo.ExecRaw(ctx, `
		CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			workspace_id TEXT DEFAULT '',
			state TEXT DEFAULT ''
		)
	`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	return repo
}

// convergenceDispatcher records every HandleTrigger call so the test can
// assert the exact operation id resolveAndWakeIfUnblocked derived.
type convergenceDispatcher struct {
	mu    sync.Mutex
	calls []convergenceCall
}

type convergenceCall struct {
	taskID  string
	trigger engine.Trigger
	payload any
	opID    string
}

func (d *convergenceDispatcher) HandleTrigger(
	_ context.Context, taskID string, trigger engine.Trigger, payload any, opID string,
) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = append(d.calls, convergenceCall{taskID, trigger, payload, opID})
	return nil
}

func newConvergenceTestService(t *testing.T, repo *sqlite.Repository, dispatcher *convergenceDispatcher) *Service {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	s := &Service{repo: repo, logger: log}
	s.SetWorkflowEngineDispatcher(dispatcher)
	return s
}

// TestResolveAndWakeIfUnblocked_DerivesSameDigestAsCascadeBlockersResolved
// is the AC-OFFICE-RUN-DEDUP-002.1 convergence test for the second blocker
// producer. office/scheduler.cascadeBlockersResolved (the other half of the
// same AC) already has a regression test
// (reactivity_blockers_resolved_test.go), but that test never exercises
// this package's resolveAndWakeIfUnblocked — it only proves
// dedupkeys.BlockerDigest is order-independent in isolation. Review round 2
// found the AC's own explicit test requirement ("both producers derive a
// byte-identical digest, driven through the shared builder from both
// packages") unproven by any executing test. This test drives
// resolveAndWakeIfUnblocked directly and asserts its persisted operation id
// carries the exact digest office/scheduler's producer would derive for the
// identical blocker set.
func TestResolveAndWakeIfUnblocked_DerivesSameDigestAsCascadeBlockersResolved(t *testing.T) {
	repo := newBlockersConvergenceTestRepo(t)
	ctx := context.Background()

	for _, row := range []struct{ id, state string }{
		{"blocked-1", "IN_PROGRESS"},
		{"blocker-conv-a", "COMPLETED"},
		{"blocker-conv-b", "COMPLETED"},
	} {
		if _, err := repo.ExecRaw(ctx,
			`INSERT INTO tasks (id, workspace_id, state) VALUES (?, 'ws-1', ?)`,
			row.id, row.state,
		); err != nil {
			t.Fatalf("insert task %s: %v", row.id, err)
		}
	}
	for _, blockerID := range []string{"blocker-conv-a", "blocker-conv-b"} {
		if err := repo.CreateTaskBlocker(ctx, &models.TaskBlocker{
			TaskID: "blocked-1", BlockerTaskID: blockerID,
		}); err != nil {
			t.Fatalf("create blocker relationship for %s: %v", blockerID, err)
		}
	}

	dispatcher := &convergenceDispatcher{}
	s := newConvergenceTestService(t, repo, dispatcher)

	if err := s.resolveAndWakeIfUnblocked(ctx, "blocked-1", "blocker-conv-b"); err != nil {
		t.Fatalf("resolveAndWakeIfUnblocked: %v", err)
	}

	dispatcher.mu.Lock()
	calls := append([]convergenceCall{}, dispatcher.calls...)
	dispatcher.mu.Unlock()
	if len(calls) != 1 {
		t.Fatalf("HandleTrigger calls = %d, want 1: %#v", len(calls), calls)
	}
	call := calls[0]
	if call.taskID != "blocked-1" {
		t.Fatalf("taskID = %q, want blocked-1", call.taskID)
	}
	if call.trigger != engine.TriggerOnBlockerResolved {
		t.Fatalf("trigger = %q, want %q", call.trigger, engine.TriggerOnBlockerResolved)
	}

	// The digest office/scheduler.cascadeBlockersResolved would derive for
	// the identical blocker set — proving the two independently-implemented
	// producers converge on the same occurrence identity, per AC-002.1.
	digest := dedupkeys.BlockerDigest([]string{"blocker-conv-a", "blocker-conv-b"})
	want := "blockers_resolved:blocked-1:" + digest
	if call.opID != want {
		t.Fatalf("operation id = %q, want %q", call.opID, want)
	}
}

// TestResolveAndWakeIfUnblocked_StillBlocked_DoesNotDispatch proves the
// negative: a task with another outstanding blocker must not wake.
func TestResolveAndWakeIfUnblocked_StillBlocked_DoesNotDispatch(t *testing.T) {
	repo := newBlockersConvergenceTestRepo(t)
	ctx := context.Background()

	for _, row := range []struct{ id, state string }{
		{"blocked-2", "IN_PROGRESS"},
		{"blocker-conv-c", "IN_PROGRESS"}, // never finishes
		{"blocker-conv-d", "COMPLETED"},
	} {
		if _, err := repo.ExecRaw(ctx,
			`INSERT INTO tasks (id, workspace_id, state) VALUES (?, 'ws-1', ?)`,
			row.id, row.state,
		); err != nil {
			t.Fatalf("insert task %s: %v", row.id, err)
		}
	}
	for _, blockerID := range []string{"blocker-conv-c", "blocker-conv-d"} {
		if err := repo.CreateTaskBlocker(ctx, &models.TaskBlocker{
			TaskID: "blocked-2", BlockerTaskID: blockerID,
		}); err != nil {
			t.Fatalf("create blocker relationship for %s: %v", blockerID, err)
		}
	}

	dispatcher := &convergenceDispatcher{}
	s := newConvergenceTestService(t, repo, dispatcher)

	if err := s.resolveAndWakeIfUnblocked(ctx, "blocked-2", "blocker-conv-d"); err != nil {
		t.Fatalf("resolveAndWakeIfUnblocked: %v", err)
	}

	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	if len(dispatcher.calls) != 0 {
		t.Fatalf("expected no HandleTrigger calls while blocker-conv-c is still outstanding, got %#v", dispatcher.calls)
	}
}
