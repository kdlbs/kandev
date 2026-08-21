package service_test

import (
	"context"
	"strings"
	"testing"

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

	svc.ExecSQL(t, `
		INSERT INTO task_session_messages (id, task_session_id, type, author_type, content, created_at) VALUES
			('m-1', 'sess-1', 'message',   'user',  'please do the thing',        '2026-08-01 10:00:00'),
			('m-2', 'sess-1', 'message',   'agent', 'first agent reply',          '2026-08-01 10:00:05'),
			('m-3', 'sess-1', 'tool_call', 'agent', 'ran a tool',                 '2026-08-01 10:00:06'),
			('m-4', 'sess-1', 'message',   'agent', 'Feasibility note posted.',   '2026-08-01 10:00:10')
	`)

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

	long := strings.Repeat("a", 600)
	svc.ExecSQL(t, `
		INSERT INTO task_session_messages (id, task_session_id, type, author_type, content, created_at)
		VALUES ('m-1', 'sess-1', 'message', 'agent', ?, '2026-08-01 10:00:00')
	`, long)

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

	svc.ExecSQL(t, `
		INSERT INTO task_session_messages (id, task_session_id, type, author_type, content, created_at)
		VALUES ('m-1', 'sess-taskless', 'message', 'agent', 'heartbeat done.', '2026-08-01 10:00:00')
	`)

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

	svc.ExecSQL(t, `
		INSERT INTO task_session_messages (id, task_session_id, type, author_type, content, created_at) VALUES
			('m-1', 'sess-1', 'message', 'agent', 'agent final answer', '2026-08-01 10:00:00'),
			('m-2', 'sess-1', 'message', 'user',  'thanks, thats all',  '2026-08-01 10:00:05')
	`)

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
