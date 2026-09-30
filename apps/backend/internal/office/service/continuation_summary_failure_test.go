package service_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

// lastLaunchedSession resolves the run session tasklessTestLauncher most
// recently reserved, by its own naming convention ("run-session-<n>" where n
// is the shared call counter across every run this launcher instance has
// launched), and reads back its real attempt number. The launcher's attempt
// counter is global to the launcher instance rather than per-run, so a
// second launch — whether for a second run or a relaunch of a recovered one
// — always carries attempt 2, never 1; a lifecycle event that hardcodes
// RunAttempt: 1 for that second launch silently fails to resolve to a
// claimed run (resolveLifecycleRun's exact-run-session-event guard treats
// it as a stale predecessor and no-ops), which would make a "did the event
// clobber anything" assertion vacuously true instead of meaningful.
func lastLaunchedSession(
	t *testing.T, ctx context.Context, svc *service.Service, launcher *tasklessTestLauncher,
) (sessionID string, attempt int) {
	t.Helper()
	sessionID = fmt.Sprintf("run-session-%d", len(launcher.calls))
	session, err := svc.RepoForTest().GetRunSession(ctx, sessionID)
	if err != nil || session == nil {
		t.Fatalf("get launched run session %s: %v (session=%v)", sessionID, err, session)
	}
	return sessionID, session.Attempt
}

// TestContinuationSummary_FailedTasklessAttempt_PreservesPriorSummary_AgentScope
// pins REQ-OFFICE-TASKLESS-001's event-path guarantee: a failed/interrupted
// attempt must not clobber the last successful continuation summary.
// refreshContinuationSummary is called only from
// handleTasklessAgentCompleted; handleTasklessAgentFailed never calls it,
// so a failed attempt structurally cannot reach the writer. This drives a
// real success (which writes the summary through the production writer,
// never by hand) followed by a real failure for a second run under the
// same agent:<id> scope, and asserts the row is byte-identical afterward.
func TestContinuationSummary_FailedTasklessAttempt_PreservesPriorSummary_AgentScope(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	launcher := &tasklessTestLauncher{svc: svc}
	svc.SetRunSessionLauncher(launcher)

	agent := &models.AgentInstance{
		ID: "taskless-fail-agent", WorkspaceID: "ws-1", Name: "taskless-fail-agent",
		Role: models.AgentRoleCEO, Status: models.AgentStatusIdle,
		ExecutorPreference: `{"type":"local_pc"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, "fail-first"); err != nil {
		t.Fatalf("queue first: %v", err)
	}
	runs, err := svc.ListRuns(ctx, agent.WorkspaceID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("list first run: runs=%#v err=%v", runs, err)
	}
	first := runs[0]
	service.RunSchedulerTick(svc, ctx)
	if err := eb.Publish(ctx, events.AgentCompleted, bus.NewEvent(events.AgentCompleted, "test", lifecycle.AgentEventPayload{
		AgentExecutionID: "execution-run-session-1", AgentID: "test-adapter", AgentProfileID: agent.ID,
		RunID: first.ID, RunSessionID: "run-session-1", RunAttempt: 1, OwnerKind: lifecycle.ExecutionOwnerRun,
		WorkspaceID: agent.WorkspaceID, Status: "COMPLETED",
	})); err != nil {
		t.Fatalf("publish first completion: %v", err)
	}
	before, err := svc.GetContinuationSummaryForTest(ctx, agent.ID, "agent:"+agent.ID)
	if err != nil || before == nil || before.Content == "" {
		t.Fatalf("writer did not produce a baseline summary: %v (row=%v)", err, before)
	}

	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, "fail-second"); err != nil {
		t.Fatalf("queue second: %v", err)
	}
	runs, err = svc.ListRuns(ctx, agent.WorkspaceID)
	if err != nil || len(runs) != 2 {
		t.Fatalf("list second run: runs=%#v err=%v", runs, err)
	}
	second := runs[0]
	if second.ID == first.ID {
		second = runs[1]
	}
	service.RunSchedulerTick(svc, ctx)
	sessionID2, attempt2 := lastLaunchedSession(t, ctx, svc, launcher)
	if err := eb.Publish(ctx, events.AgentFailed, bus.NewEvent(events.AgentFailed, "test", lifecycle.AgentEventPayload{
		AgentExecutionID: "execution-" + sessionID2, AgentID: "test-adapter", AgentProfileID: agent.ID,
		RunID: second.ID, RunSessionID: sessionID2, RunAttempt: attempt2, OwnerKind: lifecycle.ExecutionOwnerRun,
		WorkspaceID: agent.WorkspaceID, Status: "FAILED", ErrorMessage: "agent crashed",
	})); err != nil {
		t.Fatalf("publish second failure: %v", err)
	}

	after, err := svc.GetContinuationSummaryForTest(ctx, agent.ID, "agent:"+agent.ID)
	if err != nil || after == nil {
		t.Fatalf("read summary after failed attempt: %v (row=%v)", err, after)
	}
	if after.Content != before.Content || after.UpdatedByRunID != before.UpdatedByRunID {
		t.Fatalf("failed attempt clobbered continuation summary: before=%#v after=%#v", before, after)
	}
}

// TestContinuationSummary_FailedTasklessAttempt_PreservesPriorSummary_RoutineScope
// is the routine:<id> counterpart. QueueRun has no context_snapshot
// parameter, so a routine-scoped run is seeded via raw SQL exactly as
// CreateRun would persist it (continuation_scope decided once, at
// creation) — mirroring continuation_summary_reader_test.go's established
// pattern, including that pattern's choice to carry only RunID (no
// RunSessionID) on the lifecycle event — while the summary itself is
// always produced by the real completion-event writer, never written by
// hand.
func TestContinuationSummary_FailedTasklessAttempt_PreservesPriorSummary_RoutineScope(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	createTestAgent(t, svc, "ws-1", "agent-scope-r-fail")

	claimedAt := time.Now().UTC()
	svc.ExecSQL(t, `
		INSERT INTO runs (
			id, agent_profile_id, reason, payload, status, coalesced_count,
			context_snapshot, continuation_scope, requested_at, claimed_at
		) VALUES (
			'run-r-fail-1', 'agent-scope-r-fail', 'routine_trigger', '{}',
			'claimed', 1, '{"routine_id":"rt-fail"}', 'routine:rt-fail', ?, ?
		)
	`, claimedAt, claimedAt)

	if err := eb.Publish(ctx, events.AgentCompleted, bus.NewEvent(events.AgentCompleted, "test", lifecycle.AgentEventPayload{
		AgentExecutionID: "execution-r-fail-1", AgentID: "test-adapter", AgentProfileID: "agent-scope-r-fail",
		RunID: "run-r-fail-1", WorkspaceID: "ws-1", Status: "COMPLETED",
	})); err != nil {
		t.Fatalf("publish completion: %v", err)
	}
	before, err := svc.GetContinuationSummaryForTest(ctx, "agent-scope-r-fail", "routine:rt-fail")
	if err != nil || before == nil || before.Content == "" {
		t.Fatalf("writer did not produce a summary under routine:rt-fail: %v (row=%v)", err, before)
	}

	claimedAt2 := claimedAt.Add(time.Minute)
	svc.ExecSQL(t, `
		INSERT INTO runs (
			id, agent_profile_id, reason, payload, status, coalesced_count,
			context_snapshot, continuation_scope, requested_at, claimed_at
		) VALUES (
			'run-r-fail-2', 'agent-scope-r-fail', 'routine_trigger', '{}',
			'claimed', 1, '{"routine_id":"rt-fail"}', 'routine:rt-fail', ?, ?
		)
	`, claimedAt2, claimedAt2)

	if err := eb.Publish(ctx, events.AgentFailed, bus.NewEvent(events.AgentFailed, "test", lifecycle.AgentEventPayload{
		AgentExecutionID: "execution-r-fail-2", AgentID: "test-adapter", AgentProfileID: "agent-scope-r-fail",
		RunID: "run-r-fail-2", WorkspaceID: "ws-1", Status: "FAILED", ErrorMessage: "boom",
	})); err != nil {
		t.Fatalf("publish failure: %v", err)
	}

	after, err := svc.GetContinuationSummaryForTest(ctx, "agent-scope-r-fail", "routine:rt-fail")
	if err != nil || after == nil {
		t.Fatalf("read summary after failure: %v (row=%v)", err, after)
	}
	if after.Content != before.Content || after.UpdatedByRunID != before.UpdatedByRunID {
		t.Fatalf("failed attempt clobbered continuation summary: before=%#v after=%#v", before, after)
	}
}

// TestContinuationSummary_InterruptedAttemptRecoveredAtStartup_PreservesPriorSummary
// (the third failure mode: an attempt interrupted by a backend restart) now
// lives in internal/backendapp/office_run_session_launcher_reconcile_test.go,
// where it can drive the real officeRunSessionLauncher.ReconcileRunSessions
// against a real office_run_sessions row instead of reproducing its effects
// by hand — that function lives outside this package's scope.

// TestContinuationSummary_UpsertFailure_PreservesRunCompletionAndRecordsRunEvent
// pins the plan's one allowed production change: when the continuation
// summary upsert itself fails, refreshContinuationSummary records a new run
// event naming the scope, does not roll anything back, and the run still
// reaches its terminal state. The table is renamed away so the write fails
// with a genuine SQL error; summary.LoadInputs's own sub-helpers swallow
// their errors internally (confirmed: no path in
// internal/office/summary/inputs.go returns non-nil to its caller), so this
// is the only reachable failure surface today — the load-error branch added
// alongside it is defensive, not currently triggerable through the public
// API.
func TestContinuationSummary_UpsertFailure_PreservesRunCompletionAndRecordsRunEvent(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	launcher := &tasklessTestLauncher{svc: svc}
	svc.SetRunSessionLauncher(launcher)

	agent := &models.AgentInstance{
		ID: "taskless-upsert-fail-agent", WorkspaceID: "ws-1", Name: "taskless-upsert-fail-agent",
		Role: models.AgentRoleCEO, Status: models.AgentStatusIdle,
		ExecutorPreference: `{"type":"local_pc"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, "upsert-fail-first"); err != nil {
		t.Fatalf("queue first: %v", err)
	}
	runs, err := svc.ListRuns(ctx, agent.WorkspaceID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("list first run: runs=%#v err=%v", runs, err)
	}
	first := runs[0]
	service.RunSchedulerTick(svc, ctx)
	if err := eb.Publish(ctx, events.AgentCompleted, bus.NewEvent(events.AgentCompleted, "test", lifecycle.AgentEventPayload{
		AgentExecutionID: "execution-run-session-1", AgentID: "test-adapter", AgentProfileID: agent.ID,
		RunID: first.ID, RunSessionID: "run-session-1", RunAttempt: 1, OwnerKind: lifecycle.ExecutionOwnerRun,
		WorkspaceID: agent.WorkspaceID, Status: "COMPLETED",
	})); err != nil {
		t.Fatalf("publish first completion: %v", err)
	}
	before, err := svc.GetContinuationSummaryForTest(ctx, agent.ID, "agent:"+agent.ID)
	if err != nil || before == nil || before.Content == "" {
		t.Fatalf("writer did not produce a baseline summary: %v (row=%v)", err, before)
	}

	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, "upsert-fail-second"); err != nil {
		t.Fatalf("queue second: %v", err)
	}
	runs, err = svc.ListRuns(ctx, agent.WorkspaceID)
	if err != nil || len(runs) != 2 {
		t.Fatalf("list second run: runs=%#v err=%v", runs, err)
	}
	second := runs[0]
	if second.ID == first.ID {
		second = runs[1]
	}
	service.RunSchedulerTick(svc, ctx)
	sessionID2, attempt2 := lastLaunchedSession(t, ctx, svc, launcher)

	svc.ExecSQL(t, `ALTER TABLE agent_continuation_summaries RENAME TO agent_continuation_summaries_bak`)

	if err := eb.Publish(ctx, events.AgentCompleted, bus.NewEvent(events.AgentCompleted, "test", lifecycle.AgentEventPayload{
		AgentExecutionID: "execution-" + sessionID2, AgentID: "test-adapter", AgentProfileID: agent.ID,
		RunID: second.ID, RunSessionID: sessionID2, RunAttempt: attempt2, OwnerKind: lifecycle.ExecutionOwnerRun,
		WorkspaceID: agent.WorkspaceID, Status: "COMPLETED",
	})); err != nil {
		t.Fatalf("publish second completion: %v", err)
	}

	runsAfter, err := svc.ListRuns(ctx, agent.WorkspaceID)
	if err != nil || len(runsAfter) != 2 {
		t.Fatalf("list runs after upsert failure: runs=%#v err=%v", runsAfter, err)
	}
	var secondAfter *models.Run
	for _, r := range runsAfter {
		if r.ID == second.ID {
			secondAfter = r
		}
	}
	if secondAfter == nil || secondAfter.Status != service.RunStatusFinished {
		t.Fatalf("second run after upsert failure = %#v, want status %q", secondAfter, service.RunStatusFinished)
	}

	runEvents, err := svc.ListRunEventsForTest(ctx, second.ID)
	if err != nil {
		t.Fatalf("list run events: %v", err)
	}
	found := false
	for _, e := range runEvents {
		if string(e.EventType) == "continuation_summary.upsert_failed" &&
			string(e.Level) == "warn" && strings.Contains(e.Payload, `"scope":"agent:`+agent.ID+`"`) {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a continuation_summary.upsert_failed run event naming the scope, got %#v", runEvents)
	}

	svc.ExecSQL(t, `ALTER TABLE agent_continuation_summaries_bak RENAME TO agent_continuation_summaries`)
	after, err := svc.GetContinuationSummaryForTest(ctx, agent.ID, "agent:"+agent.ID)
	if err != nil || after == nil {
		t.Fatalf("read summary after restoring table: %v (row=%v)", err, after)
	}
	if after.Content != before.Content || after.UpdatedByRunID != before.UpdatedByRunID {
		t.Fatalf("failed upsert wrote a partial row despite the INSERT erroring: before=%#v after=%#v", before, after)
	}
}
