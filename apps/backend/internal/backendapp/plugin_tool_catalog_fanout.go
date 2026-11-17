package backendapp

import (
	"context"
	"errors"
	"sync"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/mcp/plugintools"
	"go.uber.org/zap"
)

// pluginToolCatalogSubscriber receives an atomic filtered-catalog refresh from
// a backend-owned MCP server that is not an agentctl execution.
type pluginToolCatalogSubscriber interface {
	RefreshPluginTools(context.Context) error
}

// pluginToolCatalogFanout pushes a lifecycle catalog change to both agentctl
// executions and in-process MCP servers such as the external endpoint. It
// implements pluginToolCatalogLifecycle for the refresher.
type pluginToolCatalogFanout struct {
	mu          sync.RWMutex
	executions  pluginToolCatalogLifecycle
	subscribers map[pluginToolCatalogSubscriber]struct{}
	logger      *logger.Logger
}

func newPluginToolCatalogFanout(executions pluginToolCatalogLifecycle, log *logger.Logger) *pluginToolCatalogFanout {
	return &pluginToolCatalogFanout{
		executions:  executions,
		subscribers: make(map[pluginToolCatalogSubscriber]struct{}),
		logger:      log,
	}
}

// Subscribe registers an in-process MCP server. The returned function removes
// it and waits for any refresh already reading the subscriber set.
func (f *pluginToolCatalogFanout) Subscribe(subscriber pluginToolCatalogSubscriber) func() {
	if f == nil || subscriber == nil {
		return func() {}
	}
	f.mu.Lock()
	f.subscribers[subscriber] = struct{}{}
	f.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			f.mu.Lock()
			delete(f.subscribers, subscriber)
			f.mu.Unlock()
		})
	}
}

// SetPluginToolsForAllExecutions fans one projected snapshot out to every
// agentctl execution and refreshes each registered in-process server.
func (f *pluginToolCatalogFanout) SetPluginToolsForAllExecutions(ctx context.Context, snapshot plugintools.Snapshot) error {
	if f == nil {
		return nil
	}
	f.mu.RLock()
	executions := f.executions
	subscribers := make([]pluginToolCatalogSubscriber, 0, len(f.subscribers))
	for subscriber := range f.subscribers {
		subscribers = append(subscribers, subscriber)
	}
	defer f.mu.RUnlock()

	var errs []error
	// Refresh in-process servers first: a stalled remote agentctl push must
	// not delay tools/list_changed for healthy external clients.
	for _, subscriber := range subscribers {
		if err := subscriber.RefreshPluginTools(ctx); err != nil {
			if f.logger != nil {
				f.logger.Warn("in-process plugin tool catalog refresh failed", zap.Error(err))
			}
			errs = append(errs, err)
		}
	}
	if executions != nil {
		if err := executions.SetPluginToolsForAllExecutions(ctx, snapshot); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
