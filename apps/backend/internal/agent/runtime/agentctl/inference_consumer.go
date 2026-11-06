package client

import (
	"context"
	"errors"
	"fmt"
)

// InferenceIdentity carries server-resolved relationships, never admission keys.
// Each child receives its own server-issued attempt token independently of these.
type InferenceIdentity struct {
	TaskID             string
	SessionID          string
	LogicalProfileID   string
	ExecutionProfileID string
	CallID             string
	ReviewRunID        string
	ParentExecutionID  string
	OwnerKind          string
	ExecutorType       string
}

type inferenceIdentityKey struct{}

// WithInferenceIdentity installs verified caller relationships before fallback.
// Candidate selection must not replace LogicalProfileID or the parent call/run.
func WithInferenceIdentity(ctx context.Context, identity InferenceIdentity) context.Context {
	return context.WithValue(ctx, inferenceIdentityKey{}, identity)
}

func InferenceIdentityFromContext(ctx context.Context) InferenceIdentity {
	identity, _ := ctx.Value(inferenceIdentityKey{}).(InferenceIdentity)
	return identity
}

// InferenceCapacityError is a wait/refusal, not a provider or model failure.
// Callers must not bypass it through host fallback or dynamic candidate retry.
type InferenceCapacityError struct {
	Reason string
	Cause  error
}

func (e *InferenceCapacityError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("inference admission %s: %v", e.Reason, e.Cause)
	}
	return "inference admission " + e.Reason
}

func (e *InferenceCapacityError) Unwrap() error { return e.Cause }

func IsInferenceCapacityError(err error) bool {
	var refusal *InferenceCapacityError
	return errors.As(err, &refusal)
}
