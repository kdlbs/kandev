package service

import (
	"context"

	"github.com/kandev/kandev/internal/office/shared"
	"github.com/kandev/kandev/internal/office/wakeup"
)

// The persisted routine identity is server-authored wake context. Resolve the
// registered description instead of mistaking a taskless wake for monitoring.
func (si *SchedulerIntegration) enrichRoutineWorkContext(ctx context.Context, pc *PromptContext, reason, snapshot string) {
	switch reason {
	case shared.RunReasonRoutineDispatchCron, shared.RunReasonRoutineDispatchEvent, shared.RunReasonRoutineDispatch:
	default:
		return
	}
	var payload wakeup.RoutinePayload
	if wakeup.UnmarshalPayload(snapshot, &payload) != nil || payload.RoutineID == "" {
		return
	}
	routine, err := si.svc.GetRoutine(ctx, payload.RoutineID)
	if err != nil || routine == nil {
		return
	}
	pc.RoutineID = routine.ID
	pc.RoutineName = routine.Name
	pc.RoutineDescription = routine.Description
}
