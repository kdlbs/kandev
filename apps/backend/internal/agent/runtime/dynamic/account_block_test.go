package dynamic

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agent/runtime/routingpolicy"
)

func accountBlockedFailure() *routingerr.Error {
	return routingerr.Classify(routingerr.Input{
		Phase: routingerr.PhasePromptSend,
		Stderr: `Error from provider example-gateway: 403 Forbidden {"error":{"code":"user_blocked",` +
			`"message":"Your access has been restricted due to repeated policy violations."}}`,
	})
}

// accountBlockProfile has a blocked candidate, a sibling on the same
// credential binding, and an unrelated candidate, in that order.
func accountBlockProfile(hard routingpolicy.Policy) Profile {
	policies := routingpolicy.DefaultDocument()
	policies.Hard = hard
	return Profile{
		ID: "dynamic-account-block", Version: 1,
		Candidates: []Candidate{
			{ID: "blocked", Enabled: true, BindingKey: "credential:shared", Policies: policies},
			{ID: "sibling", Enabled: true, BindingKey: "credential:shared", Policies: policies},
			{ID: "other", Enabled: true, BindingKey: "credential:other", Policies: policies},
		},
	}
}

func holdAccount(t *testing.T, engine *Engine, profile Profile, sessionID string) RouteDecision {
	t.Helper()
	initial, err := engine.Select(sessionID, profile, 0, "")
	if err != nil || initial.ExecutionProfileID != "blocked" {
		t.Fatalf("initial selection = %#v, %v; want blocked", initial, err)
	}
	decision, err := engine.ApplyFailure(sessionID, profile, initial.Generation, initial.ExecutionProfileID, accountBlockedFailure())
	if err != nil {
		t.Fatalf("ApplyFailure: %v", err)
	}
	return decision
}

func TestAccountBlockedHoldsOnlyFailingProfile(t *testing.T) {
	now := time.Unix(1000, 0)
	engine := NewEngine(WithClock(func() time.Time { return now }))
	hard := routingpolicy.DefaultPolicy()
	hard.Retry = routingpolicy.RetryPolicy{Enabled: true, MaxRetries: 3, InitialIntervalSeconds: 5}
	profile := accountBlockProfile(hard)

	decision := holdAccount(t, engine, profile, "session-a")

	if decision.ExecutionProfileID != "sibling" || decision.Reason != "policy_skip" {
		t.Fatalf("decision = %#v, want the credential-sharing sibling by policy_skip", decision)
	}
	if engine.Circuits().IsOpen("credential:shared", now) {
		t.Fatal("account block opened the shared credential circuit")
	}
	if !engine.Circuits().IsOpen(accountBlockKey("blocked"), now) {
		t.Fatal("account block did not hold the failing profile")
	}
	for _, id := range []string{"sibling", "other"} {
		if engine.Circuits().IsOpen(accountBlockKey(id), now) {
			t.Fatalf("account block held %s", id)
		}
	}
	fresh, err := engine.Select("session-b", profile, 0, "")
	if err != nil || fresh.ExecutionProfileID != "sibling" {
		t.Fatalf("fresh selection = %#v, %v; want sibling, the next candidate in order", fresh, err)
	}
}

func TestAccountBlockedStopOutcomeStillHolds(t *testing.T) {
	now := time.Unix(1000, 0)
	engine := NewEngine(WithClock(func() time.Time { return now }))
	stop := routingpolicy.DefaultPolicy()
	stop.OnExhausted = routingpolicy.OutcomeStop
	profile := accountBlockProfile(stop)
	initial, err := engine.Select("session-stop", profile, 0, "")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}

	decision, err := engine.ApplyFailure("session-stop", profile, initial.Generation, "blocked", accountBlockedFailure())

	if !errors.Is(err, ErrNoEligibleCandidate) || decision.Status != routeStatusActionRequired ||
		decision.ExecutionProfileID != "blocked" {
		t.Fatalf("decision = %#v, %v; want manual recovery on the failed candidate", decision, err)
	}
	if !engine.Circuits().IsOpen(accountBlockKey("blocked"), now) {
		t.Fatal("stop outcome did not hold the failing profile")
	}
	fresh, err := engine.Select("session-next", profile, 0, "")
	if err != nil || fresh.ExecutionProfileID != "sibling" {
		t.Fatalf("fresh selection = %#v, %v; want sibling", fresh, err)
	}
}

func TestAccountBlockedHoldHasNoExpiryOrProbe(t *testing.T) {
	now := time.Unix(1000, 0)
	engine := NewEngine(WithClock(func() time.Time { return now }))
	profile := accountBlockProfile(routingpolicy.DefaultPolicy())
	holdAccount(t, engine, profile, "session-a")
	key := accountBlockKey("blocked")

	now = now.Add(400 * 24 * time.Hour)
	if _, ok := engine.Circuits().AcquireProbe(key, time.Minute); ok {
		t.Fatal("held circuit granted a probe")
	}
	engine.Circuits().Open(key, now.Add(time.Minute), routingerr.CodeRateLimited)
	if !engine.Circuits().IsHeld(key) {
		t.Fatal("a timed failure replaced the hold")
	}
	later, err := engine.Select("session-later", profile, 0, "")
	if err != nil || later.ExecutionProfileID != "sibling" {
		t.Fatalf("selection after a year = %#v, %v; want sibling", later, err)
	}
}

func TestAccountBlockedHoldSurvivesRestore(t *testing.T) {
	now := time.Unix(1000, 0)
	store := newStubCircuitPersistence()
	clock := WithCircuitClock(func() time.Time { return now })
	engine := NewEngine(WithClock(func() time.Time { return now }),
		WithCircuitRegistry(NewCircuitRegistry(WithCircuitPersistence(store), clock)))
	profile := accountBlockProfile(routingpolicy.DefaultPolicy())
	holdAccount(t, engine, profile, "session-a")

	restored := NewCircuitRegistry(WithCircuitPersistence(store), clock)
	if err := restored.Restore(context.Background()); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	now = now.Add(30 * 24 * time.Hour)
	if !restored.IsOpen(accountBlockKey("blocked"), now) {
		t.Fatal("hold did not survive restart")
	}
	if _, ok := restored.AcquireProbe(accountBlockKey("blocked"), time.Minute); ok {
		t.Fatal("restored hold granted a probe")
	}
}

func TestAccountBlockedHoldRetriesFailedSave(t *testing.T) {
	now := time.Unix(1000, 0)
	store := newStubCircuitPersistence()
	clock := WithCircuitClock(func() time.Time { return now })
	registry := NewCircuitRegistry(WithCircuitPersistence(store), clock)
	key := accountBlockKey("blocked")
	store.setFailing(true)

	if _, err := registry.Hold(key, routingerr.CodeAccountBlocked); err == nil {
		t.Fatal("hold during outage reported no error")
	}
	if !registry.IsHeld(key) {
		t.Fatal("unsaved hold was not kept in memory")
	}
	store.setFailing(false)
	if _, err := registry.Hold(key, routingerr.CodeAccountBlocked); err != nil {
		t.Fatalf("repeated hold after outage: %v", err)
	}

	restored := NewCircuitRegistry(WithCircuitPersistence(store), clock)
	if err := restored.Restore(context.Background()); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if !restored.IsHeld(key) {
		t.Fatal("a repeated hold did not save the hold its first save lost")
	}
}

// A route that was waiting to retry a profile must not resume on it after
// another session held that profile, neither by schedule nor by Retry now.
func TestAccountBlockedHoldStopsPendingResume(t *testing.T) {
	for _, manual := range []bool{false, true} {
		now := time.Unix(1000, 0)
		engine := NewEngine(WithClock(func() time.Time { return now }))
		document := routingpolicy.DefaultDocument()
		document.Transient.Retry = routingpolicy.RetryPolicy{Enabled: true, MaxRetries: 2, InitialIntervalSeconds: 5}
		profile := Profile{
			ID: "dynamic-pending", Version: 1,
			Candidates: []Candidate{
				{ID: "blocked", Enabled: true, Policies: document},
				{ID: "next", Enabled: true, Policies: document},
			},
		}
		initial, err := engine.Select("session-waiting", profile, 0, "")
		if err != nil || initial.ExecutionProfileID != "blocked" {
			t.Fatalf("initial selection = %#v, %v; want blocked", initial, err)
		}
		if _, err := engine.ApplyFailure("session-waiting", profile, initial.Generation, "blocked", &routingerr.Error{
			Code: routingerr.CodeRateLimited, Class: routingerr.ClassTransient, FallbackAllowed: true,
		}); !errors.Is(err, ErrRecoveryPending) {
			t.Fatalf("ApplyFailure = %v, want a pending retry", err)
		}
		if err := engine.HoldAccountBlock("blocked", routingerr.CodeAccountBlocked); err != nil {
			t.Fatalf("HoldAccountBlock: %v", err)
		}
		now = now.Add(5 * time.Second)

		resume := engine.ResumePending
		if manual {
			resume = engine.ResumePendingNow
		}
		decision, err := resume(context.Background(), "session-waiting", initial.Generation)

		if !errors.Is(err, ErrExecutionProfileHeld) || !errors.Is(err, ErrRecoveryPending) {
			t.Fatalf("manual=%v resume = %#v, %v; want the held profile refused", manual, decision, err)
		}
		state, ok := engine.State("session-waiting")
		if !ok || state.Status != routeStatusActionRequired || state.ExecutionProfileID != "blocked" {
			t.Fatalf("manual=%v route state = %#v, %v; want manual recovery on the held profile", manual, state, ok)
		}
	}
}

func TestAccountBlockedHoldKeepsRouteManual(t *testing.T) {
	now := time.Unix(1000, 0)
	engine := NewEngine(WithClock(func() time.Time { return now }))
	profile := Profile{
		ID: "dynamic-single", Version: 1,
		Candidates: []Candidate{{ID: "blocked", Enabled: true, BindingKey: "credential:shared", Policies: routingpolicy.DefaultDocument()}},
	}
	initial, err := engine.Select("session-a", profile, 0, "")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if _, err := engine.ApplyFailure("session-a", profile, initial.Generation, "blocked", accountBlockedFailure()); err == nil {
		t.Fatal("a fully held route selected a candidate")
	}
}

func TestAccountBlockedLegacyRetryTriesNext(t *testing.T) {
	engine := NewEngine()
	profile := Profile{
		ID: "dynamic-legacy", Version: 1,
		Candidates: []Candidate{
			{ID: "blocked", Enabled: true, Rules: map[string]Action{"on_provider_error": ActionRetrySame}},
			{ID: "next", Enabled: true},
		},
	}
	initial, err := engine.Select("session-a", profile, 0, "")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	decision, err := engine.ApplyFailure("session-a", profile, initial.Generation, "blocked", accountBlockedFailure())
	if err != nil || decision.ExecutionProfileID != "next" {
		t.Fatalf("legacy retry decision = %#v, %v; want next", decision, err)
	}
}

func TestReleaseAccountBlockClosesOnlyHeldProfile(t *testing.T) {
	now := time.Unix(1000, 0)
	store := newStubCircuitPersistence()
	engine := NewEngine(WithClock(func() time.Time { return now }),
		WithCircuitRegistry(NewCircuitRegistry(WithCircuitPersistence(store),
			WithCircuitClock(func() time.Time { return now }))))
	profile := accountBlockProfile(routingpolicy.DefaultPolicy())
	holdAccount(t, engine, profile, "session-a")
	engine.Circuits().Open("credential:shared", now.Add(time.Hour), routingerr.CodeRateLimited)
	engine.Circuits().Open("credential:other", now.Add(time.Hour), routingerr.CodeQuotaLimited)

	for _, id := range []string{"sibling", "unknown", ""} {
		if released, err := engine.ReleaseAccountBlock(id); released || err != nil {
			t.Fatalf("release %q = %v, %v; want no hold", id, released, err)
		}
	}
	released, err := engine.ReleaseAccountBlock("blocked")
	if err != nil || !released {
		t.Fatalf("release blocked = %v, %v; want released", released, err)
	}
	if again, err := engine.ReleaseAccountBlock("blocked"); again || err != nil {
		t.Fatalf("repeated release = %v, %v; want no-op", again, err)
	}
	if engine.Circuits().IsOpen(accountBlockKey("blocked"), now) {
		t.Fatal("hold still open after release")
	}
	if row, ok := store.row(accountBlockKey("blocked")); !ok || row.State != CircuitClosed {
		t.Fatalf("persisted hold = %#v, %v; want closed", row, ok)
	}
	for _, key := range []string{"credential:shared", "credential:other"} {
		if !engine.Circuits().IsOpen(key, now) {
			t.Fatalf("release closed %s", key)
		}
	}
}

func TestReleaseAccountBlockKeepsHoldWhenSaveFails(t *testing.T) {
	now := time.Unix(1000, 0)
	store := newStubCircuitPersistence()
	registry := NewCircuitRegistry(WithCircuitPersistence(store), WithCircuitClock(func() time.Time { return now }))
	key := accountBlockKey("blocked")
	if _, err := registry.Hold(key, routingerr.CodeAccountBlocked); err != nil {
		t.Fatalf("Hold: %v", err)
	}
	store.setFailing(true)

	released, err := registry.ReleaseHold(key)
	if released || err == nil {
		t.Fatalf("release during outage = %v, %v; want error", released, err)
	}
	if !registry.IsOpen(key, now) {
		t.Fatal("failed release dropped the hold in memory")
	}
	store.setFailing(false)
	if released, err := registry.ReleaseHold(key); !released || err != nil {
		t.Fatalf("release after outage = %v, %v; want released", released, err)
	}
}

// A newly established hold and its owner release each emit one routing event
// with the execution profile, so the operational transition is observable. A
// repeated hold or release that changes nothing emits nothing.
func TestAccountBlockHoldAndReleaseEmitRoutingEvents(t *testing.T) {
	log, observed := newObservedLogger()
	engine := NewEngine(WithEngineLogger(log))

	// An empty execution profile id holds nothing and must not emit.
	if err := engine.HoldAccountBlock("", routingerr.CodeAccountBlocked); err != nil {
		t.Fatalf("empty-id HoldAccountBlock: %v", err)
	}
	if emitted := observed.Len(); emitted != 0 {
		t.Fatalf("empty-id hold emitted %d events, want 0", emitted)
	}

	if err := engine.HoldAccountBlock("blocked", routingerr.CodeAccountBlocked); err != nil {
		t.Fatalf("HoldAccountBlock: %v", err)
	}
	if err := engine.HoldAccountBlock("blocked", routingerr.CodeAccountBlocked); err != nil {
		t.Fatalf("repeated HoldAccountBlock: %v", err)
	}
	if released, err := engine.ReleaseAccountBlock("blocked"); err != nil || !released {
		t.Fatalf("ReleaseAccountBlock = %v, %v; want released", released, err)
	}
	if released, err := engine.ReleaseAccountBlock("blocked"); err != nil || released {
		t.Fatalf("repeated ReleaseAccountBlock = %v, %v; want no-op", released, err)
	}

	holds := observed.FilterMessage(eventAccountBlockHold).All()
	if len(holds) != 1 {
		t.Fatalf("hold events = %d, want 1", len(holds))
	}
	if got := holds[0].ContextMap()["execution_profile_id"]; got != "blocked" {
		t.Fatalf("hold execution_profile_id = %v, want blocked", got)
	}
	releases := observed.FilterMessage(eventAccountBlockRelease).All()
	if len(releases) != 1 {
		t.Fatalf("release events = %d, want 1", len(releases))
	}
	if got := releases[0].ContextMap()["execution_profile_id"]; got != "blocked" {
		t.Fatalf("release execution_profile_id = %v, want blocked", got)
	}
}

// Concurrent holds of one profile race for a single atomic registry
// transition, so the routing event is emitted exactly once regardless of the
// interleaving.
func TestAccountBlockConcurrentHoldsEmitOneEvent(t *testing.T) {
	log, observed := newObservedLogger()
	engine := NewEngine(WithEngineLogger(log))

	const callers = 16
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if err := engine.HoldAccountBlock("blocked", routingerr.CodeAccountBlocked); err != nil {
				t.Errorf("HoldAccountBlock: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if holds := observed.FilterMessage(eventAccountBlockHold).All(); len(holds) != 1 {
		t.Fatalf("concurrent hold events = %d, want 1", len(holds))
	}
}
