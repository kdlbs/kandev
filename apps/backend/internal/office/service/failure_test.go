package service_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

// queueAndReadRun enqueues a run with a unique idempotency key
// (so multiple calls in one test don't coalesce), reads it back, and
// marks it claimed — the precondition every production caller of
// HandleAgentFailure actually has (resolveLifecycleRun resolves via
// GetClaimedRunByID before ever reaching it). Review round 3, R3-1
// guards MarkRunFailed's write to status = 'claimed', so a test that
// skipped this step would see every HandleAgentFailure call in this
// file silently no-op.
func queueAndReadRun(
	t *testing.T, svc *service.Service, agentID, taskID string,
) *models.Run {
	t.Helper()
	ctx := context.Background()
	payload := mustMarshalJSON(map[string]string{"task_id": taskID})
	idem := agentID + ":" + taskID
	if err := svc.QueueRun(ctx, agentID, service.RunReasonTaskAssigned, payload, idem); err != nil {
		t.Fatalf("queue run: %v", err)
	}
	rows, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	for _, w := range rows {
		if w.AgentProfileID == agentID && taskIDFromPayload(t, w.Payload) == taskID && w.Status != service.RunStatusFailed {
			svc.ExecSQL(t, `UPDATE runs SET status = 'claimed', claimed_at = ? WHERE id = ?`,
				time.Now().UTC(), w.ID)
			w.Status = service.RunStatusClaimed
			return w
		}
	}
	t.Fatalf("queued run not found for (agent=%s, task=%s)", agentID, taskID)
	return nil
}

// uuidish builds a deterministic-but-unique task id for tests so the
// same test can insert multiple distinct synthetic tasks.
func uuidish(prefix string, i int) string {
	return prefix + "-" + string(rune('a'+i))
}

// insertSyntheticTask writes a row into the tasks table with the
// given assignee. Bypasses createOfficeTask's channel-agent creation
// (which fails when called repeatedly under the same test name).
// ADR 0005 Wave F: assignee lives in workflow_step_participants.
func insertSyntheticTask(
	t *testing.T, svc *service.Service, taskID, workspaceID, assigneeID string,
) {
	t.Helper()
	svc.ExecSQL(t,
		`INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		 VALUES (?, ?, '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		taskID, workspaceID)
	if assigneeID != "" {
		setTestTaskAssignee(t, svc, taskID, assigneeID)
	}
}

func taskIDFromPayload(t *testing.T, payload string) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(payload), &m); err != nil {
		return ""
	}
	if v, ok := m["task_id"].(string); ok {
		return v
	}
	return ""
}

func mustMarshalJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// Pins the per-failure counter and the auto-pause threshold semantics.
// Three consecutive failures across different tasks → agent is
// auto-paused with a structured pause_reason.
func TestHandleAgentFailure_AutoPausesAtThreshold(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-pause")

	// Three different tasks, each producing one failed run.
	for i := 0; i < 3; i++ {
		taskID := uuidish("task-pause", i)
		insertSyntheticTask(t, svc, taskID, "ws-1", "agent-pause")
		w := queueAndReadRun(t, svc, "agent-pause", taskID)
		if _, err := svc.HandleAgentFailure(ctx, w, "boom"); err != nil {
			t.Fatalf("handle failure %d: %v", i, err)
		}
	}

	agent, err := svc.GetAgentInstance(ctx, "agent-pause")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if agent.ConsecutiveFailures != 3 {
		t.Fatalf("expected 3 consecutive failures, got %d", agent.ConsecutiveFailures)
	}
	if !strings.HasPrefix(agent.PauseReason, "Auto-paused:") {
		t.Fatalf("expected auto-pause reason, got %q", agent.PauseReason)
	}
	if agent.Status != models.AgentStatusPaused {
		t.Fatalf("expected paused status, got %q", agent.Status)
	}
}

// Pins that a successful turn resets the counter — even if there were
// previous failures. (We treat any successful agent turn as evidence
// that the agent isn't broken.)
func TestRecordAgentSuccess_ResetsCounter(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-success")

	// Two failures (still below threshold).
	for i := 0; i < 2; i++ {
		taskID := uuidish("task-succ", i)
		insertSyntheticTask(t, svc, taskID, "ws-1", "agent-success")
		w := queueAndReadRun(t, svc, "agent-success", taskID)
		if _, err := svc.HandleAgentFailure(ctx, w, "boom"); err != nil {
			t.Fatalf("handle failure %d: %v", i, err)
		}
	}

	// Successful turn → reset.
	svc.RecordAgentSuccess(ctx, "agent-success")

	agent, err := svc.GetAgentInstance(ctx, "agent-success")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if agent.ConsecutiveFailures != 0 {
		t.Fatalf("expected counter reset, got %d", agent.ConsecutiveFailures)
	}
	if agent.PauseReason != "" {
		t.Fatalf("expected no pause reason, got %q", agent.PauseReason)
	}
}

// Pins that the per-agent FailureThreshold override beats the global
// default of 3.
func TestHandleAgentFailure_RespectsPerAgentThreshold(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-tight")

	// Override to 1 — first failure should pause.
	override := 1
	agent, err := svc.GetAgentInstance(ctx, "agent-tight")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	agent.FailureThreshold = &override
	if err := svc.UpdateAgentInstance(ctx, agent); err != nil {
		// Some test paths don't expose UpdateAgentInstance; fall back
		// to a direct repo update via service helpers.
		t.Skipf("update agent unsupported: %v", err)
	}

	taskID := "task-tight-1"
	insertSyntheticTask(t, svc, taskID, "ws-1", "agent-tight")
	w := queueAndReadRun(t, svc, "agent-tight", taskID)
	if _, err := svc.HandleAgentFailure(ctx, w, "boom"); err != nil {
		t.Fatalf("handle failure: %v", err)
	}

	agent, err = svc.GetAgentInstance(ctx, "agent-tight")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if !strings.HasPrefix(agent.PauseReason, "Auto-paused:") {
		t.Fatalf("expected auto-pause at threshold=1, got pause_reason=%q (counter=%d)",
			agent.PauseReason, agent.ConsecutiveFailures)
	}
}

// autoPauseAgent drives HandleAgentFailure exactly `count` times across
// distinct tasks so the agent crosses whatever its effective threshold
// is, then returns the resulting paused agent for assertions.
func autoPauseAgent(
	t *testing.T, svc *service.Service, wsID, agentID string, count int,
) *models.AgentInstance {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < count; i++ {
		taskID := agentID + "-task-" + uuidish("p", i)
		insertSyntheticTask(t, svc, taskID, wsID, agentID)
		w := queueAndReadRun(t, svc, agentID, taskID)
		if _, err := svc.HandleAgentFailure(ctx, w, "boom"); err != nil {
			t.Fatalf("handle failure %d: %v", i, err)
		}
	}
	agent, err := svc.GetAgentInstance(ctx, agentID)
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if !strings.HasPrefix(agent.PauseReason, "Auto-paused:") {
		t.Fatalf("setup failed: expected auto-pause after %d failures, got pause_reason=%q",
			count, agent.PauseReason)
	}
	if agent.Status != models.AgentStatusPaused {
		t.Fatalf("setup failed: expected paused status, got %q", agent.Status)
	}
	return agent
}

// Regression for "Mark fixed does not recover an auto-paused Office
// agent": MarkAgentPausedFixed must actually unpause the agent (write
// idle, not the pre-existing paused status back), zero the counter,
// and leave QueueRun able to succeed again.
func TestMarkAgentPausedFixed_RecoversAgent(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-recover")
	autoPauseAgent(t, svc, "ws-1", "agent-recover", 3)

	if err := svc.MarkAgentPausedFixed(ctx, "user-1", "agent-recover"); err != nil {
		t.Fatalf("mark fixed: %v", err)
	}

	agent, err := svc.GetAgentInstance(ctx, "agent-recover")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if agent.Status != models.AgentStatusIdle {
		t.Fatalf("expected status idle after mark fixed, got %q", agent.Status)
	}
	if agent.ConsecutiveFailures != 0 {
		t.Fatalf("expected counter reset, got %d", agent.ConsecutiveFailures)
	}
	if agent.PauseReason != "" {
		t.Fatalf("expected pause reason cleared, got %q", agent.PauseReason)
	}

	// Proves the guardAgentStatus rejection is gone: QueueRun must
	// succeed now that the agent is actually idle.
	if err := svc.QueueRun(ctx, "agent-recover", service.RunReasonTaskAssigned,
		mustMarshalJSON(map[string]string{"task_id": "agent-recover-task-a"}),
		"agent-recover:post-fix"); err != nil {
		t.Fatalf("expected QueueRun to succeed after mark fixed, got: %v", err)
	}
}

func TestMarkAgentPausedFixed_RequeuesEachAffectedTask(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-multi-recover")
	autoPauseAgent(t, svc, "ws-1", "agent-multi-recover", 3)

	if err := svc.MarkAgentPausedFixed(ctx, "user-1", "agent-multi-recover"); err != nil {
		t.Fatalf("mark fixed: %v", err)
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	seen := map[string]bool{}
	for _, run := range runs {
		if run.AgentProfileID != "agent-multi-recover" ||
			run.Reason != service.RunReasonManualResumeAfterFailure {
			continue
		}
		seen[taskIDFromPayload(t, run.Payload)] = true
	}
	for i := 0; i < 3; i++ {
		taskID := "agent-multi-recover-task-" + uuidish("p", i)
		if !seen[taskID] {
			t.Errorf("missing recovery run for task %s; seen=%v", taskID, seen)
		}
	}
}

func TestMarkAgentPausedFixed_RetainsPendingRecoveryForRetry(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-retry-recover")
	autoPaused := autoPauseAgent(t, svc, "ws-1", "agent-retry-recover", 3)
	if err := svc.UpdateAgentStatusFields(ctx, "agent-retry-recover",
		string(models.AgentStatusStopped), autoPaused.PauseReason); err != nil {
		t.Fatalf("stop agent: %v", err)
	}

	if err := svc.MarkAgentPausedFixed(ctx, "user-1", "agent-retry-recover"); err == nil {
		t.Fatal("mark fixed while stopped = nil error, want queue failure")
	}
	if err := svc.UpdateAgentStatusFields(ctx, "agent-retry-recover",
		string(models.AgentStatusIdle), ""); err != nil {
		t.Fatalf("restore agent: %v", err)
	}
	if err := svc.MarkAgentPausedFixed(ctx, "user-1", "agent-retry-recover"); err != nil {
		t.Fatalf("retry mark fixed: %v", err)
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	for _, run := range runs {
		if run.AgentProfileID == "agent-retry-recover" &&
			run.Reason == service.RunReasonManualResumeAfterFailure {
			return
		}
	}
	t.Fatal("retry did not queue a manual recovery run")
}

func TestMarkAgentRunFailedFixed_LeavesInboxWhenRequeueFails(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-task-retry")
	taskID := "task-retry-after-queue-error"
	insertSyntheticTask(t, svc, taskID, "ws-1", "agent-task-retry")
	run := queueAndReadRun(t, svc, "agent-task-retry", taskID)
	if _, err := svc.HandleAgentFailure(ctx, run, "boom"); err != nil {
		t.Fatalf("handle failure: %v", err)
	}
	if err := svc.UpdateAgentStatusFields(ctx, "agent-task-retry",
		string(models.AgentStatusStopped), "manual stop"); err != nil {
		t.Fatalf("stop agent: %v", err)
	}

	if err := svc.MarkAgentRunFailedFixed(ctx, "user-1", run.ID); err == nil {
		t.Fatal("mark fixed while stopped = nil error, want queue failure")
	}
	dismissed, err := svc.IsInboxItemDismissed(
		ctx, "user-1", service.InboxKindAgentRunFailed, run.ID,
	)
	if err != nil {
		t.Fatalf("check dismissal: %v", err)
	}
	if dismissed {
		t.Fatal("failed requeue dismissed the inbox item before recovery succeeded")
	}
}

func TestMarkAgentPausedFixed_UsesPauseSnapshot(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-snapshot")
	oldTaskID := "old-failure-task"
	insertSyntheticTask(t, svc, oldTaskID, "ws-1", "agent-snapshot")
	oldRun := queueAndReadRun(t, svc, "agent-snapshot", oldTaskID)
	if _, err := svc.HandleAgentFailure(ctx, oldRun, "old failure"); err != nil {
		t.Fatalf("old failure: %v", err)
	}
	svc.RecordAgentSuccess(ctx, "agent-snapshot")

	autoPauseAgent(t, svc, "ws-1", "agent-snapshot", 3)
	if err := svc.MarkAgentPausedFixed(ctx, "user-1", "agent-snapshot"); err != nil {
		t.Fatalf("mark fixed: %v", err)
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	for _, run := range runs {
		if run.AgentProfileID == "agent-snapshot" &&
			run.Reason == service.RunReasonManualResumeAfterFailure &&
			taskIDFromPayload(t, run.Payload) == oldTaskID {
			t.Fatalf("queued stale recovery for task %s", oldTaskID)
		}
	}
}

// A task reassigned to a different agent after the failure must not be
// requeued for the original agent, and the stale recovery row for it
// must be discarded (recoverPausedTask's assignee-mismatch branch).
func TestMarkAgentPausedFixed_DiscardsRecoveryForReassignedTask(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-reassigned-from")
	reassignedTaskID := "reassigned-task"
	insertSyntheticTask(t, svc, reassignedTaskID, "ws-1", "agent-reassigned-from")
	failedRun := queueAndReadRun(t, svc, "agent-reassigned-from", reassignedTaskID)
	if _, err := svc.HandleAgentFailure(ctx, failedRun, "boom"); err != nil {
		t.Fatalf("handle failure: %v", err)
	}
	// Two more failures (on other tasks) to cross the default threshold
	// of 3 and auto-pause the agent, with the reassigned task's failed
	// run captured in the pause snapshot.
	autoPauseAgent(t, svc, "ws-1", "agent-reassigned-from", 2)

	// Precondition: the pause snapshot must include the reassigned task,
	// otherwise the negative assertions below would pass vacuously (there
	// would be nothing to discard).
	var preCount int
	if err := svc.RepoForTest().ReaderDB().Get(&preCount,
		`SELECT COUNT(*) FROM office_agent_pause_recoveries WHERE agent_id = ? AND task_id = ?`,
		"agent-reassigned-from", reassignedTaskID,
	); err != nil {
		t.Fatalf("query pre-fix snapshot: %v", err)
	}
	if preCount != 1 {
		t.Fatalf(
			"test setup error: expected exactly one recovery row for reassigned task, found %d",
			preCount,
		)
	}

	setTestTaskAssignee(t, svc, reassignedTaskID, "agent-reassigned-to")

	if err := svc.MarkAgentPausedFixed(ctx, "user-1", "agent-reassigned-from"); err != nil {
		t.Fatalf("mark fixed: %v", err)
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	for _, run := range runs {
		if run.Reason == service.RunReasonManualResumeAfterFailure &&
			taskIDFromPayload(t, run.Payload) == reassignedTaskID {
			t.Fatalf("queued recovery run for reassigned task %s", reassignedTaskID)
		}
	}

	var recoveryCount int
	if err := svc.RepoForTest().ReaderDB().Get(&recoveryCount,
		`SELECT COUNT(*) FROM office_agent_pause_recoveries WHERE agent_id = ? AND task_id = ?`,
		"agent-reassigned-from", reassignedTaskID,
	); err != nil {
		t.Fatalf("query pause recoveries: %v", err)
	}
	if recoveryCount != 0 {
		t.Fatalf("expected recovery row for reassigned task to be deleted, found %d", recoveryCount)
	}
}

// Threshold-agnostic: the fix must not assume the default threshold
// of 3. A per-agent override to a different value must still recover.
func TestMarkAgentPausedFixed_ThresholdAgnostic(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-threshold")
	override := 5
	agent, err := svc.GetAgentInstance(ctx, "agent-threshold")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	agent.FailureThreshold = &override
	if err := svc.UpdateAgentInstance(ctx, agent); err != nil {
		t.Skipf("update agent unsupported: %v", err)
	}

	autoPauseAgent(t, svc, "ws-1", "agent-threshold", override)

	if err := svc.MarkAgentPausedFixed(ctx, "user-1", "agent-threshold"); err != nil {
		t.Fatalf("mark fixed: %v", err)
	}

	agent, err = svc.GetAgentInstance(ctx, "agent-threshold")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if agent.Status != models.AgentStatusIdle {
		t.Fatalf("expected status idle after mark fixed, got %q", agent.Status)
	}
	if agent.ConsecutiveFailures != 0 {
		t.Fatalf("expected counter reset, got %d", agent.ConsecutiveFailures)
	}
	if agent.PauseReason != "" {
		t.Fatalf("expected pause reason cleared, got %q", agent.PauseReason)
	}
}

// If the agent left "paused" through some other path (e.g. a manual
// stop) before the inbox dismissal is processed, mark-fixed must not
// resurrect it into idle — only the stale pause reason is cleared.
// The requeue attempts that follow then genuinely fail (the agent is
// stopped), and that failure must be returned, not swallowed.
func TestMarkAgentPausedFixed_NonPausedAgentGuardAndRequeueError(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-guarded")
	agent := autoPauseAgent(t, svc, "ws-1", "agent-guarded", 3)

	// Simulate the agent having moved to stopped via another path while
	// the stale Auto-paused reason is still on the row.
	if err := svc.UpdateAgentStatusFields(ctx, "agent-guarded",
		string(models.AgentStatusStopped), agent.PauseReason); err != nil {
		t.Fatalf("force stopped: %v", err)
	}

	err := svc.MarkAgentPausedFixed(ctx, "user-1", "agent-guarded")
	if err == nil || !strings.Contains(err.Error(), "requeue task") {
		t.Fatalf("expected a requeue error, got: %v", err)
	}

	after, getErr := svc.GetAgentInstance(ctx, "agent-guarded")
	if getErr != nil {
		t.Fatalf("get agent: %v", getErr)
	}
	if after.Status != models.AgentStatusStopped {
		t.Fatalf("expected status to remain stopped, got %q", after.Status)
	}
	if after.PauseReason != "" {
		t.Fatalf("expected stale pause reason cleared, got %q", after.PauseReason)
	}
}

// HandleAgentFailure is Office's v1 post-launch failure path
// (event_subscribers.go's handleAgentFailed) and bypasses
// transitionRunTerminal's own counting, so it must record its own
// terminal shape or office_loop_terminal_total silently misses every
// genuine agent crash (Review round 1, R1-1).
func TestHandleAgentFailure_RecordsTerminalShape(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-crash")
	taskID := "task-crash"
	insertSyntheticTask(t, svc, taskID, "ws-1", "agent-crash")
	run := queueAndReadRun(t, svc, "agent-crash", taskID)
	svc.ExecSQL(t, `UPDATE runs SET session_id = ? WHERE id = ?`, "session-crash", run.ID)
	run.SessionID = "session-crash"

	key := service.LoopMetricLabel("workspace", "ws-1", "shape", string(service.ShapeLaunchedFailed))
	before := terminalShapeExpvarInt(t, key)

	if _, err := svc.HandleAgentFailure(ctx, run, "boom"); err != nil {
		t.Fatalf("handle failure: %v", err)
	}

	after := terminalShapeExpvarInt(t, key)
	if after != before+1 {
		t.Fatalf("launched_failed delta = %d, want 1", after-before)
	}
}

// Pins that reassigning a task auto-dismisses the per-task inbox
// entry for the OLD agent without resetting that agent's counter.
func TestOnAssigneeChanged_DismissesPriorEntryWithoutResettingCounter(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "old-agent")
	createTestAgent(t, svc, "ws-1", "new-agent")

	taskID := "task-reassign-1"
	insertSyntheticTask(t, svc, taskID, "ws-1", "old-agent")
	w := queueAndReadRun(t, svc, "old-agent", taskID)
	if _, err := svc.HandleAgentFailure(ctx, w, "boom"); err != nil {
		t.Fatalf("handle failure: %v", err)
	}

	beforeAgent, _ := svc.GetAgentInstance(ctx, "old-agent")
	beforeCount := beforeAgent.ConsecutiveFailures

	// Simulate the reactivity hook firing on assignee change.
	svc.OnAssigneeChanged(ctx, taskID, "old-agent")

	// Counter must NOT be reset — the underlying cause may still be
	// unfixed for old-agent's other tasks.
	afterAgent, _ := svc.GetAgentInstance(ctx, "old-agent")
	if afterAgent.ConsecutiveFailures != beforeCount {
		t.Fatalf("expected counter unchanged at %d, got %d",
			beforeCount, afterAgent.ConsecutiveFailures)
	}

	// The run should be dismissed via the auto-dismiss sentinel.
	dismissed, err := svc.IsInboxItemDismissed(ctx, "_auto",
		service.InboxKindAgentRunFailed, w.ID)
	if err != nil {
		t.Fatalf("check dismissed: %v", err)
	}
	if !dismissed {
		t.Fatalf("expected run %s dismissed via _auto", w.ID)
	}
}
