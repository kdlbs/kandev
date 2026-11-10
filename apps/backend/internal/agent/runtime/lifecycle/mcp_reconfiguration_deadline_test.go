package lifecycle

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

type contextAwareMCPState struct {
	*sessionMCPStateFake
	saved     chan struct{}
	savedOnce sync.Once
}

func (r *contextAwareMCPState) GetMCPSelectionState(ctx context.Context, id string) (mcpconfig.SessionMCPSelectionState, error) {
	if err := ctx.Err(); err != nil {
		return mcpconfig.SessionMCPSelectionState{}, err
	}
	return r.sessionMCPStateFake.GetMCPSelectionState(ctx, id)
}
func (r *contextAwareMCPState) SaveMCPSelectionState(ctx context.Context, id string, state mcpconfig.SessionMCPSelectionState) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	err := r.sessionMCPStateFake.SaveMCPSelectionState(ctx, id, state)
	if r.saved != nil {
		r.savedOnce.Do(func() { close(r.saved) })
	}
	return err
}

type mcpContextObservation struct {
	err      error
	deadline time.Time
	bounded  bool
}
type deadlineMCPResolver struct{ observed chan mcpContextObservation }

func (r deadlineMCPResolver) Resolve(ctx context.Context, _ mcpconfig.ResolutionContext, _ mcpconfig.Policy) (*mcpconfig.EffectiveMCPResolution, error) {
	deadline, bounded := ctx.Deadline()
	r.observed <- mcpContextObservation{err: ctx.Err(), deadline: deadline, bounded: bounded}
	return nil, errors.New("resolution probe complete")
}

func TestMCPDetachedRequestHasHostDeadline(t *testing.T) {
	client, _, cleanup := newSessionMCPClient(t, false, false, true)
	defer cleanup()
	manager := newTestManager(t)
	observed := make(chan mcpContextObservation, 1)
	manager.mcpResolver = deadlineMCPResolver{observed: observed}
	repository := &contextAwareMCPState{sessionMCPStateFake: newSessionMCPStateFake(), saved: make(chan struct{})}
	repository.state["session-1"] = mcpconfig.SessionMCPSelectionState{DesiredRevision: 2, AppliedRevision: 1}
	manager.SetMCPSelectionStateRepository(repository)
	if err := manager.executionStore.Add(newSessionMCPExecution(client)); err != nil {
		t.Fatal(err)
	}
	request, cancel := context.WithCancel(context.Background())
	cancel()
	manager.RequestSessionMCPReconfiguration(request, "session-1")
	select {
	case result := <-observed:
		if result.err != nil || !result.bounded || time.Until(result.deadline) <= 0 || time.Until(result.deadline) > 2*time.Minute {
			t.Errorf("detached request lacks its own deadline: %+v", result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("detached request did not reach resolution")
	}
	select {
	case <-repository.saved:
	case <-time.After(2 * time.Second):
		t.Fatal("request did not settle")
	}
}

func TestMCPDeadlineReleasesLeasesAndPersistsFailure(t *testing.T) {
	for _, stalledAction := range []string{"agent.session.load", "agent.session.set_mode"} {
		t.Run(stalledAction, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			client, _, cleanup := newSessionMCPClientWithHooks(t, false, false, true, func(action string) {
				if action == stalledAction {
					close(entered)
					<-release
				}
			})
			defer func() { unblock(); cleanup() }()
			manager := newTestManager(t)
			manager.mcpResolver = &sessionMCPResolverFake{}
			repository := &contextAwareMCPState{sessionMCPStateFake: newSessionMCPStateFake()}
			repository.state["session-1"] = mcpconfig.SessionMCPSelectionState{DesiredRevision: 2, AppliedRevision: 1, AttachmentAttemptID: "previous", ApplyState: mcpconfig.SessionMCPApplyStatePendingIdle}
			manager.SetMCPSelectionStateRepository(repository)
			execution := newSessionMCPExecution(client)
			if stalledAction == "agent.session.set_mode" {
				execution.SetModeState(&CachedModeState{CurrentModeID: "plan"})
			}
			if err := manager.executionStore.Add(execution); err != nil {
				t.Fatal(err)
			}
			operation, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- manager.applyPendingSessionMCP(operation, execution.SessionID) }()
			select {
			case <-entered:
			case err := <-done:
				t.Fatalf("did not reach controlled RPC: %v", err)
			case <-time.After(2 * time.Second):
				t.Fatal("RPC not reached")
			}
			writerDone := make(chan struct{})
			go func() { execution.beginStartupAttempt(); close(writerDone) }()
			awaitMCPStartupWriter(t, execution)
			select {
			case err := <-done:
				if err != nil {
					t.Errorf("durable failure write used the expired RPC context: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("operation exceeded its deadline")
			}
			select {
			case <-writerDone:
			case <-time.After(2 * time.Second):
				t.Fatal("startup writer remained blocked")
			}
			state, _ := repository.GetMCPSelectionState(context.Background(), execution.SessionID)
			if state.ApplyState != mcpconfig.SessionMCPApplyStateFailed || state.AppliedRevision != 1 || state.AttachmentAttemptID != "previous" {
				t.Fatalf("unconfirmed setup accepted or failure lost: %+v", state)
			}
			if !execution.promptLifecycleMu.TryLock() {
				t.Fatal("prompt lease retained")
			}
			execution.promptLifecycleMu.Unlock()
			if !execution.agentctlLifecycleMu.TryLock() {
				t.Fatal("client lease retained")
			}
			execution.agentctlLifecycleMu.Unlock()
		})
	}
}
