package backendapp

import (
	"context"
	"sync"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/mcp/plugintools"
	"go.uber.org/zap"
)

type pluginToolCatalogSource interface {
	AgentToolCatalog() (plugintools.Snapshot, error)
}

type pluginToolCatalogLifecycle interface {
	SetPluginToolsForAllExecutions(context.Context, plugintools.Snapshot) error
}

type pluginToolCatalogRefresher struct {
	ctx       context.Context
	cancel    context.CancelFunc
	source    pluginToolCatalogSource
	lifecycle pluginToolCatalogLifecycle
	logger    *logger.Logger
	requests  chan struct{}
	wg        sync.WaitGroup
}

func newPluginToolCatalogRefresher(ctx context.Context, source pluginToolCatalogSource, lifecycle pluginToolCatalogLifecycle, log *logger.Logger) *pluginToolCatalogRefresher {
	workerCtx, cancel := context.WithCancel(ctx)
	r := &pluginToolCatalogRefresher{
		ctx: workerCtx, cancel: cancel, source: source, lifecycle: lifecycle,
		logger: log, requests: make(chan struct{}, 1),
	}
	r.wg.Add(1)
	go r.run()
	return r
}

func (r *pluginToolCatalogRefresher) NotifyAgentToolCatalogChanged() {
	select {
	case r.requests <- struct{}{}:
	default:
	}
}

func (r *pluginToolCatalogRefresher) Stop() {
	r.cancel()
	r.wg.Wait()
}

func (r *pluginToolCatalogRefresher) run() {
	defer r.wg.Done()
	for {
		select {
		case <-r.ctx.Done():
			return
		case <-r.requests:
			snapshot, err := r.source.AgentToolCatalog()
			if err == nil && r.lifecycle != nil {
				err = r.lifecycle.SetPluginToolsForAllExecutions(r.ctx, snapshot)
			}
			if err != nil && r.logger != nil {
				r.logger.Warn("live plugin tool catalog refresh failed", zap.Error(err))
			}
		}
	}
}
