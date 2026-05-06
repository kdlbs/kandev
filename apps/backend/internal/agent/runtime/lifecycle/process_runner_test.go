package lifecycle

import (
	"context"
	"testing"
)

func TestProcessRunnerStartErrors(t *testing.T) {
	manager := &Manager{executionStore: NewExecutionStore()}

	if _, err := manager.StartProcess(context.Background(), StartProcessRequest{
		SessionID: "missing",
		Kind:      "dev",
		Command:   "echo hi",
	}); err == nil {
		t.Fatal("expected error when no execution exists for session")
	}

	exec := &AgentExecution{ID: "exec-1", SessionID: "session-1"}
	manager.executionStore.Add(exec)
	if _, err := manager.StartProcess(context.Background(), StartProcessRequest{
		SessionID: "session-1",
		Kind:      "dev",
		Command:   "echo hi",
	}); err == nil {
		t.Fatal("expected error when agentctl client is missing")
	}
}

func TestProcessRunnerListErrors(t *testing.T) {
	manager := &Manager{executionStore: NewExecutionStore()}
	if _, err := manager.ListProcesses(context.Background(), "missing"); err == nil {
		t.Fatal("expected error when no execution exists for session")
	}

	exec := &AgentExecution{ID: "exec-1", SessionID: "session-1"}
	manager.executionStore.Add(exec)
	if _, err := manager.ListProcesses(context.Background(), "session-1"); err == nil {
		t.Fatal("expected error when agentctl client is missing")
	}
}

func TestProcessRunnerStopErrors(t *testing.T) {
	manager := &Manager{executionStore: NewExecutionStore()}
	if err := manager.StopProcess(context.Background(), "process-1"); err == nil {
		t.Fatal("expected error when process not found")
	}
}

func TestProcessRunnerGetErrors(t *testing.T) {
	manager := &Manager{executionStore: NewExecutionStore()}
	if _, err := manager.GetProcess(context.Background(), "process-1", false); err == nil {
		t.Fatal("expected error when process not found")
	}
}
