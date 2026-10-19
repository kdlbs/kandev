package backendapp

import (
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	taskusage "github.com/kandev/kandev/internal/task/usage"
)

func startTaskUsageWriter(
	repo taskusage.Repository,
	pricing taskusage.PricingLookup,
	eventBus bus.EventBus,
	log *logger.Logger,
	addCleanup func(func() error),
) (*taskusage.Writer, error) {
	writer := taskusage.NewWriter(repo, pricing, log)
	writer.Start()
	if err := writer.Subscribe(eventBus); err != nil {
		writer.Stop()
		return nil, err
	}
	addCleanup(func() error {
		writer.Stop()
		return nil
	})
	return writer, nil
}
