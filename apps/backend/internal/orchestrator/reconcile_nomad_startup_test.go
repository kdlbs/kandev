package orchestrator

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/nomad"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

func TestStartupReconciliationRetainsUnresolvedNomadIntentForCreatedSession(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	seedTaskAndSession(t, repo, "task-nomad-created", "session-nomad-created", models.TaskSessionStateCreated)

	workspaceRoot := filepath.Join(t.TempDir(), "workspace")
	workspacePath, err := nomad.EnvironmentWorkspacePath(workspaceRoot, "environment-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	state := nomad.EnvironmentState{
		Version: 1, Revision: 1, EnvironmentID: "environment-1", Generation: 1,
		OperationID: "operation-1", InstallationID: "installation-1", ExecutionID: "execution-1",
		InputDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		JobID:       "job-1", Namespace: "default", Region: "global", Datacenter: "dc1",
		NodeID: "node-1", Platform: "darwin/arm64", SSHHost: "127.0.0.1", SSHUser: "fixture",
		SSHPort: 22, SSHHostFingerprint: "SHA256:fixture", WorkspaceRoot: workspaceRoot,
		WorkspacePath: workspacePath, AllocationRoot: filepath.Join(t.TempDir(), "alloc"),
		CPUMHz: 48, MemoryMB: 1024, Phase: "intent",
	}
	metadata := map[string]interface{}{
		"env_secret_id_AGENTCTL_BOOTSTRAP_NONCE": "nonce-ref-preserved",
		"env_secret_id_AGENTCTL_AUTH_TOKEN":      "token-ref-preserved",
	}
	if err := nomad.PutEnvironmentState(metadata, state); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	row := &models.ExecutorRunning{
		ID: "session-nomad-created", SessionID: "session-nomad-created", TaskID: "task-nomad-created",
		AgentExecutionID: "execution-1", ExecutorID: "executor-1", Runtime: agentruntime.RuntimeNomad,
		Status: models.ExecutorRunningStatusStarting, Metadata: metadata, CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.UpsertExecutorRunning(ctx, row); err != nil {
		t.Fatalf("upsert Nomad intent: %v", err)
	}
	agentManager := &mockAgentManager{}
	service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)

	service.reconcileSessionsOnStartup(ctx)

	stored, err := repo.GetExecutorRunningBySessionID(ctx, row.SessionID)
	if err != nil {
		t.Fatalf("startup reconciliation deleted unresolved Nomad intent: %v", err)
	}
	if stored.Runtime != agentruntime.RuntimeNomad || stored.AgentExecutionID != state.ExecutionID || stored.Status != models.ExecutorRunningStatusStarting {
		t.Fatalf("retained Nomad row identity/status = %+v", stored)
	}
	gotState, err := nomad.EnvironmentStateFromMetadata(stored.Metadata)
	if err != nil || gotState.Phase != "intent" || gotState.OperationID != state.OperationID || gotState.JobID != state.JobID {
		t.Fatalf("retained Nomad state = %+v, %v", gotState, err)
	}
	if stored.Metadata["env_secret_id_AGENTCTL_BOOTSTRAP_NONCE"] != "nonce-ref-preserved" || stored.Metadata["env_secret_id_AGENTCTL_AUTH_TOKEN"] != "token-ref-preserved" {
		t.Fatalf("startup reconciliation changed Nomad secret references: %#v", stored.Metadata)
	}
	if len(agentManager.stopAgentWithReasonArgs) != 0 {
		t.Fatalf("startup cleanup sent stop requests for unresolved Nomad runtime: %+v", agentManager.stopAgentWithReasonArgs)
	}
}

func TestStartupReconciliationRetainsNomadRowsAcrossTerminalAndUnknownSnapshots(t *testing.T) {
	tests := []struct {
		name         string
		sessionState models.TaskSessionState
		seedSession  bool
		phase        string
		validState   bool
	}{
		{name: "failed registered operation", sessionState: models.TaskSessionStateFailed, seedSession: true, phase: "registered", validState: true},
		{name: "completed allocated operation", sessionState: models.TaskSessionStateCompleted, seedSession: true, phase: "allocated", validState: true},
		{name: "missing session and unreadable envelope", seedSession: false, validState: false},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupTestRepo(t)
			ctx := context.Background()
			taskID := fmt.Sprintf("task-nomad-retain-%d", i)
			sessionID := fmt.Sprintf("session-nomad-retain-%d", i)
			if tt.seedSession {
				seedTaskAndSession(t, repo, taskID, sessionID, tt.sessionState)
			}
			metadata := map[string]interface{}{
				"env_secret_id_AGENTCTL_BOOTSTRAP_NONCE": "nonce-ref-preserved",
				"env_secret_id_AGENTCTL_AUTH_TOKEN":      "token-ref-preserved",
			}
			if tt.validState {
				workspaceRoot := filepath.Join(t.TempDir(), "workspace")
				workspacePath, err := nomad.EnvironmentWorkspacePath(workspaceRoot, fmt.Sprintf("environment-%d", i), 1)
				if err != nil {
					t.Fatal(err)
				}
				state := nomad.EnvironmentState{
					Version: 1, Revision: 1, EnvironmentID: fmt.Sprintf("environment-%d", i), Generation: 1,
					OperationID: fmt.Sprintf("operation-%d", i), InstallationID: fmt.Sprintf("installation-%d", i), ExecutionID: fmt.Sprintf("execution-%d", i),
					InputDigest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
					JobID:       fmt.Sprintf("job-%d", i), Namespace: "default", Region: "global", Datacenter: "dc1",
					NodeID: "node-1", Platform: "darwin/arm64", SSHHost: "127.0.0.1", SSHUser: "fixture",
					SSHPort: 22, SSHHostFingerprint: "SHA256:fixture", WorkspaceRoot: workspaceRoot,
					WorkspacePath: workspacePath, AllocationRoot: filepath.Join(t.TempDir(), "alloc"),
					CPUMHz: 48, MemoryMB: 1024, Phase: tt.phase,
				}
				if err := nomad.PutEnvironmentState(metadata, state); err != nil {
					t.Fatal(err)
				}
			}
			now := time.Now().UTC()
			row := &models.ExecutorRunning{
				ID: sessionID, SessionID: sessionID, TaskID: taskID,
				AgentExecutionID: fmt.Sprintf("execution-%d", i), ExecutorID: "executor-1", Runtime: agentruntime.RuntimeNomad,
				Status: models.ExecutorRunningStatusStarting, Metadata: metadata, CreatedAt: now, UpdatedAt: now,
			}
			if err := repo.UpsertExecutorRunning(ctx, row); err != nil {
				t.Fatalf("upsert Nomad row: %v", err)
			}
			agentManager := &mockAgentManager{}
			service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)

			service.reconcileSessionsOnStartup(ctx)

			stored, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
			if err != nil {
				t.Fatalf("startup reconciliation deleted canonical Nomad row: %v", err)
			}
			if stored.Runtime != agentruntime.RuntimeNomad || stored.AgentExecutionID != row.AgentExecutionID {
				t.Fatalf("startup reconciliation changed Nomad identity: runtime=%q execution=%q", stored.Runtime, stored.AgentExecutionID)
			}
			if stored.Metadata["env_secret_id_AGENTCTL_BOOTSTRAP_NONCE"] != "nonce-ref-preserved" || stored.Metadata["env_secret_id_AGENTCTL_AUTH_TOKEN"] != "token-ref-preserved" {
				t.Fatalf("startup reconciliation changed secret references: %#v", stored.Metadata)
			}
			if tt.validState {
				gotState, err := nomad.EnvironmentStateFromMetadata(stored.Metadata)
				if err != nil || gotState.Phase != tt.phase {
					t.Fatalf("startup reconciliation changed typed phase: phase=%q err=%v", gotState.Phase, err)
				}
			} else if _, err := nomad.EnvironmentStateFromMetadata(stored.Metadata); err == nil {
				t.Fatal("test expected incomplete Nomad state to remain unknown")
			}
			if len(agentManager.stopAgentWithReasonArgs) != 0 {
				t.Fatalf("startup reconciliation attempted remote cleanup: %+v", agentManager.stopAgentWithReasonArgs)
			}
		})
	}
}
