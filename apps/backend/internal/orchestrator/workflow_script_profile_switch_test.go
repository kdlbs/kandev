package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func TestWorkflowScriptProfileSwitchBindsLifecycleToResolvedSessions(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyNew, models.WorkflowProfileSessionEndPolicyPark)
	fixture.stepGetter.steps["step-a"] = &wfmodels.WorkflowStep{
		ID: "step-a", WorkflowID: "wf1", Position: 0, Name: "Source",
		AgentProfileID: "profile-a",
		Events: wfmodels.StepEvents{
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{
				{Type: wfmodels.OnTurnCompleteRunScript, Config: map[string]interface{}{
					"command": "./complete.sh", "timeout_seconds": 5, "failure_policy": "block",
				}},
				{Type: wfmodels.OnTurnCompleteMoveToNext},
			},
			OnExit: []wfmodels.OnExitAction{{Type: wfmodels.OnExitRunScript, Config: map[string]interface{}{
				"command": "./exit.sh", "timeout_seconds": 5, "failure_policy": "block",
			}}},
		},
	}
	fixture.stepGetter.steps["step-b"] = &wfmodels.WorkflowStep{
		ID: "step-b", WorkflowID: "wf1", Position: 1, Name: "Destination",
		AgentProfileID:            "profile-b",
		ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyNew,
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{
			Type: wfmodels.OnEnterRunScript, Config: map[string]interface{}{
				"command": "./enter.sh", "timeout_seconds": 5, "failure_policy": "block",
			},
		}}},
	}

	now := time.Now().UTC()
	if err := fixture.repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-profile-script", TaskID: "t1", TaskSessionID: fixture.current.ID,
		StartedAt: now, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create active turn: %v", err)
	}
	fixture.svc.turnService = &repoTurnService{repo: fixture.repo}

	runs := &workflowScriptRunStoreFake{}
	messages := &workflowScriptMessageFake{}
	processes := &workflowScriptProcessFake{}
	fixture.svc.workflowScripts = newWorkflowScriptRunner(runs, processes, messages, testLogger())
	fixture.svc.initWorkflowEngine()

	entryDone := make(chan struct{})
	fixture.svc.onProcessOnEnterComplete = func() { close(entryDone) }
	if transitioned := fixture.svc.processOnTurnCompleteViaEngine(ctx, "t1", fixture.current); !transitioned {
		t.Fatal("profile-switch completion did not transition to the destination step")
	}

	select {
	case <-entryDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for destination on_enter scripts")
	}

	runs.mu.Lock()
	storedRuns := make([]*models.WorkflowScriptRun, 0, len(runs.runs))
	for _, run := range runs.runs {
		storedRuns = append(storedRuns, cloneWorkflowScriptRun(run))
	}
	runs.mu.Unlock()
	if len(storedRuns) != 3 {
		t.Fatalf("workflow script runs = %d, want completion, exit, and entry", len(storedRuns))
	}

	sourceRunCount := 0
	destinationRunCount := 0
	for _, run := range storedRuns {
		messages.mu.Lock()
		messageSessionID := messages.sessionIDs[run.MessageID]
		messages.mu.Unlock()
		if run.SessionID == fixture.current.ID {
			sourceRunCount++
			if run.ExecutionID != "execution-a" || messageSessionID != fixture.current.ID {
				t.Fatalf("source-bound run = %+v, message session = %q; want source session/execution", run, messageSessionID)
			}
			continue
		}
		destinationRunCount++
		if run.ExecutionID != "execution-b" || messageSessionID != run.SessionID {
			t.Fatalf("destination-bound run = %+v, message session = %q; want destination session/execution", run, messageSessionID)
		}
	}
	if sourceRunCount != 2 || destinationRunCount != 1 {
		t.Fatalf("source runs = %d, destination runs = %d; want 2 and 1", sourceRunCount, destinationRunCount)
	}

	sessions, err := fixture.repo.ListTaskSessions(ctx, "t1")
	if err != nil {
		t.Fatalf("list profile-switch sessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("task sessions = %d, want source and destination", len(sessions))
	}
	var destination *models.TaskSession
	for _, session := range sessions {
		if session.ID != fixture.current.ID {
			destination = session
			break
		}
	}
	if destination == nil || destination.AgentProfileID != "profile-b" {
		t.Fatalf("destination session = %+v, want profile-b session", destination)
	}
}
