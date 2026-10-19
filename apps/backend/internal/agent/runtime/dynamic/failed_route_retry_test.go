package dynamic

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

type failedRouteRetryFixture struct {
	t       *testing.T
	ctx     context.Context
	now     time.Time
	engine  *Engine
	profile Profile
}

func newFailedRouteRetryFixture(t *testing.T) *failedRouteRetryFixture {
	t.Helper()
	f := &failedRouteRetryFixture{
		t: t, ctx: context.Background(), now: time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC),
		profile: Profile{ID: "dynamic", Version: 1, Candidates: []Candidate{
			{ID: "a", Enabled: true, BindingKey: ResourceKey(ScopeProfile, "a")},
			{ID: "b", Enabled: true, BindingKey: ResourceKey(ScopeProfile, "b")},
		}},
	}
	f.engine = NewEngine(WithClock(func() time.Time { return f.now }))
	return f
}

// exhaust claims candidate a and then leaves the route waiting with both
// circuits open, as two capacity failures would.
func (f *failedRouteRetryFixture) exhaust(aUntil, bUntil time.Time) int64 {
	f.t.Helper()
	first, err := f.engine.Select("session", f.profile, 0, "")
	if err != nil {
		f.t.Fatalf("Select: %v", err)
	}
	f.engine.Circuits().Open(ResourceKey(ScopeProfile, "a"), aUntil, routingerr.CodeRateLimited)
	f.engine.Circuits().Open(ResourceKey(ScopeProfile, "b"), bUntil, routingerr.CodeRateLimited)
	_, err = f.engine.Select("session", f.profile, first.Generation, "a")
	var noCandidate *NoEligibleCandidateError
	if !errors.As(err, &noCandidate) {
		f.t.Fatalf("Select error = %v, want no eligible candidate", err)
	}
	return noCandidate.Generation
}

func (f *failedRouteRetryFixture) schedule(generation int64, failure FailedRouteFailure, limit int) (PendingFailedRouteRetry, bool, error) {
	return f.engine.ScheduleFailedRouteRetry(f.ctx, "session", f.profile, generation, failure, limit, time.Minute)
}

var rateLimitedOnA = FailedRouteFailure{ExecutionProfileID: "a", Code: routingerr.CodeRateLimited}

func TestScheduleFailedRouteRetryDueIsEarliestCircuitExpiryButNotBeforeMinimum(t *testing.T) {
	f := newFailedRouteRetryFixture(t)
	generation := f.exhaust(f.now.Add(10*time.Minute), f.now.Add(4*time.Minute))
	pending, scheduled, err := f.schedule(generation, rateLimitedOnA, 1)
	if err != nil || !scheduled || !pending.Due.Equal(f.now.Add(4*time.Minute)) {
		t.Fatalf("schedule = %#v scheduled=%v err=%v, want due at b's expiry", pending, scheduled, err)
	}

	short := newFailedRouteRetryFixture(t)
	generation = short.exhaust(short.now.Add(10*time.Second), short.now.Add(20*time.Second))
	pending, _, err = short.schedule(generation, rateLimitedOnA, 1)
	if err != nil || !pending.Due.Equal(short.now.Add(time.Minute)) {
		t.Fatalf("schedule = %#v err=%v, want the one-minute minimum", pending, err)
	}
}

func TestScheduleFailedRouteRetryActionRequiredWaitsForOwnCircuit(t *testing.T) {
	f := newFailedRouteRetryFixture(t)
	decision, err := f.engine.Select("session", f.profile, 0, "")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if _, err := f.engine.MarkActionRequired(f.ctx, "session", decision.Generation, "rate_limited"); err != nil {
		t.Fatalf("MarkActionRequired: %v", err)
	}
	f.engine.Circuits().Open(ResourceKey(ScopeProfile, "a"), f.now.Add(3*time.Minute), routingerr.CodeRateLimited)
	// b recovers sooner but an action_required Retry resumes a, not b.
	f.engine.Circuits().Open(ResourceKey(ScopeProfile, "b"), f.now.Add(90*time.Second), routingerr.CodeRateLimited)
	pending, _, err := f.schedule(decision.Generation, rateLimitedOnA, 1)
	if err != nil || !pending.Due.Equal(f.now.Add(3*time.Minute)) {
		t.Fatalf("schedule = %#v err=%v, want a's own circuit expiry", pending, err)
	}
}

func TestScheduleFailedRouteRetryIsIdempotentPerGeneration(t *testing.T) {
	f := newFailedRouteRetryFixture(t)
	generation := f.exhaust(f.now.Add(2*time.Minute), f.now.Add(2*time.Minute))
	first, scheduled, err := f.schedule(generation, rateLimitedOnA, 1)
	if err != nil || !scheduled {
		t.Fatalf("first schedule: scheduled=%v err=%v", scheduled, err)
	}
	f.now = f.now.Add(30 * time.Second)
	again, scheduled, err := f.schedule(generation, rateLimitedOnA, 1)
	if err != nil || scheduled || again != first {
		t.Fatalf("duplicate schedule = %#v scheduled=%v err=%v, want the existing schedule", again, scheduled, err)
	}
}

func TestClaimFailedRouteRetryConsumesOnceWhenDue(t *testing.T) {
	f := newFailedRouteRetryFixture(t)
	generation := f.exhaust(f.now.Add(2*time.Minute), f.now.Add(2*time.Minute))
	pending, _, err := f.schedule(generation, rateLimitedOnA, 1)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if _, err := f.engine.ClaimFailedRouteRetry(f.ctx, "session", generation); !errors.Is(err, ErrRecoveryNotDue) {
		t.Fatalf("early claim error = %v, want not due", err)
	}
	f.now = pending.Due
	if _, err := f.engine.ClaimFailedRouteRetry(f.ctx, "session", generation); err != nil {
		t.Fatalf("due claim: %v", err)
	}
	if _, err := f.engine.ClaimFailedRouteRetry(f.ctx, "session", generation); !errors.Is(err, ErrFailedRouteRetryDeclined) {
		t.Fatalf("second claim error = %v, want declined", err)
	}
	chain, err := f.engine.FailedRouteRetryChainFor(f.ctx, "session")
	if err != nil || chain.Attempts != 1 || chain.Pending != nil || len(chain.Failures) != 1 {
		t.Fatalf("chain = %#v err=%v, want one consumed attempt", chain, err)
	}
}

func TestFailedRouteRetryChainSurvivesNewGenerationAndCapsAttempts(t *testing.T) {
	f := newFailedRouteRetryFixture(t)
	generation := f.exhaust(f.now.Add(time.Minute), f.now.Add(time.Minute))
	pending, _, _ := f.schedule(generation, rateLimitedOnA, 2)
	f.now = pending.Due
	if _, err := f.engine.ClaimFailedRouteRetry(f.ctx, "session", generation); err != nil {
		t.Fatalf("claim: %v", err)
	}
	// The retry reselects a (its circuit expired and admits a probe), a fails
	// again, b is still limited, and the route is waiting at a later
	// generation.
	retried, err := f.engine.SelectContextWithReason(f.ctx, "session", f.profile, generation, "", "auto_failed_route_retry")
	if err != nil {
		t.Fatalf("retry select: %v", err)
	}
	f.engine.ReleaseProbe(retried, false)
	f.engine.Circuits().Open(ResourceKey(ScopeProfile, "b"), f.now.Add(time.Minute), routingerr.CodeRateLimited)
	_, err = f.engine.Select("session", f.profile, retried.Generation, retried.ExecutionProfileID)
	var noCandidate *NoEligibleCandidateError
	if !errors.As(err, &noCandidate) {
		t.Fatalf("Select error = %v, want no eligible candidate", err)
	}
	chain, _ := f.engine.FailedRouteRetryChainFor(f.ctx, "session")
	if chain.Attempts != 1 || chain.Pending != nil {
		t.Fatalf("carried chain = %#v, want one attempt and no pending schedule", chain)
	}
	if _, _, err := f.schedule(noCandidate.Generation, rateLimitedOnA, 2); !errors.Is(err, ErrFailedRouteRetryDeclined) {
		t.Fatalf("repeat of a failure already retried: err = %v, want declined", err)
	}
	onB := FailedRouteFailure{ExecutionProfileID: "b", Code: routingerr.CodeRateLimited}
	if _, _, err := f.schedule(noCandidate.Generation, onB, 1); !errors.Is(err, ErrFailedRouteRetryDeclined) {
		t.Fatalf("limit reached: err = %v, want declined", err)
	}
	if _, scheduled, err := f.schedule(noCandidate.Generation, onB, 2); err != nil || !scheduled {
		t.Fatalf("second attempt within limit: scheduled=%v err=%v", scheduled, err)
	}
}

func TestClearFailedRouteRetryResetsChain(t *testing.T) {
	f := newFailedRouteRetryFixture(t)
	generation := f.exhaust(f.now.Add(time.Minute), f.now.Add(time.Minute))
	if _, _, err := f.schedule(generation, rateLimitedOnA, 1); err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if err := f.engine.ClearFailedRouteRetry(f.ctx, "session"); err != nil {
		t.Fatalf("ClearFailedRouteRetry: %v", err)
	}
	if state, _ := f.engine.State("session"); state.PolicyStateJSON != `{"failure_code":"","failure_class":"","catalogue_version":"","policy_json":"","retry_ordinal":0,"reset_wait_used":false,"pending_outcome":""}` {
		t.Fatalf("policy state = %s, want the chain removed", state.PolicyStateJSON)
	}
	if _, scheduled, err := f.schedule(generation, rateLimitedOnA, 1); err != nil || !scheduled {
		t.Fatalf("schedule after reset: scheduled=%v err=%v", scheduled, err)
	}
}

func TestScheduleFailedRouteRetryDeclinesUnrecoverableRoutes(t *testing.T) {
	f := newFailedRouteRetryFixture(t)
	decision, err := f.engine.Select("session", f.profile, 0, "")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if _, _, err := f.schedule(decision.Generation, rateLimitedOnA, 1); !errors.Is(err, ErrFailedRouteRetryDeclined) {
		t.Fatalf("starting route: err = %v, want declined", err)
	}
	if _, err := f.engine.MarkActionRequired(f.ctx, "session", decision.Generation, "account_blocked"); err != nil {
		t.Fatalf("MarkActionRequired: %v", err)
	}
	if err := f.engine.HoldAccountBlock("a", routingerr.CodeAccountBlocked); err != nil {
		t.Fatalf("HoldAccountBlock: %v", err)
	}
	if _, _, err := f.schedule(decision.Generation, rateLimitedOnA, 1); !errors.Is(err, ErrFailedRouteRetryDeclined) {
		t.Fatalf("held candidate: err = %v, want declined", err)
	}
	if _, _, err := f.schedule(decision.Generation-1, rateLimitedOnA, 1); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("stale generation: err = %v, want stale", err)
	}
}
