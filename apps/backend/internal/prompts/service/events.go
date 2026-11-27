package service

import (
	"context"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"go.uber.org/zap"
)

func (s *Service) SetEventBus(eventBus bus.EventBus, log *logger.Logger) {
	s.eventBus, s.logger = eventBus, log
}

func (s *Service) publishChanged(ctx context.Context) {
	if s.eventBus == nil {
		return
	}
	if err := s.eventBus.Publish(ctx, events.PromptsChanged, bus.NewEvent(events.PromptsChanged, "prompts-service", map[string]any{})); err != nil && s.logger != nil {
		s.logger.Error("failed to publish saved prompt invalidation", zap.Error(err))
	}
}
