package main

import (
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func provideEventBus(cfg *config.Config, log *logger.Logger) (bus.EventBus, func() error, error) {
	provider, cleanup, err := events.Provide(cfg, log)
	if err != nil {
		return nil, nil, err
	}
	return provider.Bus, cleanup, nil
}
