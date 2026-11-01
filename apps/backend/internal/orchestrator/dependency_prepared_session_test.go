package orchestrator

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-TASK-DEPENDENCIES-001.1
func TestDependencyResolutionReusesPreparedPrimary(t *testing.T) {
	for _, scenario := range []string{"eligible", "full_ceiling", "workspace_default", "preparing_runtime", "initial_submission", "message", "turn", "provider_metadata", "resume_token", "harness_generation", "profile_mismatch", "executor_mismatch", "runner_mismatch", "failed", "completed", "secondary", "explicit_target", "workflow_profile", "active_sibling", "ordinary_start"} {
		t.Run(scenario, func(t *testing.T) { testDependencyPreparedSessionLaunch(t, scenario) })
	}
}

func testDependencyPreparedSessionLaunch(t *testing.T, scenario string) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const taskID = "prepared-dependent"
	seedAutoStartGateTask(t, repo, taskID)
	workspace, workspaceErr := repo.GetWorkspace(ctx, "ws1")
	require.NoError(t, workspaceErr)
	defaultExecutor := models.ExecutorIDWorktree
	workspace.DefaultExecutorID = &defaultExecutor
	require.NoError(t, repo.UpdateWorkspace(ctx, workspace))
	requestedExecutor := defaultExecutor
	if scenario == "workspace_default" {
		requestedExecutor = ""
	}
	task, err := repo.GetTask(ctx, taskID)
	require.NoError(t, err)
	task.Metadata = map[string]interface{}{
		models.MetaKeyDeferredLaunch: map[string]interface{}{
			"intent": "start", "agent_profile_id": "profile-focus-gate", "executor_id": requestedExecutor, "prompt": "recorded dependency prompt",
			models.DeferredLaunchStartWhenUnblockedKey: true,
		},
	}
	require.NoError(t, repo.UpdateTask(ctx, task))
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{ID: taskID, WorkspaceID: "ws1", Title: "Dependent", Description: "task brief", State: v1.TaskStateCreated}
	workspacePath := t.TempDir()
	prepared := make(chan struct{})
	started := make(chan struct{})
	var launches atomic.Int32
	mgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			if !req.StartAgent {
				seedExecutorRunning(t, repo, req.SessionID, taskID, "exec-dependent")
				close(prepared)
			} else {
				launches.Add(1)
			}
			return &executor.LaunchAgentResponse{AgentExecutionID: "exec-dependent", Status: v1.AgentStatusStarting, WorkspacePath: workspacePath}, nil
		},
		startAgentProcessFunc: func(context.Context, string) error { close(started); return nil },
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, mgr)
	reader := &launchDependencyReader{blocked: true}
	svc.SetTaskDependencyReader(reader)
	response, err := svc.LaunchSession(ctx, &LaunchSessionRequest{TaskID: taskID, Intent: IntentStart, AgentProfileID: "profile-focus-gate", ExecutorID: requestedExecutor, AutoStart: true})
	require.NoError(t, err)
	require.Equal(t, string(models.TaskSessionStateCreated), response.State)
	waitForWorkspaceLaunch(t, prepared)
	require.Eventually(t, func() bool {
		session, readErr := repo.GetTaskSession(ctx, response.SessionID)
		return readErr == nil && session.WorkspacePath == workspacePath
	}, 2*time.Second, 10*time.Millisecond)

	original, err := repo.GetTaskSession(ctx, response.SessionID)
	require.NoError(t, err)
	switch scenario {
	case "preparing_runtime":
		mgr.isAgentRunningFn = func(_ context.Context, sessionID string) bool { return sessionID == original.ID }
	case "initial_submission":
		submission, submissionErr := models.NewInitialPromptSubmission("accepted first prompt", false, nil)
		require.NoError(t, submissionErr)
		require.NoError(t, repo.SetSessionMetadataKey(ctx, original.ID, models.SessionMetaKeyInitialPromptSubmission, submission))
	case "message":
		require.NoError(t, repo.CreateTurn(ctx, &models.Turn{ID: "message-turn", TaskID: taskID, TaskSessionID: original.ID, StartedAt: time.Now().UTC()}))
		require.NoError(t, repo.CreateMessage(ctx, &models.Message{ID: "existing-message", TurnID: "message-turn", TaskID: taskID, TaskSessionID: original.ID, AuthorType: models.MessageAuthorUser, Type: models.MessageTypeMessage, Content: "existing conversation", CreatedAt: time.Now().UTC()}))
	case "turn":
		require.NoError(t, repo.CreateTurn(ctx, &models.Turn{ID: "prior-turn", TaskID: taskID, TaskSessionID: original.ID, StartedAt: time.Now().UTC()}))
	case "provider_metadata":
		require.NoError(t, repo.SetSessionMetadataKey(ctx, original.ID, "acp", map[string]interface{}{"session_id": "native-conversation"}))
	case "resume_token":
		running, readErr := repo.GetExecutorRunningBySessionID(ctx, original.ID)
		require.NoError(t, readErr)
		running.ResumeToken = "native-token"
		require.NoError(t, repo.UpsertExecutorRunning(ctx, running))
	case "harness_generation":
		require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{SessionID: original.ID, IncarnationID: original.QueueIncarnationID, Generation: 1, NativeSessionID: "native", AgentType: "codex-acp", CreationReason: "fresh", CreatedAt: time.Now().UTC(), CommittedAt: time.Now().UTC()}))
	case "profile_mismatch":
		original.AgentProfileID = "other-profile"
		require.NoError(t, repo.UpdateTaskSession(ctx, original))
	case "executor_mismatch":
		original.ExecutorID = "other-executor"
		require.NoError(t, repo.UpdateTaskSession(ctx, original))
	case "runner_mismatch":
		original.ExecutorProfileID = "other-runner"
		require.NoError(t, repo.UpdateTaskSession(ctx, original))
	case "completed":
		original.State = models.TaskSessionStateCompleted
		require.NoError(t, repo.UpdateTaskSession(ctx, original))
	case "failed":
		original.State = models.TaskSessionStateFailed
		require.NoError(t, repo.UpdateTaskSession(ctx, original))
	case "secondary":
		require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "idle-primary", TaskID: taskID, AgentProfileID: "other-profile", State: models.TaskSessionStateWaitingForInput, StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}))
		require.NoError(t, repo.SetSessionPrimary(ctx, "idle-primary"))
	case "explicit_target", "workflow_profile":
		step := &wfmodels.WorkflowStep{ID: "deferred-step", WorkflowID: "wf1", Position: 1, AgentProfileID: "workflow-profile"}
		getter := newMockStepGetter()
		if scenario == "explicit_target" {
			step.AgentProfileID = original.AgentProfileID
			step.SessionTarget = &wfmodels.WorkflowSessionTarget{Kind: wfmodels.WorkflowSessionTargetStep, StepID: "source-step"}
			getter.steps["source-step"] = &wfmodels.WorkflowStep{ID: "source-step", WorkflowID: "wf1", Position: 0, AgentProfileID: original.AgentProfileID, ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyNew}
		}
		getter.steps[step.ID] = step
		svc.workflowStepGetter = getter
		task.WorkflowStepID = step.ID
		require.NoError(t, repo.UpdateTask(ctx, task))
	case "active_sibling":
		require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "active-sibling", TaskID: taskID, AgentProfileID: original.AgentProfileID, State: models.TaskSessionStateRunning, StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}))
	}
	restored := make(chan struct{})
	if scenario == "active_sibling" {
		svc.repo = &dependencyRestoreStore{Repository: repo, restored: restored}
	}
	if scenario == "full_ceiling" {
		svc.sessionCeiling = newSessionCeilingController(1, nil, nil)
		require.True(t, svc.sessionCeiling.admit(ctx, admissionRequest{
			taskID: "occupier", sessionID: "occupier-session", origin: launchOriginAutomatic,
		}).admitted)
		population, populationErr := svc.sessionCeiling.population(ctx)
		require.NoError(t, populationErr)
		require.Equal(t, 1, population, "dependency must resolve while the only ceiling slot is occupied")
	}
	reader.blocked = false
	if scenario == "ordinary_start" {
		_, err = svc.LaunchSession(ctx, &LaunchSessionRequest{TaskID: taskID, Intent: IntentStart, AgentProfileID: original.AgentProfileID})
		require.NoError(t, err)
	} else {
		svc.evaluateDependentAfterPredecessorChange(ctx, taskID, "predecessor", v1.TaskStateCompleted)
	}
	if scenario == "active_sibling" {
		select {
		case <-restored:
		case <-time.After(3 * time.Second):
			t.Fatal("active sibling refusal did not restore launch intent")
		}

		sessions, readErr := repo.ListTaskSessions(ctx, taskID)
		require.NoError(t, readErr)
		require.Len(t, sessions, 2)
		require.Zero(t, launches.Load())
		current, readErr := repo.GetTask(ctx, taskID)
		require.NoError(t, readErr)
		require.True(t, models.HasStartWhenUnblockedIntent(current))
		return
	}

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("dependency resolution did not start the agent")
	}
	sessions, err := repo.ListTaskSessions(ctx, taskID)
	require.NoError(t, err)
	if scenario != "eligible" && scenario != "full_ceiling" && scenario != "workspace_default" && scenario != "preparing_runtime" {
		expectedCount := 2
		if scenario == "secondary" {
			expectedCount = 3
		}
		require.Len(t, sessions, expectedCount)
		untouched, readErr := repo.GetTaskSession(ctx, original.ID)
		require.NoError(t, readErr)
		require.Equal(t, original.State, untouched.State)
		require.Equal(t, original.QueueIncarnationID, untouched.QueueIncarnationID)
		if scenario == "initial_submission" {
			submission, exists, submissionErr := models.LoadInitialPromptSubmission(untouched.Metadata)
			require.NoError(t, submissionErr)
			require.True(t, exists)
			require.Equal(t, "accepted first prompt", submission.Content)
		}
		require.EqualValues(t, 1, launches.Load())
		return
	}
	require.Len(t, sessions, 1, "dependency start must reuse the prepared primary")
	primary, err := repo.GetPrimarySessionByTaskID(ctx, taskID)
	require.NoError(t, err)
	require.Equal(t, response.SessionID, primary.ID)
	require.Equal(t, original.QueueIncarnationID, primary.QueueIncarnationID)
	require.Contains(t, []models.TaskSessionState{models.TaskSessionStateStarting, models.TaskSessionStateRunning}, primary.State)
	require.Zero(t, launches.Load(), "prepared workspace must be reused")
	if scenario == "full_ceiling" {
		require.NotNil(t, primary.Metadata[ceilingManualOverrideMetadataKey], "dependency consumer retains its existing manual admission")
		current, readErr := repo.GetTask(ctx, taskID)
		require.NoError(t, readErr)
		require.False(t, models.HasCeilingDeferredIntent(current), "manual admission must not persist a replay launch")
		require.False(t, models.HasStartWhenUnblockedIntent(current), "the admitted dependency launch consumes its intent")
	}
}

// Observe restoration after the real deferred consumer handles a launch refusal.
type dependencyRestoreStore struct {
	*sqliterepo.Repository
	restored chan struct{}
}

func (r *dependencyRestoreStore) RestoreTaskDeferredLaunchWIPKeys(ctx context.Context, taskID string, record map[string]interface{}) error {
	err := r.Repository.RestoreTaskDeferredLaunchWIPKeys(ctx, taskID, record)
	close(r.restored)
	return err
}
