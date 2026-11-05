package lifecycle

import (
	"context"
	builtinintent "github.com/kandev/kandev/internal/mcp/intent"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBuiltinIntentRuntimeAdmissionRejectsChangedTarget(t *testing.T) {
	sm := &SessionManager{}
	for _, state := range []string{"selected", "excluded", "rotated", "corrupt", "legacy"} {
		t.Run(state, func(t *testing.T) {
			execution := &AgentExecution{ID: "execution", TaskID: "task-a", SessionID: "session", WorkspaceID: "workspace"}
			profile := builtinDispatchProfile(p.PackageCore, p.PackageWalkthroughs)
			if state == "excluded" {
				profile = builtinDispatchProfile(p.PackageCore)
			}
			if state != "legacy" {
				encoded, err := p.MarshalBuiltinBinding(profile)
				require.NoError(t, err)
				execution.setMetadataValue(p.BuiltinBindingMetadataKey, encoded)
			}
			if state == "corrupt" {
				execution.setMetadataValue(p.BuiltinBindingMetadataKey, "corrupt")
			}
			expected := "execution"
			if state == "rotated" {
				expected = "old"
			}
			ctx := builtinintent.Bind(context.Background(), builtinintent.Binding{Intent: builtinintent.Walkthrough, TaskID: "task-a", SessionID: "session", ExecutionID: expected})
			_, err := sm.admitExecutorPrompt(ctx, execution)
			if state == "selected" || state == "legacy" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, builtinintent.ErrUnavailable)
			}
		})
	}
}

func TestBuiltinIntentGuardsSteeringPassthroughAndStartup(t *testing.T) {
	manager := newTestManager(t)
	execution := &AgentExecution{ID: "execution", TaskID: "task", SessionID: "session", WorkspaceID: "workspace"}
	require.NoError(t, manager.executionStore.Add(execution))
	ctx := builtinintent.Bind(context.Background(), builtinintent.Binding{Intent: builtinintent.Walkthrough, TaskID: "task", SessionID: "session", ExecutionID: "retired"})
	_, err := (&SessionManager{}).SendPromptSteerWithDispatchCallback(ctx, execution, "request", false, nil, false, nil)
	require.ErrorIs(t, err, builtinintent.ErrUnavailable)
	require.ErrorIs(t, manager.WritePassthroughStdin(ctx, "session", "request"), builtinintent.ErrUnavailable)
	require.ErrorIs(t, manager.StartAgentProcess(ctx, "execution"), builtinintent.ErrUnavailable)
}
