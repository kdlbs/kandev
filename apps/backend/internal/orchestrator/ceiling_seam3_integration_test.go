package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
)

// TestPromptTask_AutomaticOverCeilingDefersPromptEnsure pins AC-47/AC-47c2 at
// the real promptTask entry point: an automatic prompt whose ensureSessionRunning
// call is refused by the ceiling gets a prompt_ensure record written from
// promptTask's own frame, and the returned error is the seam-3 sentinel marked
// deferred.
func TestPromptTask_AutomaticOverCeilingDefersPromptEnsure(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)

	agentMgr := &mockAgentManager{isAgentRunning: false}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.sessionCeiling = newSessionCeilingController(1, nil, nil)
	svc.sessionCeiling.admit(ctx, admissionRequest{taskID: "filler", sessionID: "filler-session", origin: launchOriginAutomatic, seam: "filler"})

	_, err := svc.promptTask(ctx, "task1", "session1", "hello", "", false, nil, false, launchOriginAutomatic, promptTaskOptions{})
	if err == nil {
		t.Fatal("expected the prompt to be refused by the session ceiling")
	}
	refusal, ok := isSeam3Refusal(err)
	if !ok {
		t.Fatalf("expected a seam3Refusal, got: %v", err)
	}
	if !refusal.deferred {
		t.Fatal("a reconstructable prompt refusal must be marked deferred")
	}

	record := deferredLaunchOf(t, svc, "task1")
	if record == nil || record[models.CeilingDeferredKey] != true {
		t.Fatalf("prompt_ensure was not recorded: %+v", record)
	}
	if record[models.CeilingLaunchKindKey] != string(models.CeilingLaunchPromptEnsure) {
		t.Fatalf("ceiling_launch_kind = %v, want %q", record[models.CeilingLaunchKindKey], models.CeilingLaunchPromptEnsure)
	}
	nested, _ := record[models.CeilingLaunchPayloadKey].(map[string]interface{})
	if nested["prompt"] != "hello" {
		t.Fatalf("recorded payload does not match the refused prompt: %+v", record)
	}
}

// TestPromptTask_ManualOverCeilingIsAdmitted covers AC-14 through the real
// promptTask entry point: PromptTask's own public wrapper always hardcodes
// manual, so it must be admitted even at the ceiling, and never write a
// deferral record.
func TestPromptTask_ManualOverCeilingIsAdmitted(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)

	agentMgr := &mockAgentManager{isAgentRunning: false}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.sessionCeiling = newSessionCeilingController(1, nil, nil)
	svc.sessionCeiling.admit(ctx, admissionRequest{taskID: "filler", sessionID: "filler-session", origin: launchOriginAutomatic, seam: "filler"})

	_, err := svc.PromptTask(ctx, "task1", "session1", "hello", "", false, nil, false)
	if err == nil {
		t.Fatal("expected an error (no resumable executor), but not a ceiling refusal")
	}
	if _, ok := isSeam3Refusal(err); ok {
		t.Fatalf("a manual prompt must never be refused by the ceiling, got: %v", err)
	}
	if record := deferredLaunchOf(t, svc, "task1"); record != nil {
		t.Fatalf("a manual prompt must never write a ceiling_deferred record: %+v", record)
	}
}

// TestStartSessionForWorkflowStep_PreConsultationRefusalOverCeiling pins
// AC-47f/AC-47f1: the pre-consultation admission runs before
// advanceTaskWorkflowStep mutates the task, so a refusal must leave the
// task's workflow step untouched and record workflow_step_ensure itself.
func TestStartSessionForWorkflowStep_PreConsultationRefusalOverCeiling(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{ID: "step2", WorkflowID: "wf1"}
	svc := createTestService(repo, stepGetter, newMockTaskRepo())
	svc.sessionCeiling = newSessionCeilingController(1, nil, nil)
	svc.sessionCeiling.admit(ctx, admissionRequest{taskID: "filler", sessionID: "filler-session", origin: launchOriginAutomatic, seam: "filler"})

	err := svc.StartSessionForWorkflowStep(ctx, "t1", "s1", "step2")
	if !errors.Is(err, errSeam3WorkflowStepEnsureDeferred) {
		t.Fatalf("StartSessionForWorkflowStep error = %v, want errSeam3WorkflowStepEnsureDeferred", err)
	}

	task, getErr := repo.GetTask(ctx, "t1")
	if getErr != nil {
		t.Fatalf("get task: %v", getErr)
	}
	if task.WorkflowStepID != "step1" {
		t.Fatalf("task.WorkflowStepID = %q, want unchanged %q — advanceTaskWorkflowStep must not run before the pre-consultation admits", task.WorkflowStepID, "step1")
	}

	record := deferredLaunchOf(t, svc, "t1")
	if record == nil || record[models.CeilingDeferredKey] != true {
		t.Fatalf("workflow_step_ensure was not recorded: %+v", record)
	}
	if record[models.CeilingLaunchKindKey] != string(models.CeilingLaunchWorkflowStepEnsure) {
		t.Fatalf("ceiling_launch_kind = %v, want %q", record[models.CeilingLaunchKindKey], models.CeilingLaunchWorkflowStepEnsure)
	}
	nested, _ := record[models.CeilingLaunchPayloadKey].(map[string]interface{})
	if nested[metaKeySessionID] != "s1" || nested[metaKeyWorkflowStepID] != "step2" {
		t.Fatalf("recorded payload does not match the refused pre-consultation: %+v", record)
	}
}

// TestTryEnsureExecution_ViewingShapeSwallowsRefusal covers AC-47a: a
// ceiling refusal reached from the viewing call shape (EnsureSession's
// resume-for-viewing path) has nothing to replay and nobody waiting, so it
// must be swallowed — no deferred_launch record, and the call must not
// panic despite tryEnsureExecution having no return value to report through.
func TestTryEnsureExecution_ViewingShapeSwallowsRefusal(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)

	agentMgr := &mockAgentManager{isAgentRunning: false}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.sessionCeiling = newSessionCeilingController(1, nil, nil)
	svc.sessionCeiling.admit(ctx, admissionRequest{taskID: "filler", sessionID: "filler-session", origin: launchOriginAutomatic, seam: "filler"})

	svc.tryEnsureExecution(ctx, "session1", seam3CallShapeViewing, launchOriginAutomatic, "")

	if record := deferredLaunchOf(t, svc, "task1"); record != nil {
		t.Fatalf("the viewing call shape must swallow a ceiling refusal, not record it: %+v", record)
	}
}

// TestTryEnsureExecution_QueueDrainShapeDefersRefusal covers AC-47d: a
// refusal reached from the queue-drain call shape defers, carrying the
// queued message id needed for AC-47d1's already-drained check at retry
// time.
func TestTryEnsureExecution_QueueDrainShapeDefersRefusal(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)

	agentMgr := &mockAgentManager{isAgentRunning: false}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.sessionCeiling = newSessionCeilingController(1, nil, nil)
	svc.sessionCeiling.admit(ctx, admissionRequest{taskID: "filler", sessionID: "filler-session", origin: launchOriginAutomatic, seam: "filler"})

	svc.tryEnsureExecution(ctx, "session1", seam3CallShapeQueueDrain, launchOriginAutomatic, "queued-msg-42")

	record := deferredLaunchOf(t, svc, "task1")
	if record == nil || record[models.CeilingDeferredKey] != true {
		t.Fatalf("the queue-drain call shape must record the refusal: %+v", record)
	}
	if record[models.CeilingLaunchKindKey] != string(models.CeilingLaunchQueueDrainEnsure) {
		t.Fatalf("ceiling_launch_kind = %v, want %q", record[models.CeilingLaunchKindKey], models.CeilingLaunchQueueDrainEnsure)
	}
	nested, _ := record[models.CeilingLaunchPayloadKey].(map[string]interface{})
	if nested[metaKeySessionID] != "session1" || nested["queued_message_id"] != "queued-msg-42" {
		t.Fatalf("recorded payload does not match the refused drain: %+v", record)
	}
}

// TestTryEnsureExecution_UnrecognizedShapeDefaultsToDeferring covers AC-47e1:
// an absent/unrecognized call-shape token must default to the deferring
// shape rather than silently dropping the launch.
func TestTryEnsureExecution_UnrecognizedShapeDefaultsToDeferring(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)

	agentMgr := &mockAgentManager{isAgentRunning: false}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.sessionCeiling = newSessionCeilingController(1, nil, nil)
	svc.sessionCeiling.admit(ctx, admissionRequest{taskID: "filler", sessionID: "filler-session", origin: launchOriginAutomatic, seam: "filler"})

	svc.tryEnsureExecution(ctx, "session1", seam3CallShape("unrecognized"), launchOriginAutomatic, "queued-msg-99")

	record := deferredLaunchOf(t, svc, "task1")
	if record == nil || record[models.CeilingDeferredKey] != true {
		t.Fatalf("an unrecognized call shape must default to deferring, not dropping: %+v", record)
	}
}

// TestQueueAutoStartPrompt_ThreadsQueuedMessageIDToDeferral is an end-to-end
// check that queueAutoStartPrompt's queued message id survives through
// scheduleAutoResumeForWorkflowQueue's detached goroutine into the
// queue_drain_ensure record, when the ceiling is over capacity and no
// execution is tracked for the session.
func TestQueueAutoStartPrompt_ThreadsQueuedMessageIDToDeferral(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)

	agentMgr := &mockAgentManager{isAgentRunning: false}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.sessionCeiling = newSessionCeilingController(1, nil, nil)
	svc.sessionCeiling.admit(ctx, admissionRequest{taskID: "filler", sessionID: "filler-session", origin: launchOriginAutomatic, seam: "filler"})

	err := svc.queueAutoStartPrompt(ctx, "task1", "session1", "queued prompt", false, nil, workflowMessageOrigin{}, false, nil, "")
	if err != nil {
		t.Fatalf("queueAutoStartPrompt: %v", err)
	}

	require.Eventually(t, func() bool {
		task, getErr := svc.repo.GetTask(ctx, "task1")
		if getErr != nil || task.Metadata == nil {
			return false
		}
		record, _ := task.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
		return record != nil && record[models.CeilingDeferredKey] == true
	}, 2*time.Second, 10*time.Millisecond, "expected the detached goroutine to record a queue_drain_ensure deferral")

	record := deferredLaunchOf(t, svc, "task1")
	if record[models.CeilingLaunchKindKey] != string(models.CeilingLaunchQueueDrainEnsure) {
		t.Fatalf("ceiling_launch_kind = %v, want %q", record[models.CeilingLaunchKindKey], models.CeilingLaunchQueueDrainEnsure)
	}
	nested, _ := record[models.CeilingLaunchPayloadKey].(map[string]interface{})
	queuedMessageID, _ := nested["queued_message_id"].(string)
	if queuedMessageID == "" {
		t.Fatalf("recorded payload is missing the queued message id: %+v", record)
	}
	status := svc.messageQueue.GetStatus(ctx, "session1")
	if len(status.Entries) != 1 || status.Entries[0].ID != queuedMessageID {
		t.Fatalf("recorded queued_message_id %q does not match the queue's own message: %+v", queuedMessageID, status.Entries)
	}
}
