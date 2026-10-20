package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.1
// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.3
func TestArchiveTaskRemovesDirtyWorktreeAndTaskDirectory(t *testing.T) {
	for _, kind := range []string{"unstaged", "staged", "untracked", "ignored"} {
		t.Run(kind, func(t *testing.T) {
			svc, _, repo, _, source, wt := createArchiveReclaimFixture(t)
			root := filepath.Dir(wt.Path)
			other := filepath.Join(filepath.Dir(root), "other-task", "sentinel")
			if err := os.MkdirAll(filepath.Dir(other), 0o700); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{other, filepath.Join(root, "notes.txt")} {
				if err := os.WriteFile(path, []byte("output"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join(wt.Path, "README.md")
			if kind == "untracked" || kind == "ignored" {
				path = filepath.Join(wt.Path, "agent-output.txt")
			}
			if kind == "ignored" {
				if err := os.WriteFile(filepath.Join(wt.Path, ".gitignore"), []byte("agent-output.txt\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				runGitTestCmd(t, wt.Path, "add", ".gitignore")
				runGitTestCmd(t, wt.Path, "commit", "-m", "Ignore agent output")
			}
			if err := os.WriteFile(path, []byte("local work"), 0o600); err != nil {
				t.Fatal(err)
			}
			if kind == "staged" {
				runGitTestCmd(t, wt.Path, "add", "README.md")
			}
			archiveDirectoryTestTask(t, svc, repo, wt.TaskID, false)
			job := latestCleanupJob(t, repo, wt.TaskID, models.TaskResourceCleanupTriggerArchive)
			if job.State != models.TaskResourceCleanupStateSucceeded {
				t.Fatalf("cleanup = %s: %s", job.State, job.LastError)
			}
			for _, removed := range []string{wt.Path, root} {
				if _, err := os.Stat(removed); !os.IsNotExist(err) {
					t.Fatalf("archive left directory %s: %v", removed, err)
				}
			}
			for _, preserved := range []string{source, other} {
				if _, err := os.Stat(preserved); err != nil {
					t.Fatalf("unrelated path removed: %s: %v", preserved, err)
				}
			}
			env, err := repo.GetTaskEnvironmentByTaskID(t.Context(), wt.TaskID)
			if err != nil || env == nil || len(env.Repos) != 1 || env.Repos[0].DeletedAt == nil {
				t.Fatalf("removed worktree metadata = %+v: %v", env, err)
			}
		})
	}
}

// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.2
func TestAutoArchiveRemovesDirtyManagedWorktree(t *testing.T) {
	svc, _, repo, _, _, wt := createArchiveReclaimFixture(t)
	svc.StopTaskResourceCleanupWorker()
	if err := os.WriteFile(filepath.Join(wt.Path, "untracked.txt"), []byte("local work"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := repo.DB().ExecContext(t.Context(), `INSERT INTO workflow_steps (id,workflow_id,name,position,auto_archive_after_hours,created_at,updated_at) VALUES (?,?,?,?,?,?,?)`, "step", "wf-"+wt.TaskID, "Auto archive", 0, 1, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DB().ExecContext(t.Context(), `UPDATE tasks SET updated_at = ? WHERE id = ?`, now.Add(-48*time.Hour), wt.TaskID); err != nil {
		t.Fatal(err)
	}
	handoff := NewHandoffService(repo, repo, nil, nil, nil, nil)
	handoff.SetTaskResourceCleaner(svc)
	svc.SetAutoArchiveCoordinator(handoff)
	svc.runAutoArchive(t.Context())
	task, err := repo.GetTask(t.Context(), wt.TaskID)
	if err != nil || task.ArchivedAt == nil {
		t.Fatalf("scheduled archive = %+v: %v", task, err)
	}
	job := latestCleanupJob(t, repo, wt.TaskID, models.TaskResourceCleanupTriggerCascadeArchive)
	if err := svc.processTaskResourceCleanupJob(t.Context(), job.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(wt.Path)); !os.IsNotExist(err) {
		t.Fatalf("scheduled archive retained dirty task folder: %v", err)
	}
}
