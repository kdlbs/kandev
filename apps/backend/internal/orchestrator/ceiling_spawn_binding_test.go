package orchestrator

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
)

func TestCeilingReplay_PreservesAdditionalSpawnBinding(t *testing.T) {
	for _, preserve := range []bool{true, false} {
		t.Run(map[bool]string{true: "additional spawn", false: "ordinary selector"}[preserve], func(t *testing.T) {
			ctx := context.Background()
			f := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
			var launched *executor.LaunchAgentRequest
			launchAgent := f.agentMgr.launchAgentFunc
			f.agentMgr.launchAgentFunc = func(ctx context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
				launched = req
				return launchAgent(ctx, req)
			}
			source := &wfmodels.WorkflowStep{ID: "step-a", WorkflowID: "wf1", AgentProfileID: "profile-a"}
			f.stepGetter.steps[source.ID] = source
			require.NoError(t, f.svc.recordWorkflowSourceBinding(ctx, "t1", source, f.current))
			before, err := f.repo.GetWorkflowSessionBinding(ctx, "t1", "step:step-a")
			require.NoError(t, err)
			profile := "profile-a"
			if preserve {
				profile = "profile-b"
			}
			payload := seam1StartPayload(profile, "", "", "", "independent review", "", false, false, nil,
				startTaskOptions{ProfileExplicit: true, PreserveWorkflowBinding: preserve})
			if !preserve {
				delete(payload, "preserve_workflow_binding")
			}
			require.NoError(t, f.svc.deferCeilingRefusal(ctx, "t1", "", models.CeilingLaunchStart, payload,
				ceilingReasonRefused, 1, true, 1))
			stored := deferredLaunchOf(t, f.svc, "t1")
			deferral, err := models.ReadCeilingDeferral(stored)
			require.NoError(t, err)
			task, err := f.repo.GetTask(ctx, "t1")
			require.NoError(t, err)
			f.svc.replayCeilingDeferral(ctx, task, deferral)
			require.NotNil(t, launched, "persisted replay must reach the actual external-runtime boundary")
			require.NotEqual(t, f.current.ID, launched.SessionID)
			require.Equal(t, profile, launched.AgentProfileID)
			after, err := f.repo.GetWorkflowSessionBinding(ctx, "t1", "step:step-a")
			require.NoError(t, err)
			sessions, err := f.repo.ListTaskSessions(ctx, "t1")
			require.NoError(t, err)
			require.Len(t, sessions, 2)
			if preserve {
				require.Equal(t, before, after, "persisted/replayed additional spawn must retain workflow ownership")
				target := &wfmodels.WorkflowStep{ID: "later-review", WorkflowID: "wf1", Position: 1, SessionTarget: &wfmodels.WorkflowSessionTarget{Kind: wfmodels.WorkflowSessionTargetStep, StepID: source.ID}}
				resolved, err := f.svc.resolveWorkflowSessionTarget(ctx, "t1", target)
				require.NoError(t, err)
				require.Equal(t, f.current.ID, resolved.session.ID, "downstream target must resolve the original conversation, not the new primary/profile candidate")
				require.Equal(t, f.current.AgentProfileID, resolved.profileID)
				route := &models.WorkflowSessionRoute{OperationID: "downstream-reuse", DestinationStepID: target.ID, TargetKind: "step", TargetStepID: source.ID, AgentProfileID: resolved.profileID}
				reused, created, err := f.svc.reuseResolvedWorkflowSession(ctx, "t1", f.current, resolved.session, route, models.WorkflowProfileSessionEndPolicyPark)
				require.NoError(t, err)
				require.False(t, created)
				require.Equal(t, f.current.ID, reused.ID)
			} else {
				require.NotEqual(t, before.SessionID, after.SessionID, "ordinary selector still records its new workflow source")
			}
		})
	}
}

func TestLaunchSession_PreserveWorkflowBindingIsServerOnly(t *testing.T) {
	var req LaunchSessionRequest
	require.NoError(t, json.Unmarshal([]byte(`{"task_id":"t1","preserve_workflow_binding":true,"PreserveWorkflowBinding":true}`), &req))
	require.False(t, req.PreserveWorkflowBinding)
	req.PreserveWorkflowBinding = true
	encoded, err := json.Marshal(req)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "preserve_workflow_binding")
	require.NotContains(t, string(encoded), "PreserveWorkflowBinding")
}
