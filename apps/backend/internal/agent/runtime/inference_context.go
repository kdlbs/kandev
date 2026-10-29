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

// Inference transport types retain their native implementation owner while
// higher-level capacity and host callers depend on the public runtime seam.
type InferenceClient = client.Client
type InferenceClientRef = client.InferenceClientRef
type InferenceConsumer = client.InferenceConsumer
type InferenceAttemptError = client.InferenceAttemptError
type InferenceClientOption = client.ClientOption

var ErrInferenceOwnershipUnsupported = client.ErrInferenceOwnershipUnsupported

func WithInferenceExecutionID(id string) InferenceClientOption {
	return client.WithExecutionID(id)
}
