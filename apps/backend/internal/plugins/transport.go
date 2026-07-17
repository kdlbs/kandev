package plugins

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/pkg/pluginsdk"
)

// RuntimeTransport adapts a PluginRuntime (runtime.Manager in production)
// to internal/plugins/delivery's Transport interface structurally —
// delivery does not import this package (see that package's doc comment
// for why), so this type's DeliverEvent method is all that's needed to
// satisfy it.
type RuntimeTransport struct {
	runtime PluginRuntime
}

// NewRuntimeTransport builds a delivery-facing transport over rt.
func NewRuntimeTransport(rt PluginRuntime) *RuntimeTransport {
	return &RuntimeTransport{runtime: rt}
}

// DeliverEvent calls the plugin's live subprocess DeliverEvent RPC.
// Timeout/retry policy is the caller's (internal/plugins/delivery's)
// responsibility — this is a thin, context-respecting proxy over
// PluginRuntime.Get + RemotePlugin.DeliverEvent.
func (t *RuntimeTransport) DeliverEvent(ctx context.Context, pluginID string, e *pluginsdk.Event) error {
	remote, ok := t.runtime.Get(pluginID)
	if !ok {
		return fmt.Errorf("plugins: plugin %q is not running", pluginID)
	}
	return remote.DeliverEvent(ctx, e)
}
