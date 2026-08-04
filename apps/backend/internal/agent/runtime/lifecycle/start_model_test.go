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

// fakeModelApplier records SetModel calls and returns a scripted error.
type fakeModelApplier struct {
	calls []string
	err   error
}

func (f *fakeModelApplier) SetModel(_ context.Context, modelID string) error {
	f.calls = append(f.calls, modelID)
	return f.err
}

func newPolicyTestLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	return log
}

func modelState(models ...string) *CachedModelState {
	infos := make([]streams.SessionModelInfo, 0, len(models))
	for _, id := range models {
		infos = append(infos, streams.SessionModelInfo{ModelID: id})
	}
	return &CachedModelState{Models: infos}
}

func methodNotFoundErr() error {
	return &acp.RequestError{Code: -32601, Message: "method not found"}
}

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

func TestApplyStartModelPolicy_StrictGoneModelNoSession(t *testing.T) {
	// Empty advertised list = "unknown" — fall back to SetModel result.
	applier := &fakeModelApplier{err: errors.New("model rejected")}
	_, _, err := applyStartModelPolicy(
		context.Background(), newPolicyTestLogger(), applier,
		&CachedModelState{}, StartModelPolicy{Model: "claude-sonnet"},
	)
	if err == nil {
		t.Fatal("expected error when SetModel fails and list is unknown")
	}
}

func TestApplyStartModelPolicy_StrictGoneModelUnsupportedContinues(t *testing.T) {
	applier := &fakeModelApplier{err: methodNotFoundErr()}
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

func TestApplyStartModelPolicy_FallbackModelFailureFails(t *testing.T) {
	applier := &fakeModelApplier{err: errors.New("rejected")}
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

func TestApplyStartModelPolicy_AutoFallbackLegacyBestEffort(t *testing.T) {
	// Gone model + auto-fallback: legacy behavior — try, warn, continue.
	applier := &fakeModelApplier{err: errors.New("not in available models")}
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
