package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func assertAutomationRetryInitialStart(t *testing.T, restoredWorkspace bool, terminalFailure ...bool) {
	t.Helper()
	ctx := context.Background()
	base := setupAutomationRetentionFixture(t)
	taskID := "owned-launch-task"
	seedAutomationTask(t, base.repo, taskID, models.TaskOriginAutomationRun, false)
	task, err := base.repo.GetTask(ctx, taskID)
	require.NoError(t, err)
	task.Description = "retry prompt"
	require.NoError(t, base.repo.UpdateTask(ctx, task))
	tasks := newMockTaskRepo()
	seedMockTaskState(tasks, taskID, v1.TaskStateInProgress)
	durable := len(terminalFailure) > 2 && terminalFailure[2]
	prepared := len(terminalFailure) > 3 && terminalFailure[3]
	revoked := len(terminalFailure) > 4 && terminalFailure[4]
	var launches, starts, dispatches atomic.Int32
	var providerPrompt, sourcePrompt string
	var providerAttachments []v1.MessageAttachment
	processStarted := make(chan error, 1)
	manager := &mockAgentManager{}
	manager.getExecutionIDForSessionFunc = func(context.Context, string) (string, error) {
		if launches.Load() == 0 && !restoredWorkspace {
			return "", lifecycle.ErrNoExecutionForSession
		}
		return "owned-execution", nil
	}
	manager.launchAgentFunc = func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		launches.Add(1)
		sourcePrompt = req.TaskDescription
		providerPrompt = sourcePrompt
		launch, err := base.autoStore.GetRetryLaunch(ctx, "owned-run")
		if err != nil {
			return nil, err
		}
		if req.SessionID != launch.SessionID || req.InitialDeliverySubmissionID != launch.SubmissionID {
			return nil, errors.New("runtime received a different reserved identity")
		}
		return &executor.LaunchAgentResponse{AgentExecutionID: "owned-execution"}, nil
	}
	manager.startAgentProcessFunc = func(context.Context, string) error {
		launch, err := base.autoStore.GetRetryLaunch(ctx, "owned-run")
		if err != nil {
			processStarted <- err
			return err
		}
		if launch.State != automation.RetryLaunchDispatching || launch.ExecutionID != "owned-execution" {
			err := errors.New("harness started without the exact durable launch admission")
			processStarted <- err
			return err
		}
		if durable {
			stored, err := base.repo.ListAgentDeliverySubmissions(ctx, launch.SessionID)
			if err != nil || len(stored) != 0 {
				err = errors.Join(err, errors.New("source intent was hashed before runtime preparation"))
				processStarted <- err
				return err
			}
		}
		starts.Add(1)
		manager.mu.Lock()
		before, onAccepted := manager.initialPromptAdmissionCallback, manager.initialPromptDispatchCallback
		manager.mu.Unlock()
		if before == nil || onAccepted == nil {
			err := errors.New("provider acceptance boundaries missing")
			processStarted <- err
			return err
		}
		if prepared {
			providerPrompt += "\nprepared runtime context"
			providerAttachments = []v1.MessageAttachment{{Type: "resource", AttachmentID: "attachment", Name: "bundle-2.zip", MimeType: "application/zip", SizeBytes: 16, DeliveryMode: "prompt"}}
		}
		if revoked {
			if err := base.autoStore.RevokeRetryAdmissions(ctx, launch.AutomationID, "owned-group"); err != nil {
				processStarted <- err
				return err
			}
		}
		if err := before(ctx, providerPrompt, providerAttachments); err != nil {
			processStarted <- err
			return err
		}
		var admitted *models.AgentDeliverySubmission
		if durable {
			stored, err := base.repo.GetAgentDeliverySubmission(ctx, launch.SubmissionID)
			if err != nil || stored.State != models.DeliverySubmissionDispatching || stored.DispatchAttemptID != launch.SubmissionID {
				err = errors.Join(err, errors.New("provider dispatch preceded canonical reserved payload admission"))
				processStarted <- err
				return err
			}
			admitted = stored
		}
		if durable && prepared {
			if err := before(ctx, providerPrompt+" changed", providerAttachments); err == nil {
				err = errors.New("duplicate admission rewrote an immutable payload")
				processStarted <- err
				return err
			}
			after, err := base.repo.GetAgentDeliverySubmission(ctx, launch.SubmissionID)
			if err != nil || !bytes.Equal(admitted.Payload, after.Payload) || admitted.PayloadHash != after.PayloadHash {
				err = errors.Join(err, errors.New("rejected duplicate changed canonical payload evidence"))
				processStarted <- err
				return err
			}
		}
		dispatches.Add(1)
		onAccepted()
		return nil
	}
	runtime := &retryLaunchAgentManager{mockAgentManager: manager, durable: durable}
	runtime.configured.Store(!restoredWorkspace)
	svc := createTestServiceWithScheduler(base.repo, newMockStepGetter(), tasks, runtime)
	svc.executor.SetOnSessionStateTransition(svc.transitionTaskSessionState)
	// Provider acceptance can precede the asynchronous RUNNING transition.
	svc.executor.SetOnAgentProcessStarted(func(context.Context, string, string, string) {
		processStarted <- nil
	})
	turnBus := bus.NewMemoryEventBus(testLogger())
	if restoredWorkspace {
		_, err := turnBus.Subscribe(events.TurnStarted, func(ctx context.Context, event *bus.Event) error {
			data, ok := event.Data.(map[string]interface{})
			if !ok {
				return errors.New("turn event missing persisted identity")
			}
			sessionID, _ := data["session_id"].(string)
			launch, err := base.autoStore.GetRetryLaunch(ctx, "owned-run")
			if err != nil {
				return err
			}
			if sessionID != launch.SessionID {
				return errors.New("workspace restored for a foreign session")
			}
			return base.repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: sessionID, TaskID: taskID, SessionID: sessionID, AgentExecutionID: "owned-execution", Status: models.ExecutorRunningStatusPrepared})
		})
		require.NoError(t, err)
	}
	svc.SetTurnService(taskservice.NewService(taskservice.Repos{Tasks: base.repo, TaskRepos: base.repo, Workspaces: base.repo, Workflows: base.repo, Messages: base.repo, Turns: base.repo, Sessions: base.repo, Executors: base.repo, TaskEnvironments: base.repo}, turnBus, testLogger(), taskservice.RepositoryDiscoveryConfig{}))
	svc.SetAutomationService(base.autoSvc)
	queuedStart := len(terminalFailure) > 1 && terminalFailure[1]
	if queuedStart {
		svc.sessionCeiling = newSessionCeilingController(1, nil, nil)
		require.True(t, svc.sessionCeiling.admit(ctx, admissionRequest{
			taskID: "occupier", sessionID: "retry-queue-occupier", origin: launchOriginAutomatic, seam: "test-setup",
		}).admitted)
	}
	a := &automation.Automation{ID: "owned-automation", WorkspaceID: task.WorkspaceID, Name: "owned", Enabled: true, AgentProfileID: "profile-1", MaxConcurrentRuns: 1}
	if len(terminalFailure) > 0 && terminalFailure[0] {
		a.RetryPolicy = automation.RetryPolicy{Mode: automation.RetryModeFinite, MaxRetries: "1", DelaySeconds: "0", Backoff: automation.RetryBackoffFixed, HistoryMode: automation.RetryHistoryAttempts}
	}
	require.NoError(t, base.autoStore.CreateAutomation(ctx, a))
	run := &automation.AutomationRun{ID: "owned-run", AutomationID: a.ID, TaskID: taskID, Status: automation.RunStatusTriggered, RetryGroupID: "owned-group", RetryGroupGeneration: 1, RetryState: automation.RetryStateTriggered, RetryLaunchConfigVersion: automation.RetryLaunchConfigVersion}
	run.RetryLaunchConfigSnapshot = retrySnapshotForTest(run, a)
	run.RetryPolicySnapshot = a.RetryPolicyJSON
	require.NoError(t, base.autoStore.CreateRetryAdmission(ctx, run, &automation.RetryGroup{ID: run.RetryGroupID, AutomationID: a.ID, Generation: 1, State: automation.RetryGroupLive}))
	svc.autoStartAutomationTaskForRun(ctx, a, task, "", run.ID, automation.ThreadActionCreated, "original", true, nil)
	var queued *automation.RetryLaunch
	if queuedStart {
		queued, err = base.autoStore.GetRetryLaunch(ctx, run.ID)
		require.NoError(t, err)
		require.Equal(t, automation.RetryLaunchQueued, queued.State)
		require.True(t, svc.automationRunAwaitsLaunch(ctx, run.ID), "a prebound retry still awaits queue release")
		require.Zero(t, starts.Load())
		require.Zero(t, launches.Load())
		_, err = base.repo.GetTaskSession(ctx, queued.SessionID)
		require.Error(t, err, "the reserved identity does not fabricate a started session")
		require.NoError(t, base.autoStore.RecoverRetryLedger(ctx, time.Now().Add(24*time.Hour)))
		svc.drainDeferredCeilingLaunches(ctx)
		require.Zero(t, launches.Load(), "a still occupied ceiling cannot start the retry")
		svc.sessionCeiling.release("retry-queue-occupier")
		svc.drainDeferredCeilingLaunches(ctx)
	}
	select {
	case err := <-processStarted:
		if revoked {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("synthetic process startup did not complete")
	}
	launch, err := base.autoStore.GetRetryLaunch(ctx, run.ID)
	require.NoError(t, err)
	if revoked {
		require.True(t, launch.CancelRequested)
		require.NotEqual(t, automation.RetryLaunchAccepted, launch.State)
		require.NotEqual(t, automation.RetryLaunchStopped, launch.State, "revocation does not certify termination")
		require.EqualValues(t, 1, starts.Load())
		require.EqualValues(t, 1, launches.Load())
		require.Zero(t, dispatches.Load())
		require.Equal(t, "owned-execution", launch.ExecutionID)
		submissions, err := base.repo.ListAgentDeliverySubmissions(ctx, launch.SessionID)
		require.NoError(t, err)
		require.Empty(t, submissions, "revoked ownership rejects admission before SQL payload writes")
		return
	}
	require.Equal(t, automation.RetryLaunchAccepted, launch.State)
	if queuedStart {
		require.Equal(t, queued.SessionID, launch.SessionID)
		require.Equal(t, queued.TurnID, launch.TurnID)
		require.Equal(t, queued.SubmissionID, launch.SubmissionID)
		require.NotEqual(t, queued.Token, launch.Token)
		var replayCalls atomic.Int32
		_, err = svc.startQueuedAutomationRun(ctx, taskID, &automationRunLaunch{
			RunID: run.ID, ThreadAction: automation.ThreadActionCreated, ThreadReason: "changed", Launch: queued,
		}, func() (*executor.TaskExecution, error) {
			replayCalls.Add(1)
			return nil, errors.New("duplicate queue replay reached provider")
		})
		require.ErrorIs(t, err, automation.ErrRetryLaunchUncertain)
		require.Zero(t, replayCalls.Load())
	}
	require.EqualValues(t, 1, starts.Load())
	require.EqualValues(t, 1, dispatches.Load())
	require.EqualValues(t, 1, launches.Load())
	require.Equal(t, "owned-execution", launch.ExecutionID)
	session, err := base.repo.GetTaskSession(ctx, launch.SessionID)
	require.NoError(t, err)
	require.Equal(t, taskID, session.TaskID)
	require.Equal(t, models.TaskSessionStateRunning, session.State)
	if queuedStart {
		require.NotEqual(t, models.TaskSessionStateCancelled, session.State, "queue acceptance uncertainty must retain its owned session")
		manager.mu.Lock()
		cancellations := len(manager.stopAgentWithReasonArgs)
		manager.mu.Unlock()
		require.Zero(t, cancellations, "an acceptance error must not stop the reserved execution")
	}
	turn, err := base.repo.GetActiveTurnBySessionID(ctx, launch.SessionID)
	require.NoError(t, err)
	require.Equal(t, launch.TurnID, turn.ID)
	if durable {
		stored, err := base.repo.GetAgentDeliverySubmission(ctx, launch.SubmissionID)
		require.NoError(t, err)
		require.Equal(t, launch.SubmissionID, stored.ID)
		require.Equal(t, launch.SubmissionID, stored.DispatchAttemptID)
		require.Equal(t, launch.SessionID, stored.SessionID)
		require.Equal(t, models.DeliverySubmissionDispatching, stored.State)
		var payload agentDeliveryPromptPayload
		require.NoError(t, json.Unmarshal(stored.Payload, &payload))
		require.NotEmpty(t, providerPrompt)
		require.Equal(t, providerPrompt, payload.Text)
		require.Equal(t, providerAttachments, payload.Attachments)
		require.Equal(t, journal.SubmissionHash(stored.Payload), stored.PayloadHash)
		require.Contains(t, sourcePrompt, "Kandev Task ID: "+taskID)
		require.True(t, strings.HasSuffix(sourcePrompt, "\n\nretry prompt"))
		if prepared {
			require.Equal(t, sourcePrompt+"\nprepared runtime context", providerPrompt)
		}
	}
	for _, action := range []string{recoveryActionResume, recoveryActionFreshStart, recoveryActionContinueHistory} {
		_, recoveryErr := svc.RecoverSessionWithOptions(ctx, taskID, launch.SessionID, action, RecoverSessionOptions{SettingsPolicy: executor.ResumeSettingsPolicyStrict})
		require.ErrorIs(t, recoveryErr, automation.ErrRetryLaunchUncertain)
		currentTurn, turnErr := base.repo.GetActiveTurnBySessionID(ctx, launch.SessionID)
		require.NoError(t, turnErr)
		require.Equal(t, launch.TurnID, currentTurn.ID, "recovery must preserve uncertain retry ownership before settlement")
		require.EqualValues(t, 1, launches.Load())
	}
	submission, found, err := models.LoadInitialPromptSubmission(session.Metadata)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, models.InitialPromptSubmissionAccepted, submission.State)
	require.Equal(t, launch.SubmissionID, submission.AttemptID)
	require.NoError(t, base.autoStore.RecoverRetryLedger(ctx, time.Now().Add(24*time.Hour)))
	svc.autoStartAutomationTaskForRun(ctx, a, task, "", run.ID, automation.ThreadActionCreated, "changed", true, nil)
	require.EqualValues(t, 1, launches.Load())
	svc.rememberTurnPrompt(launch.SessionID, "retry prompt", "", false, nil)
	require.False(t, svc.handleTransientFailure(ctx, watcher.AgentEventData{TaskID: taskID, SessionID: launch.SessionID, AgentExecutionID: "owned-execution"}))
	svc.retryTransientPrompt(ctx, taskID, launch.SessionID, "owned-execution")
	require.EqualValues(t, 1, launches.Load())
	if len(terminalFailure) > 0 && terminalFailure[0] {
		svc.beginPromptAttempt(launch.SessionID, "owned-execution", 7, false)
		svc.observePromptAttempt(launch.SessionID, "owned-execution", 7, true, true)
		data := watcher.AgentEventData{TaskID: taskID, SessionID: launch.SessionID, AgentExecutionID: "owned-execution", OwnerKind: string(lifecycle.ExecutionOwnerTask), PromptGeneration: 7, PromptFailureDisposition: streams.PromptFailureDispositionRetainRuntime, ErrorMessage: "provider turn failed"}
		svc.handleAgentTurnFailed(ctx, data)
		pending, err := base.autoStore.GetRetryLaunch(ctx, run.ID)
		require.NoError(t, err)
		require.Equal(t, automation.RetryLaunchAccepted, pending.State, "a missing captured turn cannot release acceptance")
		data.TurnID = "foreign-turn"
		svc.handleAgentTurnFailed(ctx, data)
		pending, err = base.autoStore.GetRetryLaunch(ctx, run.ID)
		require.NoError(t, err)
		require.Equal(t, automation.RetryLaunchAccepted, pending.State, "a different captured turn cannot release acceptance")
		data.TurnID = launch.TurnID
		data.PromptGeneration = 6
		svc.handleAgentTurnFailed(ctx, data)
		pending, err = base.autoStore.GetRetryLaunch(ctx, run.ID)
		require.NoError(t, err)
		require.Equal(t, automation.RetryLaunchAccepted, pending.State, "a stale generation cannot release acceptance")
		data.PromptGeneration = 7
		svc.handleAgentTurnFailed(ctx, data)
		terminal, err := base.autoStore.GetRetryLaunch(ctx, run.ID)
		require.NoError(t, err)
		require.Equal(t, automation.RetryLaunchStopped, terminal.State)
		settled, err := base.autoStore.GetRun(ctx, run.ID)
		require.NoError(t, err)
		require.Equal(t, automation.RunStatusFailed, settled.Status)
		attempts, err := base.autoStore.ListRuns(ctx, a.ID, 10)
		require.NoError(t, err)
		require.Len(t, attempts, 2)
		require.Equal(t, automation.RunStatusScheduledRetry, attempts[0].Status)
		var childProtocol int
		require.NoError(t, base.db.Get(&childProtocol, `SELECT version FROM automation_retry_launch_protocols WHERE run_id=?`, attempts[0].ID))
		require.Equal(t, 1, childProtocol)
		waitForStopCall(t, manager)
		svc.dynamicSuccessorWorkers.Wait()
		require.EqualValues(t, 1, starts.Load())
		return
	}
	base.autoSvc.SetRunStopper(svc)
	var stopCalls atomic.Int32
	manager.mu.Lock()
	manager.stopAgentWithReasonFunc = func(context.Context, string, string, bool) error {
		if stopCalls.Add(1) == 1 {
			return errors.New("stop acknowledgment unavailable")
		}
		return nil
	}
	manager.mu.Unlock()
	_, err = base.autoSvc.StopRun(ctx, a.ID, run.ID)
	require.Error(t, err)
	retained, err := base.autoStore.GetRetryLaunch(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, automation.RetryLaunchAccepted, retained.State)
	require.True(t, retained.CancelRequested)
	active, err := base.autoStore.CountActiveRuns(ctx, a.ID)
	require.NoError(t, err)
	require.Equal(t, 1, active)
	_, err = base.autoStore.DeleteAutomationWithCleanup(ctx, a.ID)
	require.ErrorIs(t, err, automation.ErrRetryLaunchUncertain)
	_, err = base.autoSvc.StopRun(ctx, a.ID, run.ID)
	require.NoError(t, err)
	stopped, err := base.autoStore.GetRetryLaunch(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, automation.RetryLaunchStopped, stopped.State)
	active, err = base.autoStore.CountActiveRuns(ctx, a.ID)
	require.NoError(t, err)
	require.Zero(t, active)
	assertRetiredRetryCreationBoundaries(t, svc, base, task, run)
	require.EqualValues(t, 1, starts.Load(), "retired preparation must never reach the provider")
}
