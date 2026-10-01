package handlers

import (
	"context"
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestProjectMCPDispatchRejectsGenericActions(t *testing.T) {
	tests := []struct {
		name    string
		tier    string
		surface mcpprofile.Surface
		action  string
	}{
		{name: "coordinator cannot create generic task", tier: "coordinator", surface: mcpprofile.SurfaceProjectCoordinator, action: ws.ActionMCPCreateTask},
		{name: "coordinator cannot complete workflow step", tier: "coordinator", surface: mcpprofile.SurfaceProjectCoordinator, action: ws.ActionMCPStepComplete},
		{name: "worker cannot message generic task", tier: "economy", surface: mcpprofile.SurfaceProjectWorker, action: ws.ActionMCPMessageTask},
		{name: "worker cannot read coordinator tools", tier: "economy", surface: mcpprofile.SurfaceProjectWorker, action: ws.ActionMCPGetAgentProject},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dispatcher := ws.NewDispatcher()
			handlerCalled := false
			guard := &guardedMCPDispatcher{Dispatcher: dispatcher, handlers: &Handlers{}}
			guard.RegisterFunc(tt.action, func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
				handlerCalled = true
				return ws.NewResponse(msg.ID, msg.Action, map[string]bool{"ok": true})
			})
			ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
				WorkspaceID: "workspace", CallerTaskID: "task", CallerSessionID: "session",
				AgentProjectID: "project", AgentProjectTier: tt.tier,
				ProjectMainTaskID: "coordinator", Surface: tt.surface,
			})

			response, err := dispatcher.Dispatch(ctx, makeWSMessage(t, tt.action, map[string]string{}))

			require.NoError(t, err)
			require.False(t, handlerCalled, "generic action must be rejected before handler dispatch")
			assertWSError(t, response, ws.ErrorCodeUnknownAction)
		})
	}
}

func TestProjectMCPDispatchAllowsOnlyRoleSpecificActions(t *testing.T) {
	tests := []struct {
		name    string
		tier    string
		surface mcpprofile.Surface
		action  string
	}{
		{name: "coordinator may read project", tier: "coordinator", surface: mcpprofile.SurfaceProjectCoordinator, action: ws.ActionMCPGetAgentProject},
		{name: "coordinator may ask user", tier: "coordinator", surface: mcpprofile.SurfaceProjectCoordinator, action: ws.ActionMCPAskUserQuestion},
		{name: "worker may read its project task", tier: "economy", surface: mcpprofile.SurfaceProjectWorker, action: ws.ActionMCPGetAgentProjectTask},
		{name: "worker may ask parent", tier: "frontier", surface: mcpprofile.SurfaceProjectWorker, action: ws.ActionMCPAskParentQuestion},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dispatcher := ws.NewDispatcher()
			handlerCalled := false
			guard := &guardedMCPDispatcher{Dispatcher: dispatcher, handlers: &Handlers{}}
			guard.RegisterFunc(tt.action, func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
				handlerCalled = true
				return ws.NewResponse(msg.ID, msg.Action, map[string]bool{"ok": true})
			})
			ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
				WorkspaceID: "workspace", CallerTaskID: "task", CallerSessionID: "session",
				AgentProjectID: "project", AgentProjectTier: tt.tier,
				ProjectMainTaskID: "coordinator", Surface: tt.surface,
			})

			response, err := dispatcher.Dispatch(ctx, makeWSMessage(t, tt.action, map[string]string{}))

			require.NoError(t, err)
			require.True(t, handlerCalled, "role-specific action should reach its registered handler")
			require.Equal(t, ws.MessageTypeResponse, response.Type)
		})
	}
}
