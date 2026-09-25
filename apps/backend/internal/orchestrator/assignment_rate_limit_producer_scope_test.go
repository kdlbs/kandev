package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestQueueOfficeAutoStartRun_RequestCarriesNoActorTypeField pins
// AC-OFFICE-ASSIGN-RATE-001.12 for queueOfficeAutoStartRun: unlike the
// other three out-of-scope producers (which build a raw JSON payload that
// simply omits actor_type), this one is a structural guarantee —
// engine.QueueRunRequest has no ActorType field at all, so no payload it
// builds could carry actor_type: "agent" even by accident. Exercises the
// real handleTaskMovedNoSession -> queueOfficeAutoStartRun path and
// inspects the request the engine.RunQueueAdapter actually received.
func TestQueueOfficeAutoStartRun_RequestCarriesNoActorTypeField(t *testing.T) {
	ctx := context.Background()

	repo := setupTestRepo(t)
	now := time.Now().UTC()
	requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}))
	requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}))
	requireNoError(t, repo.CreateTask(ctx, &models.Task{
		ID:                     "t-producer-scope",
		WorkspaceID:            "ws1",
		WorkflowID:             "wf1",
		WorkflowStepID:         "step1",
		ProjectID:              "proj1",
		Title:                  "Office Task",
		Description:            "prompt",
		State:                  v1.TaskStateCreated,
		AssigneeAgentProfileID: "assignee-profile",
		CreatedAt:              now,
		UpdatedAt:              now,
	}))

	stepGetter := newMockStepGetter()
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Work", Position: 1,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
		},
	}

	svc := createTestServiceWithAgent(repo, stepGetter, newMockTaskRepo(), failIfLaunched(t))
	calls := make(chan engine.QueueRunRequest, 1)
	svc.engineRunQueue = &fakeRunQueueAdapter{outcome: engine.QueueOutcomeQueued, calls: calls}
	svc.enginePrimary = &fakePrimaryAgentResolver{agentProfileID: "resolved-primary"}

	svc.handleTaskMovedNoSession(ctx, watcher.TaskMovedEventData{
		TaskID:   "t-producer-scope",
		ToStepID: "step2",
	})

	select {
	case req := <-calls:
		if req.Reason != officeAutoStartRunReason {
			t.Fatalf("reason = %q, want %q", req.Reason, officeAutoStartRunReason)
		}
		if actorType, present := req.Payload["actor_type"]; present {
			t.Fatalf("office auto-start payload carries an actor_type key (%v) — "+
				"QueueRunRequest has no such field, so this must never happen", actorType)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for QueueRun to be called")
	}
}
