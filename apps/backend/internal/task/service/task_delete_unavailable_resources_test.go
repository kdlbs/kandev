package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-DELETE-RECOVERY-002.2
func TestDeleteUnavailableDamagedCheckoutPath(t *testing.T) {
	for _, symlink := range []bool{false, true} {
		t.Run(map[bool]string{false: "file", true: "symlink"}[symlink], func(t *testing.T) {
			ctx := context.Background()
			svc, repo := setupOfficeTest(t)
			svc.StopTaskResourceCleanupWorker()
			fixture := newMissingWorktreeCleanupFixture(t, repo, "damaged-checkout")
			svc.SetWorktreeCleanup(fixture.manager)
			svc.SetEnvironmentDestroyer(&managerEnvironmentDestroyer{mgr: fixture.manager})
			path := fixture.worktreeA.Path
			if symlink {
				target := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(target, "sentinel"), []byte("keep"), 0600))
				if err := os.Symlink(target, path); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			} else {
				require.NoError(t, os.WriteFile(path, []byte("keep"), 0600))
			}
			original, err := os.Lstat(path)
			require.NoError(t, err)

			require.NoError(t, svc.DeleteTaskWithOptions(ctx, fixture.taskID, unavailableDeleteOptions(t)))
			_, err = repo.GetTask(ctx, fixture.taskID)
			require.Error(t, err)
			job := latestCleanupJob(t, repo, fixture.taskID, models.TaskResourceCleanupTriggerDelete)
			var snapshot taskResourceCleanupSnapshot
			require.NoError(t, json.Unmarshal([]byte(job.ResourceSnapshot), &snapshot))
			require.Contains(t, snapshot.CleanupPreparationErrors, fixture.worktreeA.ID)
			require.Empty(t, snapshot.WorktreeHeadOIDs[fixture.worktreeA.ID])
			require.Error(t, svc.processTaskResourceCleanupJob(ctx, job.ID))
			remaining, err := os.Lstat(path)
			require.NoError(t, err)
			require.True(t, os.SameFile(original, remaining))
			if symlink {
				content, readErr := os.ReadFile(filepath.Join(path, "sentinel"))
				require.NoError(t, readErr)
				require.Equal(t, "keep", string(content))
			} else {
				content, readErr := os.ReadFile(path)
				require.NoError(t, readErr)
				require.Equal(t, "keep", string(content))
			}
			require.NoDirExists(t, fixture.worktreeB.Path)
		})
	}
}

// @covers AC-TASKS-DELETE-RECOVERY-002.5
func TestDeleteUnavailableIndependentResources(t *testing.T) {
	for _, scenario := range []string{"stopped", "stop failed", "incomplete inventory", "stale claim"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			svc, repo := setupOfficeTest(t)
			svc.StopTaskResourceCleanupWorker()
			taskID, env := newReclaimTask(t, svc, "Independent cleanup")
			svc.SetWorktreeCleanup(&mixedUnavailableCleanup{})
			destroyer := &stubDestroyer{}
			svc.SetEnvironmentDestroyer(destroyer)
			reclaimer := &fakeSSHTaskDirReclaimer{}
			svc.SetSSHTaskDirReclaimer(reclaimer)
			root := t.TempDir()
			attachments, err := NewAttachmentService(repo, root, nil, accessTestLogger(t))
			require.NoError(t, err)
			svc.SetAttachmentService(attachments)
			attachment, err := attachments.Stage(ctx, "owner", "ws-1", "saved.txt", "text/plain", "resource", "", strings.NewReader("attachment"))
			require.NoError(t, err)
			require.NoError(t, attachments.Claim(ctx, "owner", "ws-1", taskID, "", []string{attachment.ID}))
			path := filepath.Join(root, "attachments", attachment.StorageKey)
			require.FileExists(t, path)
			require.NoError(t, repo.DeleteTask(ctx, taskID))
			snapshot := taskResourceCleanupSnapshot{
				CleanupVersion: 1, CleanupInventoryCaptured: true,
				AllowUnverifiedWorkspaceChanges: true, DiscardWorktreeChanges: true,
				TaskEnvironment: env,
				PriorTaskEnvironments: []*models.TaskEnvironment{{
					ID: "old-environment", TaskID: taskID, ContainerID: "old-container",
				}},
				Worktrees:   []*worktree.Worktree{{ID: "broken", TaskID: taskID, Path: filepath.Join(t.TempDir(), "checkout")}},
				SSHTaskDirs: []sshReclaimTarget{enabledReclaimTarget()},
			}
			if scenario == "stop failed" {
				svc.executionStopper = failingSessionStopper{}
				snapshot.StopTargets = []persistedTaskStopTarget{{SessionID: "still-running"}}
			}
			if scenario == "incomplete inventory" {
				snapshot.CleanupInventoryCaptured = false
			}
			payload, err := json.Marshal(snapshot)
			require.NoError(t, err)
			job := &models.TaskResourceCleanupJob{
				ID: "independent", OperationID: "independent", TaskID: taskID,
				State: models.TaskResourceCleanupStateRunning, Attempts: 1,
				Trigger: models.TaskResourceCleanupTriggerDelete, ResourceSnapshot: string(payload),
			}
			require.NoError(t, repo.CreateTaskResourceCleanupJob(ctx, job))
			if scenario == "stale claim" {
				job.Attempts++
			}
			err = svc.executeTaskResourceCleanupJob(ctx, job, &snapshot)
			require.Error(t, err)
			if scenario == "incomplete inventory" || scenario == "stale claim" {
				require.FileExists(t, path)
			} else {
				require.ErrorContains(t, err, "original workspace identity is unavailable")
				require.NoFileExists(t, path)
			}
			if scenario == "stop failed" {
				require.ErrorContains(t, err, "runtime stop operations failed")
			}
			if scenario == "stopped" {
				require.Equal(t, []string{"old-container"}, destroyer.containerCalls)
				require.Len(t, reclaimer.calls(), 1)
				require.Equal(t, sshReclaimOutcomeRemoved, snapshot.SSHTaskDirs[0].Outcome)
			} else {
				require.Empty(t, destroyer.containerCalls)
				require.Empty(t, reclaimer.calls())
			}
			require.False(t, snapshot.CompletedWorktrees["broken"])
		})
	}
}
