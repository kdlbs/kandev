package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
	if compatibility.RuntimeVersion != "1.1.0" || compatibility.State != "not_checked" {
		t.Fatalf("compatibility = %#v", compatibility)
	}
	if compatibility.Reason != "" {
		t.Fatalf("reason = %q, want none without a concrete check", compatibility.Reason)
	}
	if strings.Contains(response.Body.String(), "supported_versions") {
		t.Fatalf("preview projected a runtime qualification list: %s", response.Body.String())
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
