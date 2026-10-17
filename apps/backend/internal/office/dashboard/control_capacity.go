package dashboard

import (
	"context"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/system/sessioncapacity"
)

// ControlCapacitySource reads the shared admission owner's observation.
type ControlCapacitySource interface {
	SessionCapacityObservation(context.Context) sessioncapacity.Observation
}

func (s *DashboardService) SetControlCapacitySource(source ControlCapacitySource) {
	if s != nil {
		s.controlCapacity = source
	}
}

func (s *DashboardService) controlCapacityWaiting(ctx context.Context) *sqlite.ControlWaiting {
	if s == nil || s.controlCapacity == nil {
		return nil
	}
	waiting := s.controlCapacity.SessionCapacityObservation(ctx).Waiting
	if !waiting.Known {
		return &sqlite.ControlWaiting{}
	}
	total, grade := 0, 0
	for reason, count := range waiting.Counts {
		total += count
		if reason == sessioncapacity.WaitingReasonGradeLimit || reason == sessioncapacity.WaitingReasonBothLimits {
			grade += count
		}
	}
	return &sqlite.ControlWaiting{Known: true, Grade: grade, Total: total}
}
