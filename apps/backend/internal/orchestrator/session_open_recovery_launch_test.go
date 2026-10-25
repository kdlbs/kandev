package orchestrator

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// @covers AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.3
// @covers AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.5
// @covers AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.7
// @covers AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.8
func TestSessionOpenRecoveryStatusAndLaunch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSessionOpenRecoveryState(t, repo, models.TaskSessionStateWaitingForInput)

	started := false
	launchCalls := 0
	agentMgr := &sessionUpdatingAgentManager{
		mockAgentManager: &mockAgentManager{
			repoForExecutionLookup: repo,
			launchAgentFunc: func(_ context.Context, _ *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
				launchCalls++
				running, err := repo.GetExecutorRunningBySessionID(context.Background(), "session-reused")
				if err != nil {
					return nil, err
				}
				running.AgentExecutionID = "execution-successor"
				if err := repo.UpsertExecutorRunning(context.Background(), running); err != nil {
					return nil, err
				}
				return &executor.LaunchAgentResponse{
					AgentExecutionID: "execution-successor",
					Status:           v1.AgentStatusStarting,
				}, nil
			},
		},
		repo:          repo,
		sessionID:     "session-reused",
		taskID:        "task-reused",
		onStartCalled: &started,
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.sessionCeiling = newSessionCeilingController(unlimitedSessionCeiling, nil, nil)

	status, err := svc.GetTaskSessionStatus(ctx, "task-reused", "session-reused")
	if err != nil {
		t.Fatalf("GetTaskSessionStatus: %v", err)
	}
	if !status.AutoResumeAllowed {
		t.Fatalf("auto_resume_allowed = false, reason %q", status.AutoResumeBlockedReason)
	}
	if !status.NeedsResume || !status.IsResumable {
		t.Fatalf("status = %+v, want a resumable session that needs recovery", status)
	}

	passive := svc.passiveLaunchResponse(ctx, &LaunchSessionRequest{
		TaskID:           "task-reused",
		SessionID:        "session-reused",
		Intent:           IntentResume,
		ActivationSource: LaunchActivationSourceSessionOpen,
	}, IntentResume)
	if passive != nil {
		t.Fatalf("passive launch response = %+v, want normal recovery", passive)
	}
	explicit := svc.passiveLaunchResponse(ctx, &LaunchSessionRequest{
		TaskID:           "task-reused",
		SessionID:        "session-reused",
		Intent:           IntentResume,
		ActivationSource: LaunchActivationSourceUserAction,
	}, IntentResume)
	if explicit != nil {
		t.Fatalf("explicit launch response = %+v, want no passive disposition", explicit)
	}
	response, err := svc.LaunchSession(ctx, &LaunchSessionRequest{
		TaskID:           "task-reused",
		SessionID:        "session-reused",
		Intent:           IntentResume,
		ActivationSource: LaunchActivationSourceSessionOpen,
	})
	if err != nil {
		t.Fatalf("LaunchSession: %v", err)
	}
	if response == nil || !response.Success || response.AgentExecutionID != "execution-successor" {
		t.Fatalf("LaunchSession response = %+v, want successor execution", response)
	}
	if launchCalls != 1 || !started {
		t.Fatalf("launch calls = %d, started = %t, want one successful launch", launchCalls, started)
	}
	ensured, err := svc.EnsureSession(ctx, "task-reused", EnsureSessionOptions{
		ActivationSource: LaunchActivationSourceSessionOpen,
	})
	if err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	if ensured.SessionID != "session-reused" || ensured.NewlyCreated {
		t.Fatalf("EnsureSession response = %+v, want the existing session", ensured)
	}

	sessions, err := repo.ListTaskSessions(ctx, "task-reused")
	if err != nil {
		t.Fatalf("list sessions after passive inspection: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("session count after passive inspection = %d, want 1", len(sessions))
	}
}

// @covers AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.2
// @covers AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.5
// @covers AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.9
func TestSessionOpenRecoveryCapacityDisposition(t *testing.T) {
	for _, test := range []struct {
		name       string
		ceiling    int
		occupySlot bool
	}{
		{name: "free capacity resumes selected sibling", ceiling: unlimitedSessionCeiling},
		{name: "full capacity waits without replacing queued sibling", ceiling: 1, occupySlot: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedSessionOpenRecoveryState(t, repo, models.TaskSessionStateWaitingForInput)
			createSessionOpenRecoverySession(t, repo, "session-primary", models.TaskSessionStateWaitingForInput, true)
			createSessionOpenRecoverySession(t, repo, "session-queued", models.TaskSessionStateCreated, false)

			queued := models.CeilingRecordKeys(models.CeilingDeferral{
				Kind: models.CeilingLaunchStartCreated,
				Payload: map[string]interface{}{
					metaKeySessionID:      "session-queued",
					metaKeyAgentProfileID: "profile-queued",
					"prompt":              "queued prompt",
				},
				Origin:   string(launchOriginAutomatic),
				QueuedAt: time.Date(2026, 9, 16, 20, 0, 0, 0, time.UTC),
			})
			if err := repo.SetTaskMetadataKey(ctx, "task-reused", models.MetaKeyDeferredLaunch, queued); err != nil {
				t.Fatalf("set queued sibling: %v", err)
			}
			beforeTask, err := repo.GetTask(ctx, "task-reused")
			if err != nil {
				t.Fatalf("load task before recovery: %v", err)
			}
			beforeQueue := beforeTask.Metadata[models.MetaKeyDeferredLaunch]
			beforePrimary, err := repo.GetTaskSession(ctx, "session-primary")
			if err != nil {
				t.Fatalf("load primary session before recovery: %v", err)
			}

			launchCalls := 0
			started := false
			agentMgr := &sessionUpdatingAgentManager{
				mockAgentManager: &mockAgentManager{
					repoForExecutionLookup: repo,
					launchAgentFunc: func(_ context.Context, _ *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
						launchCalls++
						running, err := repo.GetExecutorRunningBySessionID(context.Background(), "session-reused")
						if err != nil {
							return nil, err
						}
						running.AgentExecutionID = "execution-capacity-successor"
						if err := repo.UpsertExecutorRunning(context.Background(), running); err != nil {
							return nil, err
						}
						return &executor.LaunchAgentResponse{
							AgentExecutionID: "execution-capacity-successor",
							Status:           v1.AgentStatusStarting,
						}, nil
					},
				},
				repo:          repo,
				sessionID:     "session-reused",
				taskID:        "task-reused",
				onStartCalled: &started,
			}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
			svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
			svc.sessionCeiling = newSessionCeilingController(test.ceiling, nil, nil)
			if test.occupySlot {
				decision := svc.sessionCeiling.admit(ctx, admissionRequest{
					taskID: "other-task", sessionID: "other-session", origin: launchOriginAutomatic, seam: "test",
				})
				if !decision.admitted {
					t.Fatalf("occupy test ceiling slot: %+v", decision)
				}
			}

			response, err := svc.LaunchSession(ctx, &LaunchSessionRequest{
				TaskID:           "task-reused",
				SessionID:        "session-reused",
				Intent:           IntentResume,
				ActivationSource: LaunchActivationSourceSessionOpen,
			})
			if err != nil {
				t.Fatalf("LaunchSession: %v", err)
			}
			if response == nil || !response.Success {
				t.Fatalf("LaunchSession response = %+v, want success", response)
			}
			if test.occupySlot {
				assertFullSessionOpenRecovery(t, response, launchCalls, started)
			} else {
				assertFreeSessionOpenRecovery(t, response, launchCalls, started)
			}

			afterTask, err := repo.GetTask(ctx, "task-reused")
			if err != nil {
				t.Fatalf("load task after recovery: %v", err)
			}
			if !reflect.DeepEqual(afterTask.Metadata[models.MetaKeyDeferredLaunch], beforeQueue) {
				t.Fatalf("queued sibling changed: before=%#v after=%#v", beforeQueue, afterTask.Metadata[models.MetaKeyDeferredLaunch])
			}
			afterPrimary, err := repo.GetTaskSession(ctx, "session-primary")
			if err != nil {
				t.Fatalf("load primary session after recovery: %v", err)
			}
			if afterPrimary.IsPrimary != beforePrimary.IsPrimary {
				t.Fatalf("primary session changed: before=%t after=%t", beforePrimary.IsPrimary, afterPrimary.IsPrimary)
			}
			selected, err := repo.GetTaskSession(ctx, "session-reused")
			if err != nil {
				t.Fatalf("load selected session after recovery: %v", err)
			}
			if selected.Metadata[ceilingManualOverrideMetadataKey] != nil {
				t.Fatalf("session_open recovery recorded a manual override: %#v", selected.Metadata[ceilingManualOverrideMetadataKey])
			}
		})
	}
}

func assertFullSessionOpenRecovery(t *testing.T, response *LaunchSessionResponse, launchCalls int, started bool) {
	t.Helper()
	if response.SessionID != "session-reused" ||
		response.ActivationDisposition != activationDispositionSuppressed ||
		response.ActivationReason != "session_capacity" || response.AgentExecutionID != "" {
		t.Fatalf("full-capacity response = %+v, want selected session suppressed for capacity", response)
	}
	if launchCalls != 0 || started {
		t.Fatalf("full-capacity launch calls = %d, started = %t, want no runtime launch", launchCalls, started)
	}
}

func assertFreeSessionOpenRecovery(t *testing.T, response *LaunchSessionResponse, launchCalls int, started bool) {
	t.Helper()
	if response.AgentExecutionID != "execution-capacity-successor" {
		t.Fatalf("free-capacity response = %+v, want successor execution", response)
	}
	if launchCalls != 1 || !started {
		t.Fatalf("free-capacity launch calls = %d, started = %t, want one launch", launchCalls, started)
	}
}

// @covers AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.2
// @covers AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.6
// @covers AC-TASKS-QUEUED-SESSION-OWNERSHIP-002.6
func TestSessionOpenRecoveryRechecksOwnership(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSessionOpenRecoveryState(t, repo, models.TaskSessionStateWaitingForInput)
	launchCalls := 0
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		launchAgentFunc: func(_ context.Context, _ *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launchCalls++
			return &executor.LaunchAgentResponse{AgentExecutionID: "should-not-launch"}, nil
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.sessionCeiling = newSessionCeilingController(unlimitedSessionCeiling, nil, nil)

	status, err := svc.GetTaskSessionStatus(ctx, "task-reused", "session-reused")
	if err != nil {
		t.Fatalf("GetTaskSessionStatus: %v", err)
	}
	if !status.AutoResumeAllowed {
		t.Fatalf("initial status = %+v, want recovery allowed", status)
	}
	if passive := svc.passiveLaunchResponse(ctx, &LaunchSessionRequest{
		TaskID:           "task-reused",
		SessionID:        "session-reused",
		Intent:           IntentResume,
		ActivationSource: LaunchActivationSourceSessionOpen,
	}, IntentResume); passive != nil {
		t.Fatalf("initial passive response = %+v, want recovery allowed", passive)
	}

	queued := models.CeilingRecordKeys(models.CeilingDeferral{
		Kind: models.CeilingLaunchStartCreated,
		Payload: map[string]interface{}{
			metaKeySessionID:      "session-reused",
			metaKeyAgentProfileID: "profile-reused",
		},
		Origin:   string(launchOriginAutomatic),
		QueuedAt: time.Date(2026, 9, 16, 20, 0, 0, 0, time.UTC),
	})
	barrier := &sessionOpenTaskReadBarrierStore{
		sessionExecutorStore: svc.repo,
		taskID:               "task-reused",
		entered:              make(chan struct{}),
		release:              make(chan struct{}),
	}
	svc.repo = barrier
	resultCh := make(chan struct {
		response *LaunchSessionResponse
		err      error
	}, 1)
	go func() {
		response, launchErr := svc.LaunchSession(ctx, &LaunchSessionRequest{
			TaskID:           "task-reused",
			SessionID:        "session-reused",
			Intent:           IntentResume,
			ActivationSource: LaunchActivationSourceSessionOpen,
		})
		resultCh <- struct {
			response *LaunchSessionResponse
			err      error
		}{response: response, err: launchErr}
	}()
	select {
	case <-barrier.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("LaunchSession did not reach the post-read eligibility barrier")
	}
	if err := repo.SetTaskMetadataKey(ctx, "task-reused", models.MetaKeyDeferredLaunch, queued); err != nil {
		t.Fatalf("set successor queue during launch: %v", err)
	}
	queuedTask, err := repo.GetTask(ctx, "task-reused")
	if err != nil {
		t.Fatalf("reload successor queue during launch: %v", err)
	}
	queuedBeforeRelease := queuedTask.Metadata[models.MetaKeyDeferredLaunch]
	close(barrier.release)
	var result struct {
		response *LaunchSessionResponse
		err      error
	}
	select {
	case result = <-resultCh:
	case <-time.After(5 * time.Second):
		t.Fatal("LaunchSession did not finish after releasing the eligibility barrier")
	}
	response, err := result.response, result.err
	if err != nil {
		t.Fatalf("LaunchSession after successor queue: %v", err)
	}
	if response == nil || response.ActivationDisposition != activationDispositionQueued ||
		response.ActivationReason != autoResumeBlockedLaunchQueued {
		t.Fatalf("guarded passive launch response = %+v, want queued", response)
	}
	if launchCalls != 0 {
		t.Fatalf("runtime launch calls = %d, want none after queue ownership changed", launchCalls)
	}

	task, err := repo.GetTask(ctx, "task-reused")
	if err != nil {
		t.Fatalf("reload task after guarded launch: %v", err)
	}
	record, ok := task.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	if !ok {
		t.Fatalf("guarded passive launch deferred record = %#v, want object", task.Metadata[models.MetaKeyDeferredLaunch])
	}
	deferred, err := models.ReadCeilingDeferral(record)
	if err != nil || models.CeilingDeferralSessionID(task, deferred) != "session-reused" {
		t.Fatalf("guarded passive launch changed deferred record: %#v", task.Metadata[models.MetaKeyDeferredLaunch])
	}
	if !reflect.DeepEqual(record, queuedBeforeRelease) {
		t.Fatalf("guarded passive launch mutated queued record: before=%#v after=%#v", queuedBeforeRelease, record)
	}
}

// @covers AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.6
// @covers AC-TASKS-QUEUED-SESSION-OWNERSHIP-002.6
func TestSessionOpenRecoveryDelayedCallbacks(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSessionOpenRecoveryState(t, repo, models.TaskSessionStateWaitingForInput)
	createSessionOpenRecoverySession(t, repo, "session-primary", models.TaskSessionStateWaitingForInput, true)
	createSessionOpenRecoverySession(t, repo, "session-queued", models.TaskSessionStateCreated, false)
	queued := models.CeilingRecordKeys(models.CeilingDeferral{
		Kind: models.CeilingLaunchStartCreated,
		Payload: map[string]interface{}{
			metaKeySessionID:      "session-queued",
			metaKeyAgentProfileID: "profile-queued",
			"prompt":              "queued prompt",
		},
		Origin:   string(launchOriginAutomatic),
		QueuedAt: time.Date(2026, 9, 16, 20, 0, 0, 0, time.UTC),
	})
	if err := repo.SetTaskMetadataKey(ctx, "task-reused", models.MetaKeyDeferredLaunch, queued); err != nil {
		t.Fatalf("set accepted successor queue: %v", err)
	}
	beforeTask, err := repo.GetTask(ctx, "task-reused")
	if err != nil {
		t.Fatalf("load task before recovery: %v", err)
	}
	beforeRoute := beforeTask.Metadata[models.MetaKeyWorkflowSessionRoute]
	beforeQueue := beforeTask.Metadata[models.MetaKeyDeferredLaunch]
	beforePrimary, err := repo.GetTaskSession(ctx, "session-primary")
	if err != nil {
		t.Fatalf("load primary session before recovery: %v", err)
	}

	started := false
	agentMgr := &sessionUpdatingAgentManager{
		mockAgentManager: &mockAgentManager{
			repoForExecutionLookup: repo,
			launchAgentFunc: func(_ context.Context, _ *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
				running, err := repo.GetExecutorRunningBySessionID(context.Background(), "session-reused")
				if err != nil {
					return nil, err
				}
				running.AgentExecutionID = "execution-successor"
				if err := repo.UpsertExecutorRunning(context.Background(), running); err != nil {
					return nil, err
				}
				return &executor.LaunchAgentResponse{
					AgentExecutionID: "execution-successor",
					Status:           v1.AgentStatusStarting,
				}, nil
			},
		},
		repo:          repo,
		sessionID:     "session-reused",
		taskID:        "task-reused",
		onStartCalled: &started,
	}
	service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	service.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	service.sessionCeiling = newSessionCeilingController(unlimitedSessionCeiling, nil, nil)
	service.turnService = &repoTurnService{repo: repo}

	response, err := service.LaunchSession(ctx, &LaunchSessionRequest{
		TaskID:           "task-reused",
		SessionID:        "session-reused",
		Intent:           IntentResume,
		ActivationSource: LaunchActivationSourceSessionOpen,
	})
	if err != nil {
		t.Fatalf("LaunchSession: %v", err)
	}
	if response == nil || response.AgentExecutionID != "execution-successor" || !started {
		t.Fatalf("successor launch response = %+v, started = %t", response, started)
	}
	if running, err := repo.GetExecutorRunningBySessionID(ctx, "session-reused"); err != nil {
		t.Fatalf("load successor executor row: %v", err)
	} else if running == nil || running.AgentExecutionID != "execution-successor" {
		t.Fatalf("successor executor row = %+v, want execution-successor", running)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID:            "turn-successor",
		TaskID:        "task-reused",
		TaskSessionID: "session-reused",
		StartedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create successor turn: %v", err)
	}
	if err := repo.UpdateTaskSessionState(ctx, "session-reused", models.TaskSessionStateRunning, ""); err != nil {
		t.Fatalf("mark successor session running: %v", err)
	}

	event := watcher.AgentEventData{
		TaskID:           "task-reused",
		SessionID:        "session-reused",
		AgentExecutionID: "execution-reused",
	}
	service.handleAgentStopped(ctx, event)
	service.handleAgentStopped(ctx, event)
	service.handleAgentCompleted(ctx, event)
	service.handleAgentCompleted(ctx, event)

	restartedAgentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	restarted := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), restartedAgentMgr)
	restarted.executor = executor.NewExecutor(restartedAgentMgr, repo, testLogger(), executor.ExecutorConfig{})
	restarted.turnService = &repoTurnService{repo: repo}
	restarted.handleAgentStopped(ctx, event)
	restarted.handleAgentCompleted(ctx, event)
	restarted.handleAgentStopped(ctx, event)
	restarted.handleAgentCompleted(ctx, event)

	after, err := repo.GetTaskSession(ctx, "session-reused")
	if err != nil {
		t.Fatalf("load selected session after delayed callbacks: %v", err)
	}
	if after.State != models.TaskSessionStateRunning {
		t.Fatalf("successor session state after delayed callbacks = %q, want RUNNING", after.State)
	}
	stop, ok := workflowProfileSwitchStopIntentFromMetadata(after.Metadata)
	if !ok || !stop.Consumed || stop.ExecutionID != "execution-reused" {
		t.Fatalf("stop intent after delayed callbacks = %#v, want consumed old-execution tombstone", after.Metadata[models.SessionMetaKeyWorkflowProfileSwitchStopIntent])
	}
	activeTurn, err := repo.GetActiveTurnBySessionID(ctx, "session-reused")
	if err != nil || activeTurn == nil || activeTurn.ID != "turn-successor" {
		t.Fatalf("active successor turn = %+v, err=%v, want turn-successor", activeTurn, err)
	}
	if open := openTurnCount(t, repo, "session-reused"); open != 1 {
		t.Fatalf("open successor turns = %d, want 1", open)
	}
	running, err := repo.GetExecutorRunningBySessionID(ctx, "session-reused")
	if err != nil {
		t.Fatalf("load executor row after delayed callbacks: %v", err)
	}
	if running == nil || running.AgentExecutionID != "execution-successor" {
		t.Fatalf("executor row after delayed callbacks = %+v, want successor execution", running)
	}
	primary, err := repo.GetTaskSession(ctx, "session-primary")
	if err != nil {
		t.Fatalf("load primary session after delayed callbacks: %v", err)
	}
	if primary.IsPrimary != beforePrimary.IsPrimary {
		t.Fatalf("primary session changed after delayed callbacks: before=%t after=%t", beforePrimary.IsPrimary, primary.IsPrimary)
	}
	afterTask, err := repo.GetTask(ctx, "task-reused")
	if err != nil {
		t.Fatalf("load task after delayed callbacks: %v", err)
	}
	if !reflect.DeepEqual(afterTask.Metadata[models.MetaKeyWorkflowSessionRoute], beforeRoute) {
		t.Fatalf("workflow route changed after delayed callbacks: before=%#v after=%#v", beforeRoute, afterTask.Metadata[models.MetaKeyWorkflowSessionRoute])
	}
	if !reflect.DeepEqual(afterTask.Metadata[models.MetaKeyDeferredLaunch], beforeQueue) {
		t.Fatalf("accepted queue changed after delayed callbacks: before=%#v after=%#v", beforeQueue, afterTask.Metadata[models.MetaKeyDeferredLaunch])
	}

	allowed, reason := restarted.autoResumeEligibility(ctx, afterTask, after)
	if !allowed || reason != "" {
		t.Fatalf("eligibility after delayed callbacks = %t, %q; want allowed", allowed, reason)
	}
}

func createSessionOpenRecoverySession(
	t *testing.T,
	repo *sqliterepo.Repository,
	sessionID string,
	state models.TaskSessionState,
	primary bool,
) {
	t.Helper()
	now := time.Now().UTC()
	if err := repo.CreateTaskSession(context.Background(), &models.TaskSession{
		ID:             sessionID,
		TaskID:         "task-reused",
		State:          state,
		IsPrimary:      primary,
		StartedAt:      now,
		UpdatedAt:      now,
		AgentProfileID: "profile-" + sessionID,
	}); err != nil {
		t.Fatalf("create session %s: %v", sessionID, err)
	}
}

type sessionOpenTaskReadBarrierStore struct {
	sessionExecutorStore
	taskID  string
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *sessionOpenTaskReadBarrierStore) GetTask(
	ctx context.Context,
	id string,
) (*models.Task, error) {
	task, err := s.sessionExecutorStore.GetTask(ctx, id)
	if id == s.taskID {
		s.once.Do(func() {
			close(s.entered)
			<-s.release
		})
	}
	return task, err
}
