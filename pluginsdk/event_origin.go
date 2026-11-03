package pluginsdk

import "context"

type hostCallReferenceKey struct{}
type unidentifiedHostCallKey struct{}

func unidentifiedHostCallContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, unidentifiedHostCallKey{}, true)
}

// HostCallReference returns only a reference restored by the Host interceptor.
// Event payloads and plugin-supplied task IDs cannot install this value.
func HostCallReference(ctx context.Context) string {
	reference, _ := ctx.Value(hostCallReferenceKey{}).(string)
	return reference
}

// UnidentifiedEventContext preserves notification delivery without granting
// domain Host authority when the event's live origin can no longer be resolved.
func UnidentifiedEventContext(ctx context.Context) context.Context {
	return unidentifiedHostCallContext(ctx)
}

// EventOriginContext resolves a plugin-emitted event against the same live
// invocation map used by Host callbacks. References never survive a restart.
func (r *RemotePlugin) EventOriginContext(ctx context.Context, reference string) (context.Context, context.CancelFunc) {
	if r.origins == nil {
		return unidentifiedHostCallContext(ctx), func() {}
	}
	r.origins.mu.Lock()
	origin := r.origins.live[reference]
	r.origins.mu.Unlock()
	if origin == nil || origin.Err() != nil {
		return unidentifiedHostCallContext(ctx), func() {}
	}
	call, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(origin, cancel)
	return pluginCallerContext{Context: call, origin: origin}, func() {
		stop()
		cancel()
	}
}
