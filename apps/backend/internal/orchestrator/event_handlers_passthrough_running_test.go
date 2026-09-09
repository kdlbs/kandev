package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type reentrantPassthroughAgentManager struct {
	*mockAgentManager

	markPassthroughRunningFunc    func(string)
	preparePassthroughRunningFunc func(string) (func(), error)
}

func (m *reentrantPassthroughAgentManager) MarkPassthroughRunning(sessionID string) error {
	if err := m.mockAgentManager.MarkPassthroughRunning(sessionID); err != nil {
		return err
	}
	if m.markPassthroughRunningFunc != nil {
		m.markPassthroughRunningFunc(sessionID)
	}
	return nil
}

func (m *reentrantPassthroughAgentManager) PreparePassthroughRunning(sessionID string) (func(), error) {
	if m.preparePassthroughRunningFunc == nil {
		return func() {}, nil
	}
	return m.preparePassthroughRunningFunc(sessionID)
}

func TestHandleAgentReady_PassthroughQueuedMessageSynchronousRunningEvent(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	baseManager := &mockAgentManager{isPassthrough: true}
	agentManager := &reentrantPassthroughAgentManager{mockAgentManager: baseManager}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)

	var runningEventCalls int
	agentManager.markPassthroughRunningFunc = func(sessionID string) {
		runningEventCalls++
		svc.handleAgentRunning(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: sessionID})
	}
	agentManager.preparePassthroughRunningFunc = func(sessionID string) (func(), error) {
		return func() {
			runningEventCalls++
			svc.handleAgentRunning(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: sessionID})
		}, nil
	}

	_, err := svc.messageQueue.QueueMessage(ctx, "s1", "t1", "queued prompt", "", "test", false, nil)
	require.NoError(t, err)

	readyDone := make(chan struct{})
	go func() {
		svc.handleAgentReady(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1"})
		close(readyDone)
	}()

	select {
	case <-readyDone:
	case <-time.After(2 * time.Second):
		t.Fatal("handleAgentReady deadlocked while delivering a passthrough queued prompt")
	}

	require.Equal(t, 1, runningEventCalls)
	require.Empty(t, baseManager.markPassthroughCalls, "guarded delivery must use the deferred lifecycle capability")
	require.Len(t, baseManager.passthroughStdinCalls, 1)
	require.Equal(t, "queued prompt\r", baseManager.passthroughStdinCalls[0].Data)
	require.Zero(t, svc.messageQueue.GetStatus(ctx, "s1").Count)

	updatedSession, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, updatedSession.State)
}

func TestHandleAgentReady_PassthroughQueuedMessagePublishesAfterWriteFailure(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	baseManager := &mockAgentManager{
		isPassthrough:       true,
		passthroughStdinErr: errors.New("pty write failed"),
	}
	agentManager := &reentrantPassthroughAgentManager{mockAgentManager: baseManager}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)

	var runningEventCalls int
	agentManager.markPassthroughRunningFunc = func(sessionID string) {
		runningEventCalls++
		svc.handleAgentRunning(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: sessionID})
	}
	agentManager.preparePassthroughRunningFunc = func(sessionID string) (func(), error) {
		return func() {
			runningEventCalls++
			svc.handleAgentRunning(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: sessionID})
		}, nil
	}

	_, err := svc.messageQueue.QueueMessage(ctx, "s1", "t1", "queued prompt", "", "test", false, nil)
	require.NoError(t, err)

	readyDone := make(chan struct{})
	go func() {
		svc.handleAgentReady(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1"})
		close(readyDone)
	}()

	select {
	case <-readyDone:
	case <-time.After(2 * time.Second):
		t.Fatal("handleAgentReady deadlocked while publishing after a passthrough write failure")
	}

	require.Equal(t, 1, runningEventCalls, "the running event must still publish after the PTY write fails")
	require.Empty(t, baseManager.markPassthroughCalls)
	require.Len(t, baseManager.passthroughStdinCalls, 1)
	require.Zero(t, svc.messageQueue.GetStatus(ctx, "s1").Count)
}
