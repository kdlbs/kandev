package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
)

// fakeModelApplier records SetModel calls and returns scripted errors, one
// per call; the last error repeats for any further calls.
type fakeModelApplier struct {
	calls []string
	errs  []error
}

// SetModel records the applied model so tests can assert what the policy sent.

func (f *fakeModelApplier) SetModel(_ context.Context, modelID string) error {
	f.calls = append(f.calls, modelID)
	if len(f.errs) == 0 {
		return nil
	}
	err := f.errs[0]
	if len(f.errs) > 1 {
		f.errs = f.errs[1:]
	}
	return err
}

// newPolicyTestLogger returns a discard logger for policy tests.

func newPolicyTestLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	return log
}

// modelState builds a cached model state advertising the given model ids.

func modelState(models ...string) *CachedModelState {
	infos := make([]streams.SessionModelInfo, 0, len(models))
	for _, id := range models {
		infos = append(infos, streams.SessionModelInfo{ModelID: id})
	}
	return &CachedModelState{Models: infos}
}

// methodNotFoundErr simulates the agent declaring no model-selection support.

func methodNotFoundErr() error {
	return &acp.RequestError{Code: -32601, Message: "method not found"}
}

// TestApplyStartModelPolicy_StrictGoneModelFails verifies a gone start model fails the session with the actionable message.

func TestApplyStartModelPolicy_StrictGoneModelFails(t *testing.T) {
	applier := &fakeModelApplier{}
	_, usingFallback, err := applyStartModelPolicy(
		context.Background(), newPolicyTestLogger(), applier,
		modelState("gpt-5"), StartModelPolicy{Model: "claude-gone"},
	)
	if err == nil {
		t.Fatal("expected error for gone start model in strict mode")
	}
	if !strings.Contains(err.Error(), "claude-gone") {
		t.Errorf("error should name the gone model, got %q", err.Error())
	}
	if usingFallback {
		t.Error("usingFallback should be false")
	}
	if len(applier.calls) != 0 {
		t.Errorf("SetModel should not be called for a gone model in strict mode, got %v", applier.calls)
	}
}

// TestApplyStartModelPolicy_StrictGoneModelNoSession verifies no model is applied when the strict policy rejects a gone start model.

func TestApplyStartModelPolicy_StrictGoneModelNoSession(t *testing.T) {
	// Empty advertised list = "unknown" — fall back to SetModel result.
	applier := &fakeModelApplier{errs: []error{errors.New("model rejected")}}
	_, _, err := applyStartModelPolicy(
		context.Background(), newPolicyTestLogger(), applier,
		&CachedModelState{}, StartModelPolicy{Model: "claude-sonnet"},
	)
	if err == nil {
		t.Fatal("expected error when SetModel fails and list is unknown")
	}
}

// TestApplyStartModelPolicy_StrictGoneModelUnsupportedContinues verifies a method-not-found SetModel error continues silently.

func TestApplyStartModelPolicy_StrictGoneModelUnsupportedContinues(t *testing.T) {
	applier := &fakeModelApplier{errs: []error{methodNotFoundErr()}}
	applied, usingFallback, err := applyStartModelPolicy(
		context.Background(), newPolicyTestLogger(), applier,
		&CachedModelState{}, StartModelPolicy{Model: "claude-sonnet"},
	)
	if err != nil {
		t.Fatalf("method-not-found should continue silently, got %v", err)
	}
	if applied != "" || usingFallback {
		t.Errorf("expected no applied model, got applied=%q fallback=%v", applied, usingFallback)
	}
}

// TestApplyStartModelPolicy_StrictModelPresentApplies verifies an advertised start model is applied.

func TestApplyStartModelPolicy_StrictModelPresentApplies(t *testing.T) {
	applier := &fakeModelApplier{}
	applied, usingFallback, err := applyStartModelPolicy(
		context.Background(), newPolicyTestLogger(), applier,
		modelState("claude-sonnet"), StartModelPolicy{Model: "claude-sonnet"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if applied != "claude-sonnet" || usingFallback {
		t.Errorf("expected applied=claude-sonnet fallback=false, got applied=%q fallback=%v", applied, usingFallback)
	}
	if len(applier.calls) != 1 || applier.calls[0] != "claude-sonnet" {
		t.Errorf("SetModel calls = %v, want [claude-sonnet]", applier.calls)
	}
}

// TestApplyStartModelPolicy_FallbackModelUsedWhenGone verifies the configured fallback replaces a gone start model.

func TestApplyStartModelPolicy_FallbackModelUsedWhenGone(t *testing.T) {
	applier := &fakeModelApplier{}
	applied, usingFallback, err := applyStartModelPolicy(
		context.Background(), newPolicyTestLogger(), applier,
		modelState("gpt-5"), StartModelPolicy{Model: "claude-gone", FallbackModel: "gpt-5"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if applied != "gpt-5" || !usingFallback {
		t.Errorf("expected applied=gpt-5 fallback=true, got applied=%q fallback=%v", applied, usingFallback)
	}
	if len(applier.calls) != 1 || applier.calls[0] != "gpt-5" {
		t.Errorf("SetModel calls = %v, want [gpt-5]", applier.calls)
	}
}

// TestApplyStartModelPolicy_FallbackWhenSetModelRejectsGoneModel verifies the fallback applies when SetModel rejects the start model even without an advertised list.

func TestApplyStartModelPolicy_FallbackWhenSetModelRejectsGoneModel(t *testing.T) {
	// Regression: at session start the advertised model list may not have
	// arrived yet (empty = "unknown"). The adapter still rejects the gone
	// start model — the fallback must be applied then too, not only when
	// the list is known.
	applier := &fakeModelApplier{errs: []error{
		errors.New(`model "claude-gone" is not in the agent's 2 available models`),
		nil,
	}}
	applied, usingFallback, err := applyStartModelPolicy(
		context.Background(), newPolicyTestLogger(), applier,
		&CachedModelState{}, StartModelPolicy{Model: "claude-gone", FallbackModel: "gpt-5"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if applied != "gpt-5" || !usingFallback {
		t.Errorf("expected applied=gpt-5 fallback=true, got applied=%q fallback=%v", applied, usingFallback)
	}
	if len(applier.calls) != 2 || applier.calls[0] != "claude-gone" || applier.calls[1] != "gpt-5" {
		t.Errorf("SetModel calls = %v, want [claude-gone gpt-5]", applier.calls)
	}
}

// TestApplyStartModelPolicy_FallbackModelFailureFails verifies a failed fallback SetModel fails the session explicitly.

func TestApplyStartModelPolicy_FallbackModelFailureFails(t *testing.T) {
	applier := &fakeModelApplier{errs: []error{errors.New("rejected")}}
	_, _, err := applyStartModelPolicy(
		context.Background(), newPolicyTestLogger(), applier,
		modelState("gpt-5"), StartModelPolicy{Model: "claude-gone", FallbackModel: "gpt-5"},
	)
	if err == nil {
		t.Fatal("expected error when the fallback model itself fails to apply")
	}
	if !strings.Contains(err.Error(), "gpt-5") {
		t.Errorf("error should name the fallback model, got %q", err.Error())
	}
}

// TestApplyStartModelPolicy_AutoFallbackLegacyBestEffort verifies auto-fallback logs and continues on SetModel failure.

func TestApplyStartModelPolicy_AutoFallbackLegacyBestEffort(t *testing.T) {
	// Gone model + auto-fallback: legacy behavior — try, warn, continue.
	applier := &fakeModelApplier{errs: []error{errors.New("not in available models")}}
	applied, usingFallback, err := applyStartModelPolicy(
		context.Background(), newPolicyTestLogger(), applier,
		modelState("gpt-5"), StartModelPolicy{Model: "claude-gone", AutoFallback: true},
	)
	if err != nil {
		t.Fatalf("auto-fallback should never fail: %v", err)
	}
	if applied != "" || usingFallback {
		t.Errorf("expected applied=\"\" fallback=false, got applied=%q fallback=%v", applied, usingFallback)
	}
	if len(applier.calls) != 1 {
		t.Errorf("legacy path should still attempt SetModel, calls=%v", applier.calls)
	}
}

// TestApplyStartModelPolicy_AutoFallbackPresentApplies verifies auto-fallback still applies an available start model.

func TestApplyStartModelPolicy_AutoFallbackPresentApplies(t *testing.T) {
	applier := &fakeModelApplier{}
	applied, usingFallback, err := applyStartModelPolicy(
		context.Background(), newPolicyTestLogger(), applier,
		modelState("claude-sonnet"), StartModelPolicy{Model: "claude-sonnet", AutoFallback: true},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if applied != "claude-sonnet" || usingFallback {
		t.Errorf("expected applied=claude-sonnet fallback=false, got applied=%q fallback=%v", applied, usingFallback)
	}
}

// TestApplyStartModelPolicy_EmptyModelNoOp verifies an empty start model is a no-op.

func TestApplyStartModelPolicy_EmptyModelNoOp(t *testing.T) {
	applier := &fakeModelApplier{}
	applied, usingFallback, err := applyStartModelPolicy(
		context.Background(), newPolicyTestLogger(), applier,
		modelState("gpt-5"), StartModelPolicy{},
	)
	if err != nil || applied != "" || usingFallback {
		t.Errorf("empty model should no-op, got applied=%q fallback=%v err=%v", applied, usingFallback, err)
	}
	if len(applier.calls) != 0 {
		t.Errorf("SetModel should not be called for empty model, got %v", applier.calls)
	}
}

// TestApplyStartModelPolicy_TransportErrorDoesNotFallback verifies a
// transport/protocol SetModel failure fails explicitly instead of switching
// to the fallback: a broken connection is not evidence the model is
// unavailable, so applying the fallback would mask the real fault.
func TestApplyStartModelPolicy_TransportErrorDoesNotFallback(t *testing.T) {
	applier := &fakeModelApplier{errs: []error{
		errors.New(`websocket: connection closed: peer disconnected`),
	}}
	_, _, err := applyStartModelPolicy(
		context.Background(), newPolicyTestLogger(), applier,
		modelState("claude-gone"), StartModelPolicy{Model: "claude-gone", FallbackModel: "gpt-5"},
	)
	if err == nil {
		t.Fatal("expected explicit failure for a transport error")
	}
	if len(applier.calls) != 1 || applier.calls[0] != "claude-gone" {
		t.Errorf("SetModel calls = %v, want only [claude-gone] (no fallback)", applier.calls)
	}
}

// TestApplyStartModelPolicy_CancelErrorDoesNotFallback verifies a cancelled
// SetModel context does not switch models either.
func TestApplyStartModelPolicy_CancelErrorDoesNotFallback(t *testing.T) {
	applier := &fakeModelApplier{errs: []error{context.Canceled}}
	_, _, err := applyStartModelPolicy(
		context.Background(), newPolicyTestLogger(), applier,
		modelState("claude-gone"), StartModelPolicy{Model: "claude-gone", FallbackModel: "gpt-5"},
	)
	if err == nil {
		t.Fatal("expected explicit failure for a cancelled SetModel")
	}
	if len(applier.calls) != 1 || applier.calls[0] != "claude-gone" {
		t.Errorf("SetModel calls = %v, want only [claude-gone] (no fallback)", applier.calls)
	}
}
