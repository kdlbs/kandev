package orchestrator

import (
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	taskdto "github.com/kandev/kandev/internal/task/dto"
)

type runtimeStorageObservationProvider interface {
	RuntimeStorageObservation(sessionID string) (agentruntime.RuntimeStorageObservation, bool)
}

// RuntimeStorageHealth projects the lifecycle-owned advisory onto the task DTO
// seam. A missing provider or observation is omitted rather than fabricated as
// healthy capacity.
func (s *Service) RuntimeStorageHealth(sessionID string) *taskdto.RuntimeStorageHealthDTO {
	if s == nil || s.agentManager == nil || sessionID == "" {
		return nil
	}
	provider, ok := s.agentManager.(runtimeStorageObservationProvider)
	if !ok {
		return nil
	}
	observation, ok := provider.RuntimeStorageObservation(sessionID)
	if !ok {
		return nil
	}
	ageSeconds := observation.AgeSeconds
	if !observation.ReceivedAt.IsZero() {
		ageSeconds = int64(time.Since(observation.ReceivedAt.UTC()).Seconds())
		if ageSeconds < 0 {
			ageSeconds = 0
		}
	}
	result := &taskdto.RuntimeStorageHealthDTO{
		State:          observation.State,
		StorageKind:    observation.StorageKind,
		Location:       observation.Location,
		ExecutorLabel:  observation.ExecutorLabel,
		TotalBytes:     observation.TotalBytes,
		AvailableBytes: observation.AvailableBytes,
		ObservedAt:     observation.ObservedAt,
		AgeSeconds:     ageSeconds,
		WritePressure:  observation.WritePressure,
		ProducerPaused: observation.ProducerPaused,
		DeliveryState:  observation.DeliveryState,
		FailureStorage: observation.FailureStorage,
		Revision:       observation.Revision,
		Epoch:          observation.Epoch,
	}
	if result.Location == "" {
		result.Location = taskdto.RuntimeStorageLocationUnknown
	}
	return result
}

var _ taskdto.RuntimeStorageHealthProvider = (*Service)(nil)
