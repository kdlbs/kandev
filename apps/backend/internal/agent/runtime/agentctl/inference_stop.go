package client

import (
	"context"
	"encoding/json"
	"io"

	"github.com/kandev/kandev/internal/agentctl/server/utility"
)

type inferenceStopObserver interface {
	ObserveInferenceStop(context.Context, *utility.AttemptSnapshot) error
}
type inferenceStopObserverKey struct{}

// WithInferenceStopObserver preserves the capacity owner across native delete contexts.
func WithInferenceStopObserver(ctx context.Context, consumer InferenceConsumer) context.Context {
	if observer, ok := consumer.(inferenceStopObserver); ok {
		return context.WithValue(ctx, inferenceStopObserverKey{}, observer)
	}
	return ctx
}

func observeDeletedInference(ctx context.Context, instanceID string, body io.Reader) error {
	observer, ok := ctx.Value(inferenceStopObserverKey{}).(inferenceStopObserver)
	if !ok {
		return nil
	}
	var result struct {
		Attempts *utility.AttemptSnapshot `json:"inference_attempts"`
	}
	// Empty/legacy, malformed, and wrong-instance responses provide no child proof.
	if err := json.NewDecoder(io.LimitReader(body, 8<<20)).Decode(&result); err != nil || result.Attempts == nil || result.Attempts.InstanceID != instanceID {
		return nil
	}
	return observer.ObserveInferenceStop(ctx, result.Attempts)
}
