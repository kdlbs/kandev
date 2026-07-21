package pluginsdk

import (
	"context"
	"sync"
)

// Plugin is the interface a plugin author implements. It is delivered RPCs
// from kandev over the go-plugin gRPC transport (§3 of
// docs/plans/plugins/GRPC-CONTRACT.md): DeliverEvent -> OnEvent,
// HandleWebhook -> HandleWebhook.
type Plugin interface {
	// OnEvent handles a single bus event delivery. A non-nil error causes
	// kandev's delivery subsystem to retry per §5 (3 retries, 5s/15s/45s).
	OnEvent(ctx context.Context, e *Event) error

	// HandleWebhook handles an inbound webhook relayed by kandev's
	// POST /api/plugins/{id}/webhooks/{key} endpoint.
	HandleWebhook(ctx context.Context, req *WebhookRequest) (*WebhookResponse, error)
}

// HostSetter is implemented by Plugin values that want Serve to inject the
// Host once the broker connection back to kandev is established (see the
// "Host injection" section of the serve.go file header). UnimplementedPlugin
// implements this, so embedding it is the easiest way to opt in.
type HostSetter interface {
	SetHost(Host)
}

// UnimplementedPlugin is an embeddable no-op base for Plugin. Authors that
// only care about one or two RPCs embed this and override the rest. It also
// implements HostSetter, storing the injected Host for retrieval via Host().
//
// SetHost is called from a background goroutine (see the GRPCPlugin doc
// comment in serve.go for why), so access to the stored Host is
// mutex-guarded — Host() may be called concurrently with SetHost from
// another goroutine.
type UnimplementedPlugin struct {
	mu   sync.RWMutex
	host Host
}

var (
	_ Plugin     = (*UnimplementedPlugin)(nil)
	_ HostSetter = (*UnimplementedPlugin)(nil)
)

// OnEvent is a no-op default: it accepts the event without doing anything.
//
// Pointer receiver (like every other UnimplementedPlugin method): the type
// embeds a sync.RWMutex, and a value receiver would trip go vet's copylocks
// check even though this method never touches it.
func (*UnimplementedPlugin) OnEvent(context.Context, *Event) error {
	return nil
}

// HandleWebhook is a no-op default: it returns 404, since an unhandled
// webhook path has nothing sensible to serve.
func (*UnimplementedPlugin) HandleWebhook(context.Context, *WebhookRequest) (*WebhookResponse, error) {
	return &WebhookResponse{Status: 404}, nil
}

// SetHost stores the Host injected by Serve. Call Host() from your
// overridden methods to reach kandev.
func (p *UnimplementedPlugin) SetHost(h Host) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.host = h
}

// Host returns the Host injected by Serve, or nil if the broker connection
// hasn't completed yet (or Serve hasn't been called, e.g. in unit tests).
func (p *UnimplementedPlugin) Host() Host {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.host
}
