package sqlite

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agent/nomad"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

func TestNomadExecutorInventoryPersistsAndCASMergesMetadata(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedNomadExecutorInventory(t, repo, 4)

	first := nomadExecutorRunningFixture("execution-one", 4, "intent")
	first.Metadata["unrelated"] = map[string]any{"owner": "launch"}
	if err := repo.CheckpointNomadExecutorInventory(ctx, first); err != nil {
		t.Fatalf("initial Nomad checkpoint: %v", err)
	}
	if first.ExpectedNomadExecutorRevision != 1 {
		t.Fatalf("initial revision = %d, want 1", first.ExpectedNomadExecutorRevision)
	}

	updated := nomadExecutorRunningFixture("execution-one", 4, "registered")
	updated.ExpectedNomadExecutorRevision = first.ExpectedNomadExecutorRevision
	// A recovered launch request needs this canonical, launch-only projection
	// to pass the executor boundary. It is not inventory authority: the typed
	// state and owner CAS remain the only persisted generation source.
	updated.Metadata["nomad_environment_generation"] = "4"
	if err := repo.CheckpointNomadExecutorInventory(ctx, updated); err != nil {
		t.Fatalf("update Nomad checkpoint: %v", err)
	}
	stored, err := repo.GetExecutorRunningBySessionID(ctx, first.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	state, err := nomad.EnvironmentStateFromMetadata(stored.Metadata)
	if err != nil || state.Phase != "registered" || state.Revision != 2 {
		t.Fatalf("stored Nomad state = %+v, %v", state, err)
	}
	if unrelated, ok := stored.Metadata["unrelated"].(map[string]interface{}); !ok || unrelated["owner"] != "launch" {
		t.Fatalf("checkpoint lost unrelated metadata: %#v", stored.Metadata)
	}
	if stored.ResumeToken != "resume-preserved" || stored.LastMessageUUID != "message-preserved" {
		t.Fatalf("checkpoint changed lifecycle-owned columns: %+v", stored)
	}
	if _, exists := stored.Metadata["nomad_environment_generation"]; exists {
		t.Fatal("checkpoint copied a launch-only generation hint into durable inventory instead of retaining typed state as authority")
	}

	stale := nomadExecutorRunningFixture("execution-one", 4, "ready")
	stale.ExpectedNomadExecutorRevision = first.ExpectedNomadExecutorRevision
	if err := repo.CheckpointNomadExecutorInventory(ctx, stale); !errors.Is(err, models.ErrExecutionRotated) {
		t.Fatalf("stale Nomad checkpoint error = %v, want ErrExecutionRotated", err)
	}
}

func TestNomadExecutorInventoryCASAddsOnlyImmutableRuntimeSecretReferences(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedNomadExecutorInventory(t, repo, 1)
	first := nomadExecutorRunningFixture("execution-one", 1, "intent")
	if err := repo.CheckpointNomadExecutorInventory(ctx, first); err != nil {
		t.Fatal(err)
	}

	updated := nomadExecutorRunningFixture("execution-one", 1, "intent")
	updated.ExpectedNomadExecutorRevision = first.ExpectedNomadExecutorRevision
	updated.Metadata["env_secret_id_AGENTCTL_BOOTSTRAP_NONCE"] = "secret-nonce-ref"
	updated.Metadata["env_secret_id_AGENTCTL_AUTH_TOKEN"] = "secret-auth-ref"
	updated.Metadata["agentctl_bootstrap_nonce"] = "must-not-persist"
	updated.Metadata["auth_token"] = "must-not-persist"
	if err := repo.CheckpointNomadExecutorInventory(ctx, updated); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.GetExecutorRunningBySessionID(ctx, first.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Metadata["env_secret_id_AGENTCTL_BOOTSTRAP_NONCE"] != "secret-nonce-ref" ||
		stored.Metadata["env_secret_id_AGENTCTL_AUTH_TOKEN"] != "secret-auth-ref" {
		t.Fatalf("secret references were not merged: %#v", stored.Metadata)
	}
	for _, key := range []string{"agentctl_bootstrap_nonce", "auth_token"} {
		if _, found := stored.Metadata[key]; found {
			t.Fatalf("raw secret value %q was persisted", key)
		}
	}

	rotated := nomadExecutorRunningFixture("execution-one", 1, "intent")
	rotated.ExpectedNomadExecutorRevision = updated.ExpectedNomadExecutorRevision
	rotated.Metadata["env_secret_id_AGENTCTL_BOOTSTRAP_NONCE"] = "rotated-ref"
	if err := repo.CheckpointNomadExecutorInventory(ctx, rotated); !errors.Is(err, models.ErrExecutionRotated) {
		t.Fatalf("credential reference rotation error = %v, want ErrExecutionRotated", err)
	}
}

func TestNomadExecutorInventoryOnlyOneCASWriterWins(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedNomadExecutorInventory(t, repo, 2)
	first := nomadExecutorRunningFixture("execution-one", 2, "intent")
	if err := repo.CheckpointNomadExecutorInventory(ctx, first); err != nil {
		t.Fatal(err)
	}

	const writers = 8
	var group sync.WaitGroup
	var mu sync.Mutex
	winners := 0
	for i := 0; i < writers; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			checkpoint := nomadExecutorRunningFixture("execution-one", 2, "registered")
			checkpoint.ExpectedNomadExecutorRevision = first.ExpectedNomadExecutorRevision
			err := repo.CheckpointNomadExecutorInventory(ctx, checkpoint)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				winners++
			} else if !errors.Is(err, models.ErrExecutionRotated) {
				t.Errorf("checkpoint error = %v", err)
			}
		}()
	}
	group.Wait()
	if winners != 1 {
		t.Fatalf("writers at one observed revision accepted = %d, want 1", winners)
	}
}

func TestListExecutorsRunningNomadRetainsTerminalInventory(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedNomadExecutorInventory(t, repo, 1)
	terminal := nomadExecutorRunningFixture("execution-one", 1, "cleanup_pending")
	terminal.Status = models.ExecutorRunningStatusStopped
	if err := repo.UpsertExecutorRunning(ctx, terminal); err != nil {
		t.Fatalf("seed stopped Nomad inventory: %v", err)
	}
	rows, err := repo.ListExecutorsRunningNomad(ctx)
	if err != nil {
		t.Fatalf("ListExecutorsRunningNomad: %v", err)
	}
	if len(rows) != 1 || rows[0].Status != models.ExecutorRunningStatusStopped {
		t.Fatalf("retained Nomad rows = %#v, want stopped cleanup inventory", rows)
	}
	state, err := nomad.EnvironmentStateFromMetadata(rows[0].Metadata)
	if err != nil || state.Phase != "cleanup_pending" {
		t.Fatalf("retained cleanup state = %+v, %v", state, err)
	}
}

func seedNomadExecutorInventory(t *testing.T, repo *Repository, generation int64) {
	t.Helper()
	ctx := context.Background()
	const taskID = "task-nomad-executor-inventory"
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Nomad executor inventory"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: "environment-nomad-inventory", TaskID: taskID, ExecutorType: string(models.ExecutorTypeNomad), OwnershipGeneration: generation}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-nomad-inventory", TaskID: taskID, TaskEnvironmentID: "environment-nomad-inventory"}); err != nil {
		t.Fatal(err)
	}
}

func nomadExecutorRunningFixture(executionID string, generation int64, phase string) *models.ExecutorRunning {
	workspaceRoot := "/var/lib/kandev/workspaces/env"
	workspacePath, err := nomad.EnvironmentWorkspacePath(workspaceRoot, "environment-nomad-inventory", generation)
	if err != nil {
		panic(err)
	}
	state := nomad.EnvironmentState{
		Version: 1, EnvironmentID: "environment-nomad-inventory", Generation: generation,
		OperationID: executionID, InstallationID: "installation-one", ExecutionID: executionID,
		InputDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		JobID:       "kandev-fixture-job", Namespace: "default", Region: "global", Datacenter: "dc1",
		NodeID: "node-b", Platform: "darwin/arm64", SSHHost: "worker-b.example", SSHUser: "kandev", SSHPort: 22,
		SSHHostFingerprint: "SHA256:abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJK",
		WorkspaceRoot:      workspaceRoot, WorkspacePath: workspacePath, AllocationRoot: "/var/lib/kandev/allocations/env",
		CPUMHz: 1500, MemoryMB: 4096, Phase: phase,
	}
	metadata := map[string]interface{}{}
	if err := nomad.PutEnvironmentState(metadata, state); err != nil {
		panic(err)
	}
	return &models.ExecutorRunning{
		ID: "session-nomad-inventory", SessionID: "session-nomad-inventory", TaskID: "task-nomad-executor-inventory",
		ExecutorID: "exec-nomad-fixture", Runtime: agentruntime.RuntimeNomad, Status: models.ExecutorRunningStatusStarting,
		AgentExecutionID: executionID, ResumeToken: "resume-preserved", LastMessageUUID: "message-preserved",
		Metadata: metadata,
	}
}
