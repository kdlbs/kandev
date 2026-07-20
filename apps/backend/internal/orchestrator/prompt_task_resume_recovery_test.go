package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/queue"
	"github.com/kandev/kandev/internal/orchestrator/scheduler"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type promptStreamRecoveringAgentManager struct {
	*mockAgentManager
	recovered atomic.Bool
}

func (m *promptStreamRecoveringAgentManager) RecoverAgentPromptStream(context.Context, string) error {
	m.recovered.Store(true)
	return nil
}

func TestPromptTask_StalePromptStreamRecoversBeforeTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(context.Background(), "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	session.AgentProfileID = "profile1"
	if err := repo.UpdateTaskSession(context.Background(), session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-stale-stream")

	baseMgr := &mockAgentManager{
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
	}
	agentMgr := &promptStreamRecoveringAgentManager{mockAgentManager: baseMgr}
	baseMgr.isAgentReadyFn = func(context.Context, string) bool {
		return agentMgr.recovered.Load()
	}

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{
		ID:    "task1",
		Title: "Test Task",
		State: v1.TaskStateInProgress,
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	_, err = svc.PromptTask(ctx, "task1", "session1", "are you stuck?", "", false, nil, false)
	if err != nil {
		t.Fatalf("expected stale prompt stream to recover, got: %v", err)
	}
	if !agentMgr.recovered.Load() {
		t.Fatal("expected prompt stream recovery to run")
	}
	if len(agentMgr.capturedPromptCalls) != 1 {
		t.Fatalf("expected prompt to be delivered after recovery, got %d calls", len(agentMgr.capturedPromptCalls))
	}
}

func TestPromptTask_ReapsPromptDeadExecutionBeforeSend(t *testing.T) {
	oldReadyTimeout := agentPromptReadyTimeout
	oldReadyInterval := agentPromptReadyInterval
	agentPromptReadyTimeout = 20 * time.Millisecond
	agentPromptReadyInterval = time.Millisecond
	t.Cleanup(func() {
		agentPromptReadyTimeout = oldReadyTimeout
		agentPromptReadyInterval = oldReadyInterval
	})

	ctx := context.Background()
	pollCtx, cancelPoll := context.WithCancel(context.Background())
	t.Cleanup(cancelPoll)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	session.AgentProfileID = "profile1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}

	now := time.Now().UTC()
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               "er1",
		SessionID:        "session1",
		TaskID:           "task1",
		AgentExecutionID: "exec-zombie",
		Status:           "running",
		Resumable:        true,
		ResumeToken:      "resume-token-123",
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("failed to seed executor running: %v", err)
	}

	var zombieRunning atomic.Bool
	zombieRunning.Store(true)
	var replacementReady atomic.Bool
	var launchCalls atomic.Int32
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentRunningFn: func(_ context.Context, _ string) bool {
			return zombieRunning.Load()
		},
		isAgentReadyFn: func(_ context.Context, _ string) bool {
			return replacementReady.Load()
		},
		stopAgentWithReasonFunc: func(_ context.Context, _ string, _ string, _ bool) error {
			zombieRunning.Store(false)
			return nil
		},
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launchCalls.Add(1)
			if req.ACPSessionID != "resume-token-123" {
				t.Errorf("expected preserved resume token, got %q", req.ACPSessionID)
			}
			running, err := repo.GetExecutorRunningBySessionID(context.Background(), req.SessionID)
			if err != nil {
				t.Errorf("reload executor running: %v", err)
			} else {
				running.AgentExecutionID = "exec-replacement"
				running.Status = "ready"
				if err := repo.UpsertExecutorRunning(context.Background(), running); err != nil {
					t.Errorf("persist replacement executor running: %v", err)
				}
			}
			go func(ctx context.Context, sessID string) {
				tick := time.NewTicker(5 * time.Millisecond)
				defer tick.Stop()
				timeout := time.NewTimer(5 * time.Second)
				defer timeout.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-tick.C:
						sess, err := repo.GetTaskSession(context.Background(), sessID)
						if err == nil && sess != nil && sess.State == models.TaskSessionStateStarting {
							sess.State = models.TaskSessionStateWaitingForInput
							sess.UpdatedAt = time.Now().UTC()
							_ = repo.UpdateTaskSession(context.Background(), sess)
							replacementReady.Store(true)
							return
						}
					case <-timeout.C:
						return
					}
				}
			}(pollCtx, req.SessionID)
			return &executor.LaunchAgentResponse{AgentExecutionID: "exec-replacement"}, nil
		},
	}

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{
		ID:    "task1",
		Title: "Test Task",
		State: v1.TaskStateInProgress,
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	if _, err := svc.PromptTask(ctx, "task1", "session1", "follow up", "", false, nil, false); err != nil {
		t.Fatalf("expected prompt-dead execution to be replaced before send, got: %v", err)
	}
	if got := launchCalls.Load(); got != 1 {
		t.Fatalf("expected one replacement launch, got %d", got)
	}
	if len(agentMgr.capturedPromptCalls) != 1 {
		t.Fatalf("expected one delivered prompt, got %d", len(agentMgr.capturedPromptCalls))
	}
	if got := agentMgr.capturedPromptCalls[0].ExecutionID; got != "exec-replacement" {
		t.Fatalf("expected prompt to target replacement execution, got %q", got)
	}

	running, err := repo.GetExecutorRunningBySessionID(ctx, "session1")
	if err != nil {
		t.Fatalf("failed to reload executor running row: %v", err)
	}
	if running.ResumeToken != "resume-token-123" {
		t.Fatalf("expected resume token to be preserved, got %q", running.ResumeToken)
	}
}

func TestPromptTask_LazyResumeExecutionNotFoundFallsBackToFreshLaunch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateIdle)
	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	session.AgentExecutionID = "exec-before-restart"
	session.AgentProfileID = "profile1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-before-restart")

	var launchCalls atomic.Int32
	launchPrompts := make(chan string, 2)
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		promptErr:              lifecycle.ErrExecutionNotFound,
		isAgentRunningFn: func(_ context.Context, _ string) bool {
			return launchCalls.Load() > 0
		},
		isAgentReadyFn: func(_ context.Context, _ string) bool {
			return launchCalls.Load() > 0
		},
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			call := launchCalls.Add(1)
			launchPrompts <- req.TaskDescription
			if call == 1 {
				go func(sessionID string) {
					tick := time.NewTicker(5 * time.Millisecond)
					defer tick.Stop()
					timeout := time.After(5 * time.Second)
					for {
						select {
						case <-tick.C:
							sess, err := repo.GetTaskSession(context.Background(), sessionID)
							if err == nil && sess != nil && sess.State == models.TaskSessionStateStarting {
								sess.State = models.TaskSessionStateWaitingForInput
								sess.UpdatedAt = time.Now().UTC()
								_ = repo.UpdateTaskSession(context.Background(), sess)
								return
							}
						case <-timeout:
							return
						}
					}
				}(req.SessionID)
			}
			return &executor.LaunchAgentResponse{AgentExecutionID: fmt.Sprintf("exec-resumed-%d", call)}, nil
		},
	}

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{
		ID:    "task1",
		Title: "Test Task",
		State: v1.TaskStateInProgress,
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	exec := executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.executor = exec
	svc.scheduler = scheduler.NewScheduler(queue.NewTaskQueue(100), exec, taskRepo, testLogger(), scheduler.DefaultSchedulerConfig())

	prompt := "follow-up after restart"
	if _, err := svc.PromptTask(ctx, "task1", "session1", prompt, "", false, nil, false); err != nil {
		t.Fatalf("expected fresh-launch fallback to recover missing execution, got: %v", err)
	}

	if got := launchCalls.Load(); got != 2 {
		t.Fatalf("expected resume launch plus fresh fallback launch, got %d launches", got)
	}
	<-launchPrompts // resume prompt; empty for the lazy resume path.
	freshPrompt := <-launchPrompts
	if !strings.Contains(freshPrompt, prompt) {
		t.Fatalf("fresh launch prompt %q does not contain original prompt %q", freshPrompt, prompt)
	}
	if len(agentMgr.capturedPromptCalls) != 1 {
		t.Fatalf("expected one failed PromptAgent attempt before fallback, got %d", len(agentMgr.capturedPromptCalls))
	}
}

func TestPromptTask_LazyResumeMissingACPSessionFallsBackToFreshLaunch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateIdle)
	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	session.AgentExecutionID = "exec-before-restart"
	session.AgentProfileID = "profile1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-before-restart")

	var launchCalls atomic.Int32
	launchPrompts := make(chan string, 2)
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		promptErr: fmt.Errorf(
			"%w: session not found: %w",
			lifecycle.ErrAgentReported,
			lifecycle.ErrExecutionNotFound,
		),
		isAgentRunningFn: func(_ context.Context, _ string) bool {
			return launchCalls.Load() > 0
		},
		isAgentReadyFn: func(_ context.Context, _ string) bool {
			return launchCalls.Load() > 0
		},
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			call := launchCalls.Add(1)
			launchPrompts <- req.TaskDescription
			if call == 1 {
				go func(sessionID string) {
					tick := time.NewTicker(5 * time.Millisecond)
					defer tick.Stop()
					timeout := time.After(5 * time.Second)
					for {
						select {
						case <-tick.C:
							sess, err := repo.GetTaskSession(context.Background(), sessionID)
							if err == nil && sess != nil && sess.State == models.TaskSessionStateStarting {
								sess.State = models.TaskSessionStateWaitingForInput
								sess.UpdatedAt = time.Now().UTC()
								_ = repo.UpdateTaskSession(context.Background(), sess)
								return
							}
						case <-timeout:
							return
						}
					}
				}(req.SessionID)
			}
			return &executor.LaunchAgentResponse{AgentExecutionID: fmt.Sprintf("exec-resumed-%d", call)}, nil
		},
	}

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{
		ID:    "task1",
		Title: "Test Task",
		State: v1.TaskStateInProgress,
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	exec := executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.executor = exec
	svc.scheduler = scheduler.NewScheduler(queue.NewTaskQueue(100), exec, taskRepo, testLogger(), scheduler.DefaultSchedulerConfig())

	prompt := "follow-up after missing ACP session"
	if _, err := svc.PromptTask(ctx, "task1", "session1", prompt, "", false, nil, false); err != nil {
		t.Fatalf("expected fresh-launch fallback to recover missing ACP session, got: %v", err)
	}

	if got := launchCalls.Load(); got != 2 {
		t.Fatalf("expected resume launch plus fresh fallback launch, got %d launches", got)
	}
	<-launchPrompts // resume prompt; empty for the lazy resume path.
	freshPrompt := <-launchPrompts
	if !strings.Contains(freshPrompt, prompt) {
		t.Fatalf("fresh launch prompt %q does not contain original prompt %q", freshPrompt, prompt)
	}
	if len(agentMgr.capturedPromptCalls) != 1 {
		t.Fatalf("expected one failed PromptAgent attempt before fallback, got %d", len(agentMgr.capturedPromptCalls))
	}
}
