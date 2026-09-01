package orchestrator

import (
	"context"
	"testing"

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
