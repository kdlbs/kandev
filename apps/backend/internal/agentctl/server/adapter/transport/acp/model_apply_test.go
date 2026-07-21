package acp

import (
	"context"
	"errors"
	"reflect"
	"testing"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/sessionmodel"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

type fakeModelApplier struct {
	configErr      error
	legacyErr      error
	configResponse sdk.SetSessionConfigOptionResponse
	calls          []string
}

func (f *fakeModelApplier) SetSessionConfigOption(_ context.Context, req sdk.SetSessionConfigOptionRequest) (sdk.SetSessionConfigOptionResponse, error) {
	f.calls = append(f.calls, "config:"+string(req.ValueId.ConfigId)+":"+string(req.ValueId.Value))
	return f.configResponse, f.configErr
}

func TestApplySessionModelWithConfigOptionsReturnsAuthoritativeResponse(t *testing.T) {
	t.Parallel()
	options := sdk.SessionConfigSelectOptionsUngrouped{{Value: "low", Name: "Low"}}
	responseConfig := []sdk.SessionConfigOption{{Select: &sdk.SessionConfigOptionSelect{
		Type: "select", Id: "reasoning_effort", Name: "Reasoning effort",
		CurrentValue: "low", Options: sdk.SessionConfigSelectOptions{Ungrouped: &options},
	}}}
	conn := &fakeModelApplier{configResponse: sdk.SetSessionConfigOptionResponse{ConfigOptions: responseConfig}}

	method, got, configID, err := applySessionModelWithConfigOptions(
		context.Background(), conn, "sess-1", "gpt-5", []streams.ConfigOption{{
			ID: "model", Category: "model",
		}},
	)

	if err != nil {
		t.Fatalf("applySessionModelWithConfigOptions() error = %v", err)
	}
	if method != sessionmodel.MethodSetConfigOption || configID != "model" {
		t.Fatalf("method/configID = %q/%q, want set_config_option/model", method, configID)
	}
	if !reflect.DeepEqual(got, responseConfig) {
		t.Fatalf("response config = %#v, want %#v", got, responseConfig)
	}
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

// TestApplySessionModel_PropagatesSetConfigOptionError pins that non
// MethodNotFound errors from the underlying SetSessionConfigOption RPC bubble
// up unchanged when the session advertises a model-shaped config option.
func TestApplySessionModel_PropagatesSetConfigOptionError(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	conn := &fakeModelApplier{configErr: boom}
	_, err := applySessionModel(context.Background(), conn, "sess-1", "claude-opus-4-8", []streams.ConfigOption{{
		ID:       "model",
		Category: "model",
	}})

	if !errors.Is(err, boom) {
		t.Fatalf("applySessionModel() error = %v, want boom", err)
	}
	wantCalls := []string{"config:model:claude-opus-4-8"}
	if !reflect.DeepEqual(conn.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", conn.calls, wantCalls)
	}
}

// TestApplySessionModel_SetConfigOptionMethodNotFoundFallsBack pins the
// partial-migration case: when the agent advertises the typed model-shaped
// config option but hasn't wired up the session/set_config_option handler,
// applySessionModel falls through to the legacy session/set_model RPC.
func TestApplySessionModel_SetConfigOptionMethodNotFoundFallsBack(t *testing.T) {
	t.Parallel()

	conn := &fakeModelApplier{configErr: sdk.NewMethodNotFound(sdk.AgentMethodSessionSetConfigOption)}
	method, err := applySessionModel(context.Background(), conn, "sess-1", "claude-opus-4-8", []streams.ConfigOption{{
		ID:       "model",
		Category: "model",
	}})

	if err != nil {
		t.Fatalf("applySessionModel() error = %v, want nil after legacy fallback", err)
	}
	if method != "session/set_model" {
		t.Fatalf("method = %q, want session/set_model", method)
	}
	wantCalls := []string{"config:model:claude-opus-4-8", "legacy:sess-1:claude-opus-4-8"}
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
