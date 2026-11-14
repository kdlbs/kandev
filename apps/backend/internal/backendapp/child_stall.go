package backendapp

import (
	"context"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator"
)

// startChildStallProducer starts the child-turn stalled producer after the
// orchestrator is running and registers its shutdown.
func startChildStallProducer(
	ctx context.Context,
	orchestratorSvc *orchestrator.Service,
	store orchestrator.ChildStallStore,
	log *logger.Logger,
	addCleanup func(func() error),
) {
	if orchestratorSvc == nil || store == nil {
		return
	}
	producer := orchestrator.NewChildStallProducer(orchestratorSvc, store, log)
	orchestratorSvc.SetChildStallProducer(producer)
	producer.Start(ctx)
	addCleanup(func() error {
		producer.Stop()
		return nil
	})
}
