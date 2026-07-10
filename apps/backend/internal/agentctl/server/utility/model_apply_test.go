package utility

import (
	"context"
	"reflect"
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

type fakeModelConn struct {
	calls []string
}

func (f *fakeModelConn) SetSessionConfigOption(_ context.Context, req acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
	f.calls = append(f.calls, "config:"+string(req.ValueId.ConfigId)+":"+string(req.ValueId.Value))
	return acp.SetSessionConfigOptionResponse{}, nil
}

func (f *fakeModelConn) UnstableSetSessionModel(_ context.Context, req acp.UnstableSetSessionModelRequest) (acp.UnstableSetSessionModelResponse, error) {
	f.calls = append(f.calls, "legacy:"+string(req.SessionId)+":"+req.ModelId)
	return acp.UnstableSetSessionModelResponse{}, nil
}

func TestApplySessionModel_UsesConfigOptionWhenSessionAdvertisesModelOption(t *testing.T) {
	t.Parallel()

	modelCat := acp.SessionConfigOptionCategoryModel
	conn := &fakeModelConn{}
	method, err := applySessionModel(context.Background(), conn, "sess-1", "gpt-5.4-mini", []acp.SessionConfigOption{
		{Select: &acp.SessionConfigOptionSelect{
			Id:       "model",
			Category: &modelCat,
			Type:     "select",
		}},
	})

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

// TestApplySessionModel_FallsBackToLegacySetModel pins that when the session
// advertises no model-shaped config option, the dispatcher falls through to
// the pre-v0.13.5 session/set_model RPC restored by the kdlbs acp-go-sdk
// fork. This keeps model switching working for unmigrated agents like
// auggie 0.29.x, which surface their models via the legacy top-level
// `models` field on session/new instead of a typed config option.
func TestApplySessionModel_FallsBackToLegacySetModel(t *testing.T) {
	t.Parallel()

	conn := &fakeModelConn{}
	method, err := applySessionModel(context.Background(), conn, "sess-1", "legacy-model", nil)

	if err != nil {
		t.Fatalf("applySessionModel() error = %v", err)
	}
	if method != "session/set_model" {
		t.Fatalf("method = %q, want session/set_model", method)
	}
	wantCalls := []string{"legacy:sess-1:legacy-model"}
	if !reflect.DeepEqual(conn.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", conn.calls, wantCalls)
	}
}

// TestApplySessionModel_NoOpWhenAgentSupportsNeitherSurface pins that when
// the session has no model-shaped config option AND the legacy
// session/set_model RPC returns JSON-RPC -32601 (method not found), the
// dispatcher treats the request as a clean no-op (empty method, nil error).
// This is how agents that support no model-selection surface at all signal
// "I don't support model selection".
func TestApplySessionModel_NoOpWhenAgentSupportsNeitherSurface(t *testing.T) {
	t.Parallel()

	conn := &fakeModelConnLegacyErr{
		err: acp.NewMethodNotFound(acp.LegacyAgentMethodSessionSetModel),
	}
	method, err := applySessionModel(context.Background(), conn, "sess-1", "legacy-model", nil)

	if err != nil {
		t.Fatalf("applySessionModel() error = %v, want nil", err)
	}
	if method != "" {
		t.Fatalf("method = %q, want empty (MethodNone)", method)
	}
	wantCalls := []string{"legacy:sess-1:legacy-model"}
	if !reflect.DeepEqual(conn.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", conn.calls, wantCalls)
	}
}

// fakeModelConnLegacyErr returns a configurable error from the legacy
// session/set_model RPC. Used to exercise the -32601 no-op path.
type fakeModelConnLegacyErr struct {
	err   error
	calls []string
}

func (f *fakeModelConnLegacyErr) SetSessionConfigOption(_ context.Context, req acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
	f.calls = append(f.calls, "config:"+string(req.ValueId.ConfigId)+":"+string(req.ValueId.Value))
	return acp.SetSessionConfigOptionResponse{}, nil
}

func (f *fakeModelConnLegacyErr) UnstableSetSessionModel(_ context.Context, req acp.UnstableSetSessionModelRequest) (acp.UnstableSetSessionModelResponse, error) {
	f.calls = append(f.calls, "legacy:"+string(req.SessionId)+":"+req.ModelId)
	return acp.UnstableSetSessionModelResponse{}, f.err
}
