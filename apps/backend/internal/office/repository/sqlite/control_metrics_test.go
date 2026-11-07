package sqlite_test

import (
	"context"
	"testing"
	"time"
)

// ensureControlMetricTables adds the session, queue and transition tables the
// control metrics read, with the is_primary flag the waiting figure filters on.
func ensureControlMetricTables(t *testing.T, ctx context.Context, exec func(context.Context, string, ...any) error) {
	t.Helper()
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS task_sessions (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			state TEXT NOT NULL DEFAULT 'CREATED',
			is_primary INTEGER NOT NULL DEFAULT 0,
			started_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS queued_messages (id TEXT PRIMARY KEY, task_id TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS task_step_transitions (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			to_workflow_step_id TEXT NOT NULL DEFAULT '',
			occurred_at TIMESTAMP NOT NULL
		)`,
		`ALTER TABLE workflow_steps ADD COLUMN complete_task_on_enter INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE tasks ADD COLUMN wip_admitted INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE tasks ADD COLUMN queued_for_step_id TEXT NOT NULL DEFAULT ''`,
	}
	for _, s := range stmts {
		if err := exec(ctx, s); err != nil {
			t.Fatalf("create control metric schema: %v", err)
		}
	}
}

func TestReadControlMetrics_WaitingInputCountsLiveTasksByPrimarySession(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()
	exec := func(ctx context.Context, q string, args ...any) error {
		_, err := repo.ExecRaw(ctx, q, args...)
		return err
	}
	ensureControlMetricTables(t, ctx, exec)
	if err := exec(ctx, `
		INSERT INTO tasks (id, workspace_id, title, state, is_ephemeral, archived_at) VALUES
			('t-live',     'ws-1', 'live',     'IN_PROGRESS', 0, NULL),
			('t-review',   'ws-1', 'review',   'REVIEW',      0, NULL),
			('t-running',  'ws-1', 'running',  'IN_PROGRESS', 0, NULL),
			('t-done',     'ws-1', 'done',     'COMPLETED',   0, NULL),
			('t-cancel',   'ws-1', 'cancel',   'CANCELLED',   0, NULL),
			('t-archived', 'ws-1', 'archived', 'IN_PROGRESS', 0, CURRENT_TIMESTAMP),
			('t-eph',      'ws-1', 'eph',      'IN_PROGRESS', 1, NULL),
			('t-other',    'ws-2', 'other',    'IN_PROGRESS', 0, NULL)
	`); err != nil {
		t.Fatalf("seed tasks: %v", err)
	}
	if err := exec(ctx, `
		INSERT INTO task_sessions (id, task_id, state, is_primary) VALUES
			('s-live',          't-live',     'WAITING_FOR_INPUT', 1),
			('s-live-extra',    't-live',     'WAITING_FOR_INPUT', 0),
			('s-review',        't-review',   'WAITING_FOR_INPUT', 1),
			('s-running',       't-running',  'RUNNING',           1),
			('s-running-extra', 't-running',  'WAITING_FOR_INPUT', 0),
			('s-done',          't-done',     'WAITING_FOR_INPUT', 1),
			('s-cancel',        't-cancel',   'WAITING_FOR_INPUT', 1),
			('s-archived',      't-archived', 'WAITING_FOR_INPUT', 1),
			('s-eph',           't-eph',      'WAITING_FOR_INPUT', 1),
			('s-other',         't-other',    'WAITING_FOR_INPUT', 1)
	`); err != nil {
		t.Fatalf("seed sessions: %v", err)
	}

	got, err := repo.ReadControlMetrics(ctx, "ws-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("ReadControlMetrics: %v", err)
	}
	// Only t-live and t-review: finished tasks, archived and ephemeral tasks,
	// other workspaces and non-primary sessions do not wait on a person.
	if got.WaitingInputTasks != 2 {
		t.Fatalf("WaitingInputTasks = %d, want 2", got.WaitingInputTasks)
	}
	// The per-state session map stays a raw recorded count over unarchived tasks.
	if got.Sessions["WAITING_FOR_INPUT"] != 6 {
		t.Fatalf("Sessions[WAITING_FOR_INPUT] = %d, want 6", got.Sessions["WAITING_FOR_INPUT"])
	}
}

func TestReadControlMetrics_CountsTasksQueuedAtAWipLimitedStep(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()
	exec := func(ctx context.Context, q string, args ...any) error {
		_, err := repo.ExecRaw(ctx, q, args...)
		return err
	}
	ensureControlMetricTables(t, ctx, exec)
	if err := exec(ctx, `
		INSERT INTO tasks (id, workspace_id, title, state, is_ephemeral, origin, metadata,
			archived_at, workflow_step_id, queued_for_step_id, wip_admitted) VALUES
			('t-wip-a',       'ws-1', 'a',      'IN_PROGRESS', 0, 'manual',        '{}',                     NULL,             'step-wip', 'step-wip', 0),
			('t-wip-b',       'ws-1', 'b',      'REVIEW',      0, 'manual',        '{}',                     NULL,             'step-wip', 'step-wip', 0),
			('t-admitted',    'ws-1', 'ok',     'IN_PROGRESS', 0, 'manual',        '{}',                     NULL,             'step-wip', 'step-wip', 1),
			('t-archived',    'ws-1', 'arch',   'IN_PROGRESS', 0, 'manual',        '{}',                     CURRENT_TIMESTAMP, 'step-wip', 'step-wip', 0),
			('t-eph',         'ws-1', 'eph',    'IN_PROGRESS', 1, 'manual',        '{}',                     NULL,             'step-wip', 'step-wip', 0),
			('t-automation',  'ws-1', 'auto',   'IN_PROGRESS', 0, 'automation_run','{}',                     NULL,             'step-wip', 'step-wip', 0),
			('t-config',      'ws-1', 'config', 'IN_PROGRESS', 0, 'manual',        '{"config_mode":true}',   NULL,             'step-wip', 'step-wip', 0),
			('t-other-ws',    'ws-2', 'other',  'IN_PROGRESS', 0, 'manual',        '{}',                     NULL,             'step-wip', 'step-wip', 0),
			('t-mismatch',    'ws-1', 'moved',  'IN_PROGRESS', 0, 'manual',        '{}',                     NULL,             'step-b',   'step-wip', 0),
			('t-no-dest',     'ws-1', 'free',   'IN_PROGRESS', 0, 'manual',        '{}',                     NULL,             'step-wip', '',         0)
	`); err != nil {
		t.Fatalf("seed tasks: %v", err)
	}

	got, err := repo.ReadControlMetrics(ctx, "ws-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("ReadControlMetrics: %v", err)
	}
	// Only t-wip-a and t-wip-b are waiting for their destination step to admit
	// them: admitted, archived, ephemeral, automation, config-mode, foreign and
	// mismatched rows are not WIP waiters.
	if got.WipWaitingTasks != 2 {
		t.Fatalf("WipWaitingTasks = %d, want 2", got.WipWaitingTasks)
	}
}
