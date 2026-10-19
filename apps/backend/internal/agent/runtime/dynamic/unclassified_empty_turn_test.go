package dynamic

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agent/runtime/routingpolicy"
)

func emptyTurnEvidence(attemptID string) UnclassifiedFailureEvidence {
	return UnclassifiedFailureEvidence{
		TaskScope: true, TaskID: "task-1", SessionID: "session-unclassified",
		LogicalProfileID: "dynamic-unclassified", ExecutionProfileID: "candidate-a",
		RouteGeneration: 1, StepID: "step-1", StepKnown: true,
		AttemptID: attemptID, Origin: UnclassifiedOriginEmptyTurn,
		Phase: routingerr.PhasePromptSend, CurrentAttempt: true, EvidenceKnown: true,
	}
}

func emptyTurnFailure() *routingerr.Error {
	return &routingerr.Error{
		Code: routingerr.CodeAgentRuntime, Class: routingerr.ClassUnclassified, Phase: routingerr.PhasePromptSend,
	}
}

func TestUnclassifiedEmptyTurnAdmission(t *testing.T) {
	if !emptyTurnEvidence("prompt-1").permits(emptyTurnFailure()) {
		t.Fatal("empty end_turn evidence without a provider diagnostic was refused")
	}
	for name, mutate := range map[string]func(*UnclassifiedFailureEvidence, *routingerr.Error){
		"output observed":  func(e *UnclassifiedFailureEvidence, _ *routingerr.Error) { e.OutputObserved = true },
		"effect observed":  func(e *UnclassifiedFailureEvidence, _ *routingerr.Error) { e.EffectObserved = true },
		"evidence unknown": func(e *UnclassifiedFailureEvidence, _ *routingerr.Error) { e.EvidenceKnown = false },
		"step veto":        func(e *UnclassifiedFailureEvidence, _ *routingerr.Error) { e.StepVeto = true },
		"step unknown":     func(e *UnclassifiedFailureEvidence, _ *routingerr.Error) { e.StepKnown = false },
		"no task scope":    func(e *UnclassifiedFailureEvidence, _ *routingerr.Error) { e.TaskScope = false },
		"provider code":    func(_ *UnclassifiedFailureEvidence, f *routingerr.Error) { f.Code = routingerr.CodeUnknownProvider },
		"streaming phase": func(e *UnclassifiedFailureEvidence, f *routingerr.Error) {
			e.Phase, f.Phase = routingerr.PhaseStreaming, routingerr.PhaseStreaming
		},
		"user action": func(_ *UnclassifiedFailureEvidence, f *routingerr.Error) { f.UserAction = true },
	} {
		t.Run(name, func(t *testing.T) {
			evidence, failure := emptyTurnEvidence("prompt-1"), emptyTurnFailure()
			mutate(&evidence, failure)
			if evidence.permits(failure) {
				t.Fatalf("%s was admitted as an empty turn failure", name)
			}
		})
	}
}

func TestUnclassifiedDiagnosticWaiverIsOriginScoped(t *testing.T) {
	for _, origin := range []UnclassifiedFailureOrigin{UnclassifiedOriginTerminalProvider, UnclassifiedOriginAgentStartup} {
		evidence := emptyTurnEvidence("prompt-1")
		evidence.Origin = origin
		failure := emptyTurnFailure()
		if origin == UnclassifiedOriginTerminalProvider {
			failure.Code = routingerr.CodeUnknownProvider
		} else {
			evidence.Phase, failure.Phase = routingerr.PhaseSessionInit, routingerr.PhaseSessionInit
		}
		if evidence.permits(failure) {
			t.Fatalf("%s origin was admitted without a complete diagnostic and provider", origin)
		}
	}
}

func TestUnclassifiedEmptyTurnFingerprint(t *testing.T) {
	failure := emptyTurnFailure()
	first, err := unclassifiedFailureFingerprint(emptyTurnEvidence("prompt-1"), failure)
	if err != nil || first == "" {
		t.Fatalf("empty turn fingerprint = %q, %v", first, err)
	}
	second := emptyTurnEvidence("prompt-2")
	second.DiagnosticText = "text that must not take part in the identity"
	second.ProviderID = "provider-x"
	got, err := unclassifiedFailureFingerprint(second, failure)
	if err != nil || got != first {
		t.Fatalf("second empty turn fingerprint = %q, %v; want %q", got, err, first)
	}
	provider, err := unclassifiedFailureFingerprint(
		safePromptEvidence(1, "prompt-3", "provider returned an unsupported response"), unclassifiedPromptFailure(),
	)
	if err != nil || provider == first {
		t.Fatalf("provider fingerprint = %q, %v; must differ from the empty turn identity", provider, err)
	}
}

func TestCandidateUnclassifiedEnabled(t *testing.T) {
	for name, testCase := range map[string]struct {
		policy *routingpolicy.UnclassifiedPolicy
		want   bool
	}{
		"enabled":           {policy: &routingpolicy.UnclassifiedPolicy{Enabled: true, ConsecutiveFailureThreshold: 2}, want: true},
		"absent":            {policy: nil},
		"disabled":          {policy: &routingpolicy.UnclassifiedPolicy{}},
		"threshold too low": {policy: &routingpolicy.UnclassifiedPolicy{Enabled: true, ConsecutiveFailureThreshold: 1}},
		"threshold too high": {
			policy: &routingpolicy.UnclassifiedPolicy{Enabled: true, ConsecutiveFailureThreshold: 11},
		},
	} {
		t.Run(name, func(t *testing.T) {
			document := routingpolicy.DefaultDocument()
			document.Unclassified = testCase.policy
			profile := Profile{ID: "dynamic-unclassified", Candidates: []Candidate{{ID: "candidate-a", Enabled: true, Policies: document}}}
			if got := CandidateUnclassifiedEnabled(profile, "candidate-a"); got != testCase.want {
				t.Fatalf("CandidateUnclassifiedEnabled = %v, want %v", got, testCase.want)
			}
			if CandidateUnclassifiedEnabled(profile, "candidate-missing") {
				t.Fatal("an unknown candidate reported an enabled policy")
			}
		})
	}
}
