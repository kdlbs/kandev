package events

import (
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
)

// ProvidedBus wraps the active event bus implementation.
type ProvidedBus struct {
	Bus    bus.EventBus
	Memory *bus.MemoryEventBus
	NATS   *bus.NATSEventBus
}

// Provide builds the configured event bus implementation.
func Provide(cfg *config.Config, log *logger.Logger) (*ProvidedBus, func() error, error) {
	if strings.TrimSpace(cfg.NATS.URL) != "" {
		natsBus, err := bus.NewNATSEventBus(cfg.NATS, log)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to initialize NATS event bus: %w", err)
		}
		cleanup := func() error {
			natsBus.Close()
			return nil
		}
		return &ProvidedBus{Bus: natsBus, NATS: natsBus}, cleanup, nil
	}

	memBus := bus.NewMemoryEventBus(log)
	return &ProvidedBus{Bus: memBus, Memory: memBus}, func() error { return nil }, nil
}
