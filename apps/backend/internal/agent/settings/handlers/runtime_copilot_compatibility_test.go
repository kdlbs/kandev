package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/agent/copilotplugins"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

func TestCopilotRuntimeCompatibilityPreviewHandlerContract(t *testing.T) {
	router, _, _ := newAgentUpdateRouter(t, &handlerRuntimeUpdater{})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, updateRequest(
		http.MethodGet,
		"/api/v1/agent-update/copilot-acp/preview",
	))
	if response.Code != http.StatusOK {
		t.Fatalf("GET preview status = %d, body = %s", response.Code, response.Body.String())
	}
	var preview dto.AgentUpdatePreviewDTO
	if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	compatibility := preview.CopilotPluginCompatibility
	if compatibility == nil {
		t.Fatal("Copilot preview omitted plugin compatibility")
	}
	if compatibility.RuntimeVersion != "1.1.0" || compatibility.Supported {
		t.Fatalf("compatibility = %#v", compatibility)
	}
	if compatibility.Reason != "runtime_unsupported" {
		t.Fatalf("reason = %q, want runtime_unsupported", compatibility.Reason)
	}
	if !reflect.DeepEqual(compatibility.SupportedVersions, copilotplugins.SupportedRuntimeVersions()) {
		t.Fatalf("supported versions = %v", compatibility.SupportedVersions)
	}
	if compatibility.SelectedProfileCount != nil {
		t.Fatalf("selected profile count = %d, want null without a repository", *compatibility.SelectedProfileCount)
	}
}

func TestRuntimeCompatibilityPreviewHandlerOmitsForeignProvider(t *testing.T) {
	router, _, _ := newAgentUpdateRouter(t, &handlerRuntimeUpdater{})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, updateRequest(
		http.MethodGet,
		"/api/v1/agent-update/claude-acp/preview",
	))
	if response.Code != http.StatusOK {
		t.Fatalf("GET preview status = %d, body = %s", response.Code, response.Body.String())
	}
	var preview dto.AgentUpdatePreviewDTO
	if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if preview.CopilotPluginCompatibility != nil {
		t.Fatalf("foreign provider compatibility = %#v, want omitted", preview.CopilotPluginCompatibility)
	}
}
