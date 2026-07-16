package plugins

import (
	"context"
	"fmt"
	"time"

	"github.com/kandev/kandev/pkg/pluginsdk"
)

// DefaultToolTimeout bounds a single agent tool invocation
// (Service.InvokeTool), per docs/specs/plugins/spec.md ("Agent tool
// invocation").
const DefaultToolTimeout = 30 * time.Second

// InvokeTool routes a tool call to id's live subprocess via the runtime
// manager's RemotePlugin.InvokeTool RPC, applying DefaultToolTimeout on top
// of ctx. Returns an error if id has no live process (not active, or
// mid-restart).
func (s *Service) InvokeTool(ctx context.Context, id string, req *pluginsdk.ToolRequest) (*pluginsdk.ToolResponse, error) {
	remote, ok := s.pluginRemote(id)
	if !ok {
		return nil, fmt.Errorf("plugins: plugin %q is not running", id)
	}
	reqCtx, cancel := context.WithTimeout(ctx, DefaultToolTimeout)
	defer cancel()
	return remote.InvokeTool(reqCtx, req)
}

// InvokeWebhook routes an inbound webhook to id's live subprocess via the
// runtime manager's RemotePlugin.HandleWebhook RPC. Used by
// POST/GET /api/plugins/:id/webhooks/:key.
func (s *Service) InvokeWebhook(ctx context.Context, id string, req *pluginsdk.WebhookRequest) (*pluginsdk.WebhookResponse, error) {
	remote, ok := s.pluginRemote(id)
	if !ok {
		return nil, fmt.Errorf("plugins: plugin %q is not running", id)
	}
	return remote.HandleWebhook(ctx, req)
}

// pluginRemote returns the live RemotePlugin for id, if the runtime manager
// is wired and currently tracking a running process for it.
func (s *Service) pluginRemote(id string) (*pluginsdk.RemotePlugin, bool) {
	if s.runtime == nil {
		return nil, false
	}
	return s.runtime.Get(id)
}
