package handlers

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/kandev/kandev/internal/steptelemetry"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

// Test-only contract coverage of the real service's external-ID outcomes.
// @covers AC-TASKS-SELF-SIBLING-001.4 AC-TASKS-SELF-SIBLING-002.2 AC-TASKS-SELF-SIBLING-002.3
func TestMCPCreateTaskSelfPlacementOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		settled, differentParent bool
	}{
		{name: "pending"},
		{name: "settled", settled: true},
		{name: "pending with another parent", differentParent: true},
		{name: "settled with another parent", settled: true, differentParent: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := newSelfPlacementFixture(t)
				selfPlacementWorkflowSteps(t, f, "")
				actualParent := f.parent.ID
				if tc.differentParent {
					other := &models.Task{WorkspaceID: f.workspace.ID, WorkflowID: f.workflow.ID, Title: "Other root"}
					require.NoError(t, f.repo.CreateTask(f.ctx, other))
					actualParent = other.ID
				}
				originalCtx := steptelemetry.WithAttribution(f.ctx, steptelemetry.Attribution{
					ActorKind: steptelemetry.ActorAgent, ActorID: "original-creator", SessionID: "original-creator",
				})
				require.NoError(t, f.repo.CreateTaskSession(f.ctx, &models.TaskSession{
					ID: "original-creator", TaskID: f.parent.ID, State: models.TaskSessionStateWaitingForInput,
				}))
				original, err := f.svc.CreateTask(originalCtx, &service.CreateTaskRequest{
					ParentID: actualParent, WorkspaceID: f.workspace.ID, WorkflowID: f.workflow.ID,
					WorkflowStepID: "start-" + f.workflow.ID, Title: "Existing work", Description: "Original prompt",
					ExternalID: "existing-work", Metadata: map[string]interface{}{models.MetaKeyAgentProfileID: "original-profile"},
				})
				require.NoError(t, err)
				if tc.settled {
					settled, _, err := f.svc.SettleExternalID(f.ctx, original.Task.ID, original.Task.ExternalID)
					require.NoError(t, err)
					require.True(t, settled)
				}
				before, err := f.svc.GetTask(f.ctx, original.Task.ID)
				require.NoError(t, err)
				beforeCount := f.taskCount(t)
				beforeLedger := ledgerRowsForTask(t, f.repo, original.Task.ID)
				launcher := newMockSessionLauncher()
				f.h.sessionLauncher = launcher
				f.svc.SetWorkspacePolicyAttacher(selfPlacementPolicyAttacher(func(context.Context, string, string, service.WorkspacePolicy) error {
					t.Error("existing task must not have workspace policy reapplied")
					return nil
				}))
				payload := selfPlacementCreatePayload(f, f.child.ID)
				payload["external_id"], payload["start_agent"], payload["description"] = "existing-work", true, "Retry prompt"
				result := selfPlacementResult(t, f.create(t, f.caller(), payload))
				require.Equal(t, original.Task.ID, result.ID)
				require.True(t, result.Deduplicated)
				require.Equal(t, tc.settled, result.CreationComplete)
				require.Equal(t, actualParent, result.ParentID)
				require.NotNil(t, result.ParentResolution)
				require.Equal(t, f.parent.ID, result.ParentResolution.ResolvedParentID)
				require.Equal(t, f.child.ID, result.ParentResolution.RequestedParentID)
				require.Contains(t, result.ParentResolution.Message, "Kanban subtask depth limit")
				require.Contains(t, result.ParentResolution.Message, "without creation or reparenting")
				require.NotContains(t, result.ParentResolution.Message, "Created a sibling")
				after, err := f.svc.GetTask(f.ctx, original.Task.ID)
				require.NoError(t, err)
				require.Equal(t, before, after)
				require.Equal(t, beforeCount, f.taskCount(t))
				require.Equal(t, beforeLedger, ledgerRowsForTask(t, f.repo, original.Task.ID))
				synctest.Wait()
				require.Nil(t, launcher.getRequest())
			})
		})
	}
}

// @covers AC-TASKS-SELF-SIBLING-001.4 AC-TASKS-SELF-SIBLING-002.3
func TestMCPCreateTaskSelfPlacementIdentityLost(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newSelfPlacementFixture(t)
		launcher := newMockSessionLauncher()
		f.h.sessionLauncher = launcher
		f.svc.SetWorkspacePolicyAttacher(selfPlacementPolicyAttacher(func(ctx context.Context, _, parentID string, _ service.WorkspacePolicy) error {
			require.Equal(t, f.parent.ID, parentID)
			released, err := f.repo.ReleaseTaskExternalID(ctx, f.workspace.ID, "released-during-creation")
			require.NotNil(t, released)
			return err
		}))
		payload := selfPlacementCreatePayload(f, f.child.ID)
		payload["external_id"], payload["start_agent"], payload["description"] = " released-during-creation ", true, "Do this work"
		result := selfPlacementResult(t, f.create(t, f.caller(), payload))
		require.False(t, result.Deduplicated)
		require.True(t, result.CreationComplete)
		require.Empty(t, result.ExternalID)
		require.Equal(t, f.parent.ID, result.ParentID)
		require.NotNil(t, result.ParentResolution)
		require.Contains(t, result.ParentResolution.Message, "Created a sibling")
		require.Equal(t, 3, f.taskCount(t))
		synctest.Wait()
		require.Nil(t, launcher.getRequest())
	})
}

// @covers AC-TASKS-SELF-SIBLING-001.3 AC-TASKS-SELF-SIBLING-002.3
func TestMCPCreateTaskSelfPlacementFailures(t *testing.T) {
	for _, name := range []string{"repository", "blocker", "workspace attachment"} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := newSelfPlacementFixture(t)
				launcher := newMockSessionLauncher()
				f.h.sessionLauncher = launcher
				f.svc.SetBlockerRepository(&memBlockerRepo{})
				payload := selfPlacementCreatePayload(f, f.child.ID)
				payload["start_agent"], payload["description"] = true, "Must not launch"
				switch name {
				case "repository":
					payload["workspace_mode"] = "new_workspace"
					payload["repositories"] = []mcpRepositoryInput{{RepositoryID: unknownReferenceUUID}}
				case "blocker":
					payload["blocked_by"] = []string{unknownReferenceUUID}
				case "workspace attachment":
					f.svc.SetWorkspacePolicyAttacher(selfPlacementPolicyAttacher(func(context.Context, string, string, service.WorkspacePolicy) error {
						return errors.New("workspace attachment unavailable")
					}))
				}
				resp := f.create(t, f.caller(), payload)
				require.Equal(t, ws.MessageTypeError, resp.Type, string(resp.Payload))
				require.NotContains(t, string(resp.Payload), "parent_resolution")
				require.Equal(t, 2, f.taskCount(t))
				synctest.Wait()
				require.Nil(t, launcher.getRequest())
			})
		})
	}
}

// @covers AC-TASKS-SELF-SIBLING-002.3
func TestMCPCreateTaskSelfPlacementLaunch(t *testing.T) {
	for _, tc := range []struct {
		name           string
		start, blocked bool
		deferStart     *bool
		wantLaunch     bool
		wantDeferred   bool
	}{
		{name: "manual"},
		{name: "launch once", start: true, wantLaunch: true},
		{name: "defer to blocker", start: true, blocked: true, wantDeferred: true},
		{name: "blocked without automatic start", start: true, blocked: true, deferStart: new(bool)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := newSelfPlacementFixture(t)
				f.svc.SetBlockerRepository(&memBlockerRepo{})
				launcher := newMockSessionLauncher()
				f.h.sessionLauncher = launcher
				payload := selfPlacementCreatePayload(f, f.child.ID)
				payload["start_agent"], payload["description"], payload["external_id"] = tc.start, "Launch prompt", "launch-work"
				if tc.blocked {
					payload["blocked_by"] = []string{f.child.ID}
				}
				if tc.deferStart != nil {
					payload["start_when_unblocked"] = *tc.deferStart
				}
				result := selfPlacementResult(t, f.create(t, f.caller(), payload))
				require.Equal(t, f.parent.ID, result.ParentID)
				require.Equal(t, tc.wantDeferred, result.StartWhenUnblocked)
				synctest.Wait()
				request := launcher.getRequest()
				require.Equal(t, tc.wantLaunch, request != nil)
				if request != nil {
					require.Equal(t, result.ID, request.TaskID)
					require.Equal(t, "Launch prompt", request.Prompt)
					require.Equal(t, "profile-1", request.AgentProfileID)
				}
				task, err := f.svc.GetTask(f.ctx, result.ID)
				require.NoError(t, err)
				require.Equal(t, tc.wantDeferred, models.HasStartWhenUnblockedIntent(task))
				if tc.wantDeferred {
					launch := task.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
					require.Equal(t, "Launch prompt", launch["prompt"])
					require.Equal(t, "profile-1", launch["agent_profile_id"])
				}
				blockers, err := f.svc.GetBlockers(f.ctx, task.ID)
				require.NoError(t, err)
				if tc.blocked {
					require.Equal(t, []string{f.child.ID}, blockers)
				} else {
					require.Empty(t, blockers)
				}
				retry := selfPlacementResult(t, f.create(t, f.caller(), payload))
				require.Equal(t, result.ID, retry.ID)
				require.True(t, retry.Deduplicated)
				synctest.Wait()
				launcher.mu.Lock()
				calls := len(launcher.requests)
				launcher.mu.Unlock()
				if tc.wantLaunch {
					require.Equal(t, 1, calls)
				} else {
					require.Zero(t, calls)
				}
			})
		})
	}
}
