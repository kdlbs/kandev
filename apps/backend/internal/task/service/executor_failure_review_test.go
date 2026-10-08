package service

import (
	"context"
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/stretchr/testify/require"
)

type failurePagingRepository struct {
	repository.TaskRepository
	targets  []models.ExecutorObservationTarget
	observed int
}

func (r *failurePagingRepository) ListExecutorObservationTargets(_ context.Context, after string, limit int) ([]models.ExecutorObservationTarget, error) {
	start := 0
	for i, t := range r.targets {
		if t.Cursor == after {
			start = i + 1
		}
	}
	end := min(start+limit, len(r.targets))
	return r.targets[start:end], nil
}
func (r *failurePagingRepository) GetExecutorFailure(context.Context, string) (*models.ExecutorFailureEpisode, error) {
	return nil, nil
}
func (r *failurePagingRepository) ObserveExecutorFailure(ctx context.Context, _ models.ExecutorObservationTarget, _ *models.ExecutorObservation) (*models.ExecutorFailureEpisode, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	r.observed++
	return nil, false, nil
}

func TestExecutorInspectionTimeoutDoesNotStarveNextPage(t *testing.T) {
	f := newSweepFixture(t, time.Hour)
	repo := &failurePagingRepository{TaskRepository: f.svc.tasks}
	for i := range 17 {
		repo.targets = append(repo.targets, models.ExecutorObservationTarget{TaskID: "task-1", ResourceKey: fmt.Sprint(i), Cursor: fmt.Sprintf("cursor-%02d", i)})
	}
	f.svc.tasks = repo
	synctest.Test(t, func(t *testing.T) {
		f.svc.SetExecutorInspector(func(ctx context.Context, _ models.ExecutorObservationTarget) (*models.ExecutorObservation, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		})
		f.svc.ReconcileExecutorFailures(t.Context())
		require.Equal(t, 16, repo.observed, "inspection deadline must not discard the page")
		require.Equal(t, "cursor-15", f.svc.executorObservationCursor)
		f.svc.ReconcileExecutorFailures(t.Context())
		require.Equal(t, 17, repo.observed, "next tick must reach the next page")
		require.Empty(t, f.svc.executorObservationCursor)
	})
}

func TestExecutorInitialInspectionRunsInBackgroundLoop(t *testing.T) {
	f := newSweepFixture(t, time.Hour)
	require.NoError(t, f.rawRepo.CreateTaskEnvironment(t.Context(), &models.TaskEnvironment{ID: "env", TaskID: "task-1", OwnershipGeneration: 1, ExecutorType: "local_docker", Status: models.TaskEnvironmentStatusReady, ContainerID: "owned"}))
	started := make(chan struct{})
	done := make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	f.svc.SetExecutorInspector(func(ctx context.Context, _ models.ExecutorObservationTarget) (*models.ExecutorObservation, error) {
		close(started)
		<-ctx.Done()
		close(done)
		return nil, ctx.Err()
	})
	f.svc.StartSessionReconciliationLoop(ctx)
	select {
	case <-started:
		cancel()
		<-done
	case <-time.After(time.Second):
		t.Fatal("initial inspection did not start asynchronously")
	}
}
