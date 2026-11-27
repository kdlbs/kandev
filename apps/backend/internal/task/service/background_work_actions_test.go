package service

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

type mockActionDispatcher struct {
	mu         sync.Mutex
	dispatched []streams.BackgroundWorkActionRequest
	err        error
	resp       streams.BackgroundWorkActionResponse
}

func (m *mockActionDispatcher) ExecuteBackgroundWorkAction(
	_ context.Context,
	_ string,
	req streams.BackgroundWorkActionRequest,
) (streams.BackgroundWorkActionResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dispatched = append(m.dispatched, req)
	if m.err != nil {
		return m.resp, m.err
	}
	return m.resp, nil
}

func TestBackgroundWorkActionAuthorization(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	wsID := uuid.New().String()
	taskID := uuid.New().String()
	sessionID := uuid.New().String()
	workID := uuid.New().String()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: wsID, Name: "A Workspace", OwnerID: "user-a"})
	_ = repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: wsID, Title: "Task"})
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID})

	now := time.Now().UTC()
	obs := streams.WorkloadRunObservation{
		SessionID: sessionID,
		WorkID:    workID,
		Kind:      streams.WorkloadKindShell,
		Title:     "npm test",
		State:     streams.RunStateRunning,
		StartedAt: &now,
	}
	_ = svc.RecordBackgroundWorkloadObservation(ctx, obs, taskID, sessionID)

	dispatcher := &mockActionDispatcher{
		resp: streams.BackgroundWorkActionResponse{
			Success: true,
			WorkID:  workID,
			Action:  streams.WorkloadActionStop,
		},
	}
	svc.SetBackgroundWorkActionDispatcher(dispatcher)

	// Unauthorized user access is denied
	otherCtx := ctxAs("user-b")
	_, err := svc.ExecuteBackgroundWorkAction(otherCtx, sessionID, streams.BackgroundWorkActionRequest{
		WorkID: workID,
		Action: streams.WorkloadActionStop,
	})
	require.Error(t, err)
	require.Empty(t, dispatcher.dispatched)

	// Owner access succeeds
	ownerCtx := ctxAs("user-a")
	resp, err := svc.ExecuteBackgroundWorkAction(ownerCtx, sessionID, streams.BackgroundWorkActionRequest{
		WorkID: workID,
		Action: streams.WorkloadActionStop,
	})
	require.NoError(t, err)
	require.True(t, resp.Success)
	require.Len(t, dispatcher.dispatched, 1)
}

func TestBackgroundWorkActionStaleRun(t *testing.T) {
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
		SessionID:  sessionID,
		WorkID:     workID,
		Kind:       streams.WorkloadKindShell,
		Title:      "finished command",
		State:      streams.RunStateCompleted,
		StartedAt:  &now,
		FinishedAt: &now,
	}
	_ = svc.RecordBackgroundWorkloadObservation(ctx, obs, taskID, sessionID)

	dispatcher := &mockActionDispatcher{
		resp: streams.BackgroundWorkActionResponse{
			Success: true,
			WorkID:  workID,
			Action:  streams.WorkloadActionStop,
		},
	}
	svc.SetBackgroundWorkActionDispatcher(dispatcher)

	// Action on completed/stale workload fails
	_, err := svc.ExecuteBackgroundWorkAction(ctx, sessionID, streams.BackgroundWorkActionRequest{
		WorkID: workID,
		Action: streams.WorkloadActionStop,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not running")
	require.Empty(t, dispatcher.dispatched)
}

func TestBackgroundWorkActionReceiptReplay(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	wsID := uuid.New().String()
	taskID := uuid.New().String()
	sessionID := uuid.New().String()
	workID := uuid.New().String()
	opID := uuid.New().String()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: wsID, Name: "Workspace"})
	_ = repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: wsID, Title: "Task"})
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID})

	now := time.Now().UTC()
	obs := streams.WorkloadRunObservation{
		SessionID: sessionID,
		WorkID:    workID,
		Kind:      streams.WorkloadKindShell,
		Title:     "running job",
		State:     streams.RunStateRunning,
		StartedAt: &now,
	}
	_ = svc.RecordBackgroundWorkloadObservation(ctx, obs, taskID, sessionID)

	dispatcher := &mockActionDispatcher{
		resp: streams.BackgroundWorkActionResponse{
			Success: true,
			WorkID:  workID,
			Action:  streams.WorkloadActionStop,
		},
	}
	svc.SetBackgroundWorkActionDispatcher(dispatcher)

	// 1. First execution creates receipt
	resp1, err := svc.ExecuteBackgroundWorkAction(ctx, sessionID, streams.BackgroundWorkActionRequest{
		WorkID:      workID,
		Action:      streams.WorkloadActionStop,
		OperationID: opID,
	})
	require.NoError(t, err)
	require.True(t, resp1.Success)
	require.Len(t, dispatcher.dispatched, 1)

	// 2. Replay with same OperationID returns recorded receipt without redispatching
	resp2, err := svc.ExecuteBackgroundWorkAction(ctx, sessionID, streams.BackgroundWorkActionRequest{
		WorkID:      workID,
		Action:      streams.WorkloadActionStop,
		OperationID: opID,
	})
	require.NoError(t, err)
	require.True(t, resp2.Success)
	require.Len(t, dispatcher.dispatched, 1, "dispatcher should NOT be called again on replay")

	// 3. Conflict: same OperationID with different workID is rejected
	_, err = svc.ExecuteBackgroundWorkAction(ctx, sessionID, streams.BackgroundWorkActionRequest{
		WorkID:      "different-work-id",
		Action:      streams.WorkloadActionStop,
		OperationID: opID,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "conflict")
}

func TestBackgroundWorkInputUncertain(t *testing.T) {
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
		Kind:      streams.WorkloadKindShell,
		Title:     "stdin job",
		State:     streams.RunStateRunning,
		StartedAt: &now,
	}
	_ = svc.RecordBackgroundWorkloadObservation(ctx, obs, taskID, sessionID)

	// 1. Input exceeding 64KB is rejected immediately
	largeInput := strings.Repeat("x", 65*1024)
	_, err := svc.ExecuteBackgroundWorkAction(ctx, sessionID, streams.BackgroundWorkActionRequest{
		WorkID: workID,
		Action: streams.WorkloadActionWriteInput,
		Data:   largeInput,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds 64KB")

	// 2. Timeout marks uncertain receipt
	dispatcher := &mockActionDispatcher{
		err: context.DeadlineExceeded,
	}
	svc.SetBackgroundWorkActionDispatcher(dispatcher)

	opID := uuid.New().String()
	_, err = svc.ExecuteBackgroundWorkAction(ctx, sessionID, streams.BackgroundWorkActionRequest{
		WorkID:      workID,
		Action:      streams.WorkloadActionWriteInput,
		Data:        "normal input",
		OperationID: opID,
	})
	require.Error(t, err)

	receipt, err := repo.GetBackgroundActionReceipt(ctx, sessionID, opID)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.True(t, receipt.Uncertain)
	require.Equal(t, "failed", receipt.Status)
}
