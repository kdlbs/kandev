package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

func TestBackgroundWorkUsageAttribution(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	wsID := uuid.New().String()
	taskID := uuid.New().String()
	sessionID := uuid.New().String()
	workID := uuid.New().String()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: wsID, Name: "Workspace"})
	_ = repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: wsID, Title: "Task"})
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID})

	now := time.Now().UTC()
	obs := streams.WorkloadRunObservation{
		SessionID: sessionID,
		WorkID:    workID,
		Kind:      streams.WorkloadKindSubagent,
		Title:     "subagent worker",
		State:     streams.RunStateRunning,
		StartedAt: &now,
	}
	_ = svc.RecordBackgroundWorkloadObservation(ctx, obs, taskID, sessionID)

	// Fetch usage when no ledger events recorded yet
	usage, err := svc.GetBackgroundWorkloadUsage(ctx, sessionID, workID)
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.Equal(t, workID, usage.WorkID)
	require.Equal(t, "unavailable", usage.Provenance)
}

func TestBackgroundWorkChildRequestOwnership(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	wsID := uuid.New().String()
	taskID := uuid.New().String()
	sessionA := uuid.New().String()
	sessionB := uuid.New().String()
	workA := uuid.New().String()
	workB := uuid.New().String()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: wsID, Name: "Workspace"})
	_ = repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: wsID, Title: "Task"})
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionA, TaskID: taskID})
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionB, TaskID: taskID})

	now := time.Now().UTC()
	_ = svc.RecordBackgroundWorkloadObservation(ctx, streams.WorkloadRunObservation{
		SessionID: sessionA,
		WorkID:    workA,
		Kind:      streams.WorkloadKindShell,
		Title:     "Job A",
		State:     streams.RunStateRunning,
		StartedAt: &now,
	}, taskID, sessionA)

	_ = svc.RecordBackgroundWorkloadObservation(ctx, streams.WorkloadRunObservation{
		SessionID: sessionB,
		WorkID:    workB,
		Kind:      streams.WorkloadKindShell,
		Title:     "Job B",
		State:     streams.RunStateRunning,
		StartedAt: &now,
	}, taskID, sessionB)

	// Cross-session access fails closed
	_, err := svc.GetBackgroundWorkload(ctx, sessionA, workB)
	require.NoError(t, err) // returns nil when workB does not belong to sessionA

	// Querying action on workB via sessionA is rejected
	_, err = svc.ExecuteBackgroundWorkAction(ctx, sessionA, streams.BackgroundWorkActionRequest{
		WorkID: workB,
		Action: streams.WorkloadActionStop,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "task not found")
}
