package lifecycle

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestApplyStartModelPolicyExecutorAuthority(t *testing.T) {
	tests := []struct {
		name          string
		state         *CachedModelState
		policy        StartModelPolicy
		applierErrors []error
		wantCalls     []string
		wantOutcome   ModelSelectionOutcome
		wantReason    string
		wantEffective string
		wantWarning   bool
		wantErr       string
	}{
		{
			name:        "requested model absent does not call executor",
			state:       modelState("executor-default"),
			policy:      StartModelPolicy{Model: "host-only-model"},
			wantOutcome: ModelSelectionOutcomeProviderDefault,
			wantReason:  ModelSelectionReasonRequestedNotAdvertised,
			wantWarning: true,
		},
		{
			name:        "empty catalog does not call executor",
			state:       &CachedModelState{},
			policy:      StartModelPolicy{Model: "host-only-model", FallbackModel: "fallback"},
			wantOutcome: ModelSelectionOutcomeProviderDefault,
			wantReason:  ModelSelectionReasonCatalogEmpty,
			wantWarning: true,
		},
		{
			name:          "advertised fallback is applied",
			state:         modelState("fallback"),
			policy:        StartModelPolicy{Model: "host-only-model", FallbackModel: "fallback"},
			wantCalls:     []string{"fallback"},
			wantOutcome:   ModelSelectionOutcomeExplicitFallback,
			wantReason:    ModelSelectionReasonRequestedNotAdvertised,
			wantEffective: "fallback",
			wantWarning:   true,
		},
		{
			name:          "advertised fallback method not supported keeps provider default",
			state:         modelState("fallback"),
			policy:        StartModelPolicy{Model: "host-only-model", FallbackModel: "fallback"},
			applierErrors: []error{methodNotFoundErr()},
			wantCalls:     []string{"fallback"},
			wantOutcome:   ModelSelectionOutcomeProviderDefault,
			wantReason:    ModelSelectionReasonSelectionUnsupported,
			wantWarning:   true,
		},
		{
			name:        "unadvertised fallback keeps provider default",
			state:       modelState("executor-default"),
			policy:      StartModelPolicy{Model: "host-only-model", FallbackModel: "host-fallback"},
			wantOutcome: ModelSelectionOutcomeProviderDefault,
			wantReason:  ModelSelectionReasonFallbackNotAdvertised,
			wantWarning: true,
		},
		{
			name:          "advertised requested model is applied",
			state:         modelState("requested"),
			policy:        StartModelPolicy{Model: "requested"},
			wantCalls:     []string{"requested"},
			wantOutcome:   ModelSelectionOutcomeApplied,
			wantEffective: "requested",
		},
		{
			name:          "method not supported keeps provider default",
			state:         modelState("requested"),
			policy:        StartModelPolicy{Model: "requested"},
			applierErrors: []error{methodNotFoundErr()},
			wantCalls:     []string{"requested"},
			wantOutcome:   ModelSelectionOutcomeProviderDefault,
			wantReason:    ModelSelectionReasonSelectionUnsupported,
			wantWarning:   true,
		},
		{
			name:          "advertised apply error is explicit",
			state:         modelState("requested"),
			policy:        StartModelPolicy{Model: "requested", FallbackModel: "fallback"},
			applierErrors: []error{errors.New("rejected")},
			wantCalls:     []string{"requested"},
			wantErr:       `failed to set start model "requested"`,
		},
		{
			name:          "auto fallback warns after advertised apply error",
			state:         modelState("requested"),
			policy:        StartModelPolicy{Model: "requested", AutoFallback: true},
			applierErrors: []error{errors.New("rejected")},
			wantCalls:     []string{"requested"},
			wantOutcome:   ModelSelectionOutcomeProviderDefault,
			wantReason:    ModelSelectionReasonSelectionFailedAutoFallback,
			wantWarning:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applier := &fakeModelApplier{errs: tt.applierErrors}
			decision, err := applyStartModelPolicy(
				context.Background(), newPolicyTestLogger(), applier, tt.state, tt.policy,
			)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want substring %q", err, tt.wantErr)
				}
			}
			if !reflect.DeepEqual(applier.calls, tt.wantCalls) {
				t.Errorf("SetModel calls = %v, want %v", applier.calls, tt.wantCalls)
			}
			if decision.Outcome != tt.wantOutcome {
				t.Errorf("outcome = %q, want %q", decision.Outcome, tt.wantOutcome)
			}
			if decision.Reason != tt.wantReason {
				t.Errorf("reason = %q, want %q", decision.Reason, tt.wantReason)
			}
			if decision.EffectiveModel != tt.wantEffective {
				t.Errorf("effective model = %q, want %q", decision.EffectiveModel, tt.wantEffective)
			}
			if decision.Warning != tt.wantWarning {
				t.Errorf("warning = %v, want %v", decision.Warning, tt.wantWarning)
			}
		})
	}
}
