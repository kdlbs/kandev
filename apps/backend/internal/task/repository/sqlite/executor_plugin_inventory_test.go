package sqlite

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

func TestPluginExecutorInventoryCAS(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedPluginExecutorInventory(t, repo, 7)

	first := pluginExecutorRunningFixture("execution-one", 7, "allocating")
	first.Metadata["unrelated"] = map[string]interface{}{"owner": "launch"}
	if err := repo.CheckpointPluginExecutorInventory(ctx, first); err != nil {
		t.Fatalf("initial checkpoint: %v", err)
	}
	if err := repo.UpdateResumeToken(ctx, first.SessionID, first.AgentExecutionID, "resume-preserved", "message-1"); err != nil {
		t.Fatalf("UpdateResumeToken: %v", err)
	}

	const updates = 12
	var group sync.WaitGroup
	errs := make(chan error, updates*2)
	var succeeded int
	var succeededMu sync.Mutex
	for index := 0; index < updates; index++ {
		group.Add(2)
		go func(index int) {
			defer group.Done()
			err := repo.UpdateResumeToken(ctx, first.SessionID, first.AgentExecutionID, "resume-race", "message-race")
			errs <- err
		}(index)
		go func(index int) {
			defer group.Done()
			checkpoint := pluginExecutorRunningFixture(first.AgentExecutionID, 7, "ready")
			checkpoint.ExpectedPluginExecutorRevision = first.ExpectedPluginExecutorRevision
			checkpoint.Metadata["unrelated"] = map[string]interface{}{"owner": "stale snapshot"}
			checkpoint.Metadata["checkpoint_number"] = index
			err := repo.CheckpointPluginExecutorInventory(ctx, checkpoint)
			if err == nil {
				succeededMu.Lock()
				succeeded++
				succeededMu.Unlock()
			} else if !errors.Is(err, models.ErrExecutionRotated) {
				errs <- err
				return
			}
			errs <- nil
		}(index)
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent metadata/resume update: %v", err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("checkpoint updates accepted for one observed revision = %d, want 1", succeeded)
	}

	stored, err := repo.GetExecutorRunningBySessionID(ctx, first.SessionID)
	if err != nil {
		t.Fatalf("GetExecutorRunningBySessionID: %v", err)
	}
	if stored.AgentExecutionID != first.AgentExecutionID || stored.ResumeToken != "resume-race" || stored.LastMessageUUID != "message-race" {
		t.Fatalf("checkpoint changed lifecycle-owned resume fields: %+v", stored)
	}
	if stored.Metadata["plugin_executor"] == nil {
		t.Fatalf("plugin executor envelope was lost: %#v", stored.Metadata)
	}
	if owner, ok := stored.Metadata["unrelated"].(map[string]interface{}); !ok || owner["owner"] != "launch" {
		t.Fatalf("checkpoint replaced unrelated metadata from a stale snapshot: %#v", stored.Metadata["unrelated"])
	}

	settleAbsent := pluginExecutorRunningFixture(first.AgentExecutionID, 7, "absent")
	settleAbsent.ExpectedPluginExecutorRevision, err = pluginExecutorInventoryRevision(stored.Metadata)
	if err != nil {
		t.Fatalf("read current plugin inventory revision: %v", err)
	}
	if err := repo.CheckpointPluginExecutorInventory(ctx, settleAbsent); err != nil {
		t.Fatalf("settle inventory absent: %v", err)
	}
	staleRevision := settleAbsent.ExpectedPluginExecutorRevision - 1
	staleReady := pluginExecutorRunningFixture(first.AgentExecutionID, 7, "ready")
	staleReady.ExpectedPluginExecutorRevision = staleRevision
	if err := repo.CheckpointPluginExecutorInventory(ctx, staleReady); !errors.Is(err, models.ErrExecutionRotated) {
		t.Fatalf("stale ready checkpoint after cleanup = %v, want ErrExecutionRotated", err)
	}
	invalidTransition := pluginExecutorRunningFixture(first.AgentExecutionID, 7, "ready")
	invalidTransition.ExpectedPluginExecutorRevision = settleAbsent.ExpectedPluginExecutorRevision
	if err := repo.CheckpointPluginExecutorInventory(ctx, invalidTransition); !errors.Is(err, models.ErrExecutionRotated) {
		t.Fatalf("ready transition from absent = %v, want ErrExecutionRotated", err)
	}
	stored, err = repo.GetExecutorRunningBySessionID(ctx, first.SessionID)
	if err != nil {
		t.Fatalf("GetExecutorRunningBySessionID after stale checkpoint: %v", err)
	}
	envelope, ok := stored.Metadata["plugin_executor"].(map[string]interface{})
	if !ok || envelope["phase"] != "absent" {
		t.Fatalf("stale checkpoint resurrected cleaned resource: envelope=%#v", stored.Metadata["plugin_executor"])
	}

	staleExecution := pluginExecutorRunningFixture("execution-stale", 7, "stale")
	if err := repo.CheckpointPluginExecutorInventory(ctx, staleExecution); !errors.Is(err, models.ErrExecutionRotated) {
		t.Fatalf("stale execution checkpoint error = %v, want ErrExecutionRotated", err)
	}
	staleOwner := pluginExecutorRunningFixture(first.AgentExecutionID, 6, "stale-owner")
	if err := repo.CheckpointPluginExecutorInventory(ctx, staleOwner); !errors.Is(err, models.ErrExecutionRotated) {
		t.Fatalf("stale environment generation checkpoint error = %v, want ErrExecutionRotated", err)
	}
}

func seedPluginExecutorInventory(t *testing.T, repo *Repository, generation int64) {
	t.Helper()
	ctx := context.Background()
	const taskID = "task-plugin-executor-inventory"
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Plugin executor inventory"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "environment-plugin-executor-inventory", TaskID: taskID,
		ExecutorType: string(models.ExecutorTypePluginRemote), OwnershipGeneration: generation,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-plugin-executor-inventory", TaskID: taskID,
		TaskEnvironmentID: "environment-plugin-executor-inventory",
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
}

func pluginExecutorRunningFixture(executionID string, generation int64, phase string) *models.ExecutorRunning {
	return &models.ExecutorRunning{
		ID: "session-plugin-executor-inventory", SessionID: "session-plugin-executor-inventory",
		TaskID: "task-plugin-executor-inventory", ExecutorID: "exec-plugin-fixture",
		Runtime: agentruntime.RuntimePluginRemote, Status: models.ExecutorRunningStatusStarting,
		AgentExecutionID: executionID,
		Metadata: map[string]interface{}{
			"plugin_executor": map[string]interface{}{
				"environment_generation": generation,
				"phase":                  phase,
			},
		},
	}
}
