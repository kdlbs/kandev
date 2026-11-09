package agents

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/config"
	"github.com/kandev/kandev/internal/office/models"
	"gopkg.in/yaml.v3"
)

// @covers AC-OFFICE-AGENT-NAMES-001.1, AC-OFFICE-AGENT-NAMES-001.2,
// AC-OFFICE-AGENT-NAMES-001.3, AC-OFFICE-AGENT-NAMES-001.4,
// AC-OFFICE-AGENT-NAMES-001.5, AC-OFFICE-AGENT-NAMES-001.6
func TestAgentNameUniqueness_HTTPExport(t *testing.T) {
	t.Run("reject_occupied_name", testNameHTTPRejectAndExport)
	tests := []struct {
		name           string
		body           map[string]string
		otherWorkspace string
		wantName       string
	}{
		{name: "unchanged_name", body: map[string]string{"name": nameBeta}, wantName: nameBeta},
		{name: "omitted_name", body: map[string]string{}, wantName: nameBeta},
		{name: "unused_name", body: map[string]string{"name": "Gamma"}, wantName: "Gamma"},
		{name: "cross_workspace_name", body: map[string]string{"name": nameAlpha},
			otherWorkspace: nameWorkspaceB, wantName: nameAlpha},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := newTestAgentService(t)
			workspace := tt.otherWorkspace
			if workspace == "" {
				workspace = nameWorkspaceA
			}
			other := createNameTestAgent(t, svc, workspace, nameAlpha)
			target := createNameTestAgent(t, svc, nameWorkspaceA, nameBeta)
			tt.body["icon"] = nameSavedIcon
			rec := nameHTTP(t, svc, http.MethodPatch, "/agents/"+target.ID, tt.body)
			assertNameHTTPResponse(t, rec, http.StatusOK, "")
			var response AgentResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode saved agent: %v", err)
			}
			if response.Agent.ID != target.ID || response.Agent.Name != tt.wantName {
				t.Errorf("response identity = %+v, want %q/%q", response.Agent, target.ID, tt.wantName)
			}
			assertNameAgentSaved(t, svc, target.ID, tt.wantName)
			assertNameAgentUnchanged(t, svc, other)
			manifest := nameExport(t, svc, nameWorkspaceA)
			assertNameExportAgent(t, manifest, tt.wantName, nameSavedIcon)
			if workspace == nameWorkspaceB {
				assertNameExportAgent(t, nameExport(t, svc, workspace), nameAlpha, other.Icon)
			}
		})
	}
}

func testNameHTTPRejectAndExport(t *testing.T) {
	svc, repo := newTestAgentService(t)
	alpha := createNameTestAgent(t, svc, nameWorkspaceA, nameAlpha)
	beta := createNameTestAgent(t, svc, nameWorkspaceA, nameBeta)
	before := nameExport(t, svc, nameWorkspaceA)
	assertNameExportAgent(t, before, nameAlpha, alpha.Icon)
	assertNameExportAgent(t, before, nameBeta, beta.Icon)
	rec := nameHTTP(t, svc, http.MethodPatch, "/agents/"+beta.ID, map[string]string{
		"name": nameAlpha, "icon": nameSavedIcon, "reports_to": alpha.ID,
	})
	assertNameHTTPResponse(t, rec, http.StatusBadRequest, "agent_name_already_exists")
	assertNameAgentUnchanged(t, svc, alpha)
	assertNameAgentUnchanged(t, svc, beta)
	rows, err := repo.ListAgentInstances(context.Background(), nameWorkspaceA)
	if err != nil {
		t.Fatalf("list agents after rejected rename: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("agent count = %d, want 2", len(rows))
	}
	exporter := config.NewConfigService(repo, nil, nil, logger.Default(), &testActivityLogger{})
	after, err := exporter.ExportManifest(context.Background(), nameWorkspaceA)
	if err != nil {
		t.Errorf("export after rejected rename: %v", err)
		return
	}
	if !reflect.DeepEqual(before, after) {
		t.Errorf("manifest changed after rejected rename: before=%+v after=%+v", before, after)
	}
	assertNameExportAgent(t, after, nameAlpha, alpha.Icon)
	assertNameExportAgent(t, after, nameBeta, beta.Icon)
}

// @covers AC-OFFICE-AGENT-NAMES-001.1, AC-OFFICE-AGENT-NAMES-001.2
func TestAgentNameUniqueness_HTTPCreate(t *testing.T) {
	svc, repo := newTestAgentService(t)
	original := createNameTestAgent(t, svc, nameWorkspaceA, nameAlpha)
	rec := nameHTTP(t, svc, http.MethodPost, "/workspaces/"+nameWorkspaceA+"/agents", map[string]string{
		"name": nameAlpha, "role": string(models.AgentRoleWorker),
	})
	assertNameHTTPResponse(t, rec, http.StatusBadRequest, "agent_name_already_exists")
	assertNameAgentUnchanged(t, svc, original)
	rows, err := repo.ListAgentInstances(context.Background(), nameWorkspaceA)
	if err != nil || len(rows) != 1 {
		t.Errorf("agents after rejected POST = %d, err=%v; want 1", len(rows), err)
	}
}

// @covers AC-OFFICE-AGENT-NAMES-001.1, AC-OFFICE-AGENT-NAMES-001.2
func TestAgentNameUniqueness_HTTPCEOPrecedence(t *testing.T) {
	tests := []struct {
		name          string
		method        string
		requestedName string
		wantConflict  bool
	}{
		{name: "duplicate_ceo_post", method: http.MethodPost, requestedName: nameAlpha, wantConflict: true},
		{name: "duplicate_ceo_promotion_patch", method: http.MethodPatch, requestedName: nameAlpha, wantConflict: true},
		{name: "distinct_ceo_post", method: http.MethodPost, requestedName: "Gamma"},
		{name: "distinct_ceo_promotion_patch", method: http.MethodPatch, requestedName: "Gamma"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newTestAgentService(t)
			ceo := createAndGetAgent(t, svc, repo, &models.AgentInstance{
				WorkspaceID: nameWorkspaceA, Name: nameAlpha, Role: models.AgentRoleCEO,
			})
			route, wantCount := "/workspaces/"+nameWorkspaceA+"/agents", 1
			var worker *models.AgentInstance
			if tt.method == http.MethodPatch {
				worker = createNameTestAgent(t, svc, nameWorkspaceA, nameBeta)
				route, wantCount = "/agents/"+worker.ID, 2
			}
			before := nameExport(t, svc, nameWorkspaceA)
			rec := nameHTTP(t, svc, tt.method, route, map[string]string{
				"name": tt.requestedName, "role": string(models.AgentRoleCEO), "icon": nameSavedIcon,
			})
			if tt.wantConflict {
				assertNameHTTPResponse(t, rec, http.StatusBadRequest, "agent_name_already_exists")
			} else {
				assertNameHTTPCEOGuard(t, rec)
			}
			assertNameAgentUnchanged(t, svc, ceo)
			if worker != nil {
				assertNameAgentUnchanged(t, svc, worker)
			}
			rows, err := repo.ListAgentInstances(context.Background(), nameWorkspaceA)
			if err != nil || len(rows) != wantCount {
				t.Errorf("agents after CEO request = %d, err=%v; want %d", len(rows), err, wantCount)
			}
			if after := nameExport(t, svc, nameWorkspaceA); !reflect.DeepEqual(before, after) {
				t.Errorf("manifest changed after rejected CEO request: before=%+v after=%+v", before, after)
			}
		})
	}
}

func assertNameHTTPCEOGuard(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	var response struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode CEO guard response: %v", err)
	}
	if rec.Code < http.StatusBadRequest || response.Error != ErrAgentCEOAlreadyExists.Error() ||
		response.Code == "agent_name_already_exists" {
		t.Errorf("distinct-name CEO request bypassed CEO guard: status=%d response=%+v", rec.Code, response)
	}
}

func nameHTTP(t *testing.T, svc *AgentService, method, route string, body map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1/office"), svc, logger.Default())
	req := httptest.NewRequest(method, "/api/v1/office"+route, strings.NewReader(mustJSON(t, body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func assertNameHTTPResponse(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Errorf("HTTP status = %d, want %d; body=%s", rec.Code, status, rec.Body.String())
	}
	if code == "" {
		return
	}
	var response struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Errorf("decode validation response: %v", err)
		return
	}
	if response.Error == "" || response.Code != code {
		t.Errorf("validation response = %+v, want nonempty error and code %q", response, code)
	}
}

func nameExport(t *testing.T, svc *AgentService, workspace string) *config.ExportManifest {
	t.Helper()
	exporter := config.NewConfigService(svc.repo, nil, nil, logger.Default(), &testActivityLogger{})
	manifest, err := exporter.ExportManifest(context.Background(), workspace)
	if err != nil {
		t.Fatalf("export workspace %q: %v", workspace, err)
	}
	return manifest
}

func assertNameExportAgent(t *testing.T, manifest *config.ExportManifest, name, icon string) {
	t.Helper()
	count := 0
	for _, file := range manifest.Files {
		if file.Path != ".kandev/agents/"+name+".yml" {
			continue
		}
		count++
		var agent config.AgentConfig
		if err := yaml.Unmarshal([]byte(file.Content), &agent); err != nil {
			t.Fatalf("decode exported agent: %v", err)
		}
		if agent.Name != name || agent.Icon != icon || agent.Role != string(models.AgentRoleWorker) {
			t.Errorf("exported agent = %+v, want Worker %q with icon %q", agent, name, icon)
		}
	}
	if count != 1 {
		t.Errorf("export entries for %q = %d, want 1", name, count)
	}
}
