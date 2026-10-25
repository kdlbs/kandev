package orchestrator

import (
	"context"
	"encoding/json"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"go.uber.org/zap"
)

func (s *Service) subscribeRuntimeRecoveryEvents() {
	if s.eventBus == nil {
		return
	}
	if _, err := s.eventBus.Subscribe(events.AgentRuntimeAvailabilityChanged, s.handleRuntimeRecoveryAvailable); err != nil {
		s.logger.Error("failed to subscribe to runtime recovery", zap.Error(err))
	}
}

func (s *Service) handleRuntimeRecoveryAvailable(_ context.Context, event *bus.Event) error {
	if event == nil {
		return nil
	}
	encoded, err := json.Marshal(event.Data)
	if err != nil {
		return err
	}
	var snapshot runtimeapi.AvailabilitySnapshot
	if err := json.Unmarshal(encoded, &snapshot); err != nil {
		return err
	}
	if snapshot.Status == runtimeapi.AvailabilityStatusAvailable && snapshot.RuntimeEpoch > 0 {
		if ctx := s.recoveryLifecycleContext(); ctx != nil {
			s.startSilentRestoreRecovery(ctx)
		}
	}
	return nil
}
