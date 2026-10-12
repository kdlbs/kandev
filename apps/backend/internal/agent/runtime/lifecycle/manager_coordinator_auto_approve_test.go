package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/common/mcpmode"
	"github.com/kandev/kandev/internal/task/models"
)

// TestLaunchBuildExecutorRequestCoordinatorModeForcesAutoApproveOverrideFalse
// pins the launch path half of the Permission Policy fail-closed contract
// (docs/specs/coordinator/system-design/copilot.md#permission-policy): a
// coordinator-origin launch must force AutoApprovePermissionsOverride to
// false regardless of the resolved profile's own AutoApprove value, since
// only agentctl's exact six-tool coordinator allowlist may auto-approve.
func TestLaunchBuildExecutorRequestCoordinatorModeForcesAutoApproveOverrideFalse(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	backend := &createInstanceExecutor{MockExecutor: MockExecutor{name: executor.NameDocker}}
	registry.Register(backend)
	mgr := &Manager{
		executorRegistry:       registry,
		executorFallbackPolicy: ExecutorFallbackDeny,
		logger:                 log,
	}

	req := &LaunchRequest{
		ExecutorType:         "local_docker",
		EnvironmentFinalized: true,
		SessionID:            "session-1",
		McpMode:              mcpmode.Coordinator,
		Metadata:             map[string]interface{}{"executor_id": "executor-1"},
	}

	_, _, _, err := mgr.launchBuildExecutorRequest(
		context.Background(), "instance-1", req, &testAgent{id: "agent"}, &AgentProfileInfo{AutoApprove: true}, "", "", "", nil,
	)
	if err != nil {
		t.Fatalf("launchBuildExecutorRequest: %v", err)
	}
	if backend.lastRequest == nil {
		t.Fatal("expected CreateInstance to run")
	}
	if backend.lastRequest.AutoApprovePermissionsOverride == nil || *backend.lastRequest.AutoApprovePermissionsOverride {
		t.Fatalf("AutoApprovePermissionsOverride = %v, want a non-nil false override despite profile.AutoApprove=true",
			backend.lastRequest.AutoApprovePermissionsOverride)
	}
	if backend.lastRequest.McpMode != mcpmode.Coordinator {
		t.Fatalf("McpMode = %q, want %q", backend.lastRequest.McpMode, mcpmode.Coordinator)
	}
}

// TestLaunchBuildExecutorRequestNonCoordinatorModeKeepsProfileAutoApprove is
// the control: a non-coordinator launch still honors the resolved profile's
// AutoApprove value, proving the coordinator branch is additive and does not
// change behavior for ordinary task sessions.
func TestLaunchBuildExecutorRequestNonCoordinatorModeKeepsProfileAutoApprove(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	backend := &createInstanceExecutor{MockExecutor: MockExecutor{name: executor.NameDocker}}
	registry.Register(backend)
	mgr := &Manager{
		executorRegistry:       registry,
		executorFallbackPolicy: ExecutorFallbackDeny,
		logger:                 log,
	}

	req := &LaunchRequest{
		ExecutorType:         "local_docker",
		EnvironmentFinalized: true,
		SessionID:            "session-1",
		Metadata:             map[string]interface{}{"executor_id": "executor-1"},
	}

	_, _, _, err := mgr.launchBuildExecutorRequest(
		context.Background(), "instance-1", req, &testAgent{id: "agent"}, &AgentProfileInfo{AutoApprove: true}, "", "", "", nil,
	)
	if err != nil {
		t.Fatalf("launchBuildExecutorRequest: %v", err)
	}
	if backend.lastRequest == nil {
		t.Fatal("expected CreateInstance to run")
	}
	if backend.lastRequest.AutoApprovePermissionsOverride == nil || !*backend.lastRequest.AutoApprovePermissionsOverride {
		t.Fatalf("AutoApprovePermissionsOverride = %v, want a non-nil true override for a non-coordinator profile.AutoApprove=true",
			backend.lastRequest.AutoApprovePermissionsOverride)
	}
}

// coordinatorAutoApproveProfileResolver always resolves to a profile with
// AutoApprove: true, so a coordinator-mode WorkspaceInfo lookup can prove the
// override forces it back to false rather than merely observing an
// already-false default.
type coordinatorAutoApproveProfileResolver struct{}

func (coordinatorAutoApproveProfileResolver) ResolveProfile(_ context.Context, profileID string) (*AgentProfileInfo, error) {
	return &AgentProfileInfo{ProfileID: profileID, AgentID: "augment-agent", AutoApprove: true}, nil
}

// TestPrepareExecutionCreateRequestCoordinatorModeForcesAutoApproveFalse pins
// the workspace-only restore/admission path half of the same fail-closed
// contract: prepareExecutionCreateRequest must force both AutoApprovePermissions
// and its override to false when WorkspaceInfo.McpMode is coordinator, even
// though the resolved profile has AutoApprove: true, and must propagate
// McpMode onto the ExecutorCreateRequest so agentctl enforces the six-tool
// allowlist downstream.
func TestPrepareExecutionCreateRequestCoordinatorModeForcesAutoApproveFalse(t *testing.T) {
	mgr := newTestManager(t)
	mgr.profileResolver = coordinatorAutoApproveProfileResolver{}

	prepared, err := mgr.prepareExecutionCreateRequest(context.Background(), "task-1", &WorkspaceInfo{
		TaskID:         "task-1",
		SessionID:      "session-2",
		WorkspacePath:  "/workspace",
		AgentID:        "auggie",
		ExecutorType:   string(models.ExecutorTypeLocalDocker),
		AgentProfileID: "profile-1",
		McpMode:        mcpmode.Coordinator,
	}, "execution-2")
	if err != nil {
		t.Fatalf("prepareExecutionCreateRequest() error = %v", err)
	}

	if prepared.request.AutoApprovePermissions {
		t.Fatal("AutoApprovePermissions must be false in coordinator mode despite profile.AutoApprove=true")
	}
	if prepared.request.AutoApprovePermissionsOverride == nil || *prepared.request.AutoApprovePermissionsOverride {
		t.Fatalf("AutoApprovePermissionsOverride = %v, want a non-nil false override in coordinator mode",
			prepared.request.AutoApprovePermissionsOverride)
	}
	if prepared.request.McpMode != mcpmode.Coordinator {
		t.Fatalf("McpMode = %q, want %q propagated onto the executor create request", prepared.request.McpMode, mcpmode.Coordinator)
	}
}

// TestPrepareExecutionCreateRequestNonCoordinatorModeKeepsProfileAutoApprove
// is the control for the workspace-only restore path: a non-coordinator
// WorkspaceInfo still honors the resolved profile's AutoApprove value.
func TestPrepareExecutionCreateRequestNonCoordinatorModeKeepsProfileAutoApprove(t *testing.T) {
	mgr := newTestManager(t)
	mgr.profileResolver = coordinatorAutoApproveProfileResolver{}

	prepared, err := mgr.prepareExecutionCreateRequest(context.Background(), "task-1", &WorkspaceInfo{
		TaskID:         "task-1",
		SessionID:      "session-2",
		WorkspacePath:  "/workspace",
		AgentID:        "auggie",
		ExecutorType:   string(models.ExecutorTypeLocalDocker),
		AgentProfileID: "profile-1",
	}, "execution-3")
	if err != nil {
		t.Fatalf("prepareExecutionCreateRequest() error = %v", err)
	}

	if !prepared.request.AutoApprovePermissions {
		t.Fatal("AutoApprovePermissions must remain true outside coordinator mode when profile.AutoApprove=true")
	}
	if prepared.request.AutoApprovePermissionsOverride == nil || !*prepared.request.AutoApprovePermissionsOverride {
		t.Fatalf("AutoApprovePermissionsOverride = %v, want a non-nil true override outside coordinator mode",
			prepared.request.AutoApprovePermissionsOverride)
	}
}
