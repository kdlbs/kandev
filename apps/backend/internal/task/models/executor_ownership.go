package models

import (
	"context"
	"encoding/json"
	"errors"
)

type executorActorKey struct{}

// WithExecutorActor binds private-compute authority to a trusted source.
// An explicit empty actor preserves legacy shared-resource access only.
func WithExecutorActor(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, executorActorKey{}, userID)
}

func ExecutorActorFromContext(ctx context.Context) (string, bool) {
	actor, bound := ctx.Value(executorActorKey{}).(string)
	return actor, bound
}

var ErrExecutorOwnershipChanged = errors.New("executor ownership changed")
var ErrExecutorOwnershipInvalid = errors.New("executor ownership assignment is invalid")

// OptionalOwner distinguishes omission from an explicit null that shares a resource.
type OptionalOwner struct {
	Set   bool
	Value string
}

func (o *OptionalOwner) UnmarshalJSON(data []byte) error {
	var value *string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	o.Set = true
	o.Value = ""
	if value != nil {
		o.Value = *value
	}
	return nil
}

func (o OptionalOwner) MarshalJSON() ([]byte, error) {
	if o.Value == "" {
		return []byte("null"), nil
	}
	return json.Marshal(o.Value)
}

func (o OptionalOwner) IsZero() bool { return !o.Set }

// ExecutorOwnershipIntent binds a mutation to the ownership that authorized it.
type ExecutorOwnershipIntent struct {
	ExpectedOwnerUserID       string
	ExpectedParentOwnerUserID *string
	OwnerUserID               *string
}

type executorWorkspaceKey struct{}

func WithExecutorWorkspace(ctx context.Context, workspaceID string) context.Context {
	return context.WithValue(ctx, executorWorkspaceKey{}, workspaceID)
}
func ExecutorWorkspaceFromContext(ctx context.Context) string {
	workspaceID, _ := ctx.Value(executorWorkspaceKey{}).(string)
	return workspaceID
}
