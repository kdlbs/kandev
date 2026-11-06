package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	d "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

// pausingStatusClaimer pauses one same-generation status write after the
// engine has read its route snapshot and before the write reaches the row.
type pausingStatusClaimer struct {
	*Repository
	mu      sync.Mutex
	armed   bool
	entered chan struct{}
	release chan struct{}
}

func (p *pausingStatusClaimer) arm() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.armed = true
	p.entered = make(chan struct{})
	p.release = make(chan struct{})
}

func (p *pausingStatusClaimer) ClaimRouteStateFrom(
	ctx context.Context,
	expectedGeneration int64,
	expectedStatus string,
	state d.RouteState,
) (bool, error) {
	p.mu.Lock()
	armed := p.armed
	p.armed = false
	entered, release := p.entered, p.release
	p.mu.Unlock()
	if armed {
		close(entered)
		<-release
	}
	return p.Repository.ClaimRouteStateFrom(ctx, expectedGeneration, expectedStatus, state)
}

func TestContinuationSurvivesConcurrentStatusTransition(t *testing.T) {
	cases := []struct {
		name       string
		prepare    func(*continuationHarness, *testing.T)
		transition func(*continuationHarness) error
		status     string
	}{
		{
			name: "policy failure",
			transition: func(h *continuationHarness) error {
				_, err := h.engine.ApplyFailureContext(context.Background(), h.decision.SessionID, h.profile,
					h.decision.Generation, "A", rateLimited())
				if errors.Is(err, d.ErrRecoveryPending) {
					return nil
				}
				return err
			},
			status: "retry_wait",
		},
		{
			name:    "cancel pending",
			prepare: (*continuationHarness).wait,
			transition: func(h *continuationHarness) error {
				_, err := h.engine.CancelPending(context.Background(), h.decision.SessionID, h.decision.Generation, "stop")
				return err
			},
			status: "action_required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var claimer *pausingStatusClaimer
			h := newContinuationHarnessWith(t, func(r *Repository) d.Persistence {
				claimer = &pausingStatusClaimer{Repository: r}
				return claimer
			})
			if tc.prepare != nil {
				tc.prepare(h, t)
			}
			claimer.arm()
			transitionDone := make(chan error, 1)
			go func() { transitionDone <- tc.transition(h) }()
			<-claimer.entered

			saveDone := make(chan error, 1)
			go func() {
				saveDone <- h.conductor.PersistContinuation(context.Background(), h.decision,
					d.Continuation{Conversation: "late handoff"})
			}()
			// Give an unfenced save the chance to land inside the paused transition.
			select {
			case err := <-saveDone:
				saveDone <- err
			case <-time.After(100 * time.Millisecond):
			}
			close(claimer.release)
			if err := <-transitionDone; err != nil {
				t.Fatalf("transition: %v", err)
			}
			if err := <-saveDone; err != nil {
				t.Fatalf("continuation save: %v", err)
			}
			h.want(t, "late handoff", tc.status)
			cached, ok := h.engine.State(h.decision.SessionID)
			var got d.Continuation
			if !ok || json.Unmarshal([]byte(cached.ContinuationJSON), &got) != nil || got.Conversation != "late handoff" {
				t.Fatalf("cached handoff lost: %q", cached.ContinuationJSON)
			}
		})
	}
}

func rateLimited() *routingerr.Error {
	return &routingerr.Error{Code: routingerr.CodeRateLimited, Class: routingerr.ClassTransient, FallbackAllowed: true}
}
