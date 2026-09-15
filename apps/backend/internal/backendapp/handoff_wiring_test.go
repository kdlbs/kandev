package backendapp

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	agentsettingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	officeruntime "github.com/kandev/kandev/internal/office/runtime"
	"github.com/kandev/kandev/internal/orchestrator"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// newHandoffOnlyProfileExecutionResolver builds a real, non-nil
// ProfileExecutionResolver whose backing profile store knows about
// validProfileID only. ValidateProfile therefore succeeds for validProfileID
// and fails for any other profile id, including a workflow step's stale
// pinned profile — mirroring
// orchestrator.newExplicitOnlyProfileExecutionResolver, which is unexported
// and unreachable from this package.
func newHandoffOnlyProfileExecutionResolver(t *testing.T, validProfileID string) *agentruntime.ProfileExecutionResolver {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo, cleanup, err := agentsettingsstore.Provide(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cleanup() })
	ctx := context.Background()
	if err := repo.CreateAgent(ctx, &agentsettingsmodels.Agent{ID: "handoff-agent", Name: "handoff-agent"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateAgentProfile(ctx, &agentsettingsmodels.AgentProfile{
		ID: validProfileID, AgentID: "handoff-agent", Name: "Valid", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	return agentruntime.NewProfileExecutionResolver(repo, dynamicruntime.NewEngine(), true)
}

// TestHandoffLaunchAdapter_ProfileExplicitBypassesStepPinnedProfile is
// AC-CROSS-WORKSPACE-TASK-HANDOFF-PROFILES-001.9's launch-dispatch half:
// handoffLaunchAdapter.LaunchSession must set ProfileExplicit: true on the
// orchestrator request it builds, so the delivery task's launch uses the
// caller-selected profile exactly as supplied rather than being overridden
// by (and preflight-validated against) the destination step's own pinned
// profile. Deleting that field from the adapter would leave every other
// handoff test green (they never wire a step with a stale pin) while
// silently reintroducing the failure this test targets.
func TestHandoffLaunchAdapter_ProfileExplicitBypassesStepPinnedProfile(t *testing.T) {
	const explicitProfileID = "handoff-explicit-profile"
	const stepPinnedProfileID = "stale-step-pinned-profile" // deliberately never registered

	h := newRoutineCronHarness(t)
	h.orchestrator.SetProfileExecutionResolver(newHandoffOnlyProfileExecutionResolver(t, explicitProfileID))

	ctx := context.Background()
	now := time.Now().UTC()
	const workflowID = "handoff-adapter-wf"
	const stepID = "handoff-adapter-step"
	const taskID = "handoff-adapter-task"
	if err := h.taskRepo.CreateWorkflow(ctx, &taskmodels.Workflow{
		ID: workflowID, WorkspaceID: h.workspaceID, Name: "Handoff adapter WF", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := h.workflowSvc.CreateStep(ctx, &wfmodels.WorkflowStep{
		ID: stepID, WorkflowID: workflowID, Name: "Delivery", Position: 0, AgentProfileID: stepPinnedProfileID,
	}); err != nil {
		t.Fatalf("create step: %v", err)
	}
	if err := h.taskRepo.CreateTask(ctx, &taskmodels.Task{
		ID: taskID, WorkspaceID: h.workspaceID, WorkflowID: workflowID, WorkflowStepID: stepID,
		Title: "Delivery task", Description: "do the delivery work", State: v1.TaskStateCreated,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	adapter := handoffLaunchAdapter{svc: h.orchestrator}
	if err := adapter.LaunchSession(ctx, officeruntime.HandoffLaunchRequest{
		TaskID:         taskID,
		AgentProfileID: explicitProfileID,
		WorkflowStepID: stepID,
		Prompt:         "do the delivery work",
	}); err != nil {
		t.Fatalf("LaunchSession() error = %v, want nil (ProfileExplicit must bypass the step's stale pinned profile)", err)
	}

	launched := h.agentMgr.awaitLaunch(t)
	if launched.AgentProfileID != explicitProfileID {
		t.Errorf("launched agent profile = %q, want %q (the caller-supplied profile, not the step's pin)",
			launched.AgentProfileID, explicitProfileID)
	}
}

// TestHandoffLaunchAdapter_ControlWithoutProfileExplicitFailsOnStalePin is
// the control for the test above: without ProfileExplicit, the same stale
// step-pinned profile fails orchestrator.Service's own preflight validation
// before a launch is ever attempted. This proves the adapter's
// ProfileExplicit: true is load-bearing, not incidental — removing it would
// reproduce this failure for every handoff whose destination step carries an
// invalid or unrelated pinned profile.
func TestHandoffLaunchAdapter_ControlWithoutProfileExplicitFailsOnStalePin(t *testing.T) {
	const explicitProfileID = "handoff-explicit-profile"
	const stepPinnedProfileID = "stale-step-pinned-profile"

	h := newRoutineCronHarness(t)
	h.orchestrator.SetProfileExecutionResolver(newHandoffOnlyProfileExecutionResolver(t, explicitProfileID))

	ctx := context.Background()
	now := time.Now().UTC()
	const workflowID = "handoff-adapter-control-wf"
	const stepID = "handoff-adapter-control-step"
	const taskID = "handoff-adapter-control-task"
	if err := h.taskRepo.CreateWorkflow(ctx, &taskmodels.Workflow{
		ID: workflowID, WorkspaceID: h.workspaceID, Name: "Handoff adapter control WF", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := h.workflowSvc.CreateStep(ctx, &wfmodels.WorkflowStep{
		ID: stepID, WorkflowID: workflowID, Name: "Delivery", Position: 0, AgentProfileID: stepPinnedProfileID,
	}); err != nil {
		t.Fatalf("create step: %v", err)
	}
	if err := h.taskRepo.CreateTask(ctx, &taskmodels.Task{
		ID: taskID, WorkspaceID: h.workspaceID, WorkflowID: workflowID, WorkflowStepID: stepID,
		Title: "Delivery task", Description: "do the delivery work", State: v1.TaskStateCreated,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	_, err := h.orchestrator.LaunchSession(ctx, &orchestrator.LaunchSessionRequest{
		TaskID:         taskID,
		Intent:         orchestrator.IntentStart,
		AgentProfileID: explicitProfileID,
		WorkflowStepID: stepID,
		Prompt:         "do the delivery work",
		// ProfileExplicit deliberately omitted (false).
	})
	if err == nil {
		t.Fatal("LaunchSession() error = nil, want the preflight to reject the step's stale pinned profile")
	}
}
