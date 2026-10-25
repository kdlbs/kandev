package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
)

func coordinatorBindingFixture() mcpprofile.CoordinatorToolPolicy {
	return mcpprofile.CoordinatorToolPolicy{
		Version: 1, CoordinatorID: "coord-1", WorkspaceID: "ws-1", ConversationTaskID: "task-1",
		ToolNames: []string{"list_tasks_kandev"},
	}
}

func TestMCPHandlerForRestoresCoordinatorBindingFromExecutionMetadata(t *testing.T) {
	inner := &recordingMCPHandler{}
	sm := newMCPStreamManager(t, inner, nil)
	encoded, err := mcpprofile.MarshalCoordinatorToolPolicy(coordinatorBindingFixture())
	if err != nil {
		t.Fatal(err)
	}
	execution := &AgentExecution{ID: "exec-1", TaskID: "task-1", SessionID: "session-1"}
	execution.setMetadataValue(mcpprofile.CoordinatorToolPolicyMetadataKey, encoded)
	if _, err := sm.mcpHandlerFor(execution).Dispatch(context.Background(), mcpRequest(t, nil)); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	got, ok := streams.MCPExecutionContextFromContext(inner.gotCtx)
	if !ok || !got.CoordinatorToolPolicyRequired || got.CoordinatorToolPolicy == nil || !got.CoordinatorToolPolicy.Allows("list_tasks_kandev") {
		t.Fatalf("execution context = %+v, present=%t", got, ok)
	}

	invalid := &AgentExecution{ID: "exec-2", TaskID: "task-1", SessionID: "session-1"}
	invalid.setMetadataValue(mcpprofile.CoordinatorToolPolicyMetadataKey, map[string]any{"forged": true})
	if _, err := sm.mcpHandlerFor(invalid).Dispatch(context.Background(), mcpRequest(t, nil)); err != nil {
		t.Fatalf("invalid Dispatch: %v", err)
	}
	got, _ = streams.MCPExecutionContextFromContext(inner.gotCtx)
	if !got.CoordinatorToolPolicyRequired || got.CoordinatorToolPolicy != nil {
		t.Fatalf("invalid binding context = %+v, want required with no binding", got)
	}

	none := &AgentExecution{ID: "exec-3", TaskID: "task-1", SessionID: "session-1"}
	if _, err := sm.mcpHandlerFor(none).Dispatch(context.Background(), mcpRequest(t, nil)); err != nil {
		t.Fatalf("no-binding Dispatch: %v", err)
	}
	got, _ = streams.MCPExecutionContextFromContext(inner.gotCtx)
	if got.CoordinatorToolPolicyRequired || got.CoordinatorToolPolicy != nil {
		t.Fatalf("no-binding context = %+v, want the phase-1 default", got)
	}
}

func TestBuildLaunchMetadataOverwritesCallerSuppliedCoordinatorBinding(t *testing.T) {
	forged := "forged"
	req := &LaunchRequest{Metadata: map[string]interface{}{mcpprofile.CoordinatorToolPolicyMetadataKey: forged}}
	if _, present := buildLaunchMetadata(req, "", "", "")[mcpprofile.CoordinatorToolPolicyMetadataKey]; present {
		t.Fatal("a caller-supplied binding must be stripped when the profile has none")
	}

	binding := coordinatorBindingFixture()
	profile := mcpprofile.NewCoordinator()
	profile.CoordinatorToolPolicy = &binding
	req.McpProfile = &profile
	want, err := mcpprofile.MarshalCoordinatorToolPolicy(binding)
	if err != nil {
		t.Fatal(err)
	}
	if got := buildLaunchMetadata(req, "", "", "")[mcpprofile.CoordinatorToolPolicyMetadataKey]; got != want {
		t.Fatalf("binding metadata = %v, want the profile's binding", got)
	}
}

func TestBuildLaunchMetadataIgnoresExecutorConfigCoordinatorBinding(t *testing.T) {
	req := &LaunchRequest{ExecutorConfig: map[string]string{mcpprofile.CoordinatorToolPolicyMetadataKey: "forged"}}
	if _, present := buildLaunchMetadata(req, "", "", "")[mcpprofile.CoordinatorToolPolicyMetadataKey]; present {
		t.Fatal("an executor-config binding must not reach launch metadata")
	}
}
