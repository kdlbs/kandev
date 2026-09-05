package dynamic

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agent/runtime/routingpolicy"
)

func TestEngineSelectsFixedOrderAndFencesGenerations(t *testing.T) {
	now := time.Unix(100, 0)
	engine := NewEngine(WithClock(func() time.Time { return now }))
	profile := Profile{
		ID:      "dynamic-1",
		Version: 4,
		Candidates: []Candidate{
			{ID: "disabled", Enabled: false, BindingKey: "disabled"},
			{ID: "open", Enabled: true, BindingKey: "open"},
			{ID: "fallback", Enabled: true, BindingKey: "fallback"},
		},
	}
	engine.Circuits().Open("open", now.Add(time.Hour), routingerr.CodeProviderUnavailable)

	decision, err := engine.Select("session-1", profile, 0, "")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if decision.ExecutionProfileID != "fallback" || decision.Generation != 1 || decision.ProfileVersion != 4 {
		t.Fatalf("decision = %#v", decision)
	}
	if _, err := engine.Select("session-1", profile, 0, ""); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("stale select error = %v, want %v", err, ErrStaleGeneration)
	}
	engine.Circuits().Open("open", now.Add(-time.Second), routingerr.CodeProviderUnavailable)
	decision, err = engine.Select("session-1", profile, 1, "fallback")
	if err != nil {
		t.Fatalf("Try next select: %v", err)
	}
	if decision.ExecutionProfileID != "open" || decision.Generation != 2 {
		t.Fatalf("try next decision = %#v", decision)
	}
}

func TestEnginePreferenceKeepsRetryOnCurrentCandidate(t *testing.T) {
	engine := NewEngine()
	profile := Profile{
		ID: "dynamic-1",
		Candidates: []Candidate{
			{ID: "first", Enabled: true},
			{ID: "second", Enabled: true},
		},
	}
	decision, err := engine.SelectContextWithPreference(
		context.Background(), "session-1", profile, 0, "", "second",
	)
	if err != nil {
		t.Fatalf("SelectContextWithPreference: %v", err)
	}
	if decision.ExecutionProfileID != "second" {
		t.Fatalf("preferred decision = %#v", decision)
	}
}

func TestEngineAppliesCandidateErrorActions(t *testing.T) {
	profile := Profile{
		ID:      "dynamic-1",
		Version: 1,
		Candidates: []Candidate{
			{ID: "first", Enabled: true, BindingKey: "first", Rules: map[string]Action{
				string(routingerr.CodeQuotaLimited): ActionTryNext,
				string(routingerr.CodeRateLimited):  ActionRetrySame,
				string(routingerr.CodeTask):         ActionStop,
			}},
			{ID: "second", Enabled: true, BindingKey: "second"},
		},
	}
	engine := NewEngine()
	if got := engine.ActionFor(profile, "first", routingerr.CodeQuotaLimited); got != ActionTryNext {
		t.Fatalf("quota action = %q, want %q", got, ActionTryNext)
	}
	if got := engine.ActionFor(profile, "first", routingerr.CodeRateLimited); got != ActionRetrySame {
		t.Fatalf("rate action = %q, want %q", got, ActionRetrySame)
	}
	if got := engine.ActionFor(profile, "first", routingerr.CodeTask); got != ActionStop {
		t.Fatalf("task action = %q, want %q", got, ActionStop)
	}
	if got := engine.ActionFor(profile, "second", routingerr.CodeQuotaLimited); got != ActionStop {
		t.Fatalf("unconfigured action = %q, want %q", got, ActionStop)
	}
}

// Contract coverage: Codex's complete usage-limit envelope must reach the
// existing hard-error policy without adding a provider branch to the engine.
func TestEngineRoutesCodexUsageLimitThroughHardPolicy(t *testing.T) {
	failure := routingerr.Classify(routingerr.Input{
		Phase:      routingerr.PhasePromptSend,
		ProviderID: "codex-acp",
		Stderr:     `{"code":-32603,"message":"Internal error","data":{"codexErrorInfo":"usageLimitExceeded","message":"You've hit your usage limit. Visit https://chatgpt.com/codex/settings/usage to purchase more credits or try again at Sep 1st, 2026 3:14 PM."}}`,
	})
	if failure.Code != routingerr.CodeQuotaLimited || failure.Class != routingerr.ClassHard {
		t.Fatalf("classification = %+v, want hard quota_limited", failure)
	}

	profile := Profile{
		ID: "dynamic-1", Version: 1,
		Candidates: []Candidate{
			{ID: "codex", Enabled: true, BindingKey: "codex", Policies: routingpolicy.DefaultDocument()},
			{ID: "fallback", Enabled: true, BindingKey: "fallback", Policies: routingpolicy.DefaultDocument()},
		},
	}
	engine := NewEngine()
	initial, err := engine.Select("session-1", profile, 0, "")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}

	decision, err := engine.ApplyFailure(
		"session-1", profile, initial.Generation, initial.ExecutionProfileID, failure,
	)
	if err != nil {
		t.Fatalf("ApplyFailure: %v", err)
	}
	if decision.ExecutionProfileID != "fallback" || decision.Reason != "policy_skip" {
		t.Fatalf("decision = %#v, want fallback selected by hard policy", decision)
	}
}

func TestEngineRetrySameKeepsTheFailedCandidate(t *testing.T) {
	profile := Profile{
		ID: "dynamic-1", Version: 1,
		Candidates: []Candidate{
			{ID: "first", Enabled: true, Rules: map[string]Action{
				string(routingerr.CodeRateLimited): ActionRetrySame,
			}},
			{ID: "second", Enabled: true},
		},
	}
	engine := NewEngine()
	initial, err := engine.Select("session-1", profile, 0, "")
	if err != nil {
		t.Fatalf("initial Select: %v", err)
	}
	initialDecision, err := engine.ApplyFailure(
		"session-1", profile, initial.Generation, initial.ExecutionProfileID,
		&routingerr.Error{Code: routingerr.CodeRateLimited},
	)
	if err != nil {
		t.Fatalf("ApplyFailure: %v", err)
	}
	if initialDecision.ExecutionProfileID != "first" || initialDecision.Generation != 2 {
		t.Fatalf("retry decision = %#v, want first at generation 2", initialDecision)
	}
}

func TestEnginePersistsPolicyWaitAndResumesWithGenerationFence(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	engine := NewEngine(WithClock(func() time.Time { return now }))
	document := routingpolicy.DefaultDocument()
	document.Transient.Retry = routingpolicy.RetryPolicy{
		Enabled: true, MaxRetries: 2, InitialIntervalSeconds: 5,
	}
	profile := Profile{
		ID: "dynamic-policy", Version: 1,
		Candidates: []Candidate{
			{ID: "first", Enabled: true, Policies: document},
			{ID: "second", Enabled: true},
		},
	}
	initial, err := engine.Select("policy-session", profile, 0, "")
	if err != nil {
		t.Fatalf("initial Select: %v", err)
	}
	decision, err := engine.ApplyFailure(
		"policy-session", profile, initial.Generation, initial.ExecutionProfileID,
		&routingerr.Error{
			Code: routingerr.CodeRateLimited, Class: routingerr.ClassTransient,
			FallbackAllowed: true,
		},
	)
	if !errors.Is(err, ErrRecoveryPending) {
		t.Fatalf("ApplyFailure error = %v, want pending recovery", err)
	}
	if decision.Status != "retry_wait" || decision.Generation != initial.Generation {
		t.Fatalf("pending decision = %#v", decision)
	}
	state, ok := engine.State("policy-session")
	if !ok || state.PolicyStateJSON == "" {
		t.Fatalf("policy state was not persisted: %#v, ok=%v", state, ok)
	}
	now = now.Add(5 * time.Second)
	due, err := engine.ResumePending(context.Background(), "policy-session", initial.Generation)
	if err != nil {
		t.Fatalf("ResumePending: %v", err)
	}
	if due.Status != "retrying" || due.ExecutionProfileID != "first" {
		t.Fatalf("due decision = %#v", due)
	}
	if _, err := engine.ResumePending(context.Background(), "policy-session", initial.Generation-1); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("stale ResumePending error = %v", err)
	}
}

func TestEngineManualRetryNowBypassesPolicyDeadlineWithoutAdvancingGeneration(t *testing.T) {
	now := time.Unix(300, 0).UTC()
	engine := NewEngine(WithClock(func() time.Time { return now }))
	document := routingpolicy.DefaultDocument()
	document.Transient.Retry = routingpolicy.RetryPolicy{Enabled: true, MaxRetries: 1, InitialIntervalSeconds: 60}
	profile := Profile{ID: "manual-policy", Version: 1, Candidates: []Candidate{
		{ID: "first", Enabled: true, Policies: document},
		{ID: "second", Enabled: true},
	}}
	initial, err := engine.Select("manual-session", profile, 0, "")
	if err != nil {
		t.Fatalf("initial Select: %v", err)
	}
	pending, err := engine.ApplyFailure(
		"manual-session", profile, initial.Generation, initial.ExecutionProfileID,
		&routingerr.Error{Code: routingerr.CodeRateLimited, Class: routingerr.ClassTransient, FallbackAllowed: true},
	)
	if !errors.Is(err, ErrRecoveryPending) || pending.Status != "retry_wait" {
		t.Fatalf("pending decision = %#v, error = %v", pending, err)
	}
	manual, err := engine.ResumePendingNow(context.Background(), "manual-session", initial.Generation)
	if err != nil {
		t.Fatalf("ResumePendingNow: %v", err)
	}
	if manual.Generation != initial.Generation || manual.ExecutionProfileID != "first" || manual.Status != "retrying" {
		t.Fatalf("manual decision = %#v", manual)
	}
}

func TestEngineManualSkipRecordsItsReason(t *testing.T) {
	engine := NewEngine()
	profile := Profile{ID: "manual-skip", Version: 1, Candidates: []Candidate{
		{ID: "first", Enabled: true},
		{ID: "second", Enabled: true},
	}}
	initial, err := engine.Select("skip-session", profile, 0, "")
	if err != nil {
		t.Fatalf("initial Select: %v", err)
	}
	next, err := engine.SelectContextWithReason(
		context.Background(), "skip-session", profile, initial.Generation, initial.ExecutionProfileID, "manual_skip",
	)
	if err != nil {
		t.Fatalf("manual skip: %v", err)
	}
	if next.ExecutionProfileID != "second" || next.Reason != "manual_skip" {
		t.Fatalf("manual skip decision = %#v", next)
	}
}

func TestEngineCancelPendingStopsWithoutAdvancingGeneration(t *testing.T) {
	engine := NewEngine()
	document := routingpolicy.DefaultDocument()
	document.Transient.Retry = routingpolicy.RetryPolicy{Enabled: true, MaxRetries: 1, InitialIntervalSeconds: 5}
	profile := Profile{ID: "cancel-policy", Version: 1, Candidates: []Candidate{
		{ID: "first", Enabled: true, Policies: document},
	}}
	initial, err := engine.Select("cancel-session", profile, 0, "")
	if err != nil {
		t.Fatalf("initial Select: %v", err)
	}
	pending, err := engine.ApplyFailure(
		"cancel-session", profile, initial.Generation, initial.ExecutionProfileID,
		&routingerr.Error{Code: routingerr.CodeRateLimited, Class: routingerr.ClassTransient, FallbackAllowed: true},
	)
	if !errors.Is(err, ErrRecoveryPending) || pending.Status != "retry_wait" {
		t.Fatalf("pending decision = %#v, error = %v", pending, err)
	}
	stopped, err := engine.CancelPending(context.Background(), "cancel-session", initial.Generation, "manual_stop")
	if err != nil {
		t.Fatalf("CancelPending: %v", err)
	}
	if stopped.Status != "action_required" || stopped.Generation != initial.Generation || stopped.Reason != "manual_stop" {
		t.Fatalf("stopped decision = %#v", stopped)
	}
}

func TestEngineMarkActiveCompletesStartingPhase(t *testing.T) {
	engine := NewEngine()
	profile := Profile{ID: "dynamic-active", Candidates: []Candidate{{ID: "first", Enabled: true}}}
	initial, err := engine.Select("active-session", profile, 0, "")
	if err != nil {
		t.Fatalf("initial Select: %v", err)
	}
	state, ok := engine.State("active-session")
	if !ok || state.Status != routeStatusStarting {
		t.Fatalf("precondition: state = %#v, ok=%v, want starting", state, ok)
	}
	if err := engine.MarkActive(context.Background(), "active-session", initial.Generation); err != nil {
		t.Fatalf("MarkActive: %v", err)
	}
	state, ok = engine.State("active-session")
	if !ok || state.Status != routeStatusActive || state.Generation != initial.Generation {
		t.Fatalf("state after MarkActive = %#v, ok=%v", state, ok)
	}
	// A repeat call (e.g. from a duplicate callback) is a no-op, not an error.
	if err := engine.MarkActive(context.Background(), "active-session", initial.Generation); err != nil {
		t.Fatalf("repeat MarkActive: %v", err)
	}
	if err := engine.MarkActive(context.Background(), "active-session", initial.Generation-1); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("stale MarkActive error = %v, want %v", err, ErrStaleGeneration)
	}
	if err := engine.MarkActive(context.Background(), "missing-session", 1); !errors.Is(err, ErrRouteStateNotFound) {
		t.Fatalf("missing session MarkActive error = %v, want %v", err, ErrRouteStateNotFound)
	}
}

func TestEngineMarkActiveIsNoOpWhenRouteAlreadyLeftStarting(t *testing.T) {
	engine := NewEngine()
	profile := Profile{ID: "dynamic-active-skip", Candidates: []Candidate{
		{ID: "first", Enabled: true},
		{ID: "second", Enabled: true},
	}}
	initial, err := engine.Select("active-skip-session", profile, 0, "")
	if err != nil {
		t.Fatalf("initial Select: %v", err)
	}
	next, err := engine.SelectContextWithReason(
		context.Background(), "active-skip-session", profile, initial.Generation, initial.ExecutionProfileID, "manual_skip",
	)
	if err != nil {
		t.Fatalf("manual skip: %v", err)
	}
	// A late MarkActive call for the superseded generation must not resurrect
	// it as active over the newer claim.
	if err := engine.MarkActive(context.Background(), "active-skip-session", initial.Generation); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("MarkActive for superseded generation error = %v, want %v", err, ErrStaleGeneration)
	}
	state, ok := engine.State("active-skip-session")
	if !ok || state.Generation != next.Generation || state.Status != routeStatusStarting {
		t.Fatalf("state after stale MarkActive = %#v, ok=%v", state, ok)
	}
}

func TestEngineMarkActionRequiredFromStartingAndFencesGeneration(t *testing.T) {
	engine := NewEngine()
	profile := Profile{ID: "dynamic-action-required", Candidates: []Candidate{{ID: "first", Enabled: true}}}
	initial, err := engine.Select("action-required-session", profile, 0, "")
	if err != nil {
		t.Fatalf("initial Select: %v", err)
	}
	// A route sitting in "starting" (the exact stranded case this exists to
	// fix) must be acceptable input.
	decision, err := engine.MarkActionRequired(context.Background(), "action-required-session", initial.Generation, "launch_failed")
	if err != nil {
		t.Fatalf("MarkActionRequired: %v", err)
	}
	if decision.Status != routeStatusActionRequired || decision.Reason != "launch_failed" || decision.Generation != initial.Generation {
		t.Fatalf("decision = %#v", decision)
	}
	state, ok := engine.State("action-required-session")
	if !ok || state.Status != routeStatusActionRequired {
		t.Fatalf("state after MarkActionRequired = %#v, ok=%v", state, ok)
	}
	// A second call (a different failure path re-declaring the same route) is
	// idempotent, not an error, and keeps its own reason.
	decision, err = engine.MarkActionRequired(context.Background(), "action-required-session", initial.Generation, "second_reason")
	if err != nil {
		t.Fatalf("repeat MarkActionRequired: %v", err)
	}
	if decision.Reason != "second_reason" || decision.Status != routeStatusActionRequired {
		t.Fatalf("repeat decision = %#v", decision)
	}
	if _, err := engine.MarkActionRequired(context.Background(), "action-required-session", initial.Generation-1, "stale"); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("stale MarkActionRequired error = %v, want %v", err, ErrStaleGeneration)
	}
	if _, err := engine.MarkActionRequired(context.Background(), "missing-session", 1, "missing"); !errors.Is(err, ErrRouteStateNotFound) {
		t.Fatalf("missing session MarkActionRequired error = %v, want %v", err, ErrRouteStateNotFound)
	}
}

// TestEngineMarkActionRequiredIsNoOpOnceRouteIsActive is the regression test
// for review finding F1: a route a launch already carried to "active" must
// not be demoted to action_required by an unrelated later failure (for
// example a permission-denied or resume-corrupted error the classifier
// forbids fallback for), which would surface a "Try next provider" recovery
// banner on a healthy session.
func TestEngineMarkActionRequiredIsNoOpOnceRouteIsActive(t *testing.T) {
	engine := NewEngine()
	profile := Profile{ID: "dynamic-action-required-active", Candidates: []Candidate{{ID: "first", Enabled: true}}}
	initial, err := engine.Select("active-route-session", profile, 0, "")
	if err != nil {
		t.Fatalf("initial Select: %v", err)
	}
	if err := engine.MarkActive(context.Background(), "active-route-session", initial.Generation); err != nil {
		t.Fatalf("MarkActive: %v", err)
	}
	decision, err := engine.MarkActionRequired(context.Background(), "active-route-session", initial.Generation, "permission_denied")
	if err != nil {
		t.Fatalf("MarkActionRequired on active route: %v", err)
	}
	if decision.Status != routeStatusActive {
		t.Fatalf("decision.Status = %q, want unchanged %q", decision.Status, routeStatusActive)
	}
	state, ok := engine.State("active-route-session")
	if !ok || state.Status != routeStatusActive {
		t.Fatalf("state after no-op MarkActionRequired = %#v, ok=%v, want unchanged active", state, ok)
	}
}

// TestEngineMarkActionRequiredFromRetryingTransitionsAtSameGeneration is the
// regression test restoring the deferred recovery guard in
// LaunchDynamicRouteAction for the timer-recovery path: persistDynamicPolicyRecovery
// leaves a resumed route at "retrying" (resume never advances the generation),
// so a launch that fails after that resume must still reach action_required
// instead of sticking at "retrying" with a dead Retry/Stop banner.
func TestEngineMarkActionRequiredFromRetryingTransitionsAtSameGeneration(t *testing.T) {
	engine := NewEngine()
	document := routingpolicy.DefaultDocument()
	document.Transient.Retry = routingpolicy.RetryPolicy{Enabled: true, MaxRetries: 1, InitialIntervalSeconds: 60}
	profile := Profile{ID: "dynamic-retrying-action-required", Candidates: []Candidate{
		{ID: "first", Enabled: true, Policies: document},
	}}
	initial, err := engine.Select("retrying-action-required-session", profile, 0, "")
	if err != nil {
		t.Fatalf("initial Select: %v", err)
	}
	if _, err := engine.ApplyFailure(
		"retrying-action-required-session", profile, initial.Generation, initial.ExecutionProfileID,
		&routingerr.Error{Code: routingerr.CodeRateLimited, Class: routingerr.ClassTransient, FallbackAllowed: true},
	); !errors.Is(err, ErrRecoveryPending) {
		t.Fatalf("ApplyFailure: %v", err)
	}
	resumed, err := engine.ResumePendingNow(context.Background(), "retrying-action-required-session", initial.Generation)
	if err != nil {
		t.Fatalf("ResumePendingNow: %v", err)
	}
	if resumed.Status != routeStatusRetrying {
		t.Fatalf("precondition: resumed status = %q, want %q", resumed.Status, routeStatusRetrying)
	}

	decision, err := engine.MarkActionRequired(
		context.Background(), "retrying-action-required-session", initial.Generation, "route_action_launch_failed",
	)
	if err != nil {
		t.Fatalf("MarkActionRequired from retrying: %v", err)
	}
	if decision.Status != routeStatusActionRequired || decision.Generation != initial.Generation {
		t.Fatalf("decision = %#v", decision)
	}
	state, ok := engine.State("retrying-action-required-session")
	if !ok || state.Status != routeStatusActionRequired || state.Generation != initial.Generation {
		t.Fatalf("state after MarkActionRequired = %#v, ok=%v", state, ok)
	}
}

func TestBindingFingerprinterUsesOpaqueStableHMACKeys(t *testing.T) {
	key := []byte("installation-secret")
	fingerprinter := NewBindingFingerprinter(key)
	descriptor := CredentialBindingDescriptor{
		Version:              1,
		AgentFamilyID:        "claude-acp",
		AuthenticationMethod: "subscription",
		CredentialSourceKind: "credential_file",
		CredentialLocator:    "home:claude",
		ExecutorNamespace:    "local",
		AuthorizationScope:   "messages",
		WorkspaceScope:       "workspace-1",
	}
	a := fingerprinter.Fingerprint(descriptor)
	b := fingerprinter.Fingerprint(descriptor)
	if a == "" || a != b || len(a) != 64 {
		t.Fatalf("fingerprints = %q and %q", a, b)
	}
	changed := descriptor
	changed.WorkspaceScope = "workspace-2"
	if a == fingerprinter.Fingerprint(changed) {
		t.Fatal("different binding scopes shared a fingerprint")
	}
	if containsSensitive(a, "installation-secret") || containsSensitive(a, "home:claude") {
		t.Fatal("fingerprint exposed descriptor material")
	}
	if fallback := fingerprinter.FallbackFingerprint("profile-1"); fallback != "profile:profile-1" {
		t.Fatalf("fallback fingerprint = %q", fallback)
	}
}

func TestCredentialBindingResolverCanonicalizesAndFallsBackToProfileScope(t *testing.T) {
	resolver := NewCredentialBindingResolver([]byte("installation-secret"))
	base := CredentialBindingDescriptor{
		Version: 1, AgentFamilyID: "Claude-ACP", AuthenticationMethod: "Subscription",
		CredentialSourceKind: "credential_file", CredentialLocator: " home:claude ",
	}
	if resolver.Fingerprint(base) != resolver.Fingerprint(CredentialBindingDescriptor{
		Version: 1, AgentFamilyID: "claude-acp", AuthenticationMethod: "subscription",
		CredentialSourceKind: "credential_file", CredentialLocator: "home:claude",
	}) {
		t.Fatal("canonical descriptors produced different fingerprints")
	}
	if got := resolver.Resolve(CredentialBindingDescriptor{}, "profile-1"); got != "profile:profile-1" {
		t.Fatalf("fallback binding = %q", got)
	}
}

func TestCircuitProbeLeaseIsExclusive(t *testing.T) {
	now := time.Unix(200, 0)
	registry := NewCircuitRegistry(WithCircuitClock(func() time.Time { return now }))
	registry.Open("provider:claude", now.Add(-time.Second), routingerr.CodeQuotaLimited)
	lease, ok := registry.AcquireProbe("provider:claude", time.Minute)
	if !ok || lease.Key != "provider:claude" {
		t.Fatalf("first probe lease = %#v, ok=%v", lease, ok)
	}
	if _, ok := registry.AcquireProbe("provider:claude", time.Minute); ok {
		t.Fatal("second worker acquired the same half-open probe")
	}
	now = now.Add(2 * time.Minute)
	lease2, ok := registry.AcquireProbe("provider:claude", time.Minute)
	if !ok {
		t.Fatal("expired probe lease was not replaced")
	}
	registry.ReleaseProbe(lease, true, time.Second)
	if _, ok := registry.AcquireProbe("provider:claude", time.Minute); ok {
		t.Fatal("stale probe lease changed the current half-open circuit")
	}
	registry.ReleaseProbe(lease2, false, time.Second)
	now = now.Add(2 * time.Second)
	if _, ok := registry.AcquireProbe("provider:claude", time.Minute); !ok {
		t.Fatal("probe lease was not released after failure backoff")
	}
}

func TestEngineUsesExclusiveProbeForExpiredCircuit(t *testing.T) {
	now := time.Unix(500, 0)
	engine := NewEngine(WithClock(func() time.Time { return now }))
	engine.Circuits().Open("shared-binding", now.Add(-time.Second), routingerr.CodeQuotaLimited)
	profile := Profile{ID: "dynamic-probe", Candidates: []Candidate{
		{ID: "first", Enabled: true, BindingKey: "shared-binding"},
		{ID: "second", Enabled: true, BindingKey: "other"},
	}}
	first, err := engine.Select("probe-session-1", profile, 0, "")
	if err != nil || first.ExecutionProfileID != "first" {
		t.Fatalf("first selection = %#v, %v; want probe candidate", first, err)
	}
	second, err := engine.Select("probe-session-2", profile, 0, "")
	if err != nil || second.ExecutionProfileID != "second" {
		t.Fatalf("second selection = %#v, %v; want other candidate while probe is held", second, err)
	}
	engine.ReleaseProbe(first, true)
}

func containsSensitive(value, sensitive string) bool {
	return sensitive != "" && strings.Contains(value, sensitive)
}
