package instance

import (
	"testing"
	"time"
)

// TestInstanceInfoReflectsLiveWorkspaceSourceRootsAndProviderSessionID pins
// AC-EXECUTORS-SURVIVAL-002.14's "workspace source roots" and "provider
// session identity" reconstruction rows: both are read back live from the
// process manager on every Info() call, not snapshotted at creation, so a
// rescan/rebind or a completed session/new is reflected immediately.
func TestInstanceInfoReflectsLiveWorkspaceSourceRootsAndProviderSessionID(t *testing.T) {
	inst := &Instance{
		ID:        "instance-1",
		Port:      5001,
		Status:    "running",
		CreatedAt: time.Now(),
		manager: &fakeProcessManager{
			workspaceSourceRoots: []string{"/ws/task-1/backend", "/ws/task-1/frontend"},
			sessionID:            "provider-session-9",
		},
	}

	info := inst.Info()

	if len(info.WorkspaceSourceRoots) != 2 ||
		info.WorkspaceSourceRoots[0] != "/ws/task-1/backend" ||
		info.WorkspaceSourceRoots[1] != "/ws/task-1/frontend" {
		t.Errorf("WorkspaceSourceRoots = %v, want the live manager allowlist", info.WorkspaceSourceRoots)
	}
	if info.ProviderSessionID != "provider-session-9" {
		t.Errorf("ProviderSessionID = %q, want provider-session-9", info.ProviderSessionID)
	}
}

// TestInstanceInfoWithoutManagerLeavesRecoveryRowsEmpty pins the nil-manager
// guard: an Instance never wired to a process manager (defensive test-only
// shape) must not panic Info(), and both rows report empty rather than a
// stale or fabricated value.
func TestInstanceInfoWithoutManagerLeavesRecoveryRowsEmpty(t *testing.T) {
	inst := &Instance{
		ID:        "instance-1",
		Port:      5001,
		Status:    "running",
		CreatedAt: time.Now(),
	}

	info := inst.Info()

	if info.WorkspaceSourceRoots != nil {
		t.Errorf("WorkspaceSourceRoots = %v, want nil without a manager", info.WorkspaceSourceRoots)
	}
	if info.ProviderSessionID != "" {
		t.Errorf("ProviderSessionID = %q, want empty without a manager", info.ProviderSessionID)
	}
}
