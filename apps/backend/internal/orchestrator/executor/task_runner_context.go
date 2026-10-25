package executor

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type taskRunnerProfileExplicitKey struct{}

// WithTaskRunnerProfileExplicit records whether the launch caller supplied the
// task runner profile. A false value means the profile was resolved from task
// metadata and must be checked again when the session transaction takes the
// task lock.
func WithTaskRunnerProfileExplicit(ctx context.Context, explicit bool) context.Context {
	return context.WithValue(ctx, taskRunnerProfileExplicitKey{}, explicit)
}

func taskRunnerProfileExplicit(ctx context.Context, fallback bool) bool {
	if explicit, ok := ctx.Value(taskRunnerProfileExplicitKey{}).(bool); ok {
		return explicit
	}
	return fallback
}

func taskRunnerProfileID(task *v1.Task) string {
	if task == nil {
		return ""
	}
	profileID, _ := task.Metadata[models.MetaKeyExecutorProfileID].(string)
	return profileID
}

func taskRunnerResolution(task *v1.Task, executorProfileID string, explicit bool) (bool, string) {
	resolvedProfileID := taskRunnerProfileID(task)
	return !explicit && executorProfileID != "" && executorProfileID == resolvedProfileID, resolvedProfileID
}

func (e *Executor) reloadTaskForRunnerRetry(ctx context.Context, taskID string) (*v1.Task, string, error) {
	refreshed, err := e.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, "", err
	}
	if refreshed == nil {
		return nil, "", nil
	}
	task := refreshed.ToAPI()
	return task, taskRunnerProfileID(task), nil
}
