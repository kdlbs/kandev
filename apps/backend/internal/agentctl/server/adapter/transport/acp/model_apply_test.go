package acp

import (
	"context"
	"reflect"
	"testing"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

type fakeModelApplier struct {
	configErr error
	legacyErr error
	calls     []string
}

func (f *fakeModelApplier) SetSessionConfigOption(_ context.Context, req sdk.SetSessionConfigOptionRequest) (sdk.SetSessionConfigOptionResponse, error) {
	f.calls = append(f.calls, "config:"+string(req.ValueId.ConfigId)+":"+string(req.ValueId.Value))
	return sdk.SetSessionConfigOptionResponse{}, f.configErr
}

func (f *fakeModelApplier) UnstableSetSessionModel(_ context.Context, req sdk.UnstableSetSessionModelRequest) (sdk.UnstableSetSessionModelResponse, error) {
	f.calls = append(f.calls, "legacy:"+string(req.SessionId)+":"+req.ModelId)
	return sdk.UnstableSetSessionModelResponse{}, f.legacyErr
}

func TestApplySessionModel_UsesConfigOptionForModelConfig(t *testing.T) {
	t.Parallel()

	conn := &fakeModelApplier{}
	method, err := applySessionModel(context.Background(), conn, "sess-1", "gpt-5.4-mini", []streams.ConfigOption{{
		ID:       "model",
		Category: "model",
	}})

	if err != nil {
		t.Fatalf("applySessionModel() error = %v", err)
	}
	if method != "session/set_config_option" {
		t.Fatalf("method = %q, want session/set_config_option", method)
	}
	wantCalls := []string{"config:model:gpt-5.4-mini"}
	if !reflect.DeepEqual(conn.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", conn.calls, wantCalls)
	}
}

// TestApplySessionModel_PropagatesSetConfigOptionError pins that errors from
// the underlying SetSessionConfigOption RPC bubble up unchanged when the
// session does advertise a model-shaped config option (the legacy fallback
// is exercised separately in TestApplySessionModel_FallsBackToLegacySetModel).
func TestApplySessionModel_PropagatesSetConfigOptionError(t *testing.T) {
	t.Parallel()

	conn := &fakeModelApplier{configErr: sdk.NewMethodNotFound(sdk.AgentMethodSessionSetConfigOption)}
	_, err := applySessionModel(context.Background(), conn, "sess-1", "claude-opus-4-8", []streams.ConfigOption{{
		ID:       "model",
		Category: "model",
	}})

	if err == nil {
		t.Fatalf("applySessionModel() error = nil, want method-not-found")
	}
	wantCalls := []string{"config:model:claude-opus-4-8"}
	if !reflect.DeepEqual(conn.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", conn.calls, wantCalls)
	}
}

// TestApplySessionModel_FallsBackToLegacySetModel pins the SDK-side fallback
// path: when the session has no model-shaped config option (e.g. auggie 0.29.x
// which surfaces models via the legacy top-level `models` field), the
// dispatcher uses the kdlbs-fork-restored session/set_model RPC instead.
func TestApplySessionModel_FallsBackToLegacySetModel(t *testing.T) {
	t.Parallel()

	conn := &fakeModelApplier{}
	method, err := applySessionModel(context.Background(), conn, "sess-1", "claude-opus-4-7", nil)

	if err != nil {
		t.Fatalf("applySessionModel() error = %v", err)
	}
	if method != "session/set_model" {
		t.Fatalf("method = %q, want session/set_model", method)
	}
	wantCalls := []string{"legacy:sess-1:claude-opus-4-7"}
	if !reflect.DeepEqual(conn.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", conn.calls, wantCalls)
	}
}
