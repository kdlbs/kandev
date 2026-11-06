package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/statussummary"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestCopilotPluginStartupFailureProjection(t *testing.T) {
	t.Run("ordinary launch persists safe recovery evidence through reload", func(t *testing.T) {
		assertCopilotPluginStartupFailureProjection(t, "ordinary", false)
	})
	t.Run("MCP start_agent launch uses the same durable projection", func(t *testing.T) {
		assertCopilotPluginStartupFailureProjection(t, "mcp-start-agent", false)
	})
	t.Run("MCP initial creation prompt prepare/start uses the durable projection", func(t *testing.T) {
		assertCopilotPluginStartupFailureProjection(t, "mcp-initial-create-prompt", false)
	})
	t.Run("late failure from a superseded attempt is fenced", func(t *testing.T) {
		assertCopilotPluginStartupFailureProjection(t, "ordinary", true)
	})
}

func assertCopilotPluginStartupFailureProjection(t *testing.T, scenario string, stale bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	const (
		taskID            = "copilot-plugin-startup-task"
		profileID         = "copilot-profile"
		executionID       = "copilot-plugin-startup-execution"
		pluginSecret      = "private-plugin-id"
		pathSecret        = "/Users/alice/.config/copilot/plugins/private"
		privateDiagnostic = "private-provider-diagnostic-427"
	)
	repo := setupTestRepo(t)
	seedAutoStartGateTask(t, repo, taskID)
	task, err := repo.GetTask(ctx, taskID)
	require.NoError(t, err)
	task.Description = "inspect the selected plugins"
	if scenario == "mcp-initial-create-prompt" {
		task.WorkflowStepID = "step-explicit"
	}
	require.NoError(t, repo.UpdateTask(ctx, task))
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{
		ID: taskID, WorkspaceID: "ws1", Title: "Copilot plugin startup", Description: task.Description,
		State: v1.TaskStateCreated,
	}

	startupFailure := &lifecycle.BootstrapFailure{
		Operation:      models.AgentErrorCauseOperationStart,
		Code:           models.AgentErrorCauseCodePluginPreparationFailed,
		Reason:         "invalid_inventory",
		Detail:         models.AgentErrorCausePluginPreparationDetail,
		RuntimeVersion: "1.2.3+build.4",
		Cause:          errors.New("plugin " + pluginSecret + " at " + pathSecret + ": " + privateDiagnostic),
	}
	startEntered := make(chan string, 1)
	failureSettled := make(chan struct{})
	var finishOnce sync.Once
	releaseFailure := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(releaseFailure) }) })
	manager := &mockAgentManager{
		repoForExecutionLookup: repo,
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			require.True(t, req.StartAgent)
			seedExecutorRunning(t, repo, req.SessionID, taskID, executionID)
			return &executor.LaunchAgentResponse{
				AgentExecutionID: executionID,
				Status:           v1.AgentStatusStarting,
				WorkspacePath:    t.TempDir(),
			}, nil
		},
		startAgentProcessFunc: func(startCtx context.Context, _ string) error {
			go func() {
				<-startCtx.Done()
				finishOnce.Do(func() { close(failureSettled) })
			}()
			startEntered <- taskID
			if stale {
				select {
				case <-releaseFailure:
				case <-startCtx.Done():
					return startCtx.Err()
				}
			}
			return startupFailure
		},
	}
	stepGetter := newMockStepGetter()
	if scenario == "mcp-initial-create-prompt" {
		stepGetter.steps[task.WorkflowStepID] = &wfmodels.WorkflowStep{
			ID: task.WorkflowStepID, WorkflowID: "wf1", Name: "Explicit",
		}
	}
	svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, manager)
	svc.messageCreator = newServiceBackedMessageCreator(repo)
	failureHandled := make(chan bool, 1)
	type transitionResult struct {
		changed bool
		state   models.TaskSessionState
		err     error
	}
	projectionResults := make(chan transitionResult, 1)
	svc.executor.SetOnAgentStartFailed(func(callbackCtx context.Context, callbackTaskID, sessionID, callbackExecutionID string, startErr error, fromResume bool) bool {
		handled := svc.handleAgentStartFailed(callbackCtx, callbackTaskID, sessionID, callbackExecutionID, startErr, fromResume)
		failureHandled <- handled
		return handled
	})
	svc.executor.SetOnBootstrapFailureTransition(func(
		callbackCtx context.Context,
		callbackTaskID, sessionID, callbackExecutionID string,
		expectedState models.TaskSessionState,
		expectedStamp, expectedStartAttemptID string,
		failure models.LastAgentError,
	) (bool, models.TaskSessionState, error) {
		changed, finalState, transitionErr := svc.transitionBootstrapFailure(
			callbackCtx, callbackTaskID, sessionID, callbackExecutionID,
			expectedState, expectedStamp, expectedStartAttemptID, failure,
		)
		projectionResults <- transitionResult{changed: changed, state: finalState, err: transitionErr}
		return changed, finalState, transitionErr
	})
	svc.executor.SetOnBootstrapFailureMessageRepair(svc.persistBootstrapFailureMessage)

	eventBus := bus.NewMemoryEventBus(testLogger())
	projector := statussummary.NewProjector(statussummary.ProjectorConfig{
		Store:    repo,
		EventBus: eventBus,
		ResolveWorkspace: func(loadCtx context.Context, id string) (string, error) {
			loaded, loadErr := repo.GetTask(loadCtx, id)
			if loadErr != nil {
				return "", loadErr
			}
			if loaded == nil {
				return "", nil
			}
			return loaded.WorkspaceID, nil
		},
		RetryBackoff: func(context.Context, int) error { return nil },
	})
	require.NoError(t, projector.Start(ctx))
	svc.eventBus = eventBus
	t.Cleanup(func() {
		projector.Close()
		eventBus.Close()
	})

	var summaryEvent statussummary.SummaryUpdated
	_, err = eventBus.Subscribe(events.TaskStatusSummaryUpdated, func(_ context.Context, event *bus.Event) error {
		if update, ok := event.Data.(statussummary.SummaryUpdated); ok && update.TaskID == taskID {
			summaryEvent = update
		}
		return nil
	})
	require.NoError(t, err)

	request := &LaunchSessionRequest{
		TaskID: taskID, Intent: IntentStart, AgentProfileID: profileID,
		ExecutorID: models.ExecutorIDWorktree, Prompt: task.Description,
	}
	if scenario == "ordinary" {
		request.ProfileExplicit = true
	}
	var response *LaunchSessionResponse
	if scenario == "mcp-initial-create-prompt" {
		// This mirrors launchAutoStartTask's two-phase request when a new task
		// has an explicit workflow step and non-empty creation prompt.
		prepared, prepareErr := svc.LaunchSession(ctx, &LaunchSessionRequest{
			TaskID: taskID, Intent: IntentPrepare, AgentProfileID: profileID,
			ExecutorID: models.ExecutorIDWorktree, WorkflowStepID: task.WorkflowStepID,
			InitialPromptPreview: models.NewInitialPromptPreview(strings.TrimSpace(task.Description), nil),
			DeferredStart:        true,
		})
		require.NoError(t, prepareErr)
		require.NotNil(t, prepared)
		require.NotEmpty(t, prepared.SessionID)
		response, err = svc.LaunchSession(ctx, &LaunchSessionRequest{
			TaskID: taskID, Intent: IntentStartCreated, SessionID: prepared.SessionID,
			AgentProfileID: profileID, ExecutorID: models.ExecutorIDWorktree,
			WorkflowStepID: task.WorkflowStepID, Prompt: task.Description,
			InitialCreatePrompt: true,
		})
	} else {
		// launchAutoStartTask's normal create_task request is IntentStart with
		// AutoStart omitted. Only the ordinary UI launch carries ProfileExplicit.
		response, err = svc.LaunchSession(ctx, request)
	}
	require.NoError(t, err)
	require.NotNil(t, response)
	require.NotEmpty(t, response.SessionID)
	select {
	case startedTaskID := <-startEntered:
		require.Equal(t, taskID, startedTaskID)
	case <-ctx.Done():
		t.Fatal("timed out waiting for the runtime start callback")
	}

	if stale {
		session, loadErr := repo.GetTaskSession(ctx, response.SessionID)
		require.NoError(t, loadErr)
		attemptID := models.StringFromAny(session.Metadata[models.SessionMetaKeyAgentStartAttemptID])
		require.NotEmpty(t, attemptID)
		require.NoError(t, repo.SetSessionMetadataKey(
			ctx, response.SessionID, models.SessionMetaKeyAgentStartAttemptID, "successor-attempt",
		))
		releaseOnce.Do(func() { close(releaseFailure) })
	}
	select {
	case <-failureSettled:
	case <-ctx.Done():
		t.Fatal("timed out waiting for the executor failure handling to finish")
	}

	if stale {
		current, readErr := repo.GetTaskSession(ctx, response.SessionID)
		require.NoError(t, readErr)
		require.NotNil(t, current)
		require.Equal(t, models.TaskSessionStateStarting, current.State)
		_, hasError := models.LoadLastAgentError(current.Metadata)
		require.False(t, hasError)
		rows, readErr := repo.LoadTaskStatusSummaries(ctx, []string{taskID})
		require.NoError(t, readErr)
		if summary := rows[taskID]; summary != nil {
			require.Nil(t, summary.ActiveError)
		}
		messages, readErr := repo.ListMessages(ctx, response.SessionID)
		require.NoError(t, readErr)
		for _, message := range messages {
			require.NotContains(t, message.Content, pluginSecret)
			require.NotContains(t, message.Content, pathSecret)
			require.NotContains(t, message.Content, privateDiagnostic)
		}
		return
	}

	var reloaded *models.TaskSession
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		reloaded, err = repo.GetTaskSession(ctx, response.SessionID)
		if err == nil && reloaded != nil && reloaded.State == models.TaskSessionStateFailed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.NoError(t, err)
	require.NotNil(t, reloaded)
	if reloaded.State != models.TaskSessionStateFailed {
		running, runningErr := repo.GetExecutorRunningBySessionID(ctx, response.SessionID)
		var handled any = "callback not called"
		select {
		case value := <-failureHandled:
			handled = value
		default:
		}
		var result any = "transition result unavailable"
		select {
		case value := <-projectionResults:
			result = value
		default:
		}
		t.Fatalf("session state = %s, want FAILED; execution=%q attempt=%q running=%+v runningErr=%v metadata=%#v start-failure-handled=%v transition=%+v", reloaded.State, reloaded.AgentExecutionID, reloaded.Metadata[models.SessionMetaKeyAgentStartAttemptID], running, runningErr, reloaded.Metadata, handled, result)
	}

	reloaded, err = repo.GetTaskSession(ctx, response.SessionID)
	require.NoError(t, err)
	lastError, ok := models.LoadLastAgentError(reloaded.Metadata)
	require.True(t, ok)
	require.Equal(t, models.LaunchErrorPhaseBootstrap, lastError.Phase)
	require.Len(t, lastError.Causes, 1)
	require.Equal(t, models.AgentErrorCauseCodePluginPreparationFailed, lastError.Causes[0].Code)
	require.Equal(t, "invalid_inventory", lastError.Causes[0].Reason)
	require.Equal(t, "1.2.3+build.4", lastError.Causes[0].RuntimeVersion)
	require.Equal(t, models.AgentErrorCausePluginPreparationDetail, lastError.Causes[0].Detail)

	rows, err := repo.LoadTaskStatusSummaries(ctx, []string{taskID})
	require.NoError(t, err)
	require.NotNil(t, rows[taskID])
	require.NotNil(t, rows[taskID].ActiveError)
	require.Len(t, rows[taskID].ActiveError.Causes, 1)
	require.Equal(t, "1.2.3+build.4", rows[taskID].ActiveError.Causes[0].RuntimeVersion)
	require.Equal(t, "invalid_inventory", rows[taskID].ActiveError.Causes[0].Reason)
	require.NotNil(t, summaryEvent.Summary.ActiveError)
	require.Len(t, summaryEvent.Summary.ActiveError.Causes, 1)
	require.Equal(t, "1.2.3+build.4", summaryEvent.Summary.ActiveError.Causes[0].RuntimeVersion)

	messages, err := repo.ListMessages(ctx, response.SessionID)
	require.NoError(t, err)
	require.NotEmpty(t, messages)
	var privacySurface strings.Builder
	metadataJSON, err := json.Marshal(reloaded.Metadata)
	require.NoError(t, err)
	privacySurface.Write(metadataJSON)
	for _, message := range messages {
		privacySurface.WriteString(message.Content)
		metadataJSON, marshalErr := json.Marshal(message.Metadata)
		require.NoError(t, marshalErr)
		privacySurface.Write(metadataJSON)
	}
	for _, secret := range []string{pluginSecret, pathSecret, privateDiagnostic} {
		require.NotContains(t, privacySurface.String(), secret)
	}
}
