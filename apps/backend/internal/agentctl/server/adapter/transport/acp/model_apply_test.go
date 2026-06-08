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
	modelErr  error
	calls     []string
}

func (f *fakeModelApplier) SetSessionConfigOption(_ context.Context, req sdk.SetSessionConfigOptionRequest) (sdk.SetSessionConfigOptionResponse, error) {
	f.calls = append(f.calls, "config:"+string(req.ValueId.ConfigId)+":"+string(req.ValueId.Value))
	return sdk.SetSessionConfigOptionResponse{}, f.configErr
}

func (f *fakeModelApplier) UnstableSetSessionModel(_ context.Context, req sdk.UnstableSetSessionModelRequest) (sdk.UnstableSetSessionModelResponse, error) {
	f.calls = append(f.calls, "model:"+string(req.ModelId))
	return sdk.UnstableSetSessionModelResponse{}, f.modelErr
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

func TestApplySessionModel_FallsBackToSetModelWhenSetConfigOptionMissing(t *testing.T) {
	t.Parallel()

	conn := &fakeModelApplier{configErr: sdk.NewMethodNotFound(sdk.AgentMethodSessionSetConfigOption)}
	method, err := applySessionModel(context.Background(), conn, "sess-1", "claude-opus-4-8", []streams.ConfigOption{{
		ID:       "model",
		Category: "model",
	}})

	if err != nil {
		t.Fatalf("applySessionModel() error = %v", err)
	}
	if method != "session/set_model" {
		t.Fatalf("method = %q, want session/set_model", method)
	}
	wantCalls := []string{"config:model:claude-opus-4-8", "model:claude-opus-4-8"}
	if !reflect.DeepEqual(conn.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", conn.calls, wantCalls)
	}
}
