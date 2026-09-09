package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

func TestQueueAndInterruptForPeerMessage_DispatchesSurvivingAutoMergedID(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	agentManager := &mockAgentManager{
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
		promptDone:             make(chan struct{}),
	}
	service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)
	service.executor = executor.NewExecutor(agentManager, repo, testLogger(), executor.ExecutorConfig{})
	service.messageCreator = &mockMessageCreator{}
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateRunning)
	seedExecutorRunning(t, repo, "session1", "task1", "exec-1")
	metadata := map[string]interface{}{messagequeue.MetadataSenderTaskID: "parent-task"}
	target, err := service.messageQueue.QueueMessageWithMetadata(
		ctx, "session1", "task1", "first parent message", "", messagequeue.QueuedByAgent, false, nil, metadata,
	)
	if err != nil {
		t.Fatalf("queue target: %v", err)
	}

	queued, dispatched, err := service.QueueAndInterruptForPeerMessage(
		ctx, "task1", "session1", "second parent message", metadata,
	)
	if err != nil || !dispatched {
		t.Fatalf("queue and interrupt: dispatched=%v err=%v", dispatched, err)
	}
	if queued == nil || queued.ID != target.ID {
		t.Fatalf("returned entry = %+v, want surviving target %s", queued, target.ID)
	}
	select {
	case <-agentManager.promptDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for merged prompt dispatch")
	}
	agentManager.mu.Lock()
	prompts := append([]string(nil), agentManager.capturedPrompts...)
	agentManager.mu.Unlock()
	if len(prompts) != 1 || prompts[0] != "first parent message\n\nsecond parent message" {
		t.Fatalf("dispatched prompts = %v", prompts)
	}
	if status := service.messageQueue.GetStatus(ctx, "session1"); status.Count != 0 {
		t.Fatalf("merged survivor was not drained: %+v", status.Entries)
	}
}
