package scheduler

import (
	"context"

	"github.com/kandev/kandev/internal/office/service"
)

// BuildWakePayload constructs the JSON wake payload for a run request.
// Delegates to service.Service.BuildWakePayload.
// Returns an empty string when no task_id is present in the run payload.
func (ss *SchedulerService) BuildWakePayload(ctx context.Context, payload string) (string, error) {
	return ss.svc.BuildWakePayload(ctx, &service.RunPayloadInput{Payload: payload})
}
