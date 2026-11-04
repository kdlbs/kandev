package lifecycle

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

func TestMCPIdleUpdateCompletesWithCallbackAndQueuedStartupWriter(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	releaseRPC := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseRPC()
	client, _, cleanup := newSessionMCPClient(t, false, false, true, func() { close(entered); <-release })
	defer cleanup()
	manager := newTestManager(t)
	manager.mcpResolver = &sessionMCPResolverFake{}
	repository := newSessionMCPStateFake()
	repository.state["session-1"] = mcpconfig.SessionMCPSelectionState{DesiredRevision: 2, AppliedRevision: 1, ApplyState: mcpconfig.SessionMCPApplyStatePendingIdle}
	manager.SetMCPSelectionStateRepository(repository)
	execution := newSessionMCPExecution(client)
	execution.promptGeneration = 2
	if err := manager.executionStore.Add(execution); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- manager.applyPendingSessionMCP(context.Background(), execution.SessionID) }()
	<-entered
	callbackEntered, callbackDone := make(chan struct{}), make(chan struct{})
	go func() {
		execution.withStartupAttempt(execution.startupAttemptSnapshot(), func(string) {
			close(callbackEntered)
			manager.handleCompleteEventLeased(execution, &agentctl.AgentEvent{PromptGeneration: 1, TurnID: "previous"})
		})
		close(callbackDone)
	}()
	<-callbackEntered
	writerEntered, writerRelease, writerDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		execution.startupCallbackMu.Lock()
		close(writerEntered)
		<-writerRelease
		execution.startupCallbackMu.Unlock()
		close(writerDone)
	}()
	awaitMCPStartupWriter(t, execution)
	releaseRPC()
	select {
	case err := <-done:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(2 * time.Second):
		t.Error("MCP update deadlocked behind a startup writer and a callback waiting for its prompt lock")
		// The writer gate permits diagnostic cleanup of the old lock cycle. Put
		// back the prompt lock before its real owner reaches its deferred unlock.
		execution.promptLifecycleMu.Unlock()
		<-callbackDone
		<-writerEntered
		execution.promptLifecycleMu.Lock()
		close(writerRelease)
		<-writerDone
		<-done
		return
	}
	<-callbackDone
	<-writerEntered
	close(writerRelease)
	<-writerDone
	state, _ := repository.GetMCPSelectionState(context.Background(), execution.SessionID)
	if state.AppliedRevision != 2 || state.AttachmentAttemptID != "attempt-load" {
		t.Fatalf("wrong accepted operation: %+v", state)
	}
}

func awaitMCPStartupWriter(t *testing.T, execution *AgentExecution) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for execution.startupCallbackMu.TryRLock() {
		execution.startupCallbackMu.RUnlock()
		if time.Now().After(deadline) {
			t.Fatal("startup writer did not queue")
		}
		runtime.Gosched()
	}
}
