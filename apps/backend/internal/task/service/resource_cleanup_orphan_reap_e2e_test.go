package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/proclive"
	"github.com/kandev/kandev/internal/task/models"
)

// TestOrphanReapEndToEndKillsRealChildAfterQuickChatDirRemoval exercises the
// full pipeline through executeTaskResourceCleanupJob with no fakes for the
// host snapshot, verifier, or signaler: a real child process is spawned with
// its cwd inside a quick-chat session directory, the directory is removed by
// the ordinary cleanup path, and this test asserts the real host-wide lsof/ps
// snapshotter finds the process, the real SIGTERM reaches it, and the durable
// outcome record reflects it. The child is a process this test itself owns
// (spawned here, killed by this pipeline), so signaling it is safe.
func TestOrphanReapEndToEndKillsRealChildAfterQuickChatDirRemoval(t *testing.T) {
	taskSvc, _, repo := createTestService(t)
	ctx := context.Background()

	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-reap-e2e", WorkspaceID: "ws-reap-e2e", Title: "reap e2e",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	quickChatRoot := t.TempDir()
	taskSvc.SetQuickChatDir(quickChatRoot)
	const sessionID = "sess-reap-e2e"
	sessionDir := filepath.Join(quickChatRoot, sessionID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	cmd := exec.Command("sleep", "60")
	cmd.Dir = sessionDir
	if err := cmd.Start(); err != nil {
		t.Fatalf("start real child process: %v", err)
	}
	childPID := cmd.Process.Pid
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})
	waitDone := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(waitDone)
	}()

	job := &models.TaskResourceCleanupJob{
		ID: "reap-e2e-job", TaskID: "task-reap-e2e",
		Trigger: models.TaskResourceCleanupTriggerDelete,
		State:   models.TaskResourceCleanupStatePending,
	}
	snapshot := &taskResourceCleanupSnapshot{
		Sessions: []*models.TaskSession{{ID: sessionID, TaskID: "task-reap-e2e"}},
	}

	err := taskSvc.executeTaskResourceCleanupJob(ctx, job, snapshot)
	if err != nil {
		t.Fatalf("executeTaskResourceCleanupJob: %v", err)
	}

	if _, statErr := os.Stat(sessionDir); !os.IsNotExist(statErr) {
		t.Fatalf("expected quick-chat session directory removed, stat err = %v", statErr)
	}

	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
		t.Fatal("real child process was not reaped within 5s of the job returning")
	}
	if alive, known := proclive.Alive(int64(childPID)); known && alive {
		t.Fatalf("expected child pid %d to be dead after the reap phase", childPID)
	}

	rec, ok := findOrphanReapRecord(snapshot, childPID)
	if !ok {
		t.Fatalf("expected a durable outcome record for pid %d, got %+v", childPID, snapshot.OrphanReapRecords)
	}
	if rec.Outcome != orphanReapOutcomeTerminated && rec.Outcome != orphanReapOutcomeKilled {
		t.Fatalf("expected pid %d recorded terminated or killed, got %+v", childPID, rec)
	}
}
