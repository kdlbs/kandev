package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

// ensureTaskSessionMessagesTable creates the task_session_messages stub
// this package's tests read final agent messages from. base_test.go's
// shared task_sessions stub doesn't include this table because most
// service tests never need it.
func ensureTaskSessionMessagesTable(t *testing.T, svc *service.Service) {
	t.Helper()
	svc.ExecSQL(t, `CREATE TABLE IF NOT EXISTS task_session_messages (
		id TEXT PRIMARY KEY,
		task_session_id TEXT NOT NULL,
		type TEXT NOT NULL DEFAULT 'message',
		author_type TEXT NOT NULL DEFAULT 'user',
		content TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL
	)`)
}

func findRunByID(t *testing.T, svc *service.Service, wsID, runID string) *models.Run {
	t.Helper()
	runs, err := svc.ListRuns(context.Background(), wsID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	for _, r := range runs {
		if r.ID == runID {
			return r
		}
	}
	t.Fatalf("run %q not found among %d runs", runID, len(runs))
	return nil
}

// requireClaimedAt returns run.ClaimedAt, failing the test if it is nil.
// ClaimNextRun sets it to the real claim time, so fixture message
// timestamps in these tests are built relative to it rather than a fixed
// past literal — the run-scoped lookup in recordRunOutputSummary would
// otherwise exclude every fixture message seeded before "now".
func requireClaimedAt(t *testing.T, run *models.Run) time.Time {
	t.Helper()
	if run.ClaimedAt == nil {
		t.Fatalf("run %q has nil ClaimedAt", run.ID)
	}
	return *run.ClaimedAt
}

// TestHandleAgentCompleted_RecordsFinalAgentMessageAsOutputSummary pins
// the fix for "[Office] Finished runs never record an output summary":
// handleAgentCompleted must write the run's output_summary from the
// agent's final message before the run reaches 'finished'. An
// existence-only assertion (non-empty) would pass on any garbage write,
// so this asserts the exact projected content.
func TestHandleAgentCompleted_RecordsFinalAgentMessageAsOutputSummary(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	ensureTaskSessionMessagesTable(t, svc)

	createTestAgent(t, svc, "ws-1", "worker-1")
	taskID := createOfficeTask(t, svc, "ws-1", "worker-1")

	if err := svc.QueueRun(
		ctx, "worker-1", service.RunReasonTaskAssigned,
		`{"task_id":"`+taskID+`"}`, "run-output-init",
	); err != nil {
		t.Fatalf("queue run: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim run: %v (run=%v)", err, run)
	}
	claimedAt := requireClaimedAt(t, run)

	svc.ExecSQL(t, `
		INSERT INTO task_session_messages (id, task_session_id, type, author_type, content, created_at) VALUES
			('m-1', 'sess-1', 'message',   'user',  'please do the thing',        ?),
			('m-2', 'sess-1', 'message',   'agent', 'first agent reply',          ?),
			('m-3', 'sess-1', 'tool_call', 'agent', 'ran a tool',                 ?),
			('m-4', 'sess-1', 'message',   'agent', 'Feasibility note posted.',   ?)
	`,
		claimedAt.Add(0*time.Second),
		claimedAt.Add(5*time.Second),
		claimedAt.Add(6*time.Second),
		claimedAt.Add(10*time.Second),
	)

	completed := bus.NewEvent(events.AgentCompleted, "test", map[string]string{
		"task_id":          taskID,
		"session_id":       "sess-1",
		"agent_profile_id": "worker-1",
	})
	if pErr := eb.Publish(ctx, events.AgentCompleted, completed); pErr != nil {
		t.Fatalf("publish completed: %v", pErr)
	}

	got := findRunByID(t, svc, "ws-1", run.ID)
	if got.Status != service.RunStatusFinished {
		t.Fatalf("status = %q, want finished", got.Status)
	}
	if got.OutputSummary != "Feasibility note posted." {
		t.Errorf("output_summary = %q, want %q", got.OutputSummary, "Feasibility note posted.")
	}
	if got.FailureReason != "" {
		t.Errorf("failure_reason = %q, want empty (recording output_summary must not clobber it)", got.FailureReason)
	}
}

// TestHandleAgentCompleted_TruncatesOutputSummaryAt500Chars pins the
// truncation length reused from the child-summary precedent
// (office/repository/sqlite/blockers.go's maxCommentChars).
func TestHandleAgentCompleted_TruncatesOutputSummaryAt500Chars(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	ensureTaskSessionMessagesTable(t, svc)

	createTestAgent(t, svc, "ws-1", "worker-1")
	taskID := createOfficeTask(t, svc, "ws-1", "worker-1")

	if err := svc.QueueRun(
		ctx, "worker-1", service.RunReasonTaskAssigned,
		`{"task_id":"`+taskID+`"}`, "run-output-truncate",
	); err != nil {
		t.Fatalf("queue run: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim run: %v (run=%v)", err, run)
	}
	claimedAt := requireClaimedAt(t, run)

	long := strings.Repeat("a", 600)
	svc.ExecSQL(t, `
		INSERT INTO task_session_messages (id, task_session_id, type, author_type, content, created_at)
		VALUES ('m-1', 'sess-1', 'message', 'agent', ?, ?)
	`, long, claimedAt.Add(1*time.Second))

	completed := bus.NewEvent(events.AgentCompleted, "test", map[string]string{
		"task_id":          taskID,
		"session_id":       "sess-1",
		"agent_profile_id": "worker-1",
	})
	if pErr := eb.Publish(ctx, events.AgentCompleted, completed); pErr != nil {
		t.Fatalf("publish completed: %v", pErr)
	}

	got := findRunByID(t, svc, "ws-1", run.ID)
	if len(got.OutputSummary) != 500 {
		t.Fatalf("len(output_summary) = %d, want 500", len(got.OutputSummary))
	}
	if got.OutputSummary != strings.Repeat("a", 500) {
		t.Errorf("unexpected output_summary content")
	}
}

// TestHandleAgentCompleted_NoAgentMessageStaysBestEffort pins the
// best-effort contract: when the session has no agent message, the run
// still reaches 'finished' with an empty summary rather than failing
// the completion event.
func TestHandleAgentCompleted_NoAgentMessageStaysBestEffort(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	ensureTaskSessionMessagesTable(t, svc)

	createTestAgent(t, svc, "ws-1", "worker-1")
	taskID := createOfficeTask(t, svc, "ws-1", "worker-1")

	if err := svc.QueueRun(
		ctx, "worker-1", service.RunReasonTaskAssigned,
		`{"task_id":"`+taskID+`"}`, "run-output-none",
	); err != nil {
		t.Fatalf("queue run: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim run: %v (run=%v)", err, run)
	}

	// No task_session_messages rows for this session at all.
	completed := bus.NewEvent(events.AgentCompleted, "test", map[string]string{
		"task_id":          taskID,
		"session_id":       "sess-empty",
		"agent_profile_id": "worker-1",
	})
	if pErr := eb.Publish(ctx, events.AgentCompleted, completed); pErr != nil {
		t.Fatalf("publish completed: %v", pErr)
	}

	got := findRunByID(t, svc, "ws-1", run.ID)
	if got.Status != service.RunStatusFinished {
		t.Fatalf("status = %q, want finished (best-effort must never block completion)", got.Status)
	}
	if got.OutputSummary != "" {
		t.Errorf("output_summary = %q, want empty", got.OutputSummary)
	}
}

// TestHandleTasklessAgentCompleted_RecordsOutputSummaryAndKeepsContinuationSummary
// covers the taskless path the card explicitly asked not to be excused
// from, and pins that the new call does not displace
// refreshContinuationSummary's existing upsert.
func TestHandleTasklessAgentCompleted_RecordsOutputSummaryAndKeepsContinuationSummary(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	ensureTaskSessionMessagesTable(t, svc)

	createTestAgent(t, svc, "ws-1", "worker-taskless")

	if err := svc.QueueRun(
		ctx, "worker-taskless", service.RunReasonTaskAssigned, "{}", "run-output-taskless",
	); err != nil {
		t.Fatalf("queue run: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim run: %v (run=%v)", err, run)
	}
	claimedAt := requireClaimedAt(t, run)

	svc.ExecSQL(t, `
		INSERT INTO task_session_messages (id, task_session_id, type, author_type, content, created_at)
		VALUES ('m-1', 'sess-taskless', 'message', 'agent', 'heartbeat done.', ?)
	`, claimedAt.Add(1*time.Second))

	completed := bus.NewEvent(events.AgentCompleted, "test", map[string]string{
		"agent_id":   "worker-taskless",
		"session_id": "sess-taskless",
	})
	if pErr := eb.Publish(ctx, events.AgentCompleted, completed); pErr != nil {
		t.Fatalf("publish completed: %v", pErr)
	}

	got := findRunByID(t, svc, "ws-1", run.ID)
	if got.Status != service.RunStatusFinished {
		t.Fatalf("status = %q, want finished", got.Status)
	}
	if got.OutputSummary != "heartbeat done." {
		t.Errorf("output_summary = %q, want %q", got.OutputSummary, "heartbeat done.")
	}

	summary, err := svc.GetContinuationSummaryForTest(ctx, "worker-taskless", "agent:worker-taskless")
	if err != nil {
		t.Fatalf("continuation summary lookup: %v (refreshContinuationSummary must still upsert)", err)
	}
	if summary.UpdatedByRunID != run.ID {
		t.Errorf("continuation summary updated_by_run_id = %q, want %q", summary.UpdatedByRunID, run.ID)
	}
}

// TestHandleAgentCompleted_LastAgentMessageWinsOverLaterUserMessage pins
// the ordering rule: the projection is the last *agent* message, not
// simply the most recent row regardless of author.
func TestHandleAgentCompleted_LastAgentMessageWinsOverLaterUserMessage(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	ensureTaskSessionMessagesTable(t, svc)

	createTestAgent(t, svc, "ws-1", "worker-1")
	taskID := createOfficeTask(t, svc, "ws-1", "worker-1")

	if err := svc.QueueRun(
		ctx, "worker-1", service.RunReasonTaskAssigned,
		`{"task_id":"`+taskID+`"}`, "run-output-ordering",
	); err != nil {
		t.Fatalf("queue run: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim run: %v (run=%v)", err, run)
	}
	claimedAt := requireClaimedAt(t, run)

	svc.ExecSQL(t, `
		INSERT INTO task_session_messages (id, task_session_id, type, author_type, content, created_at) VALUES
			('m-1', 'sess-1', 'message', 'agent', 'agent final answer', ?),
			('m-2', 'sess-1', 'message', 'user',  'thanks, thats all',  ?)
	`, claimedAt.Add(0*time.Second), claimedAt.Add(5*time.Second))

	completed := bus.NewEvent(events.AgentCompleted, "test", map[string]string{
		"task_id":          taskID,
		"session_id":       "sess-1",
		"agent_profile_id": "worker-1",
	})
	if pErr := eb.Publish(ctx, events.AgentCompleted, completed); pErr != nil {
		t.Fatalf("publish completed: %v", pErr)
	}

	got := findRunByID(t, svc, "ws-1", run.ID)
	if got.OutputSummary != "agent final answer" {
		t.Errorf("output_summary = %q, want %q (later user message must not win)", got.OutputSummary, "agent final answer")
	}
}

// TestHandleAgentCompleted_SecondRunOnReusedSessionStaysEmpty pins the
// run-scoping fix raised in PR review: office task-bound sessions are
// reused across runs (same DB session_id across turns). Before the fix,
// GetFinalAgentMessage searched the whole session, so a second run that
// produces no new agent message would report the FIRST run's message as
// its own output instead of staying empty.
func TestHandleAgentCompleted_SecondRunOnReusedSessionStaysEmpty(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	ensureTaskSessionMessagesTable(t, svc)

	createTestAgent(t, svc, "ws-1", "worker-1")
	taskID := createOfficeTask(t, svc, "ws-1", "worker-1")

	// Run 1: queue, claim, produce an agent message, complete. The
	// session (sess-1) is reused for run 2 below, mirroring how office
	// task-bound sessions persist across turns.
	if err := svc.QueueRun(
		ctx, "worker-1", service.RunReasonTaskAssigned,
		`{"task_id":"`+taskID+`"}`, "run-output-reuse-1",
	); err != nil {
		t.Fatalf("queue run 1: %v", err)
	}
	run1, err := svc.ClaimNextRun(ctx)
	if err != nil || run1 == nil {
		t.Fatalf("claim run 1: %v (run=%v)", err, run1)
	}
	claimedAt1 := requireClaimedAt(t, run1)

	// created_at uses claimedAt1 exactly (no forward offset): in-process
	// tests run sub-millisecond, so any artificial forward offset risks
	// landing after run2's real claim time below and silently defeating
	// the scoping this test exists to pin. Using claimedAt1 itself still
	// satisfies run1's own since bound (created_at >= since is inclusive)
	// and is guaranteed <= run2's claim time, which happens strictly
	// later in wall-clock time.
	svc.ExecSQL(t, `
		INSERT INTO task_session_messages (id, task_session_id, type, author_type, content, created_at)
		VALUES ('m-1', 'sess-1', 'message', 'agent', 'first run reply', ?)
	`, claimedAt1)

	completed1 := bus.NewEvent(events.AgentCompleted, "test", map[string]string{
		"task_id":          taskID,
		"session_id":       "sess-1",
		"agent_profile_id": "worker-1",
	})
	if pErr := eb.Publish(ctx, events.AgentCompleted, completed1); pErr != nil {
		t.Fatalf("publish completed 1: %v", pErr)
	}

	got1 := findRunByID(t, svc, "ws-1", run1.ID)
	if got1.OutputSummary != "first run reply" {
		t.Fatalf("run 1 output_summary = %q, want %q", got1.OutputSummary, "first run reply")
	}

	// Run 2: reuses sess-1 but produces no new agent message before
	// completing (e.g. a turn that only made tool calls, or was cut
	// short). No new task_session_messages row is inserted here.
	if err := svc.QueueRun(
		ctx, "worker-1", service.RunReasonTaskComment,
		`{"task_id":"`+taskID+`"}`, "run-output-reuse-2",
	); err != nil {
		t.Fatalf("queue run 2: %v", err)
	}
	run2, err := svc.ClaimNextRun(ctx)
	if err != nil || run2 == nil {
		t.Fatalf("claim run 2: %v (run=%v)", err, run2)
	}

	completed2 := bus.NewEvent(events.AgentCompleted, "test", map[string]string{
		"task_id":          taskID,
		"session_id":       "sess-1",
		"agent_profile_id": "worker-1",
	})
	if pErr := eb.Publish(ctx, events.AgentCompleted, completed2); pErr != nil {
		t.Fatalf("publish completed 2: %v", pErr)
	}

	got2 := findRunByID(t, svc, "ws-1", run2.ID)
	if got2.OutputSummary != "" {
		t.Errorf("run 2 output_summary = %q, want empty (must not inherit run 1's message from the reused session)", got2.OutputSummary)
	}
}

func TestHandleAgentCompleted_DoesNotClearExistingSummaryWithoutMessage(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	createTestAgent(t, svc, "ws-1", "worker-1")
	taskID := createOfficeTask(t, svc, "ws-1", "worker-1")
	if err := svc.QueueRun(ctx, "worker-1", service.RunReasonTaskAssigned,
		`{"task_id":"`+taskID+`"}`, "run-output-preserve"); err != nil {
		t.Fatalf("queue run: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim run: %v (run=%v)", err, run)
	}
	svc.ExecSQL(t, `UPDATE runs SET output_summary = 'previous projection' WHERE id = ?`, run.ID)

	completed := bus.NewEvent(events.AgentCompleted, "test", map[string]string{
		"task_id":          taskID,
		"session_id":       "sess-empty",
		"agent_profile_id": "worker-1",
	})
	if err := eb.Publish(ctx, events.AgentCompleted, completed); err != nil {
		t.Fatalf("publish completed: %v", err)
	}

	got := findRunByID(t, svc, "ws-1", run.ID)
	if got.OutputSummary != "previous projection" {
		t.Fatalf("output_summary = %q, want existing projection preserved", got.OutputSummary)
	}
}

func TestHandleAgentCompleted_UsesTurnScopedMessageForSharedSession(t *testing.T) {
	stub := &stubTaskWorkspace{lastMsgByTurn: map[string]string{
		"turn-2": "second run answer",
	}}
	svc, eb := newTestServiceWithTaskWorkspace(t, stub)
	ctx := context.Background()
	createTestAgent(t, svc, "ws-1", "worker-1")
	taskID := createOfficeTask(t, svc, "ws-1", "worker-1")
	if err := svc.QueueRun(ctx, "worker-1", service.RunReasonTaskAssigned,
		`{"task_id":"`+taskID+`"}`, "run-output-turn"); err != nil {
		t.Fatalf("queue run: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim run: %v (run=%v)", err, run)
	}

	completed := bus.NewEvent(events.AgentCompleted, "test", map[string]string{
		"task_id":          taskID,
		"session_id":       "sess-shared",
		"turn_id":          "turn-2",
		"agent_profile_id": "worker-1",
	})
	if err := eb.Publish(ctx, events.AgentCompleted, completed); err != nil {
		t.Fatalf("publish completed: %v", err)
	}

	got := findRunByID(t, svc, "ws-1", run.ID)
	if got.OutputSummary != "second run answer" {
		t.Fatalf("output_summary = %q, want turn-scoped message", got.OutputSummary)
	}
}
