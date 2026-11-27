package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/automation"
	commonlogger "github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// autoStartOwnedStep is the destination step a feeder pull promotes a task
// into: it carries on_enter auto_start_agent, which is the trigger that
// races the automation's own start.
func autoStartOwnedStep() *wfmodels.WorkflowStep {
	return &wfmodels.WorkflowStep{
		ID: "step-work", WorkflowID: "wf1", Name: "Work", Position: 0,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
		},
	}
}

func seedAutoStartOwnedTask(t *testing.T, taskID string, metadata map[string]interface{}) *sqliterepo.Repository {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}))
	requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}))
	requireNoError(t, repo.CreateTask(ctx, &models.Task{
		ID:             taskID,
		WorkspaceID:    "ws1",
		WorkflowID:     "wf1",
		WorkflowStepID: "step-work",
		Title:          "Automation run",
		Description:    "prompt",
		State:          v1.TaskStateCreated,
		Metadata:       metadata,
		CreatedAt:      now,
		UpdatedAt:      now,
	}))
	return repo
}

// TestAutoStartTaskForLoadedStepHonorsAutomationStartOwnership pins the
// single-start-owner fix for the feeder double-start: a task whose initial
// start is owned by the automation run that created it must not be launched by
// the workflow auto-start chokepoint, while an otherwise identical task
// without the ownership marker still launches.
//
// @covers AC-AGENTS-SESSION-CEILING-001.10
func TestAutoStartTaskForLoadedStepHonorsAutomationStartOwnership(t *testing.T) {
	ctx := context.Background()
	step := autoStartOwnedStep()

	t.Run("skips a task whose start is owned by its automation run", func(t *testing.T) {
		repo := seedAutoStartOwnedTask(t, "t-owned", map[string]interface{}{
			models.MetaKeyAutomationStartOwned: true,
			models.MetaKeyAgentProfileID:       "automation-profile",
		})
		stepGetter := newMockStepGetter()
		stepGetter.steps[step.ID] = step
		svc := createTestServiceWithScheduler(repo, stepGetter, newMockTaskRepo(), failIfLaunched(t))
		core, logs := observer.New(zapcore.InfoLevel)
		log, err := commonlogger.NewFromZap(zap.New(core))
		requireNoError(t, err)
		svc.logger = log

		task, err := repo.GetTask(ctx, "t-owned")
		requireNoError(t, err)
		svc.autoStartTaskForLoadedStep(ctx, task, step, "task.moved", false, 0, false)

		// The launch itself runs in a goroutine; the line logged just before
		// it is dispatched shows synchronously whether a launch was started.
		if n := logs.FilterMessage("task.moved: starting task (no session, auto-start step)").Len(); n != 0 {
			t.Fatalf("automation-owned task started %d workflow auto-start launch(es), want 0", n)
		}
	})

	t.Run("control: launches a task without the ownership marker", func(t *testing.T) {
		repo := seedAutoStartOwnedTask(t, "t-plain", map[string]interface{}{
			models.MetaKeyAgentProfileID: "automation-profile",
		})
		stepGetter := newMockStepGetter()
		stepGetter.steps[step.ID] = step
		taskRepo := newMockTaskRepo()
		taskRepo.tasks["t-plain"] = &v1.Task{
			ID: "t-plain", WorkspaceID: "ws1", WorkflowID: "wf1",
			Description: "prompt", State: v1.TaskStateCreated,
			Metadata: map[string]interface{}{models.MetaKeyAgentProfileID: "automation-profile"},
		}
		launched := make(chan struct{}, 1)
		agentMgr := &mockAgentManager{
			launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
				launched <- struct{}{}
				return &executor.LaunchAgentResponse{AgentExecutionID: "exec-1"}, nil
			},
		}
		svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentMgr)

		task, err := repo.GetTask(ctx, "t-plain")
		requireNoError(t, err)
		svc.autoStartTaskForLoadedStep(ctx, task, step, "task.moved", false, 0, false)

		select {
		case <-launched:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for the control task to auto-start")
		}
	})
}

// TestCreateAutomationTaskStampsStartOwnership pins the create-time half of
// the fix: the automation must mark the task it creates as automation-owned
// before CreateTask runs the feeder pull, so the pull's task.moved cannot
// launch it first.
//
// @covers AC-AGENTS-SESSION-CEILING-001.10
func TestCreateAutomationTaskStampsStartOwnership(t *testing.T) {
	repo := setupTestRepo(t)
	creator := &stubReviewTaskCreator{task: &models.Task{ID: "t-created"}}
	autoSvc := &stubAutomationService{automation: &automation.Automation{
		ID: "a-1", WorkspaceID: "ws-1", Name: "nightly sweep", Prompt: "sweep",
		WorkflowID: "wf-1", WorkflowStepID: "step-1", Enabled: true,
	}}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.SetAutomationService(autoSvc)
	svc.reviewTaskCreator = creator
	seedAutomationWorkspaceRepo(t, repo, "ws-1")

	svc.createAutomationTask(context.Background(), &automation.AutomationTriggeredEvent{
		AutomationID: "a-1", TriggerID: "trg-1", TriggerType: automation.TriggerTypeScheduled,
	})

	requireNotNil(t, creator.got, "expected the automation to create its task")
	if !creator.got.AutomationStartOwned {
		t.Fatal("automation task request does not reserve the task's start for the dispatch")
	}
}

// TestReleaseAutomationStartOwnershipPublishesTaskUpdate pins that clearing
// the marker reaches connected clients, and that a dispatch whose context is
// already cancelled still clears it.
//
// @covers AC-AGENTS-SESSION-CEILING-001.10
func TestReleaseAutomationStartOwnershipPublishesTaskUpdate(t *testing.T) {
	repo := setupTestRepo(t)
	seedAutomationTask(t, repo, "t-release", models.TaskOriginAutomationRun, false)
	require.NoError(t, repo.SetTaskMetadataKey(context.Background(), "t-release", models.MetaKeyAutomationStartOwned, "release-owner"))
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	taskEvents := &recordingTaskUpdatedPublisher{}
	svc.SetTaskEventPublisher(taskEvents)

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	svc.releaseAutomationStartOwnership(cancelled, "t-release", "release-owner")

	task, err := repo.GetTask(context.Background(), "t-release")
	require.NoError(t, err)
	require.False(t, models.AutomationStartOwned(task.Metadata), "the marker must be cleared")
	require.Equal(t, []string{"t-release"}, taskEvents.updatedTaskIDs, "clearing the marker publishes task.updated")

	svc.releaseAutomationStartOwnership(context.Background(), "t-release", "release-owner")
	require.Len(t, taskEvents.updatedTaskIDs, 1, "a marker that is already gone publishes nothing")
}

// TestStartupRetainsUnresolvedAutomationStartOwnership covers a process that
// stopped after creating an automation task but before its dispatch resolved:
// restart alone supplies no proof that its owner terminated.
//
// @covers AC-AGENTS-SESSION-CEILING-001.10
func TestStartupRetainsUnresolvedAutomationStartOwnership(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	seedAutomationTask(t, repo, "t-abandoned", models.TaskOriginAutomationRun, false)
	seedAutomationTask(t, repo, "t-unmarked", models.TaskOriginAutomationRun, false)
	require.NoError(t, repo.SetTaskMetadataKey(ctx, "t-abandoned", models.MetaKeyAutomationStartOwned, true))
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	taskEvents := &recordingTaskUpdatedPublisher{}
	svc.SetTaskEventPublisher(taskEvents)

	svc.releaseAbandonedAutomationStartOwnership(ctx)

	task, err := repo.GetTask(ctx, "t-abandoned")
	require.NoError(t, err)
	require.True(t, models.AutomationStartOwned(task.Metadata), "restart alone cannot prove the dispatch terminated")
	require.Empty(t, taskEvents.updatedTaskIDs, "unresolved ownership is not republished as released")
}

func requireNotNil(t *testing.T, value interface{}, msg string) {
	t.Helper()
	if value == nil {
		t.Fatal(msg)
	}
}
