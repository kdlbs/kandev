package acp

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/replayfixtures"
)

// queuedReplayFakeAgent wraps replayFakeAgent to close releaseBarrier the
// instant its Prompt method is about to return its terminal error. This is
// the release trigger provider-error-recovery-02.md#replay-harness-semantics
// names for the queued case: "the release trigger must be the fake agent's
// Prompt returning its error, NOT Adapter.Prompt returning" — waiting on
// Adapter.Prompt would deadlock, since sendPrompt's own syncNotifQueue call
// on the error path cannot complete until the worker has drained past
// whatever this test holds it at.
type queuedReplayFakeAgent struct {
	replayFakeAgent
	releaseBarrier chan struct{}
}

func (f *queuedReplayFakeAgent) Prompt(ctx context.Context, req acp.PromptRequest) (acp.PromptResponse, error) {
	resp, err := f.replayFakeAgent.Prompt(ctx, req)
	close(f.releaseBarrier)
	return resp, err
}

// TestReplayFixtureQueuedCaseDeliversDiagnosticBeforePromptReturns exercises
// the "queued" case's barrier mechanics directly, for every queued fixture in
// the corpus. It artificially holds the update worker at a
// syncNotifQueueThen barrier — posted from a goroutine other than the one
// that calls Adapter.Prompt, per the primitive's own contract — while the
// fake agent emits the diagnostic chunk and returns its terminal error. It
// releases the artificial barrier only when the fake agent's Prompt method
// is about to return (via queuedReplayFakeAgent), never by waiting on
// Adapter.Prompt itself.
//
// The assertion: by the moment Adapter.Prompt returns, the diagnostic event
// is already on updatesCh. That ordering is not incidental — it depends on
// sendPrompt's own syncNotifQueue() call on the error path
// (adapter_prompt.go) draining everything queued ahead of it, including the
// diagnostic this test's artificial barrier held back, before Prompt can
// return. A regression that removes that drain would leave the diagnostic
// undrained at this exact observation point even though this test's
// artificial barrier has already released — this fixture case exists to
// catch exactly that regression.
func TestReplayFixtureQueuedCaseDeliversDiagnosticBeforePromptReturns(t *testing.T) {
	fixtures := replayfixtures.MustLoad()

	for _, fx := range fixtures {
		if fx.Case != replayfixtures.CaseQueued {
			continue
		}
		t.Run(fx.FileName, func(t *testing.T) {
			clientToAgentR, clientToAgentW := io.Pipe()
			agentToClientR, agentToClientW := io.Pipe()

			a := newTestAdapterForAgent(fx.AgentID)
			fake := &queuedReplayFakeAgent{
				replayFakeAgent: replayFakeAgent{fixture: fx},
				releaseBarrier:  make(chan struct{}),
			}

			if err := a.Connect(clientToAgentW, agentToClientR); err != nil {
				t.Fatalf("connect adapter: %v", err)
			}
			fake.conn = acp.NewAgentSideConnection(fake, agentToClientW, clientToAgentR)
			t.Cleanup(func() {
				_ = a.Close()
				_ = clientToAgentW.Close()
				_ = agentToClientW.Close()
			})

			ctx := context.Background()
			if err := a.Initialize(ctx); err != nil {
				t.Fatalf("initialize: %v", err)
			}
			if _, err := a.NewSession(ctx, nil); err != nil {
				t.Fatalf("new session: %v", err)
			}

			// barrierEntered closes only once the worker has dequeued this
			// item and started running afterBarrier, proving the artificial
			// barrier is already ahead of anything Adapter.Prompt is about to
			// enqueue. Posted from a separate goroutine: the post itself
			// blocks until the barrier closes, so it cannot run on the
			// goroutine that is about to call Adapter.Prompt.
			barrierEntered := make(chan struct{})
			go func() {
				a.syncNotifQueueThen(func() {
					close(barrierEntered)
					<-fake.releaseBarrier
				})
			}()

			select {
			case <-barrierEntered:
			case <-time.After(5 * time.Second):
				t.Fatal("update worker did not reach the artificial barrier")
			}

			promptDone := make(chan error, 1)
			go func() {
				promptDone <- a.Prompt(ctx, "continue", nil, fx.Identity.PromptGeneration)
			}()

			var promptErr error
			select {
			case promptErr = <-promptDone:
			case <-time.After(5 * time.Second):
				t.Fatal("Adapter.Prompt did not return")
			}
			if promptErr == nil {
				t.Fatal("Adapter.Prompt returned nil, want the fixture's prompt_error")
			}

			tokens := tokenizeEvents(drainEvents(a))
			wantTokens := fx.Expect.Events[:len(fx.Expect.Events)-1]
			if len(tokens) != len(wantTokens) {
				t.Fatalf("tokenized events (observed exactly when Adapter.Prompt returned) = %v, want %v", tokens, wantTokens)
			}
			for i := range tokens {
				if tokens[i] != wantTokens[i] {
					t.Fatalf("tokenized events = %v, want %v", tokens, wantTokens)
				}
			}
		})
	}
}
