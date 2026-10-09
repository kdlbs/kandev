package dynamic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agent/runtime/routingpolicy"
)

func TestUnclassifiedFallbackThreshold(t *testing.T) {
	document := routingpolicy.DefaultDocument()
	document.Unclassified = &routingpolicy.UnclassifiedPolicy{
		Enabled:                     true,
		ConsecutiveFailureThreshold: 3,
	}
	profile := Profile{
		ID:      "dynamic-unclassified",
		Version: 4,
		Candidates: []Candidate{
			{ID: "candidate-a", Enabled: true, Policies: document},
			{ID: "candidate-b", Enabled: true, Policies: document},
		},
	}
	engine := NewEngine()
	decision, err := engine.Select("session-unclassified", profile, 0, "")
	if err != nil {
		t.Fatalf("initial select: %v", err)
	}
	if decision.ExecutionProfileID != "candidate-a" {
		t.Fatalf("initial candidate = %q, want candidate-a", decision.ExecutionProfileID)
	}
	if err := engine.MarkActive(context.Background(), "session-unclassified", decision.Generation); err != nil {
		t.Fatalf("mark active: %v", err)
	}

	for attempt := 1; attempt <= 3; attempt++ {
		failure := &routingerr.Error{
			Code:            routingerr.CodeUnknownProvider,
			Class:           routingerr.ClassUnclassified,
			Phase:           routingerr.PhasePromptSend,
			FallbackAllowed: false,
		}
		evidence := safePromptEvidence(decision.Generation, fmt.Sprintf("attempt-%d", attempt), "provider returned an unsupported response")
		decision, err = engine.ApplyUnclassifiedFailureContext(
			context.Background(), "session-unclassified", profile,
			decision.Generation, "candidate-a", failure, evidence,
		)
		if attempt < 3 {
			if !errors.Is(err, ErrRecoveryPending) {
				t.Fatalf("attempt %d error = %v, want manual recovery pending", attempt, err)
			}
			if decision.ExecutionProfileID != "candidate-a" || decision.Status != routeStatusActionRequired {
				t.Fatalf("attempt %d decision = %#v, want candidate-a action_required", attempt, decision)
			}
			decision, err = engine.SelectContextWithPreference(
				context.Background(), "session-unclassified", profile,
				decision.Generation, "", "candidate-a",
			)
			if err != nil {
				t.Fatalf("manual retry after attempt %d: %v", attempt, err)
			}
			if err = engine.MarkActive(context.Background(), "session-unclassified", decision.Generation); err != nil {
				t.Fatalf("mark active after attempt %d retry: %v", attempt, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("threshold attempt error = %v, want one successor", err)
		}
		if decision.ExecutionProfileID != "candidate-b" || decision.Generation != 4 {
			t.Fatalf("threshold decision = %#v, want candidate-b at generation 4", decision)
		}
	}
}

func safePromptEvidence(generation int64, attemptID, diagnostic string) UnclassifiedFailureEvidence {
	return UnclassifiedFailureEvidence{
		TaskScope: true, TaskID: "task-1", SessionID: "session-unclassified",
		LogicalProfileID: "dynamic-unclassified", ExecutionProfileID: "candidate-a",
		RouteGeneration: generation, StepID: "step-1", StepKnown: true,
		AttemptID: attemptID, Origin: UnclassifiedOriginTerminalProvider,
		Phase: routingerr.PhasePromptSend, ProviderID: "provider-x",
		DiagnosticText: diagnostic, DiagnosticComplete: true, CurrentAttempt: true, EvidenceKnown: true,
	}
}

func TestUnclassifiedFallbackRejectsDuplicateAndUnsafeEvidence(t *testing.T) {
	profile := unclassifiedTestProfile(2)
	engine := NewEngine()
	decision, err := engine.Select("session-unclassified", profile, 0, "")
	if err != nil {
		t.Fatalf("initial select: %v", err)
	}
	if err := engine.MarkActive(context.Background(), "session-unclassified", decision.Generation); err != nil {
		t.Fatalf("mark active: %v", err)
	}
	failure := &routingerr.Error{Code: routingerr.CodeUnknownProvider, Class: routingerr.ClassUnclassified, Phase: routingerr.PhasePromptSend}
	evidence := safePromptEvidence(decision.Generation, "prompt-1", "provider returned an unsupported response")
	if _, err := engine.ApplyUnclassifiedFailureContext(context.Background(), "session-unclassified", profile, decision.Generation, "candidate-a", failure, evidence); !errors.Is(err, ErrRecoveryPending) {
		t.Fatalf("first failure error = %v, want manual recovery", err)
	}
	stateAfterFirst, ok := engine.State("session-unclassified")
	if !ok {
		t.Fatal("missing route state")
	}
	var firstPolicyState PolicyState
	if err := json.Unmarshal([]byte(stateAfterFirst.PolicyStateJSON), &firstPolicyState); err != nil {
		t.Fatalf("decode first policy state: %v", err)
	}
	if firstPolicyState.Unclassified == nil || firstPolicyState.Unclassified.Count != 1 {
		t.Fatalf("first streak = %+v, want count one", firstPolicyState.Unclassified)
	}
	if strings.Contains(stateAfterFirst.PolicyStateJSON, evidence.DiagnosticText) {
		t.Fatal("raw diagnostic was persisted instead of its fingerprint")
	}

	if _, err := engine.ApplyUnclassifiedFailureContext(context.Background(), "session-unclassified", profile, decision.Generation, "candidate-a", failure, evidence); !errors.Is(err, ErrRecoveryPending) {
		t.Fatalf("duplicate delivery error = %v, want manual recovery", err)
	}
	stateAfterDuplicate, _ := engine.State("session-unclassified")
	var duplicatePolicyState PolicyState
	if err := json.Unmarshal([]byte(stateAfterDuplicate.PolicyStateJSON), &duplicatePolicyState); err != nil {
		t.Fatalf("decode duplicate policy state: %v", err)
	}
	if duplicatePolicyState.Unclassified == nil || duplicatePolicyState.Unclassified.Count != 1 {
		t.Fatalf("duplicate delivery changed streak: %+v", duplicatePolicyState.Unclassified)
	}

	unsafe := safePromptEvidence(decision.Generation, "prompt-2", "provider returned an unsupported response")
	unsafe.EffectObserved = true
	if _, err := engine.ApplyUnclassifiedFailureContext(context.Background(), "session-unclassified", profile, decision.Generation, "candidate-a", failure, unsafe); !errors.Is(err, ErrRecoveryPending) {
		t.Fatalf("effectful failure error = %v, want manual recovery", err)
	}
	stateAfterUnsafe, _ := engine.State("session-unclassified")
	var unsafePolicyState PolicyState
	if err := json.Unmarshal([]byte(stateAfterUnsafe.PolicyStateJSON), &unsafePolicyState); err != nil {
		t.Fatalf("decode unsafe policy state: %v", err)
	}
	if unsafePolicyState.Unclassified != nil {
		t.Fatalf("effectful failure retained streak: %+v", unsafePolicyState.Unclassified)
	}

	lastSafe := safePromptEvidence(decision.Generation, "prompt-3", "provider returned an unsupported response")
	decision, err = engine.ApplyUnclassifiedFailureContext(context.Background(), "session-unclassified", profile, decision.Generation, "candidate-a", failure, lastSafe)
	if !errors.Is(err, ErrRecoveryPending) {
		t.Fatalf("safe failure after effectful failure error = %v, want manual recovery", err)
	}
	if decision.ExecutionProfileID != "candidate-a" {
		t.Fatalf("safe failure after effectful failure selected %q, want candidate-a", decision.ExecutionProfileID)
	}
	stateAfterLastSafe, _ := engine.State("session-unclassified")
	var lastSafePolicyState PolicyState
	if err := json.Unmarshal([]byte(stateAfterLastSafe.PolicyStateJSON), &lastSafePolicyState); err != nil {
		t.Fatalf("decode final safe policy state: %v", err)
	}
	if lastSafePolicyState.Unclassified == nil || lastSafePolicyState.Unclassified.Count != 1 {
		t.Fatalf("safe/effectful/safe sequence retained combined count: %+v", lastSafePolicyState.Unclassified)
	}
}

func TestUnclassifiedFallbackStaleAndDifferentDiagnostics(t *testing.T) {
	profile := unclassifiedTestProfile(3)
	engine := NewEngine()
	decision, err := engine.Select("session-unclassified", profile, 0, "")
	if err != nil {
		t.Fatalf("initial select: %v", err)
	}
	if err := engine.MarkActive(context.Background(), "session-unclassified", decision.Generation); err != nil {
		t.Fatalf("mark active: %v", err)
	}
	failure := &routingerr.Error{Code: routingerr.CodeUnknownProvider, Class: routingerr.ClassUnclassified, Phase: routingerr.PhasePromptSend}
	first := safePromptEvidence(decision.Generation, "prompt-1", "provider returned unsupported response")
	if _, err := engine.ApplyUnclassifiedFailureContext(context.Background(), "session-unclassified", profile, decision.Generation, "candidate-a", failure, first); !errors.Is(err, ErrRecoveryPending) {
		t.Fatalf("first failure error = %v, want manual recovery", err)
	}
	stale := safePromptEvidence(decision.Generation-1, "stale", first.DiagnosticText)
	if _, err := engine.ApplyUnclassifiedFailureContext(context.Background(), "session-unclassified", profile, decision.Generation, "candidate-a", failure, stale); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("stale evidence error = %v, want stale generation", err)
	}
	changed := safePromptEvidence(decision.Generation, "prompt-2", "provider returned unsupported response!")
	if _, err := engine.ApplyUnclassifiedFailureContext(context.Background(), "session-unclassified", profile, decision.Generation, "candidate-a", failure, changed); !errors.Is(err, ErrRecoveryPending) {
		t.Fatalf("changed diagnostic error = %v, want manual recovery", err)
	}
	state, _ := engine.State("session-unclassified")
	var policyState PolicyState
	if err := json.Unmarshal([]byte(state.PolicyStateJSON), &policyState); err != nil {
		t.Fatalf("decode policy state: %v", err)
	}
	if policyState.Unclassified == nil || policyState.Unclassified.Count != 1 {
		t.Fatalf("changed diagnostic streak = %+v, want reset count one", policyState.Unclassified)
	}
	backToFirst := safePromptEvidence(decision.Generation, "prompt-3", first.DiagnosticText)
	if _, err := engine.ApplyUnclassifiedFailureContext(context.Background(), "session-unclassified", profile, decision.Generation, "candidate-a", failure, backToFirst); !errors.Is(err, ErrRecoveryPending) {
		t.Fatalf("return to first diagnostic error = %v, want manual recovery", err)
	}
	state, _ = engine.State("session-unclassified")
	if err := json.Unmarshal([]byte(state.PolicyStateJSON), &policyState); err != nil {
		t.Fatalf("decode returned diagnostic policy state: %v", err)
	}
	if policyState.Unclassified == nil || policyState.Unclassified.Count != 1 {
		t.Fatalf("A/X, A/Y, A/X sequence combined different diagnostics: %+v", policyState.Unclassified)
	}
}

func TestUnclassifiedStreakResetMatrix(t *testing.T) {
	t.Run("step change resets count", func(t *testing.T) {
		engine, profile, decision := startUnclassifiedRoute(t, unclassifiedTestProfile(3))
		failure := unclassifiedPromptFailure()
		first := safePromptEvidence(decision.Generation, "prompt-1", "same diagnostic")
		if _, err := engine.ApplyUnclassifiedFailureContext(context.Background(), decision.SessionID, profile, decision.Generation, "candidate-a", failure, first); !errors.Is(err, ErrRecoveryPending) {
			t.Fatalf("first failure: %v", err)
		}
		state, _ := engine.State(decision.SessionID)
		retry, err := engine.SelectContextWithPreference(context.Background(), decision.SessionID, profile, state.Generation, "", "candidate-a")
		if err != nil {
			t.Fatalf("manual retry: %v", err)
		}
		if err := engine.MarkActive(context.Background(), decision.SessionID, retry.Generation); err != nil {
			t.Fatalf("mark active: %v", err)
		}
		changedStep := safePromptEvidence(retry.Generation, "prompt-2", "same diagnostic")
		changedStep.StepID = "step-2"
		if _, err := engine.ApplyUnclassifiedFailureContext(context.Background(), decision.SessionID, profile, retry.Generation, "candidate-a", failure, changedStep); !errors.Is(err, ErrRecoveryPending) {
			t.Fatalf("failure after step change: %v", err)
		}
		assertUnclassifiedCount(t, engine, decision.SessionID, 1)
	})

	t.Run("step revision change resets count", func(t *testing.T) {
		engine, profile, decision := startUnclassifiedRoute(t, unclassifiedTestProfile(3))
		failure := unclassifiedPromptFailure()
		first := safePromptEvidence(decision.Generation, "prompt-1", "same diagnostic")
		first.StepUpdatedAt = time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
		if _, err := engine.ApplyUnclassifiedFailureContext(context.Background(), decision.SessionID, profile, decision.Generation, "candidate-a", failure, first); !errors.Is(err, ErrRecoveryPending) {
			t.Fatalf("first failure: %v", err)
		}
		state, _ := engine.State(decision.SessionID)
		retry, err := engine.SelectContextWithPreference(context.Background(), decision.SessionID, profile, state.Generation, "", "candidate-a")
		if err != nil {
			t.Fatalf("manual retry: %v", err)
		}
		if err := engine.MarkActive(context.Background(), decision.SessionID, retry.Generation); err != nil {
			t.Fatalf("mark active: %v", err)
		}
		// The veto was enabled and then disabled between failures. The current
		// step is eligible again, but its changed revision starts a fresh streak.
		revisedStep := safePromptEvidence(retry.Generation, "prompt-2", "same diagnostic")
		revisedStep.StepUpdatedAt = first.StepUpdatedAt.Add(time.Second)
		if _, err := engine.ApplyUnclassifiedFailureContext(context.Background(), decision.SessionID, profile, retry.Generation, "candidate-a", failure, revisedStep); !errors.Is(err, ErrRecoveryPending) {
			t.Fatalf("failure after step revision change: %v", err)
		}
		assertUnclassifiedCount(t, engine, decision.SessionID, 1)
	})

	t.Run("new profile version resets count", func(t *testing.T) {
		engine, profile, decision := startUnclassifiedRoute(t, unclassifiedTestProfile(3))
		failure := unclassifiedPromptFailure()
		first := safePromptEvidence(decision.Generation, "prompt-1", "same diagnostic")
		if _, err := engine.ApplyUnclassifiedFailureContext(context.Background(), decision.SessionID, profile, decision.Generation, "candidate-a", failure, first); !errors.Is(err, ErrRecoveryPending) {
			t.Fatalf("first failure: %v", err)
		}
		profile.Version++
		state, _ := engine.State(decision.SessionID)
		retry, err := engine.SelectContextWithPreference(context.Background(), decision.SessionID, profile, state.Generation, "", "candidate-a")
		if err != nil {
			t.Fatalf("manual retry with new profile version: %v", err)
		}
		if err := engine.MarkActive(context.Background(), decision.SessionID, retry.Generation); err != nil {
			t.Fatalf("mark active: %v", err)
		}
		next := safePromptEvidence(retry.Generation, "prompt-2", "same diagnostic")
		if _, err := engine.ApplyUnclassifiedFailureContext(context.Background(), decision.SessionID, profile, retry.Generation, "candidate-a", failure, next); !errors.Is(err, ErrRecoveryPending) {
			t.Fatalf("failure after profile change: %v", err)
		}
		assertUnclassifiedCount(t, engine, decision.SessionID, 1)
	})

	t.Run("current activity clears count", func(t *testing.T) {
		engine, profile, decision := startUnclassifiedRoute(t, unclassifiedTestProfile(3))
		if _, err := engine.ApplyUnclassifiedFailureContext(context.Background(), decision.SessionID, profile, decision.Generation, "candidate-a", unclassifiedPromptFailure(), safePromptEvidence(decision.Generation, "prompt-1", "same diagnostic")); !errors.Is(err, ErrRecoveryPending) {
			t.Fatalf("first failure: %v", err)
		}
		if err := engine.ClearUnclassifiedStreak(context.Background(), decision.SessionID, decision.Generation, "candidate-a"); err != nil {
			t.Fatalf("clear after current activity: %v", err)
		}
		assertNoUnclassifiedStreak(t, engine, decision.SessionID)
	})

	t.Run("startup readiness clears only startup streak", func(t *testing.T) {
		engine, profile, decision := startUnclassifiedRoute(t, unclassifiedTestProfile(3))
		startupFailure := &routingerr.Error{Code: routingerr.CodeAgentRuntime, Class: routingerr.ClassUnclassified, Phase: routingerr.PhaseProcessStart}
		startup := safePromptEvidence(decision.Generation, "startup-1", "startup failure")
		startup.Origin = UnclassifiedOriginAgentStartup
		startup.Phase = routingerr.PhaseProcessStart
		if _, err := engine.ApplyUnclassifiedFailureContext(context.Background(), decision.SessionID, profile, decision.Generation, "candidate-a", startupFailure, startup); !errors.Is(err, ErrRecoveryPending) {
			t.Fatalf("startup failure: %v", err)
		}
		if err := engine.ClearUnclassifiedStartupStreak(context.Background(), decision.SessionID, decision.Generation, "candidate-a"); err != nil {
			t.Fatalf("clear after boot ready: %v", err)
		}
		assertNoUnclassifiedStreak(t, engine, decision.SessionID)
	})
}

func TestUnclassifiedStreakRestart(t *testing.T) {
	ctx := context.Background()
	profile := unclassifiedTestProfile(2)
	engine := NewEngine()
	decision, err := engine.Select("session-unclassified-restart", profile, 0, "")
	if err != nil {
		t.Fatalf("initial select: %v", err)
	}
	if err := engine.MarkActive(ctx, decision.SessionID, decision.Generation); err != nil {
		t.Fatalf("mark active: %v", err)
	}
	firstEvidence := safePromptEvidence(decision.Generation, "prompt-1", "same diagnostic")
	firstEvidence.SessionID = decision.SessionID
	if _, err := engine.ApplyUnclassifiedFailureContext(ctx, decision.SessionID, profile, decision.Generation, "candidate-a", unclassifiedPromptFailure(), firstEvidence); !errors.Is(err, ErrRecoveryPending) {
		t.Fatalf("first failure: %v", err)
	}
	persisted, ok := engine.State(decision.SessionID)
	if !ok {
		t.Fatal("first engine did not retain route state")
	}
	restarted := NewEngine(WithStateLoader(staticRouteStateLoader{state: persisted}))
	retry, err := restarted.SelectContextWithPreference(ctx, decision.SessionID, profile, decision.Generation, "", "candidate-a")
	if err != nil {
		t.Fatalf("same-candidate manual retry after restart: %v", err)
	}
	if err := restarted.MarkActive(ctx, decision.SessionID, retry.Generation); err != nil {
		t.Fatalf("mark active after restart: %v", err)
	}
	secondEvidence := safePromptEvidence(retry.Generation, "prompt-2", "same diagnostic")
	secondEvidence.SessionID = decision.SessionID
	successor, err := restarted.ApplyUnclassifiedFailureContext(ctx, decision.SessionID, profile, retry.Generation, "candidate-a", unclassifiedPromptFailure(), secondEvidence)
	if err != nil {
		t.Fatalf("threshold failure after restart: %v", err)
	}
	if successor.ExecutionProfileID != "candidate-b" {
		t.Fatalf("successor after restart = %q, want candidate-b", successor.ExecutionProfileID)
	}
}

func TestUnclassifiedFingerprintRequiresExactCompleteDiagnostic(t *testing.T) {
	failure := unclassifiedPromptFailure()
	base := safePromptEvidence(1, "prompt-1", "provider said unsupported response")
	fingerprint, err := unclassifiedFailureFingerprint(base, failure)
	if err != nil {
		t.Fatalf("fingerprint complete diagnostic: %v", err)
	}
	whitespace := base
	whitespace.DiagnosticText = "  provider\tsaid\nunsupported response  "
	whitespaceFingerprint, err := unclassifiedFailureFingerprint(whitespace, failure)
	if err != nil {
		t.Fatalf("fingerprint whitespace variant: %v", err)
	}
	if whitespaceFingerprint != fingerprint {
		t.Fatalf("whitespace-normalized fingerprint = %q, want %q", whitespaceFingerprint, fingerprint)
	}
	for name, diagnostic := range map[string]string{
		"case":        "Provider said unsupported response",
		"punctuation": "provider said unsupported response!",
	} {
		t.Run(name, func(t *testing.T) {
			changed := base
			changed.DiagnosticText = diagnostic
			got, err := unclassifiedFailureFingerprint(changed, failure)
			if err != nil {
				t.Fatalf("fingerprint diagnostic: %v", err)
			}
			if got == fingerprint {
				t.Fatalf("%s change produced the same fingerprint", name)
			}
		})
	}
	for name, diagnostic := range map[string]string{
		"sanitizer loss": "Authorization: Bearer abcdefghijklmnopqrstuvwxyz1234567890",
		"oversized":      strings.Repeat("x", 1025),
		"invalid utf8":   string([]byte{0xff, 0xfe}),
		"empty":          "",
	} {
		t.Run(name, func(t *testing.T) {
			incomplete := base
			incomplete.DiagnosticText = diagnostic
			if _, err := unclassifiedFailureFingerprint(incomplete, failure); err == nil {
				t.Fatalf("%s diagnostic was accepted as complete", name)
			}
		})
	}
}

type staticRouteStateLoader struct{ state RouteState }

func (l staticRouteStateLoader) LoadRouteState(context.Context, string) (*RouteState, error) {
	state := l.state
	return &state, nil
}

func startUnclassifiedRoute(t *testing.T, profile Profile) (*Engine, Profile, RouteDecision) {
	t.Helper()
	engine := NewEngine()
	decision, err := engine.Select("session-unclassified", profile, 0, "")
	if err != nil {
		t.Fatalf("initial select: %v", err)
	}
	if err := engine.MarkActive(context.Background(), decision.SessionID, decision.Generation); err != nil {
		t.Fatalf("mark active: %v", err)
	}
	return engine, profile, decision
}

func unclassifiedPromptFailure() *routingerr.Error {
	return &routingerr.Error{Code: routingerr.CodeUnknownProvider, Class: routingerr.ClassUnclassified, Phase: routingerr.PhasePromptSend}
}

func assertUnclassifiedCount(t *testing.T, engine *Engine, sessionID string, want int64) {
	t.Helper()
	state, ok := engine.State(sessionID)
	if !ok {
		t.Fatal("missing route state")
	}
	var policyState PolicyState
	if err := json.Unmarshal([]byte(state.PolicyStateJSON), &policyState); err != nil {
		t.Fatalf("decode policy state: %v", err)
	}
	if policyState.Unclassified == nil || policyState.Unclassified.Count != want {
		t.Fatalf("unclassified streak = %+v, want count %d", policyState.Unclassified, want)
	}
}

func assertNoUnclassifiedStreak(t *testing.T, engine *Engine, sessionID string) {
	t.Helper()
	state, ok := engine.State(sessionID)
	if !ok {
		t.Fatal("missing route state")
	}
	var policyState PolicyState
	if err := json.Unmarshal([]byte(state.PolicyStateJSON), &policyState); err != nil {
		t.Fatalf("decode policy state: %v", err)
	}
	if policyState.Unclassified != nil {
		t.Fatalf("unclassified streak = %+v, want nil", policyState.Unclassified)
	}
}

func unclassifiedTestProfile(threshold int64) Profile {
	document := routingpolicy.DefaultDocument()
	document.Unclassified = &routingpolicy.UnclassifiedPolicy{Enabled: true, ConsecutiveFailureThreshold: threshold}
	return Profile{
		ID: "dynamic-unclassified", Version: 4,
		Candidates: []Candidate{
			{ID: "candidate-a", Enabled: true, Policies: document},
			{ID: "candidate-b", Enabled: true, Policies: document},
		},
	}
}
