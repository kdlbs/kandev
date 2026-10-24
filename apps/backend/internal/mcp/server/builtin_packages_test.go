package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func packageTestProfile(ids ...mcpprofile.PackageID) mcpprofile.Context {
	ctx := mcpprofile.New(mcpprofile.SurfaceKanbanTask, []mcpprofile.Capability{mcpprofile.CapabilityUserQuestion}, nil)
	origins := map[mcpprofile.PackageID]string{mcpprofile.PackageCore: "required"}
	for _, id := range ids {
		if id != mcpprofile.PackageCore {
			origins[id] = "task"
		}
	}
	ctx.BuiltinPackages = &mcpprofile.BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task", Surface: ctx.Surface, Packages: ids, Origins: origins}
	return ctx
}

// @covers AC-AGENTS-BUILTIN-MCP-001.2
// @covers AC-AGENTS-BUILTIN-MCP-002.4
func TestBuiltinCoreDoesNotExposeOptionalTools(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	s := NewWithProfile(backend, "session", "task", 10005, log, "", false, packageTestProfile(mcpprofile.PackageCore, mcpprofile.PackageRichOutput))
	t.Cleanup(func() { require.NoError(t, s.Close(context.Background())) })
	names := getRegisteredToolNames(s)
	require.Contains(t, names, "step_complete_kandev")
	require.Contains(t, names, "ask_user_question_kandev")
	require.NotContains(t, names, "show_rich_output_kandev")
	require.NotContains(t, names, "show_walkthrough_kandev")
}

func builtinPackageRequest(t *testing.T, router *gin.Engine, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	initRequest := httptest.NewRequest("POST", path, strings.NewReader(`{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"package-test","version":"1"}}}`))
	initRequest.Header.Set("Content-Type", "application/json")
	initRequest.Header.Set("Accept", "application/json, text/event-stream")
	initialized := httptest.NewRecorder()
	router.ServeHTTP(initialized, initRequest)
	if initialized.Code != 200 {
		return initialized
	}
	request := httptest.NewRequest("POST", path, strings.NewReader(body))
	request.Header.Set("Mcp-Session-Id", initialized.Header().Get("Mcp-Session-Id"))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("MCP-Protocol-Version", "2025-03-26")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

// @covers AC-AGENTS-BUILTIN-MCP-002.4
func TestBuiltinPackageRouteIsolation(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	s := NewWithProfile(backend, "session", "task", 10005, log, "", false, packageTestProfile(mcpprofile.PackageCore, mcpprofile.PackageRichOutput))
	t.Cleanup(func() { require.NoError(t, s.Close(context.Background())) })
	router := gin.New()
	s.RegisterRoutes(router)
	response := builtinPackageRequest(t, router, "/mcp/packages/rich-output", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	require.Equal(t, 200, response.Code, response.Body.String())
	var reply struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &reply))
	require.Len(t, reply.Result.Tools, 1)
	require.Equal(t, "show_rich_output_kandev", reply.Result.Tools[0].Name)
	denied := builtinPackageRequest(t, router, "/mcp/packages/walkthroughs", `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	require.Equal(t, 404, denied.Code)
}

// @covers AC-AGENTS-BUILTIN-MCP-002.5
func TestBuiltinConfigurationKeepsTaskSurface(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	s := NewWithProfile(backend, "session", "task", 10005, log, "", false, packageTestProfile(mcpprofile.PackageCore, mcpprofile.PackageConfiguration))
	t.Cleanup(func() { require.NoError(t, s.Close(context.Background())) })
	view := s.builtinPackages[mcpprofile.PackageConfiguration]
	require.NotNil(t, view)
	require.Contains(t, view.registry.ListTools(), "create_workflow_kandev")
	require.Contains(t, view.registry.ListTools(), "create_automation_kandev")
	require.NotContains(t, view.registry.ListTools(), "list_workspaces_kandev")
	require.NotContains(t, view.registry.ListTools(), "transfer_task_kandev")
	require.NotContains(t, s.mcpServer.ListTools(), "create_workflow_kandev")
	require.NotContains(t, s.mcpServer.ListTools(), "transfer_task_kandev")
	require.Equal(t, mcpprofile.SurfaceKanbanTask, s.Profile().Surface)
}

// @covers AC-AGENTS-BUILTIN-MCP-003.2
func TestBuiltinPackageRebuildPreservesSelection(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	s := NewWithProfile(backend, "session", "task", 10005, log, "", false, packageTestProfile(mcpprofile.PackageCore))
	t.Cleanup(func() { require.NoError(t, s.Close(context.Background())) })
	s.SetMode(ModeTaskTitlePending)
	require.NotContains(t, getRegisteredToolNames(s), "show_rich_output_kandev")
	require.Contains(t, getRegisteredToolNames(s), "set_task_title_kandev")
	s.SetProfile(mcpprofile.New(mcpprofile.SurfaceKanbanTask, nil, []string{"github"}))
	require.NotContains(t, getRegisteredToolNames(s), "get_diagnostic_bundle_kandev")
	require.NotNil(t, s.Profile().BuiltinPackages)
	s.SetMode(ModeConfig)
	require.Equal(t, mcpprofile.SurfaceKanbanTask, s.Profile().Surface)
	require.NotContains(t, getRegisteredToolNames(s), "create_workflow_kandev")
}

// @covers AC-AGENTS-BUILTIN-MCP-002.1
// @covers AC-AGENTS-BUILTIN-MCP-002.2
func TestBuiltinDefaultUnionParity(t *testing.T) {
	for _, surface := range []mcpprofile.Surface{mcpprofile.SurfaceKanbanTask, mcpprofile.SurfaceConfiguration, mcpprofile.SurfaceOfficeTask} {
		t.Run(string(surface), func(t *testing.T) {
			log := newTestLogger(t)
			backend := NewChannelBackendClient(log)
			t.Cleanup(backend.Close)
			ctx := mcpprofile.New(surface, []mcpprofile.Capability{mcpprofile.CapabilityUserQuestion}, []string{"github", "gitlab"})
			baseline := NewWithProfile(backend, "session", "task", 10005, log, "", false, ctx)
			t.Cleanup(func() { require.NoError(t, baseline.Close(context.Background())) })
			ids := []mcpprofile.PackageID{mcpprofile.PackageCore}
			origins := map[mcpprofile.PackageID]string{mcpprofile.PackageCore: "required"}
			if surface == mcpprofile.SurfaceConfiguration {
				ids = append(ids, mcpprofile.PackageConfiguration)
				origins[mcpprofile.PackageConfiguration] = "required"
			} else {
				ids = append(ids, mcpprofile.PackageRichOutput)
				origins[mcpprofile.PackageRichOutput] = "workspace"
				if surface == mcpprofile.SurfaceKanbanTask {
					ids = append(ids, mcpprofile.PackageWalkthroughs, mcpprofile.PackageDiagnostics)
					origins[mcpprofile.PackageWalkthroughs] = "workspace"
					origins[mcpprofile.PackageDiagnostics] = "workspace"
				}
			}
			ctx.BuiltinPackages = &mcpprofile.BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task", Surface: surface, Packages: ids, Origins: origins}
			split := NewWithProfile(backend, "session", "task", 10005, log, "", false, ctx)
			t.Cleanup(func() { require.NoError(t, split.Close(context.Background())) })
			actual := split.mcpServer.ListTools()
			for _, view := range split.builtinPackages {
				for name, tool := range view.registry.ListTools() {
					require.NotContains(t, actual, name)
					actual[name] = tool
				}
			}
			require.Len(t, actual, len(baseline.mcpServer.ListTools()))
			for name, want := range baseline.mcpServer.ListTools() {
				require.Contains(t, actual, name)
				expectedJSON, err := json.Marshal(want.Tool)
				require.NoError(t, err)
				actualJSON, err := json.Marshal(actual[name].Tool)
				require.NoError(t, err)
				require.JSONEq(t, string(expectedJSON), string(actualJSON), name)
			}
		})
	}
}

// @covers AC-AGENTS-BUILTIN-MCP-002.4
func TestBuiltinPackageRejectsForeignTransportSession(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	s := NewWithProfile(backend, "session", "task", 10005, log, "", false, packageTestProfile(mcpprofile.PackageCore, mcpprofile.PackageRichOutput, mcpprofile.PackageDiagnostics))
	t.Cleanup(func() { require.NoError(t, s.Close(context.Background())) })
	router := gin.New()
	s.RegisterRoutes(router)
	initBody := `{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`
	req := httptest.NewRequest("POST", "/mcp/packages/rich-output", strings.NewReader(initBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	initialized := httptest.NewRecorder()
	router.ServeHTTP(initialized, req)
	require.Equal(t, 200, initialized.Code)
	for _, path := range []string{"/mcp/packages/rich-output", "/mcp/packages/diagnostics", "/mcp"} {
		request := httptest.NewRequest("POST", path, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json, text/event-stream")
		request.Header.Set("Mcp-Session-Id", initialized.Header().Get("Mcp-Session-Id"))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if path == "/mcp/packages/rich-output" {
			require.Equal(t, 200, response.Code)
		} else {
			require.Equal(t, 404, response.Code, path)
		}
	}
}

// @covers AC-AGENTS-BUILTIN-MCP-002.1
func TestBuiltinPackageSelectionMatrix(t *testing.T) {
	optional := []mcpprofile.PackageID{mcpprofile.PackageConfiguration, mcpprofile.PackageRichOutput, mcpprofile.PackageWalkthroughs, mcpprofile.PackageDiagnostics}
	examples := []string{"create_workflow_kandev", "show_rich_output_kandev", "show_walkthrough_kandev", "get_diagnostic_bundle_kandev"}
	for mask := 0; mask < 16; mask++ {
		t.Run(fmt.Sprint(mask), func(t *testing.T) {
			ids := []mcpprofile.PackageID{mcpprofile.PackageCore}
			for index, id := range optional {
				if mask&(1<<index) != 0 {
					ids = append(ids, id)
				}
			}
			log := newTestLogger(t)
			backend := NewChannelBackendClient(log)
			t.Cleanup(backend.Close)
			s := NewWithProfile(backend, "session", "task", 10005, log, "", false, packageTestProfile(ids...))
			t.Cleanup(func() { require.NoError(t, s.Close(context.Background())) })
			for index, id := range optional {
				require.NotContains(t, s.mcpServer.ListTools(), examples[index])
				if mask&(1<<index) != 0 {
					require.Contains(t, s.builtinPackages[id].registry.ListTools(), examples[index])
				} else {
					require.NotContains(t, s.builtinPackages, id)
				}
			}
			require.Contains(t, s.mcpServer.ListTools(), "step_complete_kandev")
			require.Contains(t, s.mcpServer.ListTools(), "ask_user_question_kandev")
		})
	}
}

// @covers AC-AGENTS-BUILTIN-MCP-003.4
func TestBuiltinMalformedProfileFailsClosed(t *testing.T) {
	ctx := packageTestProfile(mcpprofile.PackageCore)
	ctx.BuiltinPackages.Surface = mcpprofile.SurfaceOfficeTask
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	s := NewWithProfile(backend, "session", "task", 10005, log, "", false, ctx)
	t.Cleanup(func() { require.NoError(t, s.Close(context.Background())) })
	require.Empty(t, s.mcpServer.ListTools())
	require.Empty(t, s.builtinPackages)
}

// @covers AC-AGENTS-BUILTIN-MCP-004.1
func TestBuiltinCatalogDescribesRegistrationWithoutConnectionEvidence(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	s := NewWithProfile(backend, "session", "task", 10005, log, "", false, packageTestProfile(mcpprofile.PackageCore, mcpprofile.PackageRichOutput))
	t.Cleanup(func() { require.NoError(t, s.Close(context.Background())) })
	cataloguer, ok := any(s).(interface {
		BuiltinCatalog() map[mcpprofile.PackageID][]string
	})
	require.True(t, ok, "management needs expected package catalogs independently of connections")
	catalog := cataloguer.BuiltinCatalog()
	require.Equal(t, []string{"show_rich_output_kandev"}, catalog[mcpprofile.PackageRichOutput])
	require.NotContains(t, catalog, mcpprofile.PackageConfiguration)
	require.Contains(t, catalog[mcpprofile.PackageCore], "step_complete_kandev")
	catalog[mcpprofile.PackageRichOutput][0] = "changed"
	require.Equal(t, []string{"show_rich_output_kandev"}, cataloguer.BuiltinCatalog()[mcpprofile.PackageRichOutput])
}

// @covers AC-AGENTS-BUILTIN-MCP-001.2
func TestBuiltinPackageTransportNamespacing(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	s := NewWithProfile(backend, "session", "task", 10005, log, "", false, packageTestProfile(mcpprofile.PackageCore, mcpprofile.PackageRichOutput), WithMCPToolNamespacingByServer(true))
	t.Cleanup(func() { require.NoError(t, s.Close(context.Background())) })
	router := gin.New()
	s.RegisterRoutes(router)
	response := builtinPackageRequest(t, router, "/mcp/packages/rich-output", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	require.Equal(t, 200, response.Code, response.Body.String())
	var reply struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &reply))
	require.Len(t, reply.Result.Tools, 1)
	require.Equal(t, "show_rich_output", reply.Result.Tools[0].Name)
}

// @covers AC-AGENTS-BUILTIN-MCP-003.5
func TestBuiltinManifestRejectsDifferentTask(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	s := NewWithProfile(backend, "session", "different-task", 10005, log, "", false, packageTestProfile(mcpprofile.PackageCore, mcpprofile.PackageConfiguration))
	t.Cleanup(func() { require.NoError(t, s.Close(context.Background())) })
	require.Empty(t, s.mcpServer.ListTools())
	require.Empty(t, s.builtinPackages)
}

// @covers AC-AGENTS-BUILTIN-MCP-002.5
func TestBuiltinConfigurationMembership(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	ctx := mcpprofile.Legacy(ModeConfig, false, nil)
	ctx.BuiltinPackages = &mcpprofile.BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task", Surface: ctx.Surface, Packages: []mcpprofile.PackageID{mcpprofile.PackageCore, mcpprofile.PackageConfiguration}, Origins: map[mcpprofile.PackageID]string{mcpprofile.PackageCore: "required", mcpprofile.PackageConfiguration: "required"}}
	s := NewWithProfile(backend, "session", "task", 10005, log, "", false, ctx)
	t.Cleanup(func() { require.NoError(t, s.Close(context.Background())) })
	require.ElementsMatch(t, []string{
		"list_workspaces_kandev", "list_workflows_kandev", "list_workflow_steps_kandev", "list_repositories_kandev", "list_agents_kandev", "list_executor_profiles_kandev",
		"list_tasks_kandev", "move_task_kandev", "delete_task_kandev", "archive_task_kandev", "update_task_state_kandev", "get_task_conversation_kandev",
		"transfer_task_kandev", "ask_user_question_kandev", "list_task_sessions_kandev",
	}, s.BuiltinCatalog()[mcpprofile.PackageCore], "new configuration definitions must have reviewed package ownership")
	require.Len(t, s.BuiltinCatalog()[mcpprofile.PackageConfiguration], 30)
}

// @covers AC-AGENTS-BUILTIN-MCP-002.4
func TestBuiltinDirectCallCannotReachAnotherPackage(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{"id": "created"}}
	s := NewWithProfile(backend, "session", "task", 10005, newTestLogger(t), "", false, packageTestProfile(mcpprofile.PackageCore, mcpprofile.PackageConfiguration))
	t.Cleanup(func() { require.NoError(t, s.Close(context.Background())) })
	router := gin.New()
	s.RegisterRoutes(router)
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_workflow_kandev","arguments":{"workspace_id":"workspace","name":"example"}}}`
	positive := builtinPackageRequest(t, router, "/mcp/packages/configuration", body)
	require.Equal(t, 200, positive.Code, positive.Body.String())
	require.Equal(t, "mcp.create_workflow", backend.lastAction)
	backend.lastAction = ""
	denied := builtinPackageRequest(t, router, "/mcp", body)
	require.Equal(t, 200, denied.Code, denied.Body.String())
	require.Contains(t, denied.Body.String(), "error")
	require.Empty(t, backend.lastAction)
}
