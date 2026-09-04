package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func TestProcessStepExitAndEnter_UnknownSourceKeepsCurrentSessionRecoverable(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyNew, models.WorkflowProfileSessionEndPolicyPark)
	fixture.stepGetter.steps["step-b"] = &wfmodels.WorkflowStep{
		ID: "step-b", WorkflowID: "wf1", Position: 1,
		AgentProfileID:            "profile-b",
		ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyNew,
	}

	if err := fixture.svc.processStepExitAndEnter(ctx, "t1", fixture.current, "missing-source", "step-b", "Test"); err == nil {
		t.Fatal("processStepExitAndEnter returned nil for unknown source step")
	}

	source, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	if err != nil {
		t.Fatalf("reload source session: %v", err)
	}
	if source.State != models.TaskSessionStateRunning || !source.IsPrimary {
		t.Fatalf("source after unknown source lookup = state %s primary %t, want running primary", source.State, source.IsPrimary)
	}
	sessions, err := fixture.repo.ListTaskSessions(ctx, "t1")
	if err != nil {
		t.Fatalf("list task sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("session count after unknown source lookup = %d, want 1", len(sessions))
	}
}

func TestPrepareWorkflowStepSession_ProfileSwitchRequiresSourceStep(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyNew, models.WorkflowProfileSessionEndPolicyPark)
	target := &wfmodels.WorkflowStep{
		ID: "step-b", WorkflowID: "wf1", Position: 1,
		AgentProfileID:            "profile-b",
		ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyNew,
	}

	if _, _, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", fixture.current, target, nil); err == nil {
		t.Fatal("profile switch without a source step returned nil error")
	}

	source, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	if err != nil {
		t.Fatalf("reload source session: %v", err)
	}
	if source.State != models.TaskSessionStateRunning || !source.IsPrimary {
		t.Fatalf("source after missing source step = state %s primary %t, want running primary", source.State, source.IsPrimary)
	}
}

func TestHandleTaskQueuePromoted_UnknownSourceKeepsPromotionPending(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyNew, models.WorkflowProfileSessionEndPolicyPark)
	task, err := fixture.repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	task.WorkflowStepID = "step-b"
	task.WIPAdmitted = true
	task.Metadata[models.MetaKeyQueuePromotionPending] = map[string]interface{}{"from_step_id": "missing-source"}
	if err := fixture.repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("persist promoted task: %v", err)
	}
	fixture.stepGetter.steps["step-b"] = &wfmodels.WorkflowStep{
		ID: "step-b", WorkflowID: "wf1", Position: 1,
		AgentProfileID:            "profile-b",
		ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyNew,
	}

	fixture.svc.handleTaskQueuePromoted(ctx, watcher.TaskEventData{TaskID: "t1"})

	stored, err := fixture.repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("reload promoted task: %v", err)
	}
	if _, pending := stored.Metadata[models.MetaKeyQueuePromotionPending]; !pending {
		t.Fatal("queue promotion token was claimed before source step lookup succeeded")
	}
}

func TestRestoreTaskLifecycleTokenPreservesQueuePromotionSource(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "promotion-restore", "promotion-session", "source-step")
	task, err := repo.GetTask(ctx, "promotion-restore")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	descriptor := map[string]interface{}{"from_step_id": "source-step"}
	task.Metadata[models.MetaKeyQueuePromotionPending] = descriptor
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("persist promotion descriptor: %v", err)
	}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	claimed, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("reload task before claim: %v", err)
	}
	if !svc.claimTaskEventMetadata(ctx, claimed, models.MetaKeyQueuePromotionPending) {
		t.Fatal("queue-promotion token was not claimed")
	}
	svc.restoreTaskLifecycleToken(ctx, task.ID, models.MetaKeyQueuePromotionPending, descriptor, "test")

	restored, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("reload restored task: %v", err)
	}
	raw := restored.Metadata[models.MetaKeyQueuePromotionPending]
	restoredDescriptor, ok := raw.(map[string]interface{})
	if !ok {
		t.Fatalf("restored token type = %T, want descriptor", raw)
	}
	if got, _ := restoredDescriptor["from_step_id"].(string); got != "source-step" {
		t.Fatalf("restored source = %q, want source-step", got)
	}
}

func TestHandleTaskQueuePromotedUsesSourceParkPolicy(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyNew, models.WorkflowProfileSessionEndPolicyPark)
	fixture.stepGetter.steps["step-a"] = &wfmodels.WorkflowStep{
		ID: "step-a", WorkflowID: "wf1", Position: 0,
		AgentProfileID:          "profile-a",
		ProfileSessionEndPolicy: models.WorkflowProfileSessionEndPolicyPark,
	}
	fixture.stepGetter.steps["step-b"] = &wfmodels.WorkflowStep{
		ID: "step-b", WorkflowID: "wf1", Position: 1,
		AgentProfileID:            "profile-b",
		ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyNew,
	}
	task, err := fixture.repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	task.WorkflowStepID = "step-b"
	task.WIPAdmitted = true
	task.Metadata[models.MetaKeyQueuePromotionPending] = map[string]interface{}{
		"from_step_id": "step-a",
	}
	if err := fixture.repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("persist promoted task: %v", err)
	}
	entryComplete := make(chan struct{})
	fixture.svc.onTaskQueuePromotionEntryComplete = func() { close(entryComplete) }

	fixture.svc.handleTaskQueuePromoted(ctx, watcher.TaskEventData{TaskID: "t1"})
	select {
	case <-entryComplete:
	case <-time.After(2 * time.Second):
		t.Fatal("queue-promotion entry did not complete")
	}

	source, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	if err != nil {
		t.Fatalf("reload source session: %v", err)
	}
	if source.State != models.TaskSessionStateWaitingForInput || source.IsPrimary {
		t.Fatalf("source after promotion = state %s primary %t, want parked nonprimary", source.State, source.IsPrimary)
	}
	if source.CompletedAt != nil {
		t.Fatal("park-configured source was completed during queue promotion")
	}
}

func TestProcessManualMoveLifecycle_UnknownSourceKeepsLifecyclePending(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyNew, models.WorkflowProfileSessionEndPolicyPark)
	task, err := fixture.repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	task.WorkflowStepID = "step-b"
	task.Metadata[models.MetaKeyManualMoveLifecyclePending] = map[string]interface{}{"from_step_id": "missing-source"}
	if err := fixture.repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("persist pending manual move: %v", err)
	}
	target := &wfmodels.WorkflowStep{
		ID: "step-b", WorkflowID: "wf1", Position: 1,
		AgentProfileID:            "profile-b",
		ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyNew,
	}
	fixture.stepGetter.steps[target.ID] = target

	fixture.svc.processManualMoveLifecycleWithFeederBarrier(
		ctx, "t1", fixture.current, nil, target,
		"missing-source", target.ID, "Test",
	)

	stored, err := fixture.repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if _, pending := stored.Metadata[models.MetaKeyManualMoveLifecyclePending]; !pending {
		t.Fatal("manual move lifecycle token was cleared before source step lookup succeeded")
	}
	if _, completed := stored.Metadata[models.MetaKeyManualMoveLifecycleCompleted]; completed {
		t.Fatal("manual move lifecycle was marked complete after source step lookup failed")
	}
	source, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	if err != nil {
		t.Fatalf("reload source session: %v", err)
	}
	if source.State != models.TaskSessionStateRunning || !source.IsPrimary {
		t.Fatalf("source after manual move lookup failure = state %s primary %t, want running primary", source.State, source.IsPrimary)
	}
}

func TestApplyEngineTransition_UnknownSourceKeepsCommitUnapplied(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyNew, models.WorkflowProfileSessionEndPolicyPark)
	fixture.stepGetter.steps["step-b"] = &wfmodels.WorkflowStep{
		ID: "step-b", WorkflowID: "wf1", Position: 1,
		AgentProfileID: "profile-a",
	}
	commitCalled := false
	applied := fixture.svc.applyEngineTransitionWithCommit(
		ctx, "t1", fixture.current,
		engine.HandleResult{Transitioned: true, FromStepID: "missing-source", ToStepID: "step-b"},
		engine.TriggerOnTurnStart, "Test", false,
		func(context.Context) (bool, error) {
			commitCalled = true
			return true, nil
		},
	)
	if applied {
		t.Fatal("engine transition applied despite unknown source step")
	}
	if commitCalled {
		t.Fatal("engine transition committed before source step lookup succeeded")
	}
	source, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	if err != nil {
		t.Fatalf("reload source session: %v", err)
	}
	if !source.IsPrimary || source.State == models.TaskSessionStateCompleted || source.State == models.TaskSessionStateCancelled {
		t.Fatalf("source after engine lookup failure = state %s primary %t, want nonterminal primary session", source.State, source.IsPrimary)
	}
}

func TestSwitchWorkflowDispatcher_UnknownSourceKeepsCurrentSessionRecoverable(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyNew, models.WorkflowProfileSessionEndPolicyPark)
	task, err := fixture.repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	task.WorkflowStepID = "step-b"
	if err := fixture.repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("move task to destination: %v", err)
	}
	fixture.stepGetter.steps["step-b"] = &wfmodels.WorkflowStep{
		ID: "step-b", WorkflowID: "wf1", Position: 1,
		AgentProfileID:            "profile-b",
		ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyNew,
	}
	fixture.svc.initWorkflowEngine()

	err = switchWorkflowDispatcher(fixture.svc)(ctx, "t1", fixture.current.ID, engine.TriggerOnEnter, "", "missing-source")
	if err == nil {
		t.Fatal("dispatcher returned nil for unknown source step")
	}
	source, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	if err != nil {
		t.Fatalf("reload source session: %v", err)
	}
	if source.State != models.TaskSessionStateRunning || !source.IsPrimary {
		t.Fatalf("source after dispatcher lookup failure = state %s primary %t, want running primary", source.State, source.IsPrimary)
	}
	sessions, err := fixture.repo.ListTaskSessions(ctx, "t1")
	if err != nil {
		t.Fatalf("list task sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("session count after dispatcher lookup failure = %d, want 1", len(sessions))
	}
}
