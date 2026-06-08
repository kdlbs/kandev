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
	f.calls = append(f.calls, "model:"+string(req.ModelId))
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

func TestApplySessionModel_UsesSetModelWithoutModelConfigOption(t *testing.T) {
	t.Parallel()

	conn := &fakeModelConn{}
	method, err := applySessionModel(context.Background(), conn, "sess-1", "legacy-model", nil)

	if err != nil {
		t.Fatalf("applySessionModel() error = %v", err)
	}
	if method != "session/set_model" {
		t.Fatalf("method = %q, want session/set_model", method)
	}
	wantCalls := []string{"model:legacy-model"}
	if !reflect.DeepEqual(conn.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", conn.calls, wantCalls)
	}
}
