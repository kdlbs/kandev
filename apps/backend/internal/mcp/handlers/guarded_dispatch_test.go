package handlers

import (
	"context"
	"testing"

	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type dispatchProbe struct {
	calls   int
	payload []byte
}

func (p *dispatchProbe) handle(_ context.Context, msg *ws.Message) (*ws.Message, error) {
	p.calls++
	p.payload = msg.Payload
	return ws.NewResponse(msg.ID, msg.Action, map[string]any{"ok": true})
}

// Dispatching through the registered wrapper proves the guarded guard sits in
// front of every handler, independent of any tool catalogue.
func TestGuardedDispatcher_EnforcesGuardedSurfaceBeforeHandlers(t *testing.T) {
	f := newGuardedFixture(t)
	d := &guardedMCPDispatcher{Dispatcher: ws.NewDispatcher(), handlers: f.h}
	probes := map[string]*dispatchProbe{}
	for _, action := range []string{ws.ActionMCPArchiveTask, ws.ActionMCPUpdateTask, ws.ActionMCPCreateTask} {
		probe := &dispatchProbe{}
		probes[action] = probe
		d.RegisterFunc(action, probe.handle)
	}
	dispatch := func(p mcpscope.Principal, action string, payload map[string]any) *ws.Message {
		resp, err := d.Dispatch(mcpscope.WithPrincipal(context.Background(), p), makeWSMessage(t, action, payload))
		require.NoError(t, err)
		require.NotNil(t, resp)
		return resp
	}

	t.Run("a denied tool never reaches its handler", func(t *testing.T) {
		resp := dispatch(f.principal, ws.ActionMCPArchiveTask, map[string]any{"task_id": fxCaller})
		assertWSError(t, resp, ws.ErrorCodeUnknownAction)
		require.Zero(t, probes[ws.ActionMCPArchiveTask].calls)
	})
	t.Run("a write to a foreign task never reaches its handler", func(t *testing.T) {
		resp := dispatch(f.principal, ws.ActionMCPUpdateTask, map[string]any{"task_id": fxForeign, "title": "x"})
		assertWSError(t, resp, ws.ErrorCodeForbidden)
		require.Zero(t, probes[ws.ActionMCPUpdateTask].calls)
	})
	t.Run("an allowed call reaches the handler with server-derived attribution", func(t *testing.T) {
		resp := dispatch(f.principal, ws.ActionMCPCreateTask, map[string]any{"title": "x", "source_task_id": "forged"})
		require.Equal(t, ws.MessageTypeResponse, resp.Type, string(resp.Payload))
		probe := probes[ws.ActionMCPCreateTask]
		require.Equal(t, 1, probe.calls)
		require.Contains(t, string(probe.payload), `"source_task_id":"`+fxCaller+`"`)
		require.NotContains(t, string(probe.payload), "forged")
	})
	t.Run("unguarded callers reach denied-for-guarded handlers unchanged", func(t *testing.T) {
		p := f.principal
		p.Guarded = false
		resp := dispatch(p, ws.ActionMCPArchiveTask, map[string]any{"task_id": fxForeign})
		require.Equal(t, ws.MessageTypeResponse, resp.Type, string(resp.Payload))
		require.Equal(t, 1, probes[ws.ActionMCPArchiveTask].calls)
	})
}
