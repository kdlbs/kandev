package handlers

import (
	"context"
	"encoding/json"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	p "github.com/kandev/kandev/internal/mcp/profile"
	scope "github.com/kandev/kandev/internal/mcp/scope"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
	"testing"
)

// @covers AC-OFFICE-AUTOMATION-MCP-002.2, AC-OFFICE-AUTOMATION-MCP-002.5
func TestAutomationMCPRawDispatch(t *testing.T) {
	svc, _ := newTestTaskService(t)
	workspaces, err := svc.ListWorkspaces(t.Context())
	require.NoError(t, err)
	workspace := workspaces[0].ID
	manifest := &p.BuiltinManifest{Version: 2, CatalogVersion: 1, WorkspaceID: workspace, TaskID: "task", Surface: p.SurfaceAutomation, AutomationID: "automation", AutomationRevision: 1, Packages: []p.PackageID{p.PackageCore, p.PackageConfiguration}, Origins: map[p.PackageID]string{p.PackageCore: "required", p.PackageConfiguration: "automation"}}
	require.NoError(t, manifest.Validate())
	ctx := scope.WithPrincipal(t.Context(), scope.Principal{AutomationID: "automation", WorkspaceID: workspace, CallerTaskID: "task", CallerSessionID: "session", Surface: p.SurfaceAutomation})
	ctx = streams.WithMCPExecutionContext(ctx, streams.MCPExecutionContext{TaskID: "task", SessionID: "session", BuiltinAuthority: streams.BuiltinMCPAuthority{Required: true, Manifest: manifest, Actions: []string{ws.ActionMCPListSharedPrompts}}})
	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	d := &guardedMCPDispatcher{Dispatcher: ws.NewDispatcher(), handlers: h}
	for _, action := range []string{ws.ActionMCPListSharedPrompts, ws.ActionMCPGetDiagnosticBundle} {
		d.RegisterFunc(action, func(_ context.Context, msg *ws.Message) (*ws.Message, error) {
			return ws.NewResponse(msg.ID, msg.Action, nil)
		})
	}
	denied, err := d.Dispatch(ctx, makeWSMessage(t, ws.ActionMCPListSharedPrompts, nil))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, denied.Type)
	denied, err = d.Dispatch(ctx, makeWSMessage(t, ws.ActionMCPGetDiagnosticBundle, nil))
	require.NoError(t, err)
	assertWSError(t, denied, ws.ErrorCodeUnknownAction)
	denied, err = d.Dispatch(ctx, makeWSMessage(t, ws.ActionMCPListSharedPrompts, map[string]any{"workspace_id": "foreign"}))
	require.NoError(t, err)
	assertWSError(t, denied, ws.ErrorCodeNotFound)
}

func TestAutomationMCPRejectsNoncanonicalResourceKeys(t *testing.T) {
	var decoded struct {
		TaskID    string `json:"task_id"`
		SessionID string `json:"session_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(`{"taſk_id":"foreign", "ſession_id":"foreign-session"}`), &decoded))
	require.Equal(t, "foreign", decoded.TaskID)
	require.Equal(t, "foreign-session", decoded.SessionID)

	svc, _ := newTestTaskService(t)
	workspaces, err := svc.ListWorkspaces(t.Context())
	require.NoError(t, err)
	workspace := workspaces[0].ID
	manifest := &p.BuiltinManifest{Version: 2, CatalogVersion: 1, WorkspaceID: workspace, TaskID: "task", Surface: p.SurfaceAutomation, AutomationID: "automation", AutomationRevision: 1, Packages: []p.PackageID{p.PackageCore, p.PackageDiagnostics}, Origins: map[p.PackageID]string{p.PackageCore: "required", p.PackageDiagnostics: "automation"}}
	ctx := scope.WithPrincipal(t.Context(), scope.Principal{AutomationID: "automation", WorkspaceID: workspace, CallerTaskID: "task", CallerSessionID: "session", Surface: p.SurfaceAutomation})
	ctx = streams.WithMCPExecutionContext(ctx, streams.MCPExecutionContext{TaskID: "task", SessionID: "session", BuiltinAuthority: streams.BuiltinMCPAuthority{Required: true, Manifest: manifest, Actions: []string{ws.ActionMCPGetDiagnosticBundle}}})
	h := &Handlers{taskSvc: svc, logger: testLogger(t)}
	for _, fields := range []map[string]any{{}, {"TASK_ID": "foreign"}, {"SESSION_ID": "foreign"}, {"task_id": "task", "TASK_ID": "foreign"}, {"WORKSPACE_ID": "foreign"}, {"taſk_id": "foreign"}, {"ſession_id": "foreign"}, {"workſpace_id": "foreign"}, {"taſk_id": "foreign", "ſession_id": "foreign-session"}} {
		denied, _, err := h.authorizeAutomationRequest(ctx, makeWSMessage(t, ws.ActionMCPGetDiagnosticBundle, fields))
		require.NoError(t, err)
		if len(fields) == 0 {
			require.Nil(t, denied)
		} else {
			assertWSError(t, denied, ws.ErrorCodeBadRequest)
		}
	}
}

func TestAutomationMCPConfigurationProfileOwnership(t *testing.T) {
	h, settings, _ := newAgentSettingsHandlers(t)
	svc, _ := newTestTaskService(t)
	h.taskSvc = svc
	workspaces, err := svc.ListWorkspaces(t.Context())
	require.NoError(t, err)
	workspace := workspaces[0].ID
	agent := &settingsmodels.Agent{Name: "codex-acp"}
	require.NoError(t, settings.CreateAgent(t.Context(), agent))
	for _, owner := range []string{workspace, "foreign", ""} {
		profile := &settingsmodels.AgentProfile{AgentID: agent.ID, Name: "Profile", WorkspaceID: owner}
		require.NoError(t, settings.CreateAgentProfile(t.Context(), profile))
		manifest := &p.BuiltinManifest{Version: 2, CatalogVersion: 1, WorkspaceID: workspace, TaskID: "task", Surface: p.SurfaceAutomation, AutomationID: "automation", AutomationRevision: 1, Packages: []p.PackageID{p.PackageCore, p.PackageConfiguration}, Origins: map[p.PackageID]string{p.PackageCore: "required", p.PackageConfiguration: "automation"}}
		ctx := scope.WithPrincipal(t.Context(), scope.Principal{AutomationID: "automation", WorkspaceID: workspace, CallerTaskID: "task", CallerSessionID: "session", Surface: p.SurfaceAutomation})
		ctx = streams.WithMCPExecutionContext(ctx, streams.MCPExecutionContext{TaskID: "task", SessionID: "session", BuiltinAuthority: streams.BuiltinMCPAuthority{Required: true, Manifest: manifest, Actions: []string{ws.ActionMCPGetMcpConfig, ws.ActionMCPUpdateMcpConfig}}})
		for _, action := range []string{ws.ActionMCPGetMcpConfig, ws.ActionMCPUpdateMcpConfig} {
			denied, _, err := h.authorizeAutomationRequest(ctx, makeWSMessage(t, action, map[string]any{"profile_id": profile.ID}))
			require.NoError(t, err)
			if owner == "foreign" || action == ws.ActionMCPUpdateMcpConfig {
				require.NotNil(t, denied)
			} else {
				require.Nil(t, denied)
			}
		}
	}
}

func TestAutomationMCPWorkflowStepOwnership(t *testing.T) {
	svc, repo := newTestTaskService(t)
	workspaces, err := svc.ListWorkspaces(t.Context())
	require.NoError(t, err)
	workspace := workspaces[0].ID
	foreign := &taskmodels.Workspace{ID: "foreign", Name: "Foreign"}
	require.NoError(t, repo.CreateWorkspace(t.Context(), foreign))
	require.NoError(t, repo.CreateWorkflow(t.Context(), &taskmodels.Workflow{ID: "own-flow", WorkspaceID: workspace, Name: "Own"}))
	require.NoError(t, repo.CreateWorkflow(t.Context(), &taskmodels.Workflow{ID: "other-flow", WorkspaceID: foreign.ID, Name: "Other"}))
	svc.SetWorkflowStepGetter(&staticWorkflowStepGetter{steps: map[string]*workflowmodels.WorkflowStep{"own-step": {ID: "own-step", WorkflowID: "own-flow"}, "other-step": {ID: "other-step", WorkflowID: "other-flow"}}})
	manifest := &p.BuiltinManifest{Version: 2, CatalogVersion: 1, WorkspaceID: workspace, TaskID: "task", Surface: p.SurfaceAutomation, AutomationID: "automation", AutomationRevision: 1, Packages: []p.PackageID{p.PackageCore, p.PackageConfiguration}, Origins: map[p.PackageID]string{p.PackageCore: "required", p.PackageConfiguration: "automation"}}
	ctx := scope.WithPrincipal(t.Context(), scope.Principal{AutomationID: "automation", WorkspaceID: workspace, CallerTaskID: "task", CallerSessionID: "session", Surface: p.SurfaceAutomation})
	ctx = streams.WithMCPExecutionContext(ctx, streams.MCPExecutionContext{TaskID: "task", SessionID: "session", BuiltinAuthority: streams.BuiltinMCPAuthority{Required: true, Manifest: manifest, Actions: []string{ws.ActionMCPUpdateWorkflowStep}}})
	h := &Handlers{taskSvc: svc, logger: testLogger(t)}
	d := &guardedMCPDispatcher{Dispatcher: ws.NewDispatcher(), handlers: h}
	d.RegisterFunc(ws.ActionMCPUpdateWorkflowStep, func(_ context.Context, msg *ws.Message) (*ws.Message, error) {
		return ws.NewResponse(msg.ID, msg.Action, nil)
	})
	for _, id := range []string{"own-step", "other-step"} {
		response, err := d.Dispatch(ctx, makeWSMessage(t, ws.ActionMCPUpdateWorkflowStep, map[string]any{"step_id": id}))
		require.NoError(t, err)
		if id == "own-step" {
			require.Equal(t, ws.MessageTypeResponse, response.Type)
		} else {
			assertWSError(t, response, ws.ErrorCodeNotFound)
		}
	}
}

func TestAutomationMCPLocalActionsCannotReachOtherTasks(t *testing.T) {
	svc, repo := newTestTaskService(t)
	workspaces, err := svc.ListWorkspaces(t.Context())
	require.NoError(t, err)
	workspace := workspaces[0].ID
	for _, id := range []string{"self", "other"} {
		require.NoError(t, repo.CreateTask(t.Context(), &taskmodels.Task{ID: id, WorkspaceID: workspace, Title: id}))
		require.NoError(t, repo.CreateTaskSession(t.Context(), &taskmodels.TaskSession{ID: id + "-session", TaskID: id, State: taskmodels.TaskSessionStateCreated}))
	}
	actions := []string{ws.ActionMCPShowWalkthrough, ws.ActionMCPGetWalkthrough, ws.ActionMCPDeleteWalkthrough, ws.ActionMCPGetDiagnosticBundle}
	manifest := &p.BuiltinManifest{Version: 2, CatalogVersion: 1, WorkspaceID: workspace, TaskID: "self", Surface: p.SurfaceAutomation, AutomationID: "automation", AutomationRevision: 1, Packages: []p.PackageID{p.PackageCore, p.PackageWalkthroughs, p.PackageDiagnostics}, Origins: map[p.PackageID]string{p.PackageCore: "required", p.PackageWalkthroughs: "automation", p.PackageDiagnostics: "automation"}}
	ctx := scope.WithPrincipal(t.Context(), scope.Principal{AutomationID: "automation", WorkspaceID: workspace, CallerTaskID: "self", CallerSessionID: "self-session", Surface: p.SurfaceAutomation})
	ctx = streams.WithMCPExecutionContext(ctx, streams.MCPExecutionContext{TaskID: "self", SessionID: "self-session", BuiltinAuthority: streams.BuiltinMCPAuthority{Required: true, Manifest: manifest, Actions: actions}})
	h := &Handlers{taskSvc: svc, logger: testLogger(t)}
	d := &guardedMCPDispatcher{Dispatcher: ws.NewDispatcher(), handlers: h}
	for _, action := range actions {
		d.RegisterFunc(action, func(_ context.Context, msg *ws.Message) (*ws.Message, error) {
			return ws.NewResponse(msg.ID, msg.Action, nil)
		})
	}
	for _, action := range actions {
		for _, target := range []string{"", "self", "other"} {
			for _, key := range []string{"task_id", "session_id"} {
				fields := map[string]any{}
				if target != "" {
					value := target
					if key == "session_id" {
						value += "-session"
					}
					fields[key] = value
				}
				response, err := d.Dispatch(ctx, makeWSMessage(t, action, fields))
				require.NoError(t, err)
				if target == "other" {
					assertWSError(t, response, ws.ErrorCodeNotFound)
				} else {
					require.Equal(t, ws.MessageTypeResponse, response.Type)
				}
			}
		}
	}
}

func TestAutomationMCPExecutorProfileIDsAreNotAgentProfileIDs(t *testing.T) {
	svc, _ := newTestTaskService(t)
	workspaces, err := svc.ListWorkspaces(t.Context())
	require.NoError(t, err)
	workspace := workspaces[0].ID
	manifest := &p.BuiltinManifest{Version: 2, CatalogVersion: 1, WorkspaceID: workspace, TaskID: "task", Surface: p.SurfaceAutomation, AutomationID: "automation", AutomationRevision: 1, Packages: []p.PackageID{p.PackageCore, p.PackageConfiguration}, Origins: map[p.PackageID]string{p.PackageCore: "required", p.PackageConfiguration: "automation"}}
	actions := []string{ws.ActionMCPUpdateExecutorProfile, ws.ActionMCPDeleteExecutorProfile}
	ctx := scope.WithPrincipal(t.Context(), scope.Principal{AutomationID: "automation", WorkspaceID: workspace, CallerTaskID: "task", CallerSessionID: "session", Surface: p.SurfaceAutomation})
	ctx = streams.WithMCPExecutionContext(ctx, streams.MCPExecutionContext{TaskID: "task", SessionID: "session", BuiltinAuthority: streams.BuiltinMCPAuthority{Required: true, Manifest: manifest, Actions: actions}})
	h := &Handlers{taskSvc: svc, logger: testLogger(t)}
	for _, action := range actions {
		t.Run(action, func(t *testing.T) {
			denied, _, err := h.authorizeAutomationRequest(ctx, makeWSMessage(t, action, map[string]any{"profile_id": "executor-profile"}))
			require.NoError(t, err)
			require.Nil(t, denied)
			denied, _, err = h.authorizeAutomationRequest(ctx, makeWSMessage(t, action, map[string]any{"profile_id": "executor-profile", "agent_profile_id": "foreign-agent-profile"}))
			require.NoError(t, err)
			assertWSError(t, denied, ws.ErrorCodeNotFound)
		})
	}
}

func TestAutomationMCPPayloadFieldsAllowUnicodeContent(t *testing.T) {
	fields, err := automationPayloadFields(json.RawMessage(`{"title":"任務 ſ K", "metadata":{"セッション":"内容"}}`))
	require.NoError(t, err)
	require.JSONEq(t, `"任務 ſ K"`, string(fields["title"]))
	require.JSONEq(t, `{"セッション":"内容"}`, string(fields["metadata"]))
}
