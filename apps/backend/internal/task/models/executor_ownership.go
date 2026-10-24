package models

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kandev/kandev/internal/auth/authn"
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

// ExecutorSourceCreator retains explicit durable attribution, including an
// empty actor for legacy shared access, before considering an interactive user.
func ExecutorSourceCreator(ctx context.Context) string {
	if actor, bound := ExecutorActorFromContext(ctx); bound {
		return actor
	}
	if identity, ok := authn.IdentityFromContext(ctx); ok && !identity.Synthetic {
		return identity.UserID
	}
	return ""
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
