package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
)

func TestWorkflowAsyncStartFailure_PreservesPlanOnlyInputThroughRecovery(t *testing.T) {
	fixture := newWorkflowAsyncStartFailureFixture(t, errors.New("provider startup failed"))
	ctx := context.Background()
	task, err := fixture.repo.GetTask(ctx, fixture.taskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	task.Description = ""
	if err := fixture.repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("clear task description: %v", err)
	}

	if err := fixture.svc.autoStartStepPrompt(
		ctx, fixture.taskID, fixture.session, fixture.step, "", true, true, nil,
	); err != nil {
		t.Fatalf("autoStartStepPrompt returned launch error before asynchronous failure: %v", err)
	}
	select {
	case <-fixture.startEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for asynchronous startup")
	}
	close(fixture.releaseStart)
	select {
	case <-fixture.startReturned:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for startup failure callback")
	}
	waitForSessionState(t, fixture.repo, fixture.sessionID, models.TaskSessionStateFailed)

	status := fixture.svc.messageQueue.GetStatus(ctx, fixture.sessionID)
	if status.Count != 1 {
		t.Fatalf("queue count after plan-only startup failure = %d, want 1", status.Count)
	}
	if status.Entries[0].Content != "" {
		t.Fatalf("queued raw content = %q, want empty raw content", status.Entries[0].Content)
	}
	if !status.Entries[0].PlanMode {
		t.Fatal("queued plan mode = false, want true")
	}
	if present, _ := status.Entries[0].Metadata[metaKeyWorkflowDispatchInputPresent].(bool); !present {
		t.Fatal("queued dispatch input marker = false, want true")
	}

	recoveryDone := make(chan error, 1)
	go func() {
		_, recoverErr := fixture.svc.RecoverSession(ctx, fixture.taskID, fixture.sessionID, "fresh_start")
		recoveryDone <- recoverErr
	}()
	select {
	case <-fixture.secondStartEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for explicit recovery startup")
	}
	resumeAttempt, ok := fixture.svc.resumeAttemptStore().current(fixture.sessionID)
	if !ok || resumeAttempt == nil {
		t.Fatal("explicit recovery did not retain a live resume attempt")
	}
	fixture.svc.handleAgentBootReady(ctx, watcher.AgentEventData{
		TaskID:           fixture.taskID,
		SessionID:        fixture.sessionID,
		AgentExecutionID: "workflow-async-execution",
		AttemptID:        resumeAttempt.identity(),
	})
	select {
	case recoverErr := <-recoveryDone:
		if recoverErr != nil {
			t.Fatalf("explicit recovery failed: %v", recoverErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for explicit recovery")
	}
	select {
	case <-fixture.agentMgr.promptDone:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for plan-only prompt delivery")
	}

	if len(fixture.agentMgr.capturedPrompts) != 1 {
		t.Fatalf("provider prompt count = %d, want one effective prompt", len(fixture.agentMgr.capturedPrompts))
	}
	if strings.Count(fixture.agentMgr.capturedPrompts[0], sysprompt.PlanMode()) != 1 {
		t.Fatalf("provider plan instructions were duplicated: %q", fixture.agentMgr.capturedPrompts[0])
	}
	messageCreator, ok := fixture.svc.messageCreator.(*mockMessageCreator)
	if !ok {
		t.Fatal("workflow service message creator has unexpected type")
	}
	if len(messageCreator.userMessages) != 1 {
		t.Fatalf("user transcript rows = %d, want one", len(messageCreator.userMessages))
	}
	if strings.Count(messageCreator.userMessages[0].content, sysprompt.PlanMode()) != 1 {
		t.Fatalf("transcript plan instructions were duplicated: %q", messageCreator.userMessages[0].content)
	}
}

func TestWorkflowAsyncStartFailure_PreservesConfigOnlyInputThroughRecovery(t *testing.T) {
	fixture := newWorkflowAsyncStartFailureFixture(t, errors.New("provider startup failed"))
	ctx := context.Background()
	task, err := fixture.repo.GetTask(ctx, fixture.taskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	task.Description = ""
	if err := fixture.repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("clear task description: %v", err)
	}
	session, err := fixture.repo.GetTaskSession(ctx, fixture.sessionID)
	if err != nil {
		t.Fatalf("get config session: %v", err)
	}
	if session.Metadata == nil {
		session.Metadata = make(map[string]interface{})
	}
	session.Metadata["config_mode"] = true
	if err := fixture.repo.UpdateSessionMetadata(ctx, fixture.sessionID, session.Metadata); err != nil {
		t.Fatalf("enable config mode: %v", err)
	}
	session, err = fixture.repo.GetTaskSession(ctx, fixture.sessionID)
	if err != nil {
		t.Fatalf("reload config session: %v", err)
	}
	fixture.session = session

	if err := fixture.svc.autoStartStepPrompt(
		ctx, fixture.taskID, fixture.session, fixture.step, "", false, true, nil,
	); err != nil {
		t.Fatalf("autoStartStepPrompt returned launch error before asynchronous failure: %v", err)
	}
	select {
	case <-fixture.startEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for asynchronous startup")
	}
	close(fixture.releaseStart)
	select {
	case <-fixture.startReturned:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for startup failure callback")
	}
	waitForSessionState(t, fixture.repo, fixture.sessionID, models.TaskSessionStateFailed)

	status := fixture.svc.messageQueue.GetStatus(ctx, fixture.sessionID)
	if status.Count != 1 {
		t.Fatalf("queue count after config-only startup failure = %d, want 1", status.Count)
	}
	if status.Entries[0].Content != "" {
		t.Fatalf("queued raw content = %q, want empty raw content", status.Entries[0].Content)
	}
	if status.Entries[0].PlanMode {
		t.Fatal("queued plan mode = true, want false")
	}
	if present, _ := status.Entries[0].Metadata[metaKeyWorkflowDispatchInputPresent].(bool); !present {
		t.Fatal("queued config-only dispatch input marker = false, want true")
	}
	if configMode, _ := status.Entries[0].Metadata[metaKeyWorkflowConfigMode].(bool); !configMode {
		t.Fatal("queued launch config-mode marker = false, want true")
	}
	// The queue entry owns the mode that made the empty raw prompt actionable.
	// Recovery must not discard or turn it into an empty provider call if the
	// mutable session projection changes before the drain.
	if err := fixture.repo.UpdateSessionMetadata(ctx, fixture.sessionID, map[string]interface{}{"config_mode": false}); err != nil {
		t.Fatalf("disable current config mode: %v", err)
	}

	recoveryDone := make(chan error, 1)
	go func() {
		_, recoverErr := fixture.svc.RecoverSession(ctx, fixture.taskID, fixture.sessionID, "fresh_start")
		recoveryDone <- recoverErr
	}()
	select {
	case <-fixture.secondStartEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for explicit recovery startup")
	}
	resumeAttempt, ok := fixture.svc.resumeAttemptStore().current(fixture.sessionID)
	if !ok || resumeAttempt == nil {
		t.Fatal("explicit recovery did not retain a live resume attempt")
	}
	fixture.svc.handleAgentBootReady(ctx, watcher.AgentEventData{
		TaskID:           fixture.taskID,
		SessionID:        fixture.sessionID,
		AgentExecutionID: "workflow-async-execution",
		AttemptID:        resumeAttempt.identity(),
	})
	select {
	case recoverErr := <-recoveryDone:
		if recoverErr != nil {
			t.Fatalf("explicit recovery failed: %v", recoverErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for explicit recovery")
	}
	select {
	case <-fixture.agentMgr.promptDone:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for config-only prompt delivery")
	}

	if len(fixture.agentMgr.capturedPrompts) != 1 {
		t.Fatalf("provider prompt count = %d, want one effective prompt", len(fixture.agentMgr.capturedPrompts))
	}
	configContext := sysprompt.FormatConfigContext(fixture.sessionID)
	if strings.Count(fixture.agentMgr.capturedPrompts[0], configContext) != 1 {
		t.Fatalf("provider config instructions were duplicated or lost: %q", fixture.agentMgr.capturedPrompts[0])
	}
	messageCreator, ok := fixture.svc.messageCreator.(*mockMessageCreator)
	if !ok {
		t.Fatal("workflow service message creator has unexpected type")
	}
	if len(messageCreator.userMessages) != 1 {
		t.Fatalf("user transcript rows = %d, want one", len(messageCreator.userMessages))
	}
	if strings.Count(messageCreator.userMessages[0].content, configContext) != 1 {
		t.Fatalf("transcript config instructions were duplicated or lost: %q", messageCreator.userMessages[0].content)
	}
}

func TestWorkflowAsyncStartFailure_ExcludesTrulyEmptyInput(t *testing.T) {
	fixture := newWorkflowAsyncStartFailureFixture(t, errors.New("provider startup failed"))
	ctx := context.Background()

	if err := fixture.svc.autoStartStepPrompt(
		ctx, fixture.taskID, fixture.session, fixture.step, "", false, true, nil,
	); err != nil {
		t.Fatalf("autoStartStepPrompt returned launch error before asynchronous failure: %v", err)
	}
	select {
	case <-fixture.startEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for asynchronous startup")
	}
	close(fixture.releaseStart)
	select {
	case <-fixture.startReturned:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for startup failure callback")
	}
	waitForSessionState(t, fixture.repo, fixture.sessionID, models.TaskSessionStateFailed)

	if got := fixture.svc.messageQueue.GetStatus(ctx, fixture.sessionID).Count; got != 0 {
		t.Fatalf("truly empty startup failure queued %d entries, want 0", got)
	}
	messageCreator, ok := fixture.svc.messageCreator.(*mockMessageCreator)
	if !ok {
		t.Fatal("workflow service message creator has unexpected type")
	}
	if len(messageCreator.userMessages) != 0 {
		t.Fatalf("truly empty startup failure recorded %d transcript rows, want 0", len(messageCreator.userMessages))
	}
}
