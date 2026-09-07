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
			name:          "unique variation is applied when fallback is absent",
			state:         modelState("opus[1m]"),
			policy:        StartModelPolicy{Model: "opus"},
			wantCalls:     []string{"opus[1m]"},
			wantOutcome:   ModelSelectionOutcomeUniqueVariation,
			wantReason:    ModelSelectionReasonUniqueVariationApplied,
			wantEffective: "opus[1m]",
			wantWarning:   true,
		},
		{
			name:          "advertised fallback takes precedence over unique variation",
			state:         modelState("fallback", "opus[1m]"),
			policy:        StartModelPolicy{Model: "opus", FallbackModel: "fallback"},
			wantCalls:     []string{"fallback"},
			wantOutcome:   ModelSelectionOutcomeExplicitFallback,
			wantReason:    ModelSelectionReasonRequestedNotAdvertised,
			wantEffective: "fallback",
			wantWarning:   true,
		},
		{
			name:          "unadvertised fallback does not block unique variation",
			state:         modelState("opus[1m]"),
			policy:        StartModelPolicy{Model: "opus", FallbackModel: "fallback"},
			wantCalls:     []string{"opus[1m]"},
			wantOutcome:   ModelSelectionOutcomeUniqueVariation,
			wantReason:    ModelSelectionReasonUniqueVariationApplied,
			wantEffective: "opus[1m]",
			wantWarning:   true,
		},
		{
			name:        "auto fallback keeps unavailable model on provider default",
			state:       modelState("fallback", "opus[1m]"),
			policy:      StartModelPolicy{Model: "opus", FallbackModel: "fallback", AutoFallback: true},
			wantOutcome: ModelSelectionOutcomeProviderDefault,
			wantReason:  ModelSelectionReasonRequestedNotAdvertised,
			wantWarning: true,
		},
		{
			name:          "duplicate unique variation is one candidate",
			state:         modelState("opus[1m]", "opus[1m]"),
			policy:        StartModelPolicy{Model: "opus"},
			wantCalls:     []string{"opus[1m]"},
			wantOutcome:   ModelSelectionOutcomeUniqueVariation,
			wantReason:    ModelSelectionReasonUniqueVariationApplied,
			wantEffective: "opus[1m]",
			wantWarning:   true,
		},
		{
			name:        "multiple variations keep provider default",
			state:       modelState("opus[1m]", "opus[270k]"),
			policy:      StartModelPolicy{Model: "opus"},
			wantOutcome: ModelSelectionOutcomeProviderDefault,
			wantReason:  ModelSelectionReasonRequestedNotAdvertised,
			wantWarning: true,
		},
		{
			name:        "malformed variations keep provider default",
			state:       modelState("opus[]", "opus[1m", "opus[1m][fast]", "opus-pro[1m]"),
			policy:      StartModelPolicy{Model: "opus"},
			wantOutcome: ModelSelectionOutcomeProviderDefault,
			wantReason:  ModelSelectionReasonRequestedNotAdvertised,
			wantWarning: true,
		},
		{
			name:          "variation text remains opaque",
			state:         modelState("opus[1m, fast]"),
			policy:        StartModelPolicy{Model: "opus"},
			wantCalls:     []string{"opus[1m, fast]"},
			wantOutcome:   ModelSelectionOutcomeUniqueVariation,
			wantReason:    ModelSelectionReasonUniqueVariationApplied,
			wantEffective: "opus[1m, fast]",
			wantWarning:   true,
		},
		{
			name:        "case-sensitive variation matching keeps provider default",
			state:       modelState("Opus[1m]"),
			policy:      StartModelPolicy{Model: "opus"},
			wantOutcome: ModelSelectionOutcomeProviderDefault,
			wantReason:  ModelSelectionReasonRequestedNotAdvertised,
			wantWarning: true,
		},
		{
			name:        "bracketed request does not infer another variation",
			state:       modelState("opus[2m]"),
			policy:      StartModelPolicy{Model: "opus[1m]"},
			wantOutcome: ModelSelectionOutcomeProviderDefault,
			wantReason:  ModelSelectionReasonRequestedNotAdvertised,
			wantWarning: true,
		},
		{
			name:          "exact bracketed request remains exact",
			state:         modelState("opus[1m]", "opus[2m]"),
			policy:        StartModelPolicy{Model: "opus[1m]"},
			wantCalls:     []string{"opus[1m]"},
			wantOutcome:   ModelSelectionOutcomeApplied,
			wantEffective: "opus[1m]",
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
			name:          "unique variation method not supported keeps provider default",
			state:         modelState("opus[1m]"),
			policy:        StartModelPolicy{Model: "opus", FallbackModel: "fallback"},
			applierErrors: []error{methodNotFoundErr()},
			wantCalls:     []string{"opus[1m]"},
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
			if decision.Outcome == ModelSelectionOutcomeUniqueVariation && decision.FallbackModel != "" {
				t.Errorf("unique variation fallback model = %q, want empty", decision.FallbackModel)
			}
			if tt.policy.AutoFallback && decision.FallbackModel != "" {
				t.Errorf("auto-fallback fallback model = %q, want empty", decision.FallbackModel)
			}
		})
	}
}
