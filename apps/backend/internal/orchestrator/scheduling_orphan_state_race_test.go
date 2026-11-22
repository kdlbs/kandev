package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type orphanTurnReadBarrier struct {
	*sqliterepo.Repository
	read    chan struct{}
	release chan struct{}
}

func (r orphanTurnReadBarrier) ListTurnsBySession(ctx context.Context, id string) ([]*models.Turn, error) {
	turns, err := r.Repository.ListTurnsBySession(ctx, id)
	close(r.read)
	<-r.release
	return turns, err
}

func TestSchedulingOrphanRepairCannotSettleSuccessorEntry(t *testing.T) {
	for _, returnToSameStep := range []bool{false, true} {
		t.Run(map[bool]string{false: "different step", true: "later visit to same step"}[returnToSameStep], func(t *testing.T) {
			f := newOrphanInterruptionFixture(t)
			publication := &recordingTaskEventPublisher{}
			f.svc.SetTaskEventPublisher(publication)
			const taskID, sessionID = "orphan-stale-repair", "orphan-stale-repair-session"
			f.seedTask(t, taskID, orphanAutoStep, nil)
			f.seedSession(t, taskID, sessionID, models.TaskSessionStateWaitingForInput)
			_, err := f.repo.CreateTurnWithStepStamp(t.Context(), &models.Turn{
				ID: "old-accepted-turn", TaskID: taskID, TaskSessionID: sessionID,
			})
			require.NoError(t, err)
			barrier := orphanTurnReadBarrier{Repository: f.repo, read: make(chan struct{}), release: make(chan struct{})}
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(barrier.release) }) }
			t.Cleanup(func() {
				release()
				f.svc.lifecycleSweepWorkers.Wait()
			})
			f.svc.repo = barrier
			require.True(t, f.svc.startLifecycleSweepAsync(t.Context()))
			select {
			case <-barrier.read:
			case <-time.After(5 * time.Second):
				t.Fatal("startup recovery did not reach the turn-evidence boundary")
			}
			task := orphanTaskWithEntry(t, f, taskID)
			previousEntryID := task.WorkflowStepTransitionID
			task.WorkflowStepID = orphanHoldStep
			require.NoError(t, f.repo.UpdateTask(t.Context(), task))
			if returnToSameStep {
				task.WorkflowStepID = orphanAutoStep
				require.NoError(t, f.repo.UpdateTask(t.Context(), task))
			}
			require.NoError(t, f.repo.UpdateTaskState(t.Context(), taskID, v1.TaskStateScheduling))
			release()
			f.svc.lifecycleSweepWorkers.Wait()

			successor := orphanTaskWithEntry(t, f, taskID)
			require.Greater(t, successor.WorkflowStepTransitionID, previousEntryID)
			require.Equal(t, v1.TaskStateScheduling, successor.State, "old turn evidence settled a successor entry")
			require.Equal(t, task.WorkflowStepID, successor.WorkflowStepID)
			require.Equal(t, 0, orphanPromptCount(f))
			require.Equal(t, 0, f.launches.count())
			require.Empty(t, publication.stateChanges, "a superseded repair published a state change")
		})
	}
}
