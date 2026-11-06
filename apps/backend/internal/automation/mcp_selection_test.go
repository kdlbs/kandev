package automation

import (
	"context"
	"encoding/json"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	ws "github.com/kandev/kandev/pkg/websocket"
	"testing"

	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

// @covers AC-OFFICE-AUTOMATION-MCP-001.1, AC-OFFICE-AUTOMATION-MCP-001.4
func TestAutomationMCPSelectionSave(t *testing.T) {
	svc := newTestService(t)
	ctx := WithNativePluginToolIntent(context.Background())
	var create CreateAutomationRequest
	require.NoError(t, json.Unmarshal([]byte(`{"workspace_id":"ws-1","name":"Report","mcp_selection":{"builtin_packages":["core","configuration","rich-output","walkthroughs","diagnostics"],"definition_ids":[]}}`), &create))
	a, err := svc.CreateAutomation(ctx, &create)
	require.NoError(t, err)
	raw, err := json.Marshal(a)
	require.NoError(t, err)
	var saved map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &saved))
	require.Contains(t, saved, "mcp_selection")
	require.JSONEq(t, `{"builtin_packages":["core","configuration","rich-output","walkthroughs","diagnostics"],"definition_ids":[]}`, string(saved["mcp_selection"]))
	require.JSONEq(t, `1`, string(saved["mcp_selection_revision"]))
	_, err = svc.UpdateAutomation(ctx, a.ID, &UpdateAutomationRequest{})
	require.NoError(t, err)
	loaded, err := svc.GetAutomation(ctx, a.ID)
	require.NoError(t, err)
	raw, err = json.Marshal(loaded)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &saved))
	require.JSONEq(t, `1`, string(saved["mcp_selection_revision"]))
	var update UpdateAutomationRequest
	require.NoError(t, json.Unmarshal([]byte(`{"mcp_selection":{"builtin_packages":["core"],"definition_ids":[]},"expected_mcp_selection_revision":0}`), &update))
	_, err = svc.UpdateAutomation(ctx, a.ID, &update)
	require.ErrorIs(t, err, ErrMCPSelectionConflict)
	require.NoError(t, json.Unmarshal([]byte(`{"mcp_selection":{"builtin_packages":["core"],"definition_ids":[]},"expected_mcp_selection_revision":1}`), &update))
	_, err = svc.UpdateAutomation(ctx, a.ID, &update)
	require.NoError(t, err)
}

// @covers AC-OFFICE-AUTOMATION-MCP-002.3
func TestAutomationMCPSelectionNativeOnly(t *testing.T) {
	svc := newTestService(t)
	var req CreateAutomationRequest
	require.NoError(t, json.Unmarshal([]byte(`{"workspace_id":"ws-1","name":"Unsafe","mcp_selection":{"builtin_packages":["core","configuration"],"definition_ids":[]}}`), &req))
	_, err := svc.CreateAutomation(context.Background(), &req)
	require.Error(t, err, "MCP and plugin callers cannot grant selection")
}

// @covers AC-OFFICE-AUTOMATION-MCP-001.3, AC-OFFICE-AUTOMATION-MCP-001.6
func TestAutomationMCPSelectionDefaultsAndExport(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	a, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{WorkspaceID: "ws-1", Name: "Legacy"})
	require.NoError(t, err)
	require.Nil(t, a.MCPSelection)
	require.Zero(t, a.MCPSelectionRevision)
	listed, err := svc.ListAutomations(ctx, "ws-1")
	require.NoError(t, err)
	require.Nil(t, listed[0].MCPSelection)
}

func TestAutomationMCPSelectionInvalid(t *testing.T) {
	for _, selection := range []string{
		`{"builtin_packages":[],"definition_ids":[]}`,
		`{"builtin_packages":["core","core"],"definition_ids":[]}`,
		`{"builtin_packages":["core","unknown"],"definition_ids":[]}`,
		`{"builtin_packages":["core"],"definition_ids":["foreign"]}`,
		`{"builtin_packages":["core"],"definition_ids":null}`,
	} {
		t.Run(selection, func(t *testing.T) {
			svc := newTestService(t)
			var req CreateAutomationRequest
			require.NoError(t, json.Unmarshal([]byte(`{"workspace_id":"ws-1","name":"Invalid","mcp_selection":`+selection+`}`), &req))
			_, err := svc.CreateAutomation(WithNativePluginToolIntent(context.Background()), &req)
			require.Error(t, err)
		})
	}
}

func TestAutomationMCPSelectionDurableOwner(t *testing.T) {
	svc := newTestService(t)
	ctx := WithNativePluginToolIntent(t.Context())
	a, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{WorkspaceID: "workspace", Name: "Report", MCPSelection: &MCPSelection{BuiltinPackages: []p.PackageID{p.PackageCore, p.PackageConfiguration}, DefinitionIDs: []string{}}})
	require.NoError(t, err)
	_, err = svc.GetAutomationMCPBinding(ctx, "workspace", "task")
	require.Error(t, err, "metadata without a durable reference grants no authority")
	run := &AutomationRun{AutomationID: a.ID, TriggerType: TriggerTypeManual, Status: RunStatusTriggered}
	require.NoError(t, svc.store.CreateRun(ctx, run))
	require.NoError(t, svc.store.BindRunTask(ctx, run.ID, "task", ""))
	binding, err := svc.GetAutomationMCPBinding(ctx, "workspace", "task")
	require.NoError(t, err)
	require.Equal(t, a.ID, binding.AutomationID)
	require.Contains(t, binding.Packages, p.PackageConfiguration)
	_, err = svc.GetAutomationMCPBinding(ctx, "foreign", "task")
	require.Error(t, err)
}

// @covers AC-OFFICE-AUTOMATION-MCP-002.4
func TestAutomationMCPConfigurationGrantDenial(t *testing.T) {
	svc := newTestService(t)
	selection := &MCPSelection{BuiltinPackages: []p.PackageID{p.PackageCore, p.PackageConfiguration}, DefinitionIDs: []string{}}
	native := WithNativePluginToolIntent(t.Context())
	existing, err := svc.CreateAutomation(native, &CreateAutomationRequest{WorkspaceID: "workspace", Name: "Native grant", MCPSelection: selection})
	require.NoError(t, err)
	agent := streams.WithMCPExecutionContext(native, streams.MCPExecutionContext{TaskID: "task", SessionID: "session"})
	revision := existing.MCPSelectionRevision
	_, err = svc.UpdateAutomation(agent, existing.ID, &UpdateAutomationRequest{MCPSelection: selection, ExpectedMCPSelectionRevision: &revision})
	require.Error(t, err)
	_, err = svc.CreateAutomation(agent, &CreateAutomationRequest{WorkspaceID: "workspace", Name: "Agent grant", MCPSelection: selection})
	require.Error(t, err)
	dispatcher := ws.NewDispatcher()
	registerWSHandlers(dispatcher, svc, svc.logger)
	request, err := ws.NewRequest("grant", ws.ActionAutomationUpdate, map[string]any{"id": existing.ID, "mcp_selection": selection, "expected_mcp_selection_revision": revision})
	require.NoError(t, err)
	response, err := dispatcher.Dispatch(agent, request)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
	saved, err := svc.GetAutomation(native, existing.ID)
	require.NoError(t, err)
	require.Equal(t, revision, saved.MCPSelectionRevision)
	items, err := svc.ListAutomations(native, "workspace")
	require.NoError(t, err)
	require.Len(t, items, 1)
	_, err = svc.UpdateAutomation(native, existing.ID, &UpdateAutomationRequest{MCPSelection: selection, ExpectedMCPSelectionRevision: &revision})
	require.NoError(t, err)
}

func TestAutomationMCPRejectsForeignDefinition(t *testing.T) {
	svc := newTestService(t)
	svc.SetMCPSelectionReaders(automationMCPOptionsCatalog{definitions: []*mcpconfig.MCPServerDefinition{{ID: "foreign", WorkspaceID: "other", Enabled: true}}}, nil)
	err := svc.validateMCPSelection(WithNativePluginToolIntent(t.Context()), "workspace", TaskModeAutomationRun, &MCPSelection{BuiltinPackages: []p.PackageID{p.PackageCore}, DefinitionIDs: []string{"foreign"}})
	require.EqualError(t, err, "MCP definition unavailable in this workspace")
}

func TestAutomationMCPModeClearRequiresEditorRevision(t *testing.T) {
	svc := newTestService(t)
	ctx := WithNativePluginToolIntent(t.Context())
	a, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{WorkspaceID: "workspace", Name: "Report", MCPSelection: &MCPSelection{BuiltinPackages: []p.PackageID{p.PackageCore}, DefinitionIDs: []string{}}})
	require.NoError(t, err)
	mode := TaskModeNormalTask
	workflow := "workflow"
	req := &UpdateAutomationRequest{TaskMode: &mode, WorkflowID: &workflow}
	require.ErrorIs(t, svc.store.UpdateAutomation(ctx, a.ID, req), ErrMCPSelectionConflict)
	revision := a.MCPSelectionRevision
	req.ExpectedMCPSelectionRevision = &revision
	require.NoError(t, svc.store.UpdateAutomation(ctx, a.ID, req))
	current, err := svc.store.GetAutomation(ctx, a.ID)
	require.NoError(t, err)
	require.Nil(t, current.MCPSelection)
}

type changingModeMCPCatalog struct {
	automationMCPOptionsCatalog
	change func()
}

func (c changingModeMCPCatalog) GetMCPServerDefinition(ctx context.Context, w, id string) (*mcpconfig.MCPServerDefinition, error) {
	c.change()
	return c.automationMCPOptionsCatalog.GetMCPServerDefinition(ctx, w, id)
}

func TestAutomationMCPValidationCannotCrossTaskModes(t *testing.T) {
	svc := newTestService(t)
	ctx := WithNativePluginToolIntent(t.Context())
	a, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{WorkspaceID: "workspace", Name: "Report"})
	require.NoError(t, err)
	svc.SetMCPSelectionReaders(changingModeMCPCatalog{automationMCPOptionsCatalog: automationMCPOptionsCatalog{definitions: []*mcpconfig.MCPServerDefinition{{ID: "server", WorkspaceID: "workspace", Enabled: true}}}, change: func() {
		mode := TaskModeNormalTask
		workflow := "workflow"
		require.NoError(t, svc.store.UpdateAutomation(ctx, a.ID, &UpdateAutomationRequest{TaskMode: &mode, WorkflowID: &workflow}))
	}}, nil)
	revision := a.MCPSelectionRevision
	_, err = svc.UpdateAutomation(ctx, a.ID, &UpdateAutomationRequest{MCPSelection: &MCPSelection{BuiltinPackages: []p.PackageID{p.PackageCore, p.PackageConfiguration}, DefinitionIDs: []string{"server"}}, ExpectedMCPSelectionRevision: &revision})
	require.ErrorIs(t, err, ErrMCPSelectionConflict)
	saved, err := svc.GetAutomation(ctx, a.ID)
	require.NoError(t, err)
	require.Equal(t, TaskModeNormalTask, saved.TaskMode)
	require.Nil(t, saved.MCPSelection)
}

type templateDefinitionCatalog struct {
	automationMCPOptionsCatalog
	definitions map[string]*mcpconfig.MCPServerDefinition
}

func (c templateDefinitionCatalog) GetMCPServerDefinition(_ context.Context, w, id string) (*mcpconfig.MCPServerDefinition, error) {
	return c.definitions[id], nil
}

type templatePolicyReader struct{}

func (templatePolicyReader) GetBuiltinPolicySnapshot(_ context.Context, w, id string) (*mcpconfig.BuiltinPolicySnapshot, error) {
	return &mcpconfig.BuiltinPolicySnapshot{Policy: mcpconfig.DefaultBuiltinWorkspacePolicy(w)}, nil
}
func TestAutomationMCPTemplateUnavailableDefinitions(t *testing.T) {
	svc := newTestService(t)
	a := &Automation{WorkspaceID: "workspace", Name: "Template", TaskMode: TaskModeNormalTask, WorkflowID: "workflow", MCPSelection: &MCPSelection{BuiltinPackages: []p.PackageID{p.PackageCore}, DefinitionIDs: []string{"available", "disabled", "missing"}}}
	require.NoError(t, svc.store.CreateAutomation(t.Context(), a))
	svc.SetMCPSelectionReaders(templateDefinitionCatalog{definitions: map[string]*mcpconfig.MCPServerDefinition{"available": {ID: "available", WorkspaceID: "workspace", Enabled: true}, "disabled": {ID: "disabled", WorkspaceID: "workspace"}}}, templatePolicyReader{})
	_, ids, decisions, err := svc.AutomationMCPTaskTemplate(t.Context(), a.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"available"}, ids)
	require.Len(t, decisions, 2)
	require.Equal(t, "definition_disabled", decisions[0].ReasonCode)
	require.Equal(t, "definition_missing", decisions[1].ReasonCode)
}

func TestAutomationMCPUpdateReturnsCommittedRevisions(t *testing.T) {
	svc := newTestService(t)
	ctx := WithNativePluginToolIntent(t.Context())
	a, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{WorkspaceID: "workspace", Name: "Report"})
	require.NoError(t, err)
	_, err = svc.store.db.Exec(`UPDATE automations SET resource_revision=resource_revision+7 WHERE id=?`, a.ID)
	require.NoError(t, err)
	tx, err := svc.store.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	require.NoError(t, writeAutomationUpdateRow(ctx, tx, a.ID, a, automationUpdateCondition{a.MCPSelectionRevision, a.TaskMode}))
	require.NoError(t, tx.Commit())
	actual, err := svc.store.GetAutomation(ctx, a.ID)
	require.NoError(t, err)
	require.Equal(t, actual.ResourceRevision, a.ResourceRevision)
	saved, err := svc.store.updateAutomationReturning(ctx, a.ID, &UpdateAutomationRequest{})
	require.NoError(t, err)
	actual, err = svc.store.GetAutomation(ctx, a.ID)
	require.NoError(t, err)
	require.Equal(t, actual.ResourceRevision, saved.ResourceRevision)
	require.Equal(t, actual.AutomationRevision, saved.AutomationRevision)
}

type durableTemplatePolicyReader struct{}

func (durableTemplatePolicyReader) GetBuiltinPolicySnapshot(_ context.Context, workspace, _ string) (*mcpconfig.BuiltinPolicySnapshot, error) {
	policy := mcpconfig.DefaultBuiltinWorkspacePolicy(workspace)
	policy.Revision = 7
	policy.Packages[p.PackageConfiguration] = mcpconfig.BuiltinPolicyAvailable
	return &mcpconfig.BuiltinPolicySnapshot{Policy: policy}, nil
}

func TestAutomationMCPTemplateReloadsSavedSelection(t *testing.T) {
	svc := newTestService(t)
	a := &Automation{WorkspaceID: "workspace", Name: "Retry template", TaskMode: TaskModeNormalTask, WorkflowID: "workflow", MCPSelection: &MCPSelection{BuiltinPackages: []p.PackageID{p.PackageCore, p.PackageConfiguration}, DefinitionIDs: []string{"server"}}}
	require.NoError(t, svc.store.CreateAutomation(t.Context(), a))
	a.MCPSelection = nil
	reader := NewService(svc.store, svc.eventBus, svc.logger)
	reader.SetMCPSelectionReaders(templateDefinitionCatalog{definitions: map[string]*mcpconfig.MCPServerDefinition{"server": {ID: "server", WorkspaceID: "workspace", Enabled: true}}}, durableTemplatePolicyReader{})
	preferences, ids, decisions, err := reader.AutomationMCPTaskTemplate(t.Context(), a.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"server"}, ids)
	require.Empty(t, decisions)
	require.NotNil(t, preferences)
	require.Equal(t, 1, preferences.Version)
	require.EqualValues(t, 7, *preferences.ExpectedPolicyRevision)
	require.Equal(t, mcpconfig.BuiltinPackageChoices{p.PackageConfiguration: mcpconfig.BuiltinPreferenceOn, p.PackageRichOutput: mcpconfig.BuiltinPreferenceOff, p.PackageWalkthroughs: mcpconfig.BuiltinPreferenceOff, p.PackageDiagnostics: mcpconfig.BuiltinPreferenceOff}, preferences.Packages)
}
