package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func TestRunWorkflowScriptCallbackUsesSourceSessionIdentity(t *testing.T) {
	processes := &workflowScriptProcessFake{}
	runner := newWorkflowScriptRunner(&workflowScriptRunStoreFake{}, processes, &workflowScriptMessageFake{}, testLogger())
	callback := &runWorkflowScriptCallback{svc: &Service{workflowScripts: runner}}

	_, err := callback.Execute(context.Background(), engine.ActionInput{
		Trigger:        engine.TriggerOnTurnComplete,
		OperationID:    "turn-42",
		ActionPosition: 2,
		State: engine.MachineState{
			TaskID: "task-1", SessionID: "source-session", ExecutionID: "source-execution", WorkflowID: "workflow-1",
		},
		Step: engine.StepSpec{ID: "step-1", Name: "Build"},
		Action: engine.Action{
			Kind:      engine.ActionRunScript,
			RunScript: &wfmodels.WorkflowScriptAction{Command: "./check.sh", TimeoutSeconds: 5, FailurePolicy: wfmodels.WorkflowScriptFailurePolicyContinue},
		},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(processes.starts) != 1 {
		t.Fatalf("process starts = %d, want 1", len(processes.starts))
	}
	request := processes.starts[0]
	if request.SessionID != "source-session" || request.ExecutionID != "source-execution" {
		t.Fatalf("source binding = %+v", request)
	}
	run, err := runner.runs.GetWorkflowScriptRunByOccurrence(context.Background(), "on_turn_complete/turn-42/step-1/2")
	if err != nil {
		t.Fatalf("load run: %v", err)
	}
	if run.WorkflowStepName != "Build" {
		t.Fatalf("workflow step name = %q, want Build", run.WorkflowStepName)
	}
}

// TestSetSessionModeCallback_Execute covers the engine callback wired for the
// set_session_mode action (issue #1183): for a non-passthrough session it
// persists the declared mode to metadata and applies it live to the agent;
// passthrough sessions and actions with no payload are no-ops.
func TestSetSessionModeCallback_Execute(t *testing.T) {
	ctx := context.Background()

	newInput := func(passthrough bool, mode string) engine.ActionInput {
		in := engine.ActionInput{
			Trigger: engine.TriggerOnEnter,
			State:   engine.MachineState{TaskID: "t1", SessionID: "s1", IsPassthrough: passthrough},
			Action:  engine.Action{Kind: engine.ActionSetSessionMode},
		}
		if mode != "" {
			in.Action.SetSessionMode = &engine.SetSessionModeAction{Mode: mode}
		}
		return in
	}

	t.Run("applies and persists for a non-passthrough session", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		agentMgr := &mockAgentManager{}
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

		if _, err := (&setSessionModeCallback{svc: svc}).Execute(ctx, newInput(false, "acceptEdits")); err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}

		updated, _ := repo.GetTaskSession(ctx, "s1")
		if got, _ := updated.Metadata[models.SessionMetaKeySessionMode].(string); got != "acceptEdits" {
			t.Errorf("expected persisted session_mode acceptEdits, got %q", got)
		}
		if len(agentMgr.setSessionModeCalls) != 1 || agentMgr.setSessionModeCalls[0].ModeID != "acceptEdits" {
			t.Errorf("expected one live set-mode call for acceptEdits, got %+v", agentMgr.setSessionModeCalls)
		}
	})

	t.Run("skips passthrough sessions", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		agentMgr := &mockAgentManager{isPassthrough: true}
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

		if _, err := (&setSessionModeCallback{svc: svc}).Execute(ctx, newInput(true, "acceptEdits")); err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}

		updated, _ := repo.GetTaskSession(ctx, "s1")
		if _, ok := updated.Metadata[models.SessionMetaKeySessionMode]; ok {
			t.Error("passthrough session must not persist session_mode")
		}
		if len(agentMgr.setSessionModeCalls) != 0 {
			t.Errorf("passthrough session must not apply mode live, got %+v", agentMgr.setSessionModeCalls)
		}
	})

	t.Run("no-op when action carries no mode", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		agentMgr := &mockAgentManager{}
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

		if _, err := (&setSessionModeCallback{svc: svc}).Execute(ctx, newInput(false, "")); err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if len(agentMgr.setSessionModeCalls) != 0 {
			t.Errorf("expected no live set-mode call when mode is empty, got %+v", agentMgr.setSessionModeCalls)
		}
	})
}
