package models

import "context"

type taskCapacityRunKey struct{}

// TaskCapacityRunOwner is supplied only by the existing Office dispatcher.
// Admission still validates these IDs against the real claimed queue owner.
type TaskCapacityRunOwner struct{ RunID, TaskID string }

func WithTaskCapacityRun(ctx context.Context, runID, taskID string) context.Context {
	if runID == "" || taskID == "" {
		return ctx
	}
	return context.WithValue(ctx, taskCapacityRunKey{}, TaskCapacityRunOwner{RunID: runID, TaskID: taskID})
}

func TaskCapacityRunFromContext(ctx context.Context) (TaskCapacityRunOwner, bool) {
	owner, ok := ctx.Value(taskCapacityRunKey{}).(TaskCapacityRunOwner)
	return owner, ok
}

func MatchesTaskCapacityRun(ctx context.Context, run *Run) bool {
	owner, ok := TaskCapacityRunFromContext(ctx)
	return ok && run != nil && owner.RunID == run.ID && owner.TaskID == PayloadTaskID(run.Payload)
}
