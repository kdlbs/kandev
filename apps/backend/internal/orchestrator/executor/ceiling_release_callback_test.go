package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestMarkCompletedBySession_ReleasesTheCeilingReservation pins AC-51a: this
// write reaches the repository directly, bypassing every onSessionStateChange
// funnel the orchestrator package instruments itself, so the executor must
// invoke its own dependency-inverted release callback after a successful write.
func TestMarkCompletedBySession_ReleasesTheCeilingReservation(t *testing.T) {
	repo := newMockRepository()
	repo.sessions["session-1"] = &models.TaskSession{ID: "session-1", State: models.TaskSessionStateRunning}
	exec := newTestExecutor(t, nil, repo)

	var released []string
	exec.SetOnCeilingReservationRelease(func(sessionID string) { released = append(released, sessionID) })

	exec.MarkCompletedBySession(context.Background(), "session-1", v1.TaskSessionStateCompleted)

	require.Equal(t, []string{"session-1"}, released)
}

// TestMarkCompletedBySession_NoCallbackIsANoOp pins the nil-safety every other
// optional Executor callback field already follows.
func TestMarkCompletedBySession_NoCallbackIsANoOp(t *testing.T) {
	repo := newMockRepository()
	repo.sessions["session-2"] = &models.TaskSession{ID: "session-2", State: models.TaskSessionStateRunning}
	exec := newTestExecutor(t, nil, repo)

	require.NotPanics(t, func() {
		exec.MarkCompletedBySession(context.Background(), "session-2", v1.TaskSessionStateCompleted)
	})
}

// TestMarkCompletedBySession_WriteFailureDoesNotRelease pins that the release
// only fires after a successful write: an update failure must not free a
// reservation for a launch whose session state was never actually settled.
func TestMarkCompletedBySession_WriteFailureDoesNotRelease(t *testing.T) {
	repo := newMockRepository()
	repo.updateTaskSessionStateFunc = func(context.Context, string, models.TaskSessionState, string) error {
		return errors.New("update failed")
	}
	exec := newTestExecutor(t, nil, repo)

	var released []string
	exec.SetOnCeilingReservationRelease(func(sessionID string) { released = append(released, sessionID) })

	exec.MarkCompletedBySession(context.Background(), "session-3", v1.TaskSessionStateCompleted)

	require.Empty(t, released)
}
