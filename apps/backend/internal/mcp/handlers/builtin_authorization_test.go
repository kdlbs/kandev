package handlers

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	p "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestBuiltinDispatcherRejectsUnboundActionBeforeHandler(t *testing.T) {
	dispatcher := &guardedMCPDispatcher{Dispatcher: ws.NewDispatcher(), handlers: &Handlers{logger: testLogger(t)}}
	calls := 0
	for _, action := range []string{ws.ActionMCPListTasks, ws.ActionMCPCreateWorkflow} {
		dispatcher.RegisterFunc(action, func(_ context.Context, msg *ws.Message) (*ws.Message, error) {
			calls++
			return ws.NewResponse(msg.ID, msg.Action, nil)
		})
	}
	ctx := streams.WithMCPExecutionContext(context.Background(), streams.MCPExecutionContext{TaskID: "task", SessionID: "session",
		BuiltinAuthority: streams.BuiltinMCPAuthority{Required: true, Manifest: &p.BuiltinManifest{TaskID: "task", WorkspaceID: "workspace", Surface: p.SurfaceKanbanTask}, Actions: []string{ws.ActionMCPListTasks}}})
	ctx = mcpscope.WithPrincipal(ctx, mcpscope.Principal{CallerTaskID: "task", CallerSessionID: "session", WorkspaceID: "workspace", Surface: p.SurfaceKanbanTask})
	_, err := dispatcher.Dispatch(ctx, makeWSMessage(t, ws.ActionMCPListTasks, nil))
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	response, err := dispatcher.Dispatch(ctx, makeWSMessage(t, ws.ActionMCPCreateWorkflow, nil))
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	assertWSError(t, response, ws.ErrorCodeUnknownAction)
}
