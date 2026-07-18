package plugins

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/pkg/pluginsdk"
)

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
