package plugins

import (
	"context"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kdlbs/kandev/pluginsdk"
)

type pluginDispatchGeneration struct {
	version     string
	installPath string
	installedAt time.Time
}

func dispatchGeneration(record *store.Record) pluginDispatchGeneration {
	if record == nil {
		return pluginDispatchGeneration{}
	}
	return pluginDispatchGeneration{
		version: record.Version, installPath: record.InstallPath, installedAt: record.InstalledAt,
	}
}

func (g pluginDispatchGeneration) matches(record *store.Record) bool {
	return record != nil && record.Version == g.version && record.InstallPath == g.installPath &&
		record.InstalledAt.Equal(g.installedAt)
}

// beginPluginDispatch holds an active-generation read lease until release is
// called. Disable, uninstall, config restart, and upgrade take the write side,
// so an authorized request cannot cross into a replacement process.
func (s *Service) beginPluginDispatch(
	id string, expected pluginDispatchGeneration,
) (*store.Record, func(), error) {
	return s.beginPluginDispatchContext(context.Background(), id, expected)
}

// beginPluginDispatchContext is beginPluginDispatch with a cancellation
// boundary: a lifecycle mutation holding or waiting for the write lease no
// longer keeps a caller past its own deadline.
func (s *Service) beginPluginDispatchContext(
	ctx context.Context, id string, expected pluginDispatchGeneration,
) (*store.Record, func(), error) {
	release, err := s.dispatchLocks.lockFor(id).RLockContext(ctx)
	if err != nil {
		return nil, nil, err
	}
	record, err := s.Get(id)
	if err != nil {
		release()
		return nil, nil, err
	}
	if record.Status != StatusActive || !expected.matches(record) {
		release()
		return nil, nil, fmt.Errorf("plugins: plugin %q lifecycle generation is no longer active", id)
	}
	return record, release, nil
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

// InvokeAction routes an already-authenticated, host-verified browser action
// to id's live subprocess. HTTP authorization, body limits, and response
// filtering belong to the action handler; this method only owns RPC dispatch.
func (s *Service) InvokeAction(
	ctx context.Context, id string, expected pluginDispatchGeneration, req *pluginsdk.PluginActionRequest,
) (*pluginsdk.PluginActionResponse, error) {
	_, release, err := s.beginPluginDispatchContext(ctx, id, expected)
	if err != nil {
		return nil, err
	}
	defer release()
	remote, ok := s.pluginRemote(id)
	if !ok {
		return nil, fmt.Errorf("plugins: plugin %q is not running", id)
	}
	return remote.HandleAction(ctx, req)
}

// pluginRemote returns the live RemotePlugin for id, if the runtime manager
// is wired and currently tracking a running process for it.
func (s *Service) pluginRemote(id string) (*pluginsdk.RemotePlugin, bool) {
	if s.runtime == nil {
		return nil, false
	}
	return s.runtime.Get(id)
}
