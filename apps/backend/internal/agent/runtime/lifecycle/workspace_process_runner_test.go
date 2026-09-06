package lifecycle

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestStartWorkspaceProcessBindsExecutionWorkspaceAndRequestIdentity(t *testing.T) {
	fake, client := newFakeAgentctlProcessServer(t)
	manager, execution := newProcessRunnerManager(t, client)
	execution.WorkspacePath = "/workspace/session-1"

	info, err := manager.StartWorkspaceProcess(context.Background(), WorkspaceProcessRequest{
		RunID: "workflow-run-1", SessionID: execution.SessionID, ExecutionID: execution.ID,
		Command: "printf workspace", WorkingDir: execution.WorkspacePath,
		Timeout: 30 * time.Second, Kind: "workflow_script", BufferMaxBytes: 4096,
	})
	if err != nil {
		t.Fatalf("StartWorkspaceProcess: %v", err)
	}
	if info.ID != "proc-1" {
		t.Fatalf("process ID = %q, want proc-1", info.ID)
	}
	starts := fake.snapshotStarts()
	if len(starts) != 1 {
		t.Fatalf("start requests = %d, want 1", len(starts))
	}
	if starts[0].RequestID != "workflow-run-1" || starts[0].WorkingDir != execution.WorkspacePath || starts[0].Timeout != 30*time.Second || starts[0].BufferMaxBytes != 4096 {
		t.Fatalf("bound request = %+v", starts[0])
	}
}

func TestStartWorkspaceProcessRejectsStaleExecutionAndWorkspace(t *testing.T) {
	fake, client := newFakeAgentctlProcessServer(t)
	manager, execution := newProcessRunnerManager(t, client)
	execution.WorkspacePath = "/workspace/session-1"
	base := WorkspaceProcessRequest{
		RunID: "workflow-run-2", SessionID: execution.SessionID, ExecutionID: execution.ID,
		Command: "printf workspace", WorkingDir: execution.WorkspacePath, Timeout: time.Second,
	}

	stale := base
	stale.ExecutionID = "execution-replaced"
	if _, err := manager.StartWorkspaceProcess(context.Background(), stale); err == nil || !strings.Contains(err.Error(), "execution") {
		t.Fatalf("stale execution error = %v, want execution mismatch", err)
	}

	escape := base
	escape.WorkingDir = "/workspace/other-session"
	if _, err := manager.StartWorkspaceProcess(context.Background(), escape); err == nil || !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("workspace escape error = %v, want workspace mismatch", err)
	}
	if starts := fake.snapshotStarts(); len(starts) != 0 {
		t.Fatalf("rejected requests reached agentctl: %+v", starts)
	}
}

func TestWorkspaceProcessGetAndStopUseExactExecutionOwnership(t *testing.T) {
	fake, client := newFakeAgentctlProcessServer(t)
	fake.knownProcess = "proc-1"
	fake.knownSession = "session-1"
	manager, execution := newProcessRunnerManager(t, client)

	info, err := manager.GetWorkspaceProcess(context.Background(), execution.ID, "proc-1", true)
	if err != nil {
		t.Fatalf("GetWorkspaceProcess: %v", err)
	}
	if info.ID != "proc-1" || info.SessionID != execution.SessionID {
		t.Fatalf("process = %+v", info)
	}
	if err := manager.StopWorkspaceProcess(context.Background(), execution.ID, "proc-1"); err != nil {
		t.Fatalf("StopWorkspaceProcess: %v", err)
	}
	stops := fake.snapshotStops()
	if len(stops) != 1 || stops[0] != "proc-1" {
		t.Fatalf("stop requests = %v", stops)
	}
}
