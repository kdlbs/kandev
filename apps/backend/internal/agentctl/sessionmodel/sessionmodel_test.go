package sessionmodel

import (
	"context"
	"errors"
	"reflect"
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

type fakeApplier struct {
	configErr error
	legacyErr error
	calls     []string
}

func (f *fakeApplier) SetConfigOption(_ context.Context, _ string, configID, value string) error {
	f.calls = append(f.calls, "config:"+configID+":"+value)
	return f.configErr
}

func (f *fakeApplier) SetModelLegacy(_ context.Context, sessionID, modelID string) error {
	f.calls = append(f.calls, "legacy:"+sessionID+":"+modelID)
	return f.legacyErr
}

func TestApply_UsesModelConfigOption(t *testing.T) {
	t.Parallel()

	applier := &fakeApplier{}
	method, err := Apply(context.Background(), applier, Request{
		SessionID: "sess-1",
		ModelID:   "gpt-5.4-mini",
		ConfigOptions: []ConfigOption{{
			ID:       "model",
			Category: "model",
		}},
	})

	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if method != MethodSetConfigOption {
		t.Fatalf("method = %q, want %q", method, MethodSetConfigOption)
	}
	wantCalls := []string{"config:model:gpt-5.4-mini"}
	if !reflect.DeepEqual(applier.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", applier.calls, wantCalls)
	}
}

// TestApply_FallsBackToLegacySetModel verifies the legacy fallback path:
// when the session advertises no model-shaped config option, Apply issues
// the pre-v0.13.5 session/set_model RPC restored by the kdlbs fork. This
// keeps model switching working for unmigrated agents (e.g. auggie 0.29.x).
func TestApply_FallsBackToLegacySetModel(t *testing.T) {
	t.Parallel()

	applier := &fakeApplier{}
	method, err := Apply(context.Background(), applier, Request{
		SessionID:     "sess-1",
		ModelID:       "claude-opus-4-7",
		ConfigOptions: []ConfigOption{{ID: "mode", Category: "mode"}},
	})

	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if method != MethodSetModel {
		t.Fatalf("method = %q, want %q", method, MethodSetModel)
	}
	wantCalls := []string{"legacy:sess-1:claude-opus-4-7"}
	if !reflect.DeepEqual(applier.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", applier.calls, wantCalls)
	}
}

// TestApply_LegacyMethodNotFoundIsNoOp pins that when the legacy
// session/set_model RPC returns JSON-RPC -32601, Apply treats the request as
// a clean no-op (MethodNone, nil error). This is how agents that support
// neither surface (no config option AND no legacy RPC) signal "I don't
// support model selection".
func TestApply_LegacyMethodNotFoundIsNoOp(t *testing.T) {
	t.Parallel()

	notFound := acp.NewMethodNotFound(acp.LegacyAgentMethodSessionSetModel)
	applier := &fakeApplier{legacyErr: notFound}
	method, err := Apply(context.Background(), applier, Request{
		SessionID:     "sess-1",
		ModelID:       "some-model",
		ConfigOptions: []ConfigOption{{ID: "mode", Category: "mode"}},
	})

	if err != nil {
		t.Fatalf("Apply() error = %v, want nil", err)
	}
	if method != MethodNone {
		t.Fatalf("method = %q, want %q", method, MethodNone)
	}
	wantCalls := []string{"legacy:sess-1:some-model"}
	if !reflect.DeepEqual(applier.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", applier.calls, wantCalls)
	}
}

// TestApply_PropagatesLegacySetModelError pins that non-32601 errors from
// the legacy RPC bubble up unchanged with MethodSetModel attribution.
func TestApply_PropagatesLegacySetModelError(t *testing.T) {
	t.Parallel()

	invalid := acp.NewInvalidParams(map[string]any{"message": "Unknown model"})
	applier := &fakeApplier{legacyErr: invalid}
	method, err := Apply(context.Background(), applier, Request{
		SessionID:     "sess-1",
		ModelID:       "bogus",
		ConfigOptions: []ConfigOption{{ID: "mode", Category: "mode"}},
	})

	if !errors.Is(err, invalid) {
		t.Fatalf("Apply() error = %v, want %v", err, invalid)
	}
	if method != MethodSetModel {
		t.Fatalf("method = %q, want %q", method, MethodSetModel)
	}
}

func TestApply_PropagatesSetConfigOptionError(t *testing.T) {
	t.Parallel()

	invalid := acp.NewInvalidParams(map[string]any{"message": "Invalid model value"})
	applier := &fakeApplier{configErr: invalid}
	method, err := Apply(context.Background(), applier, Request{
		SessionID: "sess-1",
		ModelID:   "claude-haiku-4-5",
		ConfigOptions: []ConfigOption{{
			ID:       "model",
			Category: "model",
		}},
	})

	if !errors.Is(err, invalid) {
		t.Fatalf("Apply() error = %v, want %v", err, invalid)
	}
	if method != MethodSetConfigOption {
		t.Fatalf("method = %q, want %q", method, MethodSetConfigOption)
	}
	wantCalls := []string{"config:model:claude-haiku-4-5"}
	if !reflect.DeepEqual(applier.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", applier.calls, wantCalls)
	}
}

// TestApply_SetConfigOptionMethodNotFoundFallsBackToLegacy pins the
// partial-migration fallthrough: when the session advertises a model-shaped
// config option but the agent answers session/set_config_option with -32601,
// Apply falls through to the legacy session/set_model RPC instead of
// surfacing the error.
func TestApply_SetConfigOptionMethodNotFoundFallsBackToLegacy(t *testing.T) {
	t.Parallel()

	applier := &fakeApplier{configErr: acp.NewMethodNotFound(acp.AgentMethodSessionSetConfigOption)}
	method, err := Apply(context.Background(), applier, Request{
		SessionID: "sess-1",
		ModelID:   "claude-opus-4-7",
		ConfigOptions: []ConfigOption{{
			ID:       "model",
			Category: "model",
		}},
	})

	if err != nil {
		t.Fatalf("Apply() error = %v, want nil", err)
	}
	if method != MethodSetModel {
		t.Fatalf("method = %q, want %q", method, MethodSetModel)
	}
	wantCalls := []string{"config:model:claude-opus-4-7", "legacy:sess-1:claude-opus-4-7"}
	if !reflect.DeepEqual(applier.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", applier.calls, wantCalls)
	}
}
