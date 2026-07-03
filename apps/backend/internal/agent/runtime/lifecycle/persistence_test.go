package lifecycle

import (
	"context"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type captureExecutorRunningWriter struct {
	running *models.ExecutorRunning
}

func (w *captureExecutorRunningWriter) UpsertExecutorRunning(_ context.Context, running *models.ExecutorRunning) error {
	w.running = running
	return nil
}

func (w *captureExecutorRunningWriter) DeleteExecutorRunningBySessionID(_ context.Context, _ string) error {
	return nil
}

func TestBuildRunningFromExecutionPersistsLiveAgentctlEndpoint(t *testing.T) {
	log := newNopLogger(t)
	client := agentctl.NewClient("127.0.0.1", 45678, log)

	running := buildRunningFromExecution(&AgentExecution{
		ID:             "exec-1",
		TaskID:         "task-1",
		SessionID:      "session-1",
		RuntimeName:    agentruntime.RuntimeStandalone,
		Status:         v1.AgentStatusRunning,
		agentctl:       client,
		standalonePort: 45678,
	}, nil)

	if running.Status != models.ExecutorRunningStatusRunning {
		t.Fatalf("Status = %q, want running", running.Status)
	}
	if running.AgentctlURL != "http://127.0.0.1:45678" {
		t.Fatalf("AgentctlURL = %q, want live client URL", running.AgentctlURL)
	}
	if running.AgentctlPort != 45678 {
		t.Fatalf("AgentctlPort = %d, want 45678", running.AgentctlPort)
	}
	if running.LastSeenAt == nil {
		t.Fatal("LastSeenAt = nil, want live endpoint observation timestamp")
	}
}

func TestBuildRunningFromExecutionPersistsSSHRuntimePID(t *testing.T) {
	log := newNopLogger(t)
	client := agentctl.NewClient("127.0.0.1", 43001, log)

	running := buildRunningFromExecution(&AgentExecution{
		ID:          "exec-ssh",
		TaskID:      "task-1",
		SessionID:   "session-1",
		RuntimeName: agentruntime.RuntimeSSH,
		Status:      v1.AgentStatusRunning,
		agentctl:    client,
		Metadata: map[string]interface{}{
			MetadataKeySSHLocalForwardPort:   "43001",
			MetadataKeySSHRemoteAgentctlPID:  "9321",
			MetadataKeySSHRemoteAgentctlPort: "43000",
		},
	}, nil)

	if running.AgentctlURL != "http://127.0.0.1:43001" {
		t.Fatalf("AgentctlURL = %q, want local forward URL", running.AgentctlURL)
	}
	if running.AgentctlPort != 43001 {
		t.Fatalf("AgentctlPort = %d, want local forward port", running.AgentctlPort)
	}
	if running.PID != 9321 {
		t.Fatalf("PID = %d, want remote agentctl pid", running.PID)
	}
	if running.LastSeenAt == nil {
		t.Fatal("LastSeenAt = nil, want live endpoint observation timestamp")
	}
}

func TestMarkReadyPersistsReadyExecutorRunningStatus(t *testing.T) {
	log := newNopLogger(t)
	client := agentctl.NewClient("127.0.0.1", 45678, log)
	writer := &captureExecutorRunningWriter{}
	mgr := newTestManager(t)
	mgr.SetExecutorRunningWriter(writer)

	if err := mgr.executionStore.Add(&AgentExecution{
		ID:             "exec-ready",
		TaskID:         "task-1",
		SessionID:      "session-1",
		RuntimeName:    agentruntime.RuntimeStandalone,
		Status:         v1.AgentStatusRunning,
		agentctl:       client,
		standalonePort: 45678,
	}); err != nil {
		t.Fatalf("Add execution: %v", err)
	}

	if err := mgr.MarkReady("exec-ready"); err != nil {
		t.Fatalf("MarkReady: %v", err)
	}

	if writer.running == nil {
		t.Fatal("expected MarkReady to persist executors_running row")
	}
	if writer.running.Status != models.ExecutorRunningStatusReady {
		t.Fatalf("persisted status = %q, want ready", writer.running.Status)
	}
	if writer.running.AgentctlPort != 45678 {
		t.Fatalf("persisted port = %d, want 45678", writer.running.AgentctlPort)
	}
}

func TestUpdateStatusPersistsRunningExecutorRunningStatus(t *testing.T) {
	log := newNopLogger(t)
	client := agentctl.NewClient("127.0.0.1", 45678, log)
	writer := &captureExecutorRunningWriter{}
	mgr := newTestManager(t)
	mgr.SetExecutorRunningWriter(writer)

	if err := mgr.executionStore.Add(&AgentExecution{
		ID:             "exec-running",
		TaskID:         "task-1",
		SessionID:      "session-1",
		RuntimeName:    agentruntime.RuntimeStandalone,
		Status:         v1.AgentStatusReady,
		agentctl:       client,
		standalonePort: 45678,
	}); err != nil {
		t.Fatalf("Add execution: %v", err)
	}

	if err := mgr.UpdateStatus("exec-running", v1.AgentStatusRunning); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	if writer.running == nil {
		t.Fatal("expected UpdateStatus to persist executors_running row")
	}
	if writer.running.Status != models.ExecutorRunningStatusRunning {
		t.Fatalf("persisted status = %q, want running", writer.running.Status)
	}
}
