package executor

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/task/models"
)

func (e *Executor) cleanupFailedResumeEnvironment(ctx, cleanupCtx context.Context, session *models.TaskSession, environment *models.TaskEnvironment, reserved bool, executionID string, cause error) error {
	if reserved {
		return errors.Join(cause, e.rollbackTaskEnvironmentReservation(cleanupCtx, environment, executionID))
	}
	e.cleanupUnstartedExecutionAfterPersistError(cleanupCtx, session.ID, executionID, cause)
	if !isCancellableResumeContext(ctx) || ctx.Err() == nil {
		e.markTaskEnvironmentMaterializationFailed(ctx, environment, session.ID)
	}
	return cause
}
