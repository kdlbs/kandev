package models

import (
	"context"
)

// ReservedLaunchIdentity pins a server-owned launch before provider admission.
type ReservedLaunchIdentity struct {
	TaskID       string
	SessionID    string
	TurnID       string
	SubmissionID string
}

type reservedLaunchIdentityKey struct{}

func WithReservedLaunchIdentity(ctx context.Context, identity ReservedLaunchIdentity) context.Context {
	return context.WithValue(ctx, reservedLaunchIdentityKey{}, identity)
}

func ReservedLaunchIdentityFromContext(ctx context.Context) (ReservedLaunchIdentity, bool) {
	identity, ok := ctx.Value(reservedLaunchIdentityKey{}).(ReservedLaunchIdentity)
	return identity, ok && identity.TaskID != "" && identity.SessionID != "" && identity.TurnID != "" && identity.SubmissionID != ""
}

type explicitManualCreationKey struct{}

// WithExplicitManualCreation carries server-owned request origin to persistence.
func WithExplicitManualCreation(ctx context.Context, taskID string) context.Context {
	return context.WithValue(ctx, explicitManualCreationKey{}, taskID)
}

func ExplicitManualCreation(ctx context.Context, taskID string) bool {
	manualTaskID, _ := ctx.Value(explicitManualCreationKey{}).(string)
	return taskID != "" && manualTaskID == taskID
}
