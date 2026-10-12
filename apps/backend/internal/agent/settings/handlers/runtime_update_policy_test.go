package handlers

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/settings/controller"
	"net/http"
	"net/http/httptest"
	"testing"
)

type policySettings struct {
	raw     map[string][]byte
	failure error
}

func (s *policySettings) Get(_ context.Context, key string) ([]byte, bool, error) {
	v, ok := s.raw[key]
	return v, ok, nil
}
func (s *policySettings) Save(_ context.Context, key string, value []byte) error {
	if s.failure != nil {
		return s.failure
	}
	if s.raw == nil {
		s.raw = map[string][]byte{}
	}
	s.raw[key] = value
	return nil
}
func (s *policySettings) Delete(context.Context, string) error { return nil }

type verifiedPolicyUpdater struct{ *handlerRuntimeUpdater }

func (u verifiedPolicyUpdater) Probe(ctx context.Context, name string, command agents.Command) (hostutility.AgentCapabilities, error) {
	return u.Refresh(ctx, name, command)
}
func (verifiedPolicyUpdater) PublishCapabilities(string, hostutility.AgentCapabilities) {}

func (verifiedPolicyUpdater) ResolveVersions(context.Context, string) (controller.RuntimeVersionMetadata, error) {
	return controller.RuntimeVersionMetadata{Latest: "9.0.0", Versions: []string{"9.0.0"}}, nil
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-002.1
func TestAutomaticPolicyEndpointPersistsConsentAndFailsTruthfully(t *testing.T) {
	router, ctrl, _ := newAgentUpdateRouter(t, verifiedPolicyUpdater{&handlerRuntimeUpdater{}})
	settings := &policySettings{}
	ctrl.SetManagedRuntimeSelectionStore(handlerSelectionStore{})
	ctrl.SetRuntimeAutoUpdateStore(managedruntime.NewAutoUpdateStore(settings))
	for _, test := range []struct {
		body, path string
		want       int
	}{
		{`{"enabled":true}`, "gemini", 200},
		{`{"enabled":false}`, "gemini", 200},
		{`{}`, "gemini", 400},
		{`{"enabled":"yes"}`, "gemini", 400},
		{`{"enabled":true} {"enabled":false}`, "gemini", 400},
		{`{"enabled":true} trailing`, "gemini", 400},
		{`{"enabled":true,"unexpected":true}`, "gemini", 400},
		{`{"enabled":true}`, "cursor-acp", 400},
		{`{"enabled":true}`, "missing-agent", 404},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, updateJSONRequest(http.MethodPatch, "/api/v1/agent-update/"+test.path+"/automatic", test.body))
		if response.Code != test.want {
			t.Fatalf("%s %s status=%d body=%s", test.path, test.body, response.Code, response.Body.String())
		}
	}
	spec := agents.NewGemini().ManagedNPMRuntime()
	saved, err := managedruntime.NewAutoUpdateStore(settings).Get(context.Background(), "gemini", "npm:"+spec.Package)
	if err != nil || saved.Enabled {
		t.Fatalf("disabled consent not retained: %+v,%v", saved, err)
	}
	settings.failure = errors.New("sensitive persistence detail")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, updateJSONRequest(http.MethodPatch, "/api/v1/agent-update/gemini/automatic", `{"enabled":true}`))
	if response.Code != 500 || response.Body.String() != "{\"error\":\"failed to save automatic runtime policy\"}" {
		t.Fatalf("persistence failure lied or leaked: %d %s", response.Code, response.Body.String())
	}
}
