package runtime

import (
	"context"

	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

type InferenceIdentity = client.InferenceIdentity
type InferenceCapacityError = client.InferenceCapacityError

func WithInferenceIdentity(ctx context.Context, identity InferenceIdentity) context.Context {
	return client.WithInferenceIdentity(ctx, identity)
}

func InferenceIdentityFromContext(ctx context.Context) InferenceIdentity {
	return client.InferenceIdentityFromContext(ctx)
}

func IsInferenceCapacityError(err error) bool { return client.IsInferenceCapacityError(err) }
