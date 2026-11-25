package shared

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
)

// TaskStateWriter applies state transitions through the task repository so
// completion gates and workflow-step checks stay atomic with each write.
type TaskStateWriter interface {
	CommitTaskState(ctx context.Context, taskID string, write models.TaskStateWrite) (models.TaskStateChange, error)
}

// TaskStateBatchWriter commits a group of task state changes and related
// checkout cleanup atomically.
type TaskStateBatchWriter interface {
	CommitTaskStateBatch(ctx context.Context, write models.TaskStateBatchWrite) ([]models.TaskStateChange, error)
}

// TaskStateChangePublisher publishes one committed state receipt through the
// task service's canonical event path.
type TaskStateChangePublisher interface {
	PublishCommittedTaskStateChange(ctx context.Context, change models.TaskStateChange, activityHandled bool)
}
