package lifecycle

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type dispatchCancelFixture struct {
	manager            *Manager
	execution          *AgentExecution
	generation         uint64
	promptAccepted     <-chan struct{}
	cancelResponseSent <-chan struct{}
	disconnectStream   func()
	restoreStream      func()
}

type dispatchCancelFixtureOptions struct {
	cancelNotAcknowledged   bool
	completeBeforePromptAck bool
}

func newDispatchCancelFixture(t *testing.T, options dispatchCancelFixtureOptions) *dispatchCancelFixture {
	t.Helper()
	mock := newMockAgentServer(t)
	t.Cleanup(mock.Close)
	promptAccepted := make(chan struct{}, 2)
	cancelResponseSent := make(chan struct{})
	manager := newTestManager(t)
	execution := &AgentExecution{
		ID:            "exec-dispatch-cancel",
		TaskID:        "task-dispatch-cancel",
		SessionID:     "session-dispatch-cancel",
		Status:        v1.AgentStatusRunning,
		WorkspacePath: "/workspace",
		promptDoneCh:  make(chan PromptCompletionSignal, 1),
	}
	var completionAccepted chan bool
	if options.completeBeforePromptAck {
		completionAccepted = make(chan bool, 1)
	}
	mock.handler = func(msg ws.Message) *ws.Message {
		switch msg.Action {
		case "agent.prompt":
			if options.completeBeforePromptAck {
				prompt, exists := manager.executionStore.promptLifecycleSnapshot(execution.ID)
				if !exists {
					completionAccepted <- false
				} else {
					completionAccepted <- manager.handleCompleteEvent(execution, &agentctl.AgentEvent{
						Type:             "complete",
						SessionID:        execution.SessionID,
						PromptGeneration: prompt.generation,
					})
				}
			}
			promptAccepted <- struct{}{}
		case "agent.cancel":
			payload := map[string]any{"success": true}
			if options.cancelNotAcknowledged {
				payload = map[string]any{
					"success":          false,
					"not_acknowledged": true,
					"error":            "agent did not acknowledge cancellation",
				}
			}
			resp, _ := ws.NewResponse(msg.ID, msg.Action, payload)
			return resp
		}
		return mock.defaultHandler(msg)
	}
	mock.afterResponse = func(msg ws.Message) {
		if msg.Action == "agent.cancel" {
			close(cancelResponseSent)
		}
	}

	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)
	streamCtx, streamCancel := context.WithCancel(context.Background())
	t.Cleanup(streamCancel)
	require.NoError(t, client.StreamUpdates(streamCtx, func(_ agentctl.AgentEvent) {}, nil, nil))
	waitForWSConnected(t, mock)

	execution.agentctl = client
	require.NoError(t, manager.executionStore.Add(execution))
	_, err := manager.PromptAgent(context.Background(), execution.ID, "dispatch-only", nil, true)
	require.NoError(t, err)
	select {
	case <-promptAccepted:
	case <-time.After(time.Second):
		t.Fatal("dispatch-only prompt did not reach agentctl")
	}
	if options.completeBeforePromptAck {
		select {
		case accepted := <-completionAccepted:
			require.True(t, accepted, "completion event delivered during triggerPrompt must claim the admitted generation")
		case <-time.After(time.Second):
			t.Fatal("provider completion did not arrive before prompt acknowledgement")
		}
	}
	require.True(t, execution.dispatchedPromptPending.Load())
	prompt, exists := manager.executionStore.promptLifecycleSnapshot(execution.ID)
	require.True(t, exists)
	generation := prompt.generation
	require.NotZero(t, generation)
	if options.completeBeforePromptAck {
		require.Equal(t, generation, prompt.completedGeneration)
		require.NotEqual(t, generation, prompt.dispatchedGeneration,
			"completion during triggerPrompt must precede dispatch bookkeeping")
	}

	return &dispatchCancelFixture{
		manager:            manager,
		execution:          execution,
		generation:         generation,
		promptAccepted:     promptAccepted,
		cancelResponseSent: cancelResponseSent,
		disconnectStream: func() {
			execution.agentctlOverride.Store(agentctl.NewClient("127.0.0.1", 1, newTestLogger()))
		},
		restoreStream: func() {
			execution.agentctlOverride.Store(nil)
		},
	}
}

func (f *dispatchCancelFixture) setStaleClosedBarrier() {
	barrier := make(chan struct{})
	close(barrier)
	f.execution.promptFinishedMu.Lock()
	f.execution.promptFinished = barrier
	f.execution.promptFinishedMu.Unlock()
}

func (f *dispatchCancelFixture) complete(t *testing.T, generation uint64) {
	t.Helper()
	require.True(t, f.manager.handleCompleteEvent(f.execution, &agentctl.AgentEvent{
		Type:             "complete",
		SessionID:        f.execution.SessionID,
		PromptGeneration: generation,
	}))
}

func (f *dispatchCancelFixture) startCancel(ctx context.Context) <-chan error {
	result := make(chan error, 1)
	go func() { result <- f.manager.CancelAgent(ctx, f.execution.ID) }()
	return result
}

func (f *dispatchCancelFixture) waitForCancelAcknowledgement(t *testing.T) {
	t.Helper()
	select {
	case <-f.cancelResponseSent:
	case <-time.After(time.Second):
		t.Fatal("agentctl did not acknowledge cancellation")
	}
}

func TestManager_CancelAgent_DispatchCompletion(t *testing.T) {
	previousWait := cancelWaitTimeout
	previousEscalation := cancelEscalationTimeout
	cancelWaitTimeout = 500 * time.Millisecond
	cancelEscalationTimeout = 50 * time.Millisecond
	t.Cleanup(func() {
		cancelWaitTimeout = previousWait
		cancelEscalationTimeout = previousEscalation
	})

	for _, barrier := range []string{"nil", "stale-closed"} {
		for _, timing := range []string{"available", "delayed"} {
			t.Run(barrier+"/"+timing, func(t *testing.T) {
				fixture := newDispatchCancelFixture(t, dispatchCancelFixtureOptions{})
				if barrier == "stale-closed" {
					fixture.setStaleClosedBarrier()
				}
				if timing == "available" {
					fixture.complete(t, fixture.generation)
				}

				cancelCtx, cancel := context.WithTimeout(context.Background(), time.Second)
				t.Cleanup(cancel)
				cancelResult := fixture.startCancel(cancelCtx)
				fixture.waitForCancelAcknowledgement(t)
				if timing == "delayed" {
					select {
					case err := <-cancelResult:
						t.Fatalf("cancellation returned before provider completion: %v", err)
					case <-time.After(25 * time.Millisecond):
					}
					fixture.complete(t, fixture.generation)
				}

				select {
				case err := <-cancelResult:
					require.NoError(t, err)
				case <-time.After(time.Second):
					t.Fatal("cancellation did not finish after provider completion")
				}
				require.False(t, fixture.execution.dispatchedPromptPending.Load(),
					"confirmed completion must release the captured dispatch gate")
			})
		}
	}
}

func TestManager_CancelAgent_DispatchCompletionTimeoutEscalates(t *testing.T) {
	previousWait := cancelWaitTimeout
	previousEscalation := cancelEscalationTimeout
	cancelWaitTimeout = 80 * time.Millisecond
	cancelEscalationTimeout = 20 * time.Millisecond
	t.Cleanup(func() {
		cancelWaitTimeout = previousWait
		cancelEscalationTimeout = previousEscalation
	})

	fixture := newDispatchCancelFixture(t, dispatchCancelFixtureOptions{})
	cancelCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)
	result := fixture.startCancel(cancelCtx)
	fixture.waitForCancelAcknowledgement(t)
	select {
	case err := <-result:
		t.Fatalf("cancellation escalated before its completion budget: %v", err)
	case <-time.After(cancelWaitTimeout / 2):
	}

	require.ErrorIs(t, <-result, ErrCancelEscalated)
	require.False(t, fixture.execution.dispatchedPromptPending.Load(),
		"timeout escalation must release the captured dispatch gate")
}

func TestManager_CancelAgent_DispatchCancelNotAcknowledgedEscalatesImmediately(t *testing.T) {
	previousWait := cancelWaitTimeout
	cancelWaitTimeout = 500 * time.Millisecond
	t.Cleanup(func() { cancelWaitTimeout = previousWait })

	fixture := newDispatchCancelFixture(t, dispatchCancelFixtureOptions{cancelNotAcknowledged: true})
	started := time.Now()
	cancelCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)
	result := fixture.startCancel(cancelCtx)
	fixture.waitForCancelAcknowledgement(t)
	require.ErrorIs(t, <-result, ErrCancelEscalated)
	require.Less(t, time.Since(started), cancelWaitTimeout,
		"an unacknowledged cancellation must retain immediate escalation")
	require.False(t, fixture.execution.dispatchedPromptPending.Load())
}

func TestManager_CancelAgent_DispatchCompletionCallerCancellation(t *testing.T) {
	previousWait := cancelWaitTimeout
	cancelWaitTimeout = time.Second
	t.Cleanup(func() { cancelWaitTimeout = previousWait })

	fixture := newDispatchCancelFixture(t, dispatchCancelFixtureOptions{})
	cancelCtx, cancel := context.WithCancel(context.Background())
	result := fixture.startCancel(cancelCtx)
	fixture.waitForCancelAcknowledgement(t)
	select {
	case err := <-result:
		t.Fatalf("cancellation returned before caller cancellation: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	cancel()

	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("caller cancellation did not release the completion wait")
	}
	require.True(t, fixture.execution.dispatchedPromptPending.Load(),
		"caller cancellation must leave provider completion ownership intact")
}

func TestManager_CancelAgent_DispatchCompletionIgnoresStaleSignal(t *testing.T) {
	previousWait := cancelWaitTimeout
	cancelWaitTimeout = 500 * time.Millisecond
	t.Cleanup(func() { cancelWaitTimeout = previousWait })

	fixture := newDispatchCancelFixture(t, dispatchCancelFixtureOptions{})
	fixture.execution.promptDoneCh <- PromptCompletionSignal{
		StopReason:       "stale",
		PromptGeneration: fixture.generation + 1,
	}
	cancelCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)
	result := fixture.startCancel(cancelCtx)
	fixture.waitForCancelAcknowledgement(t)
	select {
	case err := <-result:
		t.Fatalf("stale completion ended cancellation: %v", err)
	case <-time.After(25 * time.Millisecond):
	}

	fixture.complete(t, fixture.generation)
	require.NoError(t, <-result)
	require.False(t, fixture.execution.dispatchedPromptPending.Load())
}

func TestManager_CancelAgent_DispatchCompletionCompetingWaiter(t *testing.T) {
	previousWait := cancelWaitTimeout
	cancelWaitTimeout = 500 * time.Millisecond
	t.Cleanup(func() { cancelWaitTimeout = previousWait })

	fixture := newDispatchCancelFixture(t, dispatchCancelFixtureOptions{})
	waiterLocked := make(chan struct{})
	waiterResult := make(chan error, 1)
	go func() {
		fixture.execution.promptMu.Lock()
		close(waiterLocked)
		defer fixture.execution.promptMu.Unlock()
		waiterResult <- waitForPendingDispatchedPrompt(context.Background(), fixture.execution)
	}()
	select {
	case <-waiterLocked:
	case <-time.After(time.Second):
		t.Fatal("successor prompt did not acquire the completion consumer lock")
	}

	cancelCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)
	cancelResult := fixture.startCancel(cancelCtx)
	fixture.waitForCancelAcknowledgement(t)
	fixture.complete(t, fixture.generation)
	require.NoError(t, <-waiterResult)
	require.NoError(t, <-cancelResult,
		"cancellation must observe the predecessor waiter's accepted completion")
	require.False(t, fixture.execution.dispatchedPromptPending.Load())
}

func TestManager_CancelAgent_DispatchCompletionCompetingWaiterEscalates(t *testing.T) {
	previousWait := cancelWaitTimeout
	previousEscalation := cancelEscalationTimeout
	t.Cleanup(func() {
		cancelWaitTimeout = previousWait
		cancelEscalationTimeout = previousEscalation
	})

	for _, scenario := range []struct {
		name                  string
		cancelNotAcknowledged bool
		disconnect            bool
		cancelWait            time.Duration
		waitForCancelResponse bool
	}{
		{name: "not-acknowledged", cancelNotAcknowledged: true, cancelWait: 500 * time.Millisecond, waitForCancelResponse: true},
		{name: "disconnected-stream", disconnect: true, cancelWait: 500 * time.Millisecond},
		{name: "missing-completion-timeout", cancelWait: 60 * time.Millisecond, waitForCancelResponse: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			cancelWaitTimeout = scenario.cancelWait
			cancelEscalationTimeout = 100 * time.Millisecond
			fixture := newDispatchCancelFixture(t, dispatchCancelFixtureOptions{
				cancelNotAcknowledged: scenario.cancelNotAcknowledged,
			})

			waiterLocked := make(chan struct{})
			waiterResult := make(chan error, 1)
			go func() {
				fixture.execution.promptMu.Lock()
				close(waiterLocked)
				waitErr := waitForPendingDispatchedPrompt(context.Background(), fixture.execution)
				fixture.execution.promptMu.Unlock()
				waiterResult <- waitErr
			}()
			select {
			case <-waiterLocked:
			case <-time.After(time.Second):
				t.Fatal("successor prompt did not acquire the completion consumer lock")
			}
			if scenario.disconnect {
				fixture.disconnectStream()
			}

			cancelCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			t.Cleanup(cancel)
			cancelResult := fixture.startCancel(cancelCtx)
			if scenario.waitForCancelResponse {
				fixture.waitForCancelAcknowledgement(t)
			}
			t.Cleanup(fixture.restoreStream)
			select {
			case err := <-cancelResult:
				require.ErrorIs(t, err, ErrCancelEscalated)
			case <-time.After(time.Second):
				t.Fatal("cancellation did not escalate while the predecessor waiter held promptMu")
			}
			fixture.restoreStream()
			select {
			case err := <-waiterResult:
				require.NoError(t, err, "the existing completion consumer must be released")
			case <-time.After(time.Second):
				t.Fatal("lockless escalation did not release the predecessor waiter")
			}
			require.False(t, fixture.execution.dispatchedPromptPending.Load(),
				"the predecessor completion gate must be cleared by its consumer")
			require.Equal(t, v1.AgentStatusReady, fixture.execution.Status)

			_, err := fixture.manager.PromptAgent(context.Background(), fixture.execution.ID, "successor", nil, true)
			require.NoError(t, err, "a follow-up prompt must be admitted after local escalation")
			select {
			case <-fixture.promptAccepted:
			case <-time.After(time.Second):
				t.Fatal("follow-up prompt did not reach agentctl")
			}
			successor, exists := fixture.manager.executionStore.promptLifecycleSnapshot(fixture.execution.ID)
			require.True(t, exists)
			require.Greater(t, successor.generation, fixture.generation)
			require.Equal(t, v1.AgentStatusRunning, fixture.execution.Status,
				"escalation cleanup must not overwrite the successor's running status")
			require.True(t, fixture.execution.dispatchedPromptPending.Load(),
				"escalation cleanup must not clear the successor's dispatch gate")

			require.False(t, fixture.manager.handleCompleteEvent(fixture.execution, &agentctl.AgentEvent{
				Type:             "complete",
				SessionID:        fixture.execution.SessionID,
				PromptGeneration: fixture.generation,
			}), "a late predecessor completion must be rejected after successor admission")
			require.Equal(t, v1.AgentStatusRunning, fixture.execution.Status)
			require.True(t, fixture.execution.dispatchedPromptPending.Load())
		})
	}
}

func TestManager_CancelAgent_DispatchCompletionBeforePromptAcknowledgement(t *testing.T) {
	previousWait := cancelWaitTimeout
	cancelWaitTimeout = 500 * time.Millisecond
	t.Cleanup(func() { cancelWaitTimeout = previousWait })

	for _, barrier := range []string{"nil", "stale-closed"} {
		t.Run(barrier, func(t *testing.T) {
			fixture := newDispatchCancelFixture(t, dispatchCancelFixtureOptions{
				completeBeforePromptAck: true,
			})
			if barrier == "stale-closed" {
				fixture.setStaleClosedBarrier()
			}

			cancelCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			t.Cleanup(cancel)
			result := fixture.startCancel(cancelCtx)
			fixture.waitForCancelAcknowledgement(t)
			select {
			case err := <-result:
				require.NoError(t, err)
			case <-time.After(time.Second):
				t.Fatal("cancellation did not accept the completion recorded before prompt acknowledgement")
			}
			require.False(t, fixture.execution.dispatchedPromptPending.Load(),
				"accepted pre-ack completion must release its dispatch gate")
		})
	}
}

func TestManager_CancelAgent_AdmittedDispatchWaitsForCompletion(t *testing.T) {
	previousWait := cancelWaitTimeout
	cancelWaitTimeout = 500 * time.Millisecond
	t.Cleanup(func() { cancelWaitTimeout = previousWait })

	fixture := newDispatchCancelFixture(t, dispatchCancelFixtureOptions{})
	fixture.complete(t, fixture.generation)
	fixture.setStaleClosedBarrier()

	admitted := make(chan struct{})
	releaseDispatch := make(chan struct{})
	var admittedOnce sync.Once
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseDispatch) }) }
	t.Cleanup(release)
	fixture.manager.sessionManager.beforePromptDispatchHook = func() {
		admittedOnce.Do(func() { close(admitted) })
		<-releaseDispatch
	}
	promptResult := make(chan error, 1)
	go func() {
		_, err := fixture.manager.PromptAgent(context.Background(), fixture.execution.ID, "admitted", nil, true)
		promptResult <- err
	}()
	select {
	case <-admitted:
	case <-time.After(time.Second):
		t.Fatal("prompt did not pause after admission and before dispatch")
	}

	prompt, exists := fixture.manager.executionStore.promptLifecycleSnapshot(fixture.execution.ID)
	require.True(t, exists)
	require.Greater(t, prompt.generation, fixture.generation)
	require.NotEqual(t, prompt.generation, prompt.dispatchedGeneration)
	require.NotEqual(t, prompt.generation, prompt.completedGeneration)
	require.False(t, fixture.execution.dispatchedPromptPending.Load())

	cancelCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)
	cancelResult := fixture.startCancel(cancelCtx)
	fixture.waitForCancelAcknowledgement(t)
	select {
	case err := <-cancelResult:
		t.Fatalf("cancellation returned before the admitted prompt completed: %v", err)
	case <-time.After(25 * time.Millisecond):
	}

	fixture.complete(t, prompt.generation)
	release()
	select {
	case err := <-promptResult:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("admitted prompt did not finish after dispatch was released")
	}
	select {
	case err := <-cancelResult:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("cancellation did not accept completion for the admitted generation")
	}
	require.False(t, fixture.execution.dispatchedPromptPending.Load(),
		"completion of the admitted generation must release its dispatch gate")
}

func TestManager_CancelAgent_AdmittedDispatchTimeoutReleasesPredecessor(t *testing.T) {
	previousWait := cancelWaitTimeout
	previousEscalation := cancelEscalationTimeout
	cancelWaitTimeout = 60 * time.Millisecond
	cancelEscalationTimeout = 50 * time.Millisecond
	t.Cleanup(func() {
		cancelWaitTimeout = previousWait
		cancelEscalationTimeout = previousEscalation
	})

	fixture := newDispatchCancelFixture(t, dispatchCancelFixtureOptions{})
	fixture.complete(t, fixture.generation)

	admitted := make(chan struct{})
	releaseDispatch := make(chan struct{})
	var admittedOnce sync.Once
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseDispatch) }) }
	t.Cleanup(release)
	fixture.manager.sessionManager.beforePromptDispatchHook = func() {
		admittedOnce.Do(func() { close(admitted) })
		<-releaseDispatch
	}
	promptResult := make(chan error, 1)
	go func() {
		_, err := fixture.manager.PromptAgent(context.Background(), fixture.execution.ID, "admitted", nil, true)
		promptResult <- err
	}()
	select {
	case <-admitted:
	case <-time.After(time.Second):
		t.Fatal("prompt did not pause after admission and before dispatch")
	}
	prompt, exists := fixture.manager.executionStore.promptLifecycleSnapshot(fixture.execution.ID)
	require.True(t, exists)
	require.NotEqual(t, prompt.generation, prompt.dispatchedGeneration)
	require.False(t, fixture.execution.dispatchedPromptPending.Load())

	cancelCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)
	cancelResult := fixture.startCancel(cancelCtx)
	fixture.waitForCancelAcknowledgement(t)
	select {
	case err := <-cancelResult:
		require.ErrorIs(t, err, ErrCancelEscalated)
	case <-time.After(time.Second):
		t.Fatal("cancellation did not escalate the admitted generation within its budget")
	}
	require.Equal(t, v1.AgentStatusReady, fixture.execution.Status,
		"local escalation must settle the captured generation")

	release()
	select {
	case err := <-promptResult:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("admitted prompt did not finish after dispatch was released")
	}
	require.True(t, fixture.execution.dispatchedPromptPending.Load(),
		"the accepted dispatch remains gated until its generation's signal is consumed")

	_, err := fixture.manager.PromptAgent(context.Background(), fixture.execution.ID, "successor", nil, true)
	require.NoError(t, err, "the generation-bound escalation must release the predecessor gate")
	successor, exists := fixture.manager.executionStore.promptLifecycleSnapshot(fixture.execution.ID)
	require.True(t, exists)
	require.Greater(t, successor.generation, prompt.generation)
	require.Equal(t, v1.AgentStatusRunning, fixture.execution.Status,
		"predecessor escalation must not overwrite the successor's status")
	require.True(t, fixture.execution.dispatchedPromptPending.Load(),
		"predecessor escalation must not clear the successor's gate")
	require.False(t, fixture.manager.handleCompleteEvent(fixture.execution, &agentctl.AgentEvent{
		Type:             "complete",
		SessionID:        fixture.execution.SessionID,
		PromptGeneration: prompt.generation,
	}), "a late predecessor completion must be rejected after successor admission")
	require.Equal(t, v1.AgentStatusRunning, fixture.execution.Status)
	require.True(t, fixture.execution.dispatchedPromptPending.Load())
}

func TestManager_CancelAgent_DispatchCompletionRejectsReplacementGeneration(t *testing.T) {
	previousWait := cancelWaitTimeout
	cancelWaitTimeout = 500 * time.Millisecond
	t.Cleanup(func() { cancelWaitTimeout = previousWait })

	fixture := newDispatchCancelFixture(t, dispatchCancelFixtureOptions{})
	fixture.execution.promptMu.Lock()
	cancelCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)
	result := fixture.startCancel(cancelCtx)
	fixture.waitForCancelAcknowledgement(t)

	successorGeneration, err := fixture.manager.executionStore.BeginPrompt(fixture.execution.ID)
	require.NoError(t, err)
	fixture.manager.executionStore.MarkPromptDispatched(fixture.execution.ID, successorGeneration)
	fixture.execution.dispatchedPromptPending.Store(true)
	fixture.execution.promptMu.Unlock()

	select {
	case err := <-result:
		require.ErrorIs(t, err, ErrPromptActivityNotOwned)
	case <-time.After(time.Second):
		t.Fatal("cancellation did not reject the replacement generation")
	}
	require.True(t, fixture.execution.dispatchedPromptPending.Load(),
		"cancellation of the predecessor must not clear the successor dispatch gate")
}

func TestManager_CancelAgent_DispatchCompletionAtTimeoutBoundary(t *testing.T) {
	previousWait := cancelWaitTimeout
	cancelWaitTimeout = 40 * time.Millisecond
	t.Cleanup(func() { cancelWaitTimeout = previousWait })

	fixture := newDispatchCancelFixture(t, dispatchCancelFixtureOptions{})
	fixture.execution.promptMu.Lock()
	fixture.complete(t, fixture.generation)
	err := fixture.manager.finishDispatchCancelTimeout(
		context.Background(), fixture.execution, fixture.generation, nil,
	)
	fixture.execution.promptMu.Unlock()
	require.NoError(t, err, "an accepted completion available at the deadline must win")
	require.False(t, fixture.execution.dispatchedPromptPending.Load())
}

func TestManager_CancelAgent_OrdinaryPromptRetainsCompletionConsumer(t *testing.T) {
	fixture := newDispatchCancelFixture(t, dispatchCancelFixtureOptions{})
	fixture.complete(t, fixture.generation)
	ordinaryDone := make(chan error, 1)
	ordinaryCtx, ordinaryCancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(ordinaryCancel)
	go func() {
		_, err := fixture.manager.PromptAgent(ordinaryCtx, fixture.execution.ID, "ordinary", nil, false)
		ordinaryDone <- err
	}()
	select {
	case <-fixture.promptAccepted:
	case <-time.After(time.Second):
		t.Fatal("ordinary prompt did not reach agentctl")
	}
	var generation uint64
	require.Eventually(t, func() bool {
		generation = fixture.manager.executionStore.ActivePromptGeneration(fixture.execution.ID)
		return generation != 0
	}, time.Second, time.Millisecond)
	require.NotZero(t, generation)

	cancelCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)
	cancelResult := fixture.startCancel(cancelCtx)
	fixture.waitForCancelAcknowledgement(t)
	fixture.complete(t, generation)

	require.NoError(t, <-ordinaryDone)
	require.NoError(t, <-cancelResult)
	select {
	case signal := <-fixture.execution.promptDoneCh:
		t.Fatalf("cancellation stole the ordinary prompt completion: %+v", signal)
	default:
	}
}

func TestManager_CancelAgent_DispatchCompletionTransportFailureEscalates(t *testing.T) {
	previousWait := cancelWaitTimeout
	previousEscalation := cancelEscalationTimeout
	cancelWaitTimeout = 80 * time.Millisecond
	cancelEscalationTimeout = 20 * time.Millisecond
	t.Cleanup(func() {
		cancelWaitTimeout = previousWait
		cancelEscalationTimeout = previousEscalation
	})

	fixture := newDispatchCancelFixture(t, dispatchCancelFixtureOptions{})
	cancelCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)
	result := fixture.startCancel(cancelCtx)
	fixture.waitForCancelAcknowledgement(t)
	fixture.execution.promptDoneCh <- PromptCompletionSignal{
		IsError:          true,
		Error:            "agent stream disconnected: test transport failure",
		PromptGeneration: fixture.generation,
	}
	err := <-result
	require.ErrorIs(t, err, ErrCancelEscalated)
	require.ErrorContains(t, err, "test transport failure")
	require.False(t, fixture.execution.dispatchedPromptPending.Load())
}

func TestManager_CancelAgent_DispatchCompletionTimeoutPreservesTransportFailure(t *testing.T) {
	previousWait := cancelWaitTimeout
	previousEscalation := cancelEscalationTimeout
	cancelWaitTimeout = 80 * time.Millisecond
	cancelEscalationTimeout = 20 * time.Millisecond
	t.Cleanup(func() {
		cancelWaitTimeout = previousWait
		cancelEscalationTimeout = previousEscalation
	})

	fixture := newDispatchCancelFixture(t, dispatchCancelFixtureOptions{})
	fixture.execution.promptDoneCh <- PromptCompletionSignal{
		IsError:          true,
		Error:            "provider stream failed at completion deadline",
		PromptGeneration: fixture.generation,
	}

	err := fixture.manager.finishDispatchCancelTimeout(
		context.Background(), fixture.execution, fixture.generation, nil,
	)
	require.ErrorIs(t, err, ErrCancelEscalated)
	require.ErrorContains(t, err, "dispatch completion transport failure")
	require.ErrorContains(t, err, "provider stream failed at completion deadline")
}
