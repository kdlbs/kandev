// Package dispatchcontext carries an opaque, durable server-owned context
// reference across async queues. It never derives authority from prompt text.
package dispatchcontext

import (
	"context"
	"errors"
)

const MetadataKey = "orchestration_context_ref"

var ErrStale = errors.New("assistant context changed; refresh the handoff before dispatch")

type key struct{}

func WithReference(ctx context.Context, ref string) context.Context {
	return context.WithValue(ctx, key{}, ref)
}

func Reference(ctx context.Context) (string, bool) {
	ref, ok := ctx.Value(key{}).(string)
	return ref, ok
}

func FromMetadata(ctx context.Context, metadata map[string]interface{}) context.Context {
	ref, _ := metadata[MetadataKey].(string)
	// Even an absent reference is explicit at queued dispatch: never replace a
	// queued snapshot with the task's possibly newer context.
	return WithReference(ctx, ref)
}
