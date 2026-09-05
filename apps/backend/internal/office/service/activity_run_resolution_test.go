package service_test

import (
	"context"
	"testing"
	"time"
)

// TestResolveRunForTaskAndSession_MatchesNewestClaimedRunsSession pins
// AC-19a's run-id comparison: with two claimed runs on the same task from
// different sessions, the resolver must pick the most-recently-claimed run
// and only return its id when the caller's session actually owns it. A
// resolver stub that just echoes a configured value can't catch a
// regression in the underlying "newest claim, then compare session" logic
// (GetClaimedRunByTaskID orders by claimed_at DESC), so this drives real
// rows through the real repository.
func TestResolveRunForTaskAndSession_MatchesNewestClaimedRunsSession(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('task-resolve-run', 'ws-1', 'Resolve Run Task', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	base := time.Now().UTC().Add(-time.Hour)

	// Older claimed run, session-old.
	svc.ExecSQL(t, `
		INSERT INTO runs (
			id, agent_profile_id, reason, payload, status, coalesced_count,
			context_snapshot, session_id, requested_at, claimed_at
		) VALUES (
			'run-resolve-old', 'agent-1', 'task_assigned', '{"task_id":"task-resolve-run"}',
			'claimed', 1, '{}', 'session-old', ?, ?
		)`, base, base)

	// Newer claimed run, session-new.
	svc.ExecSQL(t, `
		INSERT INTO runs (
			id, agent_profile_id, reason, payload, status, coalesced_count,
			context_snapshot, session_id, requested_at, claimed_at
		) VALUES (
			'run-resolve-new', 'agent-1', 'task_assigned', '{"task_id":"task-resolve-run"}',
			'claimed', 1, '{}', 'session-new', ?, ?
		)`, base, base.Add(time.Minute))

	if got := svc.ResolveRunForTaskAndSession(ctx, "task-resolve-run", "session-new"); got != "run-resolve-new" {
		t.Fatalf("ResolveRunForTaskAndSession(session-new) = %q, want run-resolve-new (the newest claimed run's own session)", got)
	}

	if got := svc.ResolveRunForTaskAndSession(ctx, "task-resolve-run", "session-old"); got != "" {
		t.Fatalf("ResolveRunForTaskAndSession(session-old) = %q, want \"\" (session-old owns an older claimed run, not the newest)", got)
	}
}
