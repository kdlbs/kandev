package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestGuardedGuard_DeniedActionsRefusedForGuardedCaller(t *testing.T) {
	f := newGuardedFixture(t)
	denied := []string{
		ws.ActionMCPTransferTask, ws.ActionMCPResolveAgentPermission,
		ws.ActionMCPInvokePluginTool, ws.ActionMCPListPluginTools,
		ws.ActionMCPCreateExecutorProfile, ws.ActionMCPUpdateExecutorProfile, ws.ActionMCPDeleteExecutorProfile,
		ws.ActionMCPDeleteAgentProfile, ws.ActionMCPUpdateMcpConfig, ws.ActionMCPDeleteWorkflow,
		ws.ActionMCPDeleteWorkflowStep, ws.ActionMCPDeleteTask, ws.ActionMCPArchiveTask,
	}
	for _, action := range denied {
		// Even a payload that targets the caller's own task must not unlock a denied class.
		guarded, _ := f.call(t, f.principal, action, map[string]any{"task_id": fxCaller})
		require.NotNil(t, guarded, action)
		var p ws.ErrorPayload
		require.NoError(t, json.Unmarshal(guarded.Payload, &p))
		require.Equal(t, ws.ErrorCodeUnknownAction, p.Code, action)
	}
}

func TestGuardedGuard_OversizedAndOddIdsAreRefused(t *testing.T) {
	f := newGuardedFixture(t)
	long := strings.Repeat("x", 1<<20)
	for _, id := range []string{long, "", " " + fxCaller, fxCaller + "\x00", strings.ToUpper(fxCaller)} {
		guarded, _ := f.call(t, f.principal, ws.ActionMCPUpdateTask, map[string]any{"task_id": id})
		if id == "" {
			continue // empty means "unspecified"; covered by the self-class test
		}
		assertWSError(t, guarded, ws.ErrorCodeForbidden)
	}
}
