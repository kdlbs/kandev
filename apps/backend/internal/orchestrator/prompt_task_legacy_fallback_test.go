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
	"github.com/stretchr/testify/require"
)

func TestPromptTask_LegacyCallerComposesSavedPromptOnFreshFallback(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const (
		taskID    = "task-legacy-prompt-fallback"
		sessionID = "session-legacy-prompt-fallback"
		prompt    = "Use @legacy-review-rules for this change."
		expansion = "Legacy review rules must be applied."
	)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	session.AgentExecutionID = "exec-before-restart-legacy"
	session.AgentProfileID = "profile1"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	seedExecutorRunning(t, repo, sessionID, taskID, "exec-before-restart-legacy")

	promptService := newPromptServiceForLaunchFallbackTest(t)
	_, err = promptService.CreatePrompt(ctx, "legacy-review-rules", expansion)
	require.NoError(t, err)
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{
		ID: taskID, Title: "Legacy caller task", Description: "Inspect the existing feature.",
		State: v1.TaskStateInProgress,
	}
	var launchCalls atomic.Int32
	launchPrompts := make(chan string, 2)
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		promptErr:              lifecycle.ErrExecutionNotFound,
		isAgentRunningFn:       func(context.Context, string) bool { return launchCalls.Load() > 0 },
		isAgentReadyFn:         func(context.Context, string) bool { return launchCalls.Load() > 0 },
		launchAgentFunc: func(_ context.Context, request *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			call := launchCalls.Add(1)
			launchPrompts <- request.TaskDescription
			go func(launchSessionID string) {
				tick := time.NewTicker(5 * time.Millisecond)
				defer tick.Stop()
				timeout := time.After(5 * time.Second)
				for {
					select {
					case <-tick.C:
						current, getErr := repo.GetTaskSession(context.Background(), launchSessionID)
						if getErr == nil && current != nil && current.State == models.TaskSessionStateStarting {
							current.State = models.TaskSessionStateWaitingForInput
							current.UpdatedAt = time.Now().UTC()
							_ = repo.UpdateTaskSession(context.Background(), current)
							return
						}
					case <-timeout:
						return
					}
				}
			}(request.SessionID)
			return &executor.LaunchAgentResponse{
				AgentExecutionID: fmt.Sprintf("exec-legacy-fallback-%d", call),
				Status:           v1.AgentStatusStarting,
			}, nil
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.promptExpander = promptService
	exec := executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.executor = exec
	svc.scheduler = scheduler.NewScheduler(queue.NewTaskQueue(100), exec, taskRepo, testLogger(), scheduler.DefaultSchedulerConfig())

	_, err = svc.PromptTask(ctx, taskID, sessionID, prompt, "", false, nil, false)
	require.NoError(t, err)
	require.EqualValues(t, 2, launchCalls.Load(), "expected resume and fresh-runtime fallback launches")
	<-launchPrompts // Lazy resume has no prompt.
	freshPrompt := <-launchPrompts
	if !strings.Contains(freshPrompt, expansion) {
		t.Fatalf("legacy PromptTask fallback skipped saved-prompt composition: %q", freshPrompt)
	}
	require.Len(t, agentMgr.capturedPromptCalls, 1)
}
