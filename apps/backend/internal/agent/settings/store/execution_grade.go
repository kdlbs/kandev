package store

import "context"

type executionGradeUpdateKey struct{}

// WithExecutionGradeUpdate carries explicit partial-update presence to shared writes.
func WithExecutionGradeUpdate(ctx context.Context, grade *string) context.Context {
	return context.WithValue(ctx, executionGradeUpdateKey{}, grade)
}

func executionGradeUpdate(ctx context.Context) *string {
	grade, _ := ctx.Value(executionGradeUpdateKey{}).(*string)
	return grade
}
