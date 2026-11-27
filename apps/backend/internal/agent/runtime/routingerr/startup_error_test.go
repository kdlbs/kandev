package routingerr

import (
	"context"
	"errors"
	"testing"
)

func TestAgentStartupFailureAttestsOnlyAgentStartupStages(t *testing.T) {
	cause := errors.New("agent process could not initialize")
	for _, phase := range []Phase{PhaseProcessStart, PhaseSessionInit} {
		t.Run(string(phase), func(t *testing.T) {
			wrapped := NewAgentStartupFailure(phase, "provider-x", cause)
			var startup *AgentStartupFailure
			if !errors.As(wrapped, &startup) {
				t.Fatalf("startup wrapper = %T, want AgentStartupFailure", wrapped)
			}
			if startup.Phase != phase || startup.ProviderID != "provider-x" ||
				startup.Diagnostic != cause.Error() || startup.Cause != cause ||
				startup.DiagnosticIdentityComplete || startup.DiagnosticSource != "" {
				t.Fatalf("startup evidence = %+v", startup)
			}
			classified := Classify(Input{Phase: startup.Phase, ProviderID: startup.ProviderID, StructuredErr: startup.Cause, Stderr: startup.Diagnostic})
			if classified.Code != CodeUnknownProvider || classified.Class != ClassUnclassified {
				t.Fatalf("classification = %+v, want unchanged prestart unknown_provider", classified)
			}
		})
	}
}

func TestAgentStartupFailureDoesNotWrapCancellationOrManagedRuntimePolicy(t *testing.T) {
	managed := &ManagedRuntimeStartupError{Code: CodeAgentRuntime, Details: "managed runtime is unavailable"}
	for name, testCase := range map[string]struct {
		phase Phase
		cause error
	}{
		"canceled":        {PhaseProcessStart, context.Canceled},
		"deadline":        {PhaseSessionInit, context.DeadlineExceeded},
		"managed runtime": {PhaseProcessStart, managed},
		"wrong phase":     {PhasePromptSend, errors.New("prompt failed")},
	} {
		t.Run(name, func(t *testing.T) {
			got := NewAgentStartupFailure(testCase.phase, "provider-x", testCase.cause)
			var startup *AgentStartupFailure
			if errors.As(got, &startup) {
				t.Fatalf("result %T unexpectedly carries startup evidence", got)
			}
			if got != testCase.cause {
				t.Fatalf("result = %v, want original cause %v", got, testCase.cause)
			}
		})
	}
}

func TestManagedRuntimeStartupCodeDoesNotEnableGenericRecovery(t *testing.T) {
	classified := applyInvariants(&Error{Code: CodeManagedRuntimeStartup})
	if classified.Class != ClassUnclassified || classified.AutoRetryable || classified.FallbackAllowed || !classified.UserAction {
		t.Fatalf("managed startup policy = %+v, want user action without generic retry or fallback", classified)
	}
}
