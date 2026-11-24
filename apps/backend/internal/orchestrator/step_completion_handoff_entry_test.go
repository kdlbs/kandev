package orchestrator

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
)

func TestStepCompletionEntryHandoff(t *testing.T) {
	for _, path := range []string{"legacy", "engine"} {
		for _, queued := range []bool{false, true} {
			for _, handoff := range []string{"next agent context", ""} {
				name := path + "/" + map[bool]string{false: "admitted", true: "queued"}[queued] + "/" + map[bool]string{false: "remove", true: "write"}[handoff != ""]
				t.Run(name, func(t *testing.T) {
					repo := setupTestRepo(t)
					seedSession(t, repo, "task", "session", "step1")
					require.NoError(t, repo.SetTaskMetadataKey(t.Context(), "task", models.MetaKeyStepHandoffCarry, models.StepHandoffCarryToken{StepID: "step1", Stamp: "old", Handoff: "old context"}))
					if queued {
						require.NoError(t, repo.CreateTask(t.Context(), &models.Task{ID: "occupant", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step2", Title: "Occupant", WIPAdmitted: true}))
					}
					entry := signalEntryForSession(t, repo, "session")
					_, claimed, err := repo.ClaimStepCompletionSignal(t.Context(), "task", "session", models.PendingStepCompletionSignal{StepID: "step1", WorkflowStepTransitionID: entry, Summary: "done", Handoff: handoff})
					require.NoError(t, err)
					require.True(t, claimed)
					getter := newMockStepGetter()
					getter.steps["step1"] = &wfmodels.WorkflowStep{ID: "step1", WorkflowID: "wf1", AutoAdvanceRequiresSignal: true, Events: wfmodels.StepEvents{OnTurnComplete: []wfmodels.OnTurnCompleteAction{{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": "step2"}}}}}
					getter.steps["step2"] = &wfmodels.WorkflowStep{ID: "step2", WorkflowID: "wf1", Position: 1, WIPLimit: 1}
					svc := createTestService(repo, getter, newMockTaskRepo())
					session, err := repo.GetTaskSession(t.Context(), "session")
					require.NoError(t, err)
					task, err := repo.GetTask(t.Context(), "task")
					require.NoError(t, err)
					if path == "engine" {
						svc.initWorkflowEngine()
						require.True(t, svc.processOnTurnCompleteViaEngine(t.Context(), task.ID, session))
					} else {
						require.True(t, svc.processOnTurnComplete(t.Context(), task, session))
					}
					task, err = repo.GetTask(t.Context(), "task")
					require.NoError(t, err)
					require.Equal(t, "step2", task.WorkflowStepID)
					require.Equal(t, queued, task.QueuedForStepID == "step2")
					token, present := carryToken(t, repo, "task")
					if handoff == "" {
						require.False(t, present, "blank handoff must remove the previous token in the move transaction")
					} else {
						require.True(t, present, "signal-gated move must preserve its prepared handoff")
						require.Equal(t, handoff, token.Handoff)
						require.Equal(t, "step2", token.StepID)
						require.NotEqual(t, "old", token.Stamp)
					}
				})
			}
		}
	}
}
