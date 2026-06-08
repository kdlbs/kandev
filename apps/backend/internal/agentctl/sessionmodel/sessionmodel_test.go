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
	modelErr  error
	calls     []string
}

func (f *fakeApplier) SetConfigOption(_ context.Context, _ string, configID, value string) error {
	f.calls = append(f.calls, "config:"+configID+":"+value)
	return f.configErr
}

func (f *fakeApplier) SetModel(_ context.Context, _, modelID string) error {
	f.calls = append(f.calls, "model:"+modelID)
	return f.modelErr
}

func TestApply_PrefersModelConfigOption(t *testing.T) {
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

func TestApply_FallsBackToSetModelWhenConfigOptionMethodMissing(t *testing.T) {
	t.Parallel()

	applier := &fakeApplier{configErr: acp.NewMethodNotFound(acp.AgentMethodSessionSetConfigOption)}
	method, err := Apply(context.Background(), applier, Request{
		SessionID: "sess-1",
		ModelID:   "claude-opus-4-8",
		ConfigOptions: []ConfigOption{{
			ID:       "model",
			Category: "model",
		}},
	})

	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if method != MethodSetModel {
		t.Fatalf("method = %q, want %q", method, MethodSetModel)
	}
	wantCalls := []string{"config:model:claude-opus-4-8", "model:claude-opus-4-8"}
	if !reflect.DeepEqual(applier.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", applier.calls, wantCalls)
	}
}

func TestApply_UsesSetModelWhenNoModelConfigOption(t *testing.T) {
	t.Parallel()

	applier := &fakeApplier{}
	method, err := Apply(context.Background(), applier, Request{
		SessionID:     "sess-1",
		ModelID:       "legacy-model",
		ConfigOptions: []ConfigOption{{ID: "mode", Category: "mode"}},
	})

	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if method != MethodSetModel {
		t.Fatalf("method = %q, want %q", method, MethodSetModel)
	}
	wantCalls := []string{"model:legacy-model"}
	if !reflect.DeepEqual(applier.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", applier.calls, wantCalls)
	}
}

func TestApply_DoesNotFallbackForInvalidConfigOptionValue(t *testing.T) {
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
