package sqlite

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

// descriptorSeeder creates the task, environment and session rows for one
// environment number; the PostgreSQL variant also needs a workspace and workflow.
type descriptorSeeder func(t *testing.T, repo *Repository, n int)

func seedDescriptorEnvironmentSQLite(t *testing.T, repo *Repository, n int) {
	t.Helper()
	ctx := context.Background()
	taskID := fmt.Sprintf("task-descriptor-%d", n)
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: taskID}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	seedDescriptorEnvironmentRows(t, repo, n, taskID)
}

func seedDescriptorEnvironmentRows(t *testing.T, repo *Repository, n int, taskID string) {
	t.Helper()
	ctx := context.Background()
	environmentID := fmt.Sprintf("environment-descriptor-%d", n)
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environmentID, TaskID: taskID, ExecutorType: string(models.ExecutorTypePluginRemote), OwnershipGeneration: 7,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: fmt.Sprintf("session-descriptor-%d", n), TaskID: taskID, TaskEnvironmentID: environmentID,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
}

func descriptorFixture(n int, installation, phase, path, identity string) *models.ExecutorRunning {
	envelope := map[string]interface{}{
		"environment_generation": int64(7), "phase": phase, "installation_id": installation,
		"environment_id": fmt.Sprintf("environment-descriptor-%d", n), "platform": "linux-amd64",
	}
	if path != "" || identity != "" {
		envelope["workspace_descriptor"] = map[string]interface{}{"path": path, "identity": identity}
	}
	return &models.ExecutorRunning{
		ID: fmt.Sprintf("session-descriptor-%d", n), SessionID: fmt.Sprintf("session-descriptor-%d", n),
		TaskID: fmt.Sprintf("task-descriptor-%d", n), ExecutorID: "exec-plugin-fixture",
		Runtime: agentruntime.RuntimePluginRemote, Status: models.ExecutorRunningStatusStarting,
		AgentExecutionID: fmt.Sprintf("execution-descriptor-%d", n),
		Metadata:         map[string]interface{}{"plugin_executor": envelope},
	}
}

func checkpointDescriptor(t *testing.T, repo *Repository, running *models.ExecutorRunning, revision uint64) error {
	t.Helper()
	running.ExpectedPluginExecutorRevision = revision
	return repo.CheckpointPluginExecutorInventory(context.Background(), running)
}

func storedDescriptor(t *testing.T, repo *Repository, n int) map[string]interface{} {
	t.Helper()
	stored, err := repo.GetExecutorRunningBySessionID(context.Background(), fmt.Sprintf("session-descriptor-%d", n))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	envelope, _ := stored.Metadata["plugin_executor"].(map[string]interface{})
	descriptor, _ := envelope["workspace_descriptor"].(map[string]interface{})
	return descriptor
}

func exerciseDescriptorImmutability(t *testing.T, repo *Repository, seed descriptorSeeder) {
	t.Helper()
	seed(t, repo, 1)
	if err := checkpointDescriptor(t, repo, descriptorFixture(1, "install-a", "allocating", "", ""), 0); err != nil {
		t.Fatalf("allocating: %v", err)
	}
	if err := checkpointDescriptor(t, repo, descriptorFixture(1, "install-a", "provisioned", "/srv/work", "worker-1"), 1); err != nil {
		t.Fatalf("first descriptor write: %v", err)
	}
	if err := checkpointDescriptor(t, repo, descriptorFixture(1, "install-a", "ready", "/srv/work", "worker-1"), 2); err != nil {
		t.Fatalf("the same descriptor must be accepted again: %v", err)
	}
	for name, mutate := range map[string]*models.ExecutorRunning{
		"different path":     descriptorFixture(1, "install-a", "ready", "/srv/other", "worker-1"),
		"different identity": descriptorFixture(1, "install-a", "ready", "/srv/work", "worker-2"),
		"dropped descriptor": descriptorFixture(1, "install-a", "ready", "", ""),
	} {
		if err := checkpointDescriptor(t, repo, mutate, 3); !errors.Is(err, models.ErrExecutorConflict) {
			t.Fatalf("%s: err = %v, want conflict", name, err)
		}
		if got := storedDescriptor(t, repo, 1); got["path"] != "/srv/work" || got["identity"] != "worker-1" {
			t.Fatalf("%s: a refused write changed the stored descriptor: %v", name, got)
		}
	}
	if err := checkpointDescriptor(t, repo, descriptorFixture(1, "install-a", "cleanup_pending", "", ""), 3); err != nil {
		t.Fatalf("a terminal cleanup write must never be blocked by the descriptor: %v", err)
	}
}

func exerciseDescriptorUniqueness(t *testing.T, repo *Repository, seed descriptorSeeder) {
	t.Helper()
	seed(t, repo, 1)
	seed(t, repo, 2)
	seed(t, repo, 3)
	for n := 1; n <= 3; n++ {
		if err := checkpointDescriptor(t, repo, descriptorFixture(n, "install-a", "allocating", "", ""), 0); err != nil {
			t.Fatalf("allocating %d: %v", n, err)
		}
	}
	if err := checkpointDescriptor(t, repo, descriptorFixture(1, "install-a", "provisioned", "/srv/work", "worker-1"), 1); err != nil {
		t.Fatalf("first environment: %v", err)
	}
	for name, collide := range map[string]*models.ExecutorRunning{
		"same normalized path": descriptorFixture(2, "install-a", "provisioned", "/srv//work/", "worker-2"),
		"same identity":        descriptorFixture(2, "install-a", "provisioned", "/srv/elsewhere", "worker-1"),
	} {
		if err := checkpointDescriptor(t, repo, collide, 1); !errors.Is(err, models.ErrExecutorConflict) {
			t.Fatalf("%s: err = %v, want conflict", name, err)
		}
		if storedDescriptor(t, repo, 2) != nil {
			t.Fatalf("%s: the losing environment recorded a descriptor", name)
		}
	}
	if err := checkpointDescriptor(t, repo, descriptorFixture(3, "install-b", "provisioned", "/srv/work", "worker-1"), 1); err != nil {
		t.Fatalf("another installation may reuse the path and identity: %v", err)
	}
	if err := checkpointDescriptor(t, repo, descriptorFixture(2, "install-a", "provisioned", "/srv/second", "worker-2"), 1); err != nil {
		t.Fatalf("distinct descriptor: %v", err)
	}
	if got := storedDescriptor(t, repo, 1); got["path"] != "/srv/work" {
		t.Fatalf("a second environment altered the first one's descriptor: %v", got)
	}
	if err := checkpointDescriptor(t, repo, descriptorFixture(1, "install-a", "cleanup_pending", "/srv/work", "worker-1"), 2); err != nil {
		t.Fatalf("terminal transition: %v", err)
	}
	if err := checkpointDescriptor(t, repo, descriptorFixture(2, "install-a", "ready", "/srv/second", "worker-2"), 2); err != nil {
		t.Fatalf("a terminal environment must not block the other: %v", err)
	}
}

func exerciseDescriptorRace(t *testing.T, repoA, repoB *Repository, seed descriptorSeeder) {
	t.Helper()
	seed(t, repoA, 1)
	seed(t, repoA, 2)
	for n := 1; n <= 2; n++ {
		if err := checkpointDescriptor(t, repoA, descriptorFixture(n, "install-race", "allocating", "", ""), 0); err != nil {
			t.Fatalf("allocating %d: %v", n, err)
		}
	}
	results := make([]error, 2)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i, repo := range []*Repository{repoA, repoB} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i] = checkpointDescriptor(t, repo, descriptorFixture(i+1, "install-race", "provisioned", "/srv/shared", fmt.Sprintf("worker-%d", i+1)), 1)
		}()
	}
	close(start)
	wg.Wait()
	wins, conflicts := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, models.ErrExecutorConflict):
			conflicts++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("results = %v; exactly one environment may commit the shared path", results)
	}
}

func TestPluginWorkspaceDescriptorImmutable(t *testing.T) {
	exerciseDescriptorImmutability(t, newRepoForSessionTests(t), seedDescriptorEnvironmentSQLite)
}

func TestPluginWorkspaceDescriptorUniquePerInstallation(t *testing.T) {
	exerciseDescriptorUniqueness(t, newRepoForSessionTests(t), seedDescriptorEnvironmentSQLite)
}

func TestPluginWorkspaceDescriptorRace(t *testing.T) {
	repo := newRepoForSessionTests(t)
	exerciseDescriptorRace(t, repo, repo, seedDescriptorEnvironmentSQLite)
}
