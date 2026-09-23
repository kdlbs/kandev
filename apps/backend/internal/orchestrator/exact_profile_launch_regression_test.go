package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/queue"
	"github.com/kandev/kandev/internal/orchestrator/scheduler"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type exactProfileWorkflowSwitchTaskRepo struct {
	*mockTaskRepo
	sessions *sqliterepo.Repository
	sourceID string
	once     sync.Once
}

func (r *exactProfileWorkflowSwitchTaskRepo) GetTask(ctx context.Context, taskID string) (*v1.Task, error) {
	var switchErr error
	r.once.Do(func() {
		source, err := r.sessions.GetTaskSession(ctx, r.sourceID)
		if err != nil {
			switchErr = err
			return
		}
		source.State = models.TaskSessionStateCompleted
		source.UpdatedAt = time.Now().UTC()
		switchErr = r.sessions.UpdateTaskSession(ctx, source)
	})
	if switchErr != nil {
		return nil, switchErr
	}
	return r.mockTaskRepo.GetTask(ctx, taskID)
}

func TestStartCreatedSession_PersistsExactBindingOnWorkflowRedirect(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session-original", models.TaskSessionStateCreated)
	task, err := repo.GetTask(ctx, "task1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	task.WorkspaceID = "ws1"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("set task workspace: %v", err)
	}

	revision := time.Unix(1_726_500_000, 0).UTC()
	const generation int64 = 1
	if _, err := repo.AssignExactProfileAssignment(ctx, &models.ExactProfileAssignment{
		TaskID:          "task1",
		WorkspaceID:     "ws1",
		AgentProfileID:  "profile-exact",
		ProfileRevision: revision,
		Generation:      generation,
	}); err != nil {
		t.Fatalf("assign exact profile: %v", err)
	}

	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:             "session-redirected",
		TaskID:         "task1",
		AgentProfileID: "profile-exact",
		State:          models.TaskSessionStateCreated,
		StartedAt:      time.Now().UTC().Add(time.Second),
		UpdatedAt:      time.Now().UTC().Add(time.Second),
		Metadata:       map[string]interface{}{},
	}); err != nil {
		t.Fatalf("create redirected session: %v", err)
	}

	baseTaskRepo := newMockTaskRepo()
	baseTaskRepo.tasks["task1"] = &v1.Task{
		ID:          "task1",
		WorkspaceID: "ws1",
		Title:       "Test Task",
		Description: "desc",
		State:       v1.TaskStateInProgress,
	}
	switchingTaskRepo := &exactProfileWorkflowSwitchTaskRepo{
		mockTaskRepo: baseTaskRepo,
		sessions:     repo,
		sourceID:     "session-original",
	}

	var launchedSessionID string
	var launchReturned bool
	agentMgr := &mockAgentManager{
		resolveProfileInfo: &executor.AgentProfileInfo{
			ProfileID:   "profile-exact",
			WorkspaceID: "ws1",
			Enabled:     true,
			Revision:    revision,
			Model:       "gpt-exact",
		},
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launchedSessionID = req.SessionID
			launchReturned = true
			return &executor.LaunchAgentResponse{AgentExecutionID: "exec-redirected"}, nil
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), baseTaskRepo, agentMgr)
	svc.taskRepo = switchingTaskRepo
	svc.scheduler = scheduler.NewScheduler(
		queue.NewTaskQueue(100), svc.executor, switchingTaskRepo, testLogger(), scheduler.SchedulerConfig{},
	)

	if _, err := svc.StartCreatedSession(
		ctx, "task1", "session-original", "profile-exact", "start", true, false, true, nil, nil,
	); err != nil {
		t.Fatalf("StartCreatedSession: %v", err)
	}
	if launchedSessionID != "session-redirected" {
		t.Fatalf("launched session = %q, want redirected session", launchedSessionID)
	}

	redirected, err := repo.GetTaskSession(ctx, "session-redirected")
	if err != nil {
		t.Fatalf("get redirected session: %v", err)
	}
	if redirected.ExactProfileGeneration != generation || redirected.ExactProfileRevision != revision.UnixNano() {
		t.Fatalf(
			"redirected exact binding = (%d, %d), want (%d, %d)",
			redirected.ExactProfileGeneration,
			redirected.ExactProfileRevision,
			generation,
			revision.UnixNano(),
		)
	}
	if receipt, err := repo.GetExactProfileLaunchReceipt(ctx, "task1", "session-redirected"); err != nil || receipt != nil {
		t.Fatalf("receipt before boot = (%#v, %v), want none", receipt, err)
	}
	if !launchReturned {
		t.Fatal("launch did not reach asynchronous boundary")
	}
	svc.handleAgentBootReady(ctx, watcher.AgentEventData{TaskID: "task1", SessionID: "session-redirected", AgentExecutionID: "exec-redirected"})
	receipt, err := repo.GetExactProfileLaunchReceipt(ctx, "task1", "session-redirected")
	if err != nil || receipt != nil {
		t.Fatalf("receipt after boot = (%#v, %v), want none before correlated inference progress", receipt, err)
	}
}

func TestResumeTaskSession_RecordsFailedClosedReceiptWhenPromptReadinessFails(t *testing.T) {
	oldReadyTimeout := agentPromptReadyTimeout
	oldReadyInterval := agentPromptReadyInterval
	agentPromptReadyTimeout = 20 * time.Millisecond
	agentPromptReadyInterval = time.Millisecond
	t.Cleanup(func() {
		agentPromptReadyTimeout = oldReadyTimeout
		agentPromptReadyInterval = oldReadyInterval
	})

	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	task, err := repo.GetTask(ctx, "task1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	task.WorkspaceID = "ws1"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("set task workspace: %v", err)
	}

	revision := time.Unix(1_726_500_000, 0).UTC()
	const generation int64 = 1
	if _, err := repo.AssignExactProfileAssignment(ctx, &models.ExactProfileAssignment{
		TaskID:          "task1",
		WorkspaceID:     "ws1",
		AgentProfileID:  "profile-exact",
		ProfileRevision: revision,
		Generation:      generation,
	}); err != nil {
		t.Fatalf("assign exact profile: %v", err)
	}

	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.AgentExecutionID = "exec-old"
	session.AgentProfileID = "profile-exact"
	session.ExactProfileGeneration = generation
	session.ExactProfileRevision = revision.UnixNano()
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}
	seedExecutorRunning(t, repo, session.ID, session.TaskID, session.AgentExecutionID)

	agentMgr := &mockAgentManager{
		isAgentRunning:         false,
		repoForExecutionLookup: repo,
		isAgentReadyFn: func(_ context.Context, _ string) bool {
			return false
		},
		resolveProfileInfo: &executor.AgentProfileInfo{
			ProfileID:   "profile-exact",
			WorkspaceID: "ws1",
			Enabled:     true,
			Revision:    revision,
			Model:       "gpt-exact",
		},
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			if !req.ExactProfile || req.ExactProfileModel != "gpt-exact" || req.ExactProfileRevision != revision.UnixNano() {
				t.Errorf("resume exact request = (%t, %q, %d), want (true, gpt-exact, %d)", req.ExactProfile, req.ExactProfileModel, req.ExactProfileRevision, revision.UnixNano())
			}
			go func(sessionID string) {
				ticker := time.NewTicker(5 * time.Millisecond)
				defer ticker.Stop()
				timeout := time.After(5 * time.Second)
				for {
					select {
					case <-ticker.C:
						started, loadErr := repo.GetTaskSession(context.Background(), sessionID)
						if loadErr == nil && started.State == models.TaskSessionStateStarting {
							started.State = models.TaskSessionStateWaitingForInput
							started.UpdatedAt = time.Now().UTC()
							_ = repo.UpdateTaskSession(context.Background(), started)
							return
						}
					case <-timeout:
						return
					}
				}
			}(req.SessionID)
			return &executor.LaunchAgentResponse{AgentExecutionID: "exec-resumed"}, nil
		},
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{
		ID:          "task1",
		WorkspaceID: "ws1",
		Title:       "Test Task",
		State:       v1.TaskStateInProgress,
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	execution, err := svc.ResumeTaskSession(ctx, "task1", "session1")
	if err == nil {
		t.Fatal("ResumeTaskSession succeeded despite prompt-readiness timeout")
	}
	if execution != nil {
		t.Fatalf("execution = %#v, want nil on readiness failure", execution)
	}
	if !errors.Is(err, ErrAgentNotReadyForPrompt) {
		t.Fatalf("ResumeTaskSession error = %v, want prompt-not-ready error", err)
	}

	receipt, err := repo.GetExactProfileLaunchReceipt(ctx, "task1", "session1")
	if err != nil {
		t.Fatalf("get exact launch receipt: %v", err)
	}
	if receipt == nil {
		t.Fatal("missing exact launch receipt")
	}
	if receipt.Outcome != models.ExactProfileLaunchOutcomeFailedClosed {
		t.Fatalf("receipt outcome = %q, want failed_closed", receipt.Outcome)
	}
	if receipt.InferenceStarted {
		t.Fatal("failed-closed receipt must record that inference did not start")
	}
	if receipt.FailureReason == "" {
		t.Fatal("failed-closed receipt is missing its failure reason")
	}
}

func TestStartCreatedSessionAsyncStartFailureRecordsFailedClosedExactReceipt(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateCreated)
	revision := exactProfileRecoveryAssignment(t, repo)

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{
		ID: "task1", WorkspaceID: "ws1", Title: "Test Task", Description: "start", State: v1.TaskStateInProgress,
	}
	agentManager := &mockAgentManager{
		resolveProfileInfo: exactProfileRecoveryInfo(revision),
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			return &executor.LaunchAgentResponse{AgentExecutionID: "exec-failing"}, nil
		},
		startAgentProcessErr: errors.New("delayed initial prompt startup failure"),
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentManager)
	svc.executor.SetOnAgentStartFailed(svc.handleAgentStartFailed)

	_, err := svc.StartCreatedSession(ctx, "task1", "session1", "profile-exact", "start", true, false, false, nil, nil)
	if err != nil {
		t.Fatalf("StartCreatedSession: %v", err)
	}
	receipt := waitForExactProfileReceipt(t, repo, "task1", "session1")
	if receipt.Outcome != models.ExactProfileLaunchOutcomeFailedClosed || receipt.InferenceStarted {
		t.Fatalf("async failure receipt = %#v, want failed_closed without inference", receipt)
	}
	if receipt.FailureReason == "" {
		t.Fatal("async failure receipt is missing the failure reason")
	}
}

func TestStartCreatedSessionRefusedExactAdmissionLeavesNoReceipt(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session-refused", models.TaskSessionStateCreated)
	revision := exactProfileRecoveryAssignment(t, repo)
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{ID: "task1", WorkspaceID: "ws1", Title: "Test", State: v1.TaskStateInProgress}
	startCalls := 0
	agentManager := &mockAgentManager{resolveProfileInfo: exactProfileRecoveryInfo(revision), launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		// Replace authority after resolution but before OnExecutionAdmitted.
		if _, err := repo.AssignExactProfileAssignment(ctx, &models.ExactProfileAssignment{TaskID: "task1", WorkspaceID: "ws1", AgentProfileID: "profile-exact", ProfileRevision: revision.Add(time.Second), Generation: 2}); err != nil {
			t.Fatal(err)
		}
		return &executor.LaunchAgentResponse{AgentExecutionID: "exec-refused"}, nil
	}}
	agentManager.startAgentProcessFunc = func(context.Context, string) error { startCalls++; return nil }
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentManager)
	if _, err := svc.StartCreatedSession(ctx, "task1", "session-refused", "profile-exact", "start", true, false, false, nil, nil); !errors.Is(err, executor.ErrExactAttemptAdmission) {
		t.Fatalf("StartCreatedSession error = %v", err)
	}
	if startCalls != 0 {
		t.Fatalf("StartAgentProcess calls = %d, want 0", startCalls)
	}
	if receipt, err := repo.GetExactProfileLaunchReceipt(ctx, "task1", "session-refused"); err != nil || receipt != nil {
		t.Fatalf("forged receipt = %#v, %v", receipt, err)
	}
}

func waitForExactProfileReceipt(
	t *testing.T,
	repo *sqliterepo.Repository,
	taskID, sessionID string,
) *models.ExactProfileLaunchReceipt {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		receipt, err := repo.GetExactProfileLaunchReceipt(context.Background(), taskID, sessionID)
		if err != nil {
			t.Fatalf("get exact launch receipt: %v", err)
		}
		if receipt != nil {
			return receipt
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for asynchronous exact launch receipt")
	return nil
}
