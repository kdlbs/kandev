package lifecycle

import (
	"context"
	"testing"

	p "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	mcpserver "github.com/kandev/kandev/internal/mcp/server"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func builtinDispatchProfile(ids ...p.PackageID) p.Context {
	profile := p.New(p.SurfaceKanbanTask, nil, nil)
	origins := map[p.PackageID]string{p.PackageCore: "required"}
	for _, id := range ids {
		if id != p.PackageCore {
			origins[id] = "task"
		}
	}
	profile.BuiltinPackages = &p.BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task-a", Surface: profile.Surface, Packages: ids, Origins: origins}
	return profile
}

// @covers AC-AGENTS-BUILTIN-MCP-003.2
func TestBuiltinDispatchUsesTrustedRegisteredSurface(t *testing.T) {
	for _, selected := range []bool{false, true} {
		t.Run(map[bool]string{false: "core", true: "configuration"}[selected], func(t *testing.T) {
			profile := builtinDispatchProfile(p.PackageCore)
			if selected {
				profile = builtinDispatchProfile(p.PackageCore, p.PackageConfiguration)
			}
			encoded, err := p.MarshalBuiltinBinding(profile)
			require.NoError(t, err)
			inner := &recordingMCPHandler{}
			sm := newMCPStreamManager(t, inner, nil)
			sm.builtinActionResolver = builtinTestActions
			sm.mcpPrincipalScoper = builtinTestPrincipal
			execution := &AgentExecution{ID: "execution", TaskID: "task-a", SessionID: "session-a", WorkspaceID: "workspace"}
			execution.setMetadataValue(p.BuiltinBindingMetadataKey, encoded)
			handler := sm.mcpHandlerFor(execution)
			for _, tc := range []struct {
				action  string
				allowed bool
			}{
				{ws.ActionMCPListTasks, true}, {ws.ActionMCPCreateWorkflow, selected},
				{ws.ActionMCPTransferTask, false}, {ws.ActionMCPSetTaskTitle, false},
				{ws.ActionMCPGetDiagnosticBundle, false}, {ws.ActionWorkspaceFileContentGet, false},
			} {
				before := inner.calls
				msg := mcpRequest(t, map[string]any{"builtin_packages": []string{"configuration", "diagnostics"}, "surface": "configuration"})
				msg.Action = tc.action
				response, err := handler.Dispatch(context.Background(), msg)
				require.NoError(t, err)
				if tc.allowed {
					require.Equal(t, before+1, inner.calls, tc.action)
				} else {
					require.Equal(t, before, inner.calls, tc.action)
					require.Equal(t, ws.MessageTypeError, response.Type)
				}
			}
		})
	}
}

func TestBuiltinDispatchRejectsCorruptOrForeignBinding(t *testing.T) {
	foreign := builtinDispatchProfile(p.PackageCore)
	foreign.BuiltinPackages.TaskID = "other"
	encoded, err := p.MarshalBuiltinBinding(foreign)
	require.NoError(t, err)
	for _, value := range []any{nil, "{}", encoded} {
		inner := &recordingMCPHandler{}
		sm := newMCPStreamManager(t, inner, nil)
		sm.builtinActionResolver = builtinTestActions
		sm.mcpPrincipalScoper = builtinTestPrincipal
		execution := &AgentExecution{ID: "execution", TaskID: "task-a", SessionID: "session-a", WorkspaceID: "workspace"}
		execution.setMetadataValue(p.BuiltinBindingMetadataKey, value)
		response, err := sm.mcpHandlerFor(execution).Dispatch(context.Background(), mcpRequest(t, nil))
		require.NoError(t, err)
		require.Equal(t, ws.MessageTypeError, response.Type)
		require.Zero(t, inner.calls)
	}
}

func builtinTestActions(profile p.Context) ([]string, error) {
	return mcpserver.ResolveBuiltinActions(profile, newTestLogger())
}

func TestBuiltinDispatchCacheRejectsChangedOwner(t *testing.T) {
	profile := builtinDispatchProfile(p.PackageCore, p.PackageConfiguration)
	encoded, err := p.MarshalBuiltinBinding(profile)
	require.NoError(t, err)
	execution := &AgentExecution{TaskID: "task-a", WorkspaceID: "workspace"}
	execution.setMetadataValue(p.BuiltinBindingMetadataKey, encoded)
	require.True(t, execution.builtinMCPAuthority(builtinTestActions).Allows(ws.ActionMCPCreateWorkflow))
	execution.WorkspaceID = "other"
	require.False(t, execution.builtinMCPAuthority(builtinTestActions).Allows(ws.ActionMCPCreateWorkflow))
	execution.WorkspaceID, execution.TaskID = "workspace", "other"
	require.False(t, execution.builtinMCPAuthority(builtinTestActions).Allows(ws.ActionMCPCreateWorkflow))
}

func TestBuiltinDispatchRejectsChangedPrincipal(t *testing.T) {
	for _, changed := range []string{"workspace", "surface"} {
		t.Run(changed, func(t *testing.T) {
			profile := builtinDispatchProfile(p.PackageCore, p.PackageConfiguration)
			encoded, err := p.MarshalBuiltinBinding(profile)
			require.NoError(t, err)
			execution := &AgentExecution{ID: "execution", TaskID: "task-a", SessionID: "session-a", WorkspaceID: "workspace"}
			execution.setMetadataValue(p.BuiltinBindingMetadataKey, encoded)
			inner := &recordingMCPHandler{}
			sm := newMCPStreamManager(t, inner, nil)
			sm.builtinActionResolver = builtinTestActions
			sm.mcpPrincipalScoper = builtinTestPrincipal
			sm.mcpPrincipalScoper = func(ctx context.Context, taskID, sessionID string) (context.Context, error) {
				principal := mcpscope.Principal{WorkspaceID: "workspace", CallerTaskID: taskID, CallerSessionID: sessionID, Surface: p.SurfaceKanbanTask}
				if changed == "workspace" {
					principal.WorkspaceID = "other"
				} else {
					principal.Surface = p.SurfaceConfiguration
				}
				return mcpscope.WithPrincipal(ctx, principal), nil
			}
			response, err := sm.mcpHandlerFor(execution).Dispatch(context.Background(), mcpRequest(t, nil))
			require.NoError(t, err)
			require.Equal(t, ws.MessageTypeError, response.Type)
			require.Zero(t, inner.calls)
		})
	}
}

func builtinTestPrincipal(ctx context.Context, taskID, sessionID string) (context.Context, error) {
	return mcpscope.WithPrincipal(ctx, mcpscope.Principal{WorkspaceID: "workspace", CallerTaskID: taskID, CallerSessionID: sessionID, Surface: p.SurfaceKanbanTask}), nil
}

func TestBuiltinDispatchRejectsRetiredExecutionStream(t *testing.T) {
	inner := &recordingMCPHandler{}
	sm := newMCPStreamManager(t, inner, nil)
	sm.builtinActionResolver = builtinTestActions
	sm.mcpPrincipalScoper = builtinTestPrincipal
	current := true
	sm.isExecutionCurrent = func(*AgentExecution) bool { return current }
	execution := &AgentExecution{ID: "execution", TaskID: "task-a", SessionID: "session-a", WorkspaceID: "workspace"}
	encoded, err := p.MarshalBuiltinBinding(builtinDispatchProfile(p.PackageCore))
	require.NoError(t, err)
	execution.setMetadataValue(p.BuiltinBindingMetadataKey, encoded)
	handler := sm.mcpHandlerFor(execution)
	msg := mcpRequest(t, nil)
	msg.Action = ws.ActionMCPListTasks
	_, err = handler.Dispatch(context.Background(), msg)
	require.NoError(t, err)
	require.Equal(t, 1, inner.calls)
	current = false
	response, err := handler.Dispatch(context.Background(), msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
	require.Equal(t, 1, inner.calls)
}

func TestBuiltinDispatchRejectsMissingExecutionOwner(t *testing.T) {
	inner := &recordingMCPHandler{}
	sm := newMCPStreamManager(t, inner, nil)
	sm.builtinActionResolver = builtinTestActions
	execution := &AgentExecution{ID: "execution"}
	encoded, err := p.MarshalBuiltinBinding(builtinDispatchProfile(p.PackageCore))
	require.NoError(t, err)
	execution.setMetadataValue(p.BuiltinBindingMetadataKey, encoded)
	response, err := sm.mcpHandlerFor(execution).Dispatch(context.Background(), mcpRequest(t, nil))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
	require.Zero(t, inner.calls)
}
