package handlers

import (
	"context"
	"errors"
	"maps"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// ceilingReconcileRepo keeps the task/queue/session rows in one small fake so
// the message admission path can be tested against the same authoritative
// values on every read. The embedded mock supplies the unrelated repository
// surface required by task.Service.
type ceilingReconcileRepo struct {
	mockRepository

	mu          sync.Mutex
	task        *models.Task
	session     *models.TaskSession
	deferred    map[string]interface{}
	deferredErr error
	stateWrites int
}

func (r *ceilingReconcileRepo) GetTask(_ context.Context, id string) (*models.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.task == nil || r.task.ID != id {
		return nil, errors.New("task not found")
	}
	task := *r.task
	task.Metadata = maps.Clone(r.task.Metadata)
	return &task, nil
}

func (r *ceilingReconcileRepo) GetTaskDeferredLaunch(
	_ context.Context, _ string,
) (map[string]interface{}, interface{}, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.deferredErr != nil {
		return nil, nil, r.deferredErr
	}
	return maps.Clone(r.deferred), nil, nil
}

func (r *ceilingReconcileRepo) GetTaskSession(_ context.Context, id string) (*models.TaskSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.session == nil || r.session.ID != id {
		return nil, errors.New("session not found")
	}
	session := *r.session
	session.Metadata = maps.Clone(r.session.Metadata)
	return &session, nil
}

func (r *ceilingReconcileRepo) UpdateTaskStateIfCurrentIn(
	_ context.Context, id string, state v1.TaskState, allowed []v1.TaskState,
) (v1.TaskState, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.task == nil || r.task.ID != id {
		return "", false, errors.New("task not found")
	}
	for _, candidate := range allowed {
		if r.task.State != candidate {
			continue
		}
		old := r.task.State
		r.task.State = state
		r.stateWrites++
		return old, true, nil
	}
	return r.task.State, false, nil
}

func newCeilingReconcileHandlers(t *testing.T, repo *ceilingReconcileRepo) *MessageHandlers {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	svc := service.NewService(service.Repos{
		Tasks:     repo,
		TaskRepos: repo,
		Sessions:  repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	return NewMessageHandlers(svc, nil, log)
}

func TestEnsureTaskInProgressFailsClosedWhenCeilingQueueReadFails(t *testing.T) {
	repo := &ceilingReconcileRepo{
		task: &models.Task{
			ID: "message-read-error", State: v1.TaskStateReview,
			CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		},
		deferredErr: errors.New("deferred launch storage unavailable"),
	}
	handlers := newCeilingReconcileHandlers(t, repo)

	task, err := handlers.ensureTaskInProgress(context.Background(), repo.task.ID, "session-1")
	require.Error(t, err)
	require.Nil(t, task)

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Equal(t, v1.TaskStateReview, repo.task.State)
	require.Zero(t, repo.stateWrites)
}

func TestEnsureTaskInProgressRepairsLegacyReviewQueuedCreatedSession(t *testing.T) {
	queuedAt := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	repo := &ceilingReconcileRepo{
		task: &models.Task{
			ID: "message-legacy-review", State: v1.TaskStateReview,
			CreatedAt: queuedAt, UpdatedAt: queuedAt,
		},
		session: &models.TaskSession{
			ID: "message-legacy-session", TaskID: "message-legacy-review",
			State: models.TaskSessionStateCreated,
		},
		deferred: models.CeilingRecordKeys(models.CeilingDeferral{
			Kind: models.CeilingLaunchStartCreated,
			Payload: map[string]interface{}{
				"session_id": "message-legacy-session",
			},
			Origin: "automatic", ReasonCode: "session_capacity",
			QueuedAt: queuedAt, Ceiling: 5, Population: 6, PopulationKnown: true,
		}),
	}
	handlers := newCeilingReconcileHandlers(t, repo)

	task, err := handlers.ensureTaskInProgress(
		context.Background(), repo.task.ID, repo.session.ID,
	)
	require.NoError(t, err)
	require.Equal(t, v1.TaskStateScheduling, task.State)

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Equal(t, v1.TaskStateScheduling, repo.task.State)
	require.Equal(t, 1, repo.stateWrites)
}

func TestEnsureTaskInProgressRepairsLegacyReviewWithoutSessionArgument(t *testing.T) {
	queuedAt := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	repo := &ceilingReconcileRepo{
		task: &models.Task{
			ID: "message-legacy-review-no-session", State: v1.TaskStateReview,
			CreatedAt: queuedAt, UpdatedAt: queuedAt,
		},
		session: &models.TaskSession{
			ID: "message-legacy-session-no-arg", TaskID: "message-legacy-review-no-session",
			State: models.TaskSessionStateCreated,
		},
		deferred: models.CeilingRecordKeys(models.CeilingDeferral{
			Kind:    models.CeilingLaunchStartCreated,
			Payload: map[string]interface{}{"session_id": "message-legacy-session-no-arg"},
			Origin:  "automatic", ReasonCode: "session_capacity", QueuedAt: queuedAt,
		}),
	}
	handlers := newCeilingReconcileHandlers(t, repo)

	task, err := handlers.ensureTaskInProgress(context.Background(), repo.task.ID, "")
	require.NoError(t, err)
	require.Equal(t, v1.TaskStateScheduling, task.State)

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Equal(t, v1.TaskStateScheduling, repo.task.State)
	require.Equal(t, 1, repo.stateWrites)
}
