package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestStrictStartModelFailureCarriesBootstrapCause(t *testing.T) {
	_, err := applyStartModelPolicy(
		context.Background(),
		newPolicyTestLogger(),
		&fakeModelApplier{},
		modelState("provider-default"),
		StartModelPolicy{Model: "saved-model", RequireExactModel: true},
	)
	if err == nil {
		t.Fatal("expected strict model selection to fail")
	}

	var failure *BootstrapFailure
	if !errors.As(err, &failure) {
		t.Fatalf("error %T does not preserve typed bootstrap evidence: %v", err, err)
	}
	if failure.Code != "model_unavailable" {
		t.Errorf("cause code = %q, want model_unavailable", failure.Code)
	}
	if failure.Reason != ModelSelectionReasonRequestedNotAdvertised || failure.RequestedModel != "saved-model" {
		t.Errorf("selection evidence = (%q, %q), want (requested_not_advertised, saved-model)", failure.Reason, failure.RequestedModel)
	}
	if failure.PromptNotSent == nil || !*failure.PromptNotSent {
		t.Error("prompt_not_sent evidence = false or unknown, want true")
	}
	cause, ok := failure.SafeAgentErrorCause(models.AgentErrorCauseOperationStart)
	if !ok || cause.Code != models.AgentErrorCauseCodeModelUnavailable || cause.RequestedModel != "saved-model" {
		t.Errorf("safe cause = %+v, ok = %v", cause, ok)
	}
}

func TestRequiredEmptyStartModelReportsSelectionMissing(t *testing.T) {
	_, err := applyStartModelPolicy(
		context.Background(), newPolicyTestLogger(), &fakeModelApplier{},
		modelState("provider-default"), StartModelPolicy{RequireExactModel: true},
	)
	var failure *BootstrapFailure
	if !errors.As(err, &failure) {
		t.Fatalf("error %T does not preserve typed bootstrap evidence: %v", err, err)
	}
	if failure.Code != models.AgentErrorCauseCodeModelSelectionFailed || failure.Reason != models.AgentErrorCauseReasonSelectionMissing {
		t.Errorf("cause = (%q, %q), want (model_selection_failed, selection_missing)", failure.Code, failure.Reason)
	}
	if failure.PromptNotSent == nil || !*failure.PromptNotSent {
		t.Error("prompt_not_sent evidence = false or unknown, want true")
	}
}

func TestStrictStartModelFailureDistinguishesSourceReasons(t *testing.T) {
	applyError := errors.New("provider credential=hidden-credential")
	tests := []struct {
		name        string
		state       *CachedModelState
		applierErr  error
		wantCode    string
		wantReason  string
		wantApplied bool
	}{
		{
			name:       "empty catalog",
			state:      &CachedModelState{},
			wantCode:   models.AgentErrorCauseCodeModelSelectionFailed,
			wantReason: models.AgentErrorCauseReasonCatalogEmpty,
		},
		{
			name:       "selection unsupported",
			state:      modelState("saved-model"),
			applierErr: methodNotFoundErr(),
			wantCode:   models.AgentErrorCauseCodeModelSelectionFailed,
			wantReason: models.AgentErrorCauseReasonSelectionUnsupported,
		},
		{
			name:       "application failed",
			state:      modelState("saved-model"),
			applierErr: applyError,
			wantCode:   models.AgentErrorCauseCodeModelSelectionFailed,
			wantReason: models.AgentErrorCauseReasonApplicationFailed,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := test.state
			if state != nil {
				state.CurrentModelID = "effective-model"
			}
			_, err := applyStartModelPolicy(
				context.Background(), newPolicyTestLogger(), &fakeModelApplier{errs: []error{test.applierErr}}, state,
				StartModelPolicy{Model: "saved-model", RequireExactModel: true},
			)
			if err == nil {
				t.Fatal("expected strict model selection to fail")
			}
			var failure *BootstrapFailure
			if !errors.As(err, &failure) {
				t.Fatalf("error %T does not preserve typed bootstrap evidence: %v", err, err)
			}
			if failure.Code != test.wantCode || failure.Reason != test.wantReason {
				t.Errorf("cause = (%q, %q), want (%q, %q)", failure.Code, failure.Reason, test.wantCode, test.wantReason)
			}
			if failure.RequestedModel != "saved-model" || failure.EffectiveModel != "effective-model" {
				t.Errorf("model evidence = (%q, %q), want (saved-model, effective-model)", failure.RequestedModel, failure.EffectiveModel)
			}
			if test.applierErr == applyError && !errors.Is(err, applyError) {
				t.Error("typed failure lost errors.Is access to the provider error")
			}
		})
	}
}

func TestStartModelCancellationAndDeadlineKeepTheirMeaning(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want string
	}{
		{name: "cancellation", err: context.Canceled, want: models.AgentErrorCauseCodeUnknown},
		{name: "deadline", err: context.DeadlineExceeded, want: models.AgentErrorCauseCodeTimeout},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := applyStartModelPolicy(
				context.Background(), newPolicyTestLogger(), &fakeModelApplier{errs: []error{test.err}},
				modelState("saved-model"), StartModelPolicy{Model: "saved-model", RequireExactModel: true},
			)
			if !errors.Is(err, test.err) {
				t.Fatalf("error = %v, want errors.Is(_, %v)", err, test.err)
			}
			wrapped := wrapBootstrapFailure(&AgentExecution{}, err)
			var failure *BootstrapFailure
			if test.err == context.Canceled {
				if errors.As(wrapped, &failure) {
					t.Fatalf("cancelled failure was converted to bootstrap evidence: %+v", failure)
				}
				return
			}
			if !errors.As(wrapped, &failure) || failure.SafeCode() != test.want {
				t.Fatalf("wrapped failure = %+v, want safe code %q", failure, test.want)
			}
		})
	}
}

func TestApplicationFailureIdentifiesTheModelPassedToSetModel(t *testing.T) {
	tests := []struct {
		name    string
		policy  StartModelPolicy
		state   *CachedModelState
		attempt string
	}{
		{
			name:    "advertised fallback",
			policy:  StartModelPolicy{Model: "primary", FallbackModel: "alternate"},
			state:   modelState("alternate"),
			attempt: "alternate",
		},
		{
			name:    "unique variation",
			policy:  StartModelPolicy{Model: "primary"},
			state:   modelState("primary[1m]"),
			attempt: "primary[1m]",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			applier := &fakeModelApplier{errs: []error{errors.New("provider rejected selection")}}
			_, err := applyStartModelPolicy(context.Background(), newPolicyTestLogger(), applier, test.state, test.policy)
			if err == nil {
				t.Fatal("expected model application to fail")
			}
			if len(applier.calls) != 1 || applier.calls[0] != test.attempt {
				t.Fatalf("SetModel calls = %v, want only %q", applier.calls, test.attempt)
			}
			var failure *BootstrapFailure
			if !errors.As(err, &failure) {
				t.Fatalf("error %T does not preserve typed bootstrap evidence: %v", err, err)
			}
			cause, ok := failure.SafeAgentErrorCause(models.AgentErrorCauseOperationStart)
			if !ok {
				t.Fatal("expected a normalized safe application cause")
			}
			encoded, err := json.Marshal(cause)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]interface{}
			if err := json.Unmarshal(encoded, &fields); err != nil {
				t.Fatal(err)
			}
			if fields["attempted_model"] != test.attempt {
				t.Fatalf("attempted_model = %v, want %q; full cause=%+v", fields["attempted_model"], test.attempt, cause)
			}
			if cause.RequestedModel != test.policy.Model {
				t.Fatalf("configured requested model = %q, want %q", cause.RequestedModel, test.policy.Model)
			}
		})
	}
}
