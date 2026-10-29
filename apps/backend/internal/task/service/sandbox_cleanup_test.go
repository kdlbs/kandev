package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestTaskCleanupDeletesSandboxStateOnlyForDeletedSessions(t *testing.T) {
	for _, mode := range []string{"delete", "archive", "retain"} {
		t.Run(mode, func(t *testing.T) {
			svc, repo := setupOfficeTest(t)
			svc.StopTaskResourceCleanupWorker()
			svc.quickChatDir = t.TempDir()
			created, err := svc.CreateTask(t.Context(), &CreateTaskRequest{WorkspaceID: "ws-1", Title: "Sandbox cleanup", ProjectID: "proj-1"})
			require.NoError(t, err)
			taskID := created.Task.ID
			trigger := models.TaskResourceCleanupTriggerDelete
			if mode == "archive" {
				require.NoError(t, repo.ArchiveTask(t.Context(), taskID))
				trigger = models.TaskResourceCleanupTriggerArchive
			}
			job := &models.TaskResourceCleanupJob{ID: "sandbox-job", OperationID: "delete:sandbox", TaskID: taskID, Trigger: trigger, State: models.TaskResourceCleanupStateRunning}
			snapshot := &taskResourceCleanupSnapshot{ArchiveSourceManifestCaptured: true, Sessions: []*models.TaskSession{{ID: "session", TaskID: taskID}}}
			if mode == "retain" {
				snapshot.WorkspaceCleanup = models.WorkspaceCleanupRetain
			}
			raw, err := json.Marshal(snapshot)
			require.NoError(t, err)
			job.ResourceSnapshot = string(raw)
			require.NoError(t, repo.CreateTaskResourceCleanupJob(t.Context(), job))
			if mode != "archive" {
				require.NoError(t, repo.DeleteTask(t.Context(), taskID))
			}
			blocked := errors.New("sandbox ownership uncertain")
			fail := true
			calls := 0
			svc.SetSessionSandboxCleanup(func(_ context.Context, task, session string) error {
				calls++
				require.Equal(t, taskID, task)
				require.Equal(t, "session", session)
				if fail {
					return blocked
				}
				return nil
			})
			err = svc.executeTaskResourceCleanupJob(t.Context(), job, snapshot)
			if mode == "delete" {
				require.Equal(t, 1, calls)
				require.ErrorIs(t, err, blocked)
				fail = false
				require.NoError(t, svc.executeTaskResourceCleanupJob(t.Context(), job, snapshot))
				require.Equal(t, 2, calls)
			} else {
				require.NoError(t, err)
				require.Zero(t, calls)
			}
		})
	}
}

func TestTaskSandboxCleanupWaitsForSuccessfulRuntimeStop(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "stopped", true: "failed stop"}[fail], func(t *testing.T) {
			svc, _, repo := createTestService(t)
			svc.StopTaskResourceCleanupWorker()
			svc.quickChatDir = t.TempDir()
			require.NoError(t, repo.CreateTask(t.Context(), &models.Task{ID: "task", WorkspaceID: "workspace", Title: "Cleanup"}))
			require.NoError(t, repo.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{ID: "runtime", SessionID: "session", TaskID: "task", ExecutorID: "local", Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusRunning}))
			stopper := &stubStopper{}
			if fail {
				stopper.stopSessionErr = errors.New("stop unavailable")
			}
			svc.executionStopper = stopper
			calls := 0
			svc.SetSessionSandboxCleanup(func(context.Context, string, string) error { calls++; return nil })
			job := &models.TaskResourceCleanupJob{ID: "job", TaskID: "task", Trigger: models.TaskResourceCleanupTriggerDelete}
			err := svc.executeTaskResourceCleanupJob(t.Context(), job, &taskResourceCleanupSnapshot{ArchiveSourceManifestCaptured: true})
			require.Equal(t, 1, stopper.stopSessionCalls)
			if fail {
				require.Error(t, err)
				require.Zero(t, calls)
			} else {
				require.NoError(t, err)
				require.Equal(t, 1, calls)
			}
		})
	}
}
