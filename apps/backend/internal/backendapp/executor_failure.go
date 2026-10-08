package backendapp

import (
	"fmt"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
)

func localExecutorObservationReader(availability *agentruntime.Availability, pid int, attachedAt time.Time) func(models.ExecutorObservationTarget) *models.ExecutorObservation {
	return func(target models.ExecutorObservationTarget) *models.ExecutorObservation {
		if availability == nil || pid <= 0 || target.LocalPID != pid || target.ResourceKey != fmt.Sprintf("local-pid:%d", pid) || target.ExpectedExecutorUpdatedAt.Before(attachedAt) {
			return nil
		}
		snapshot, ok := availability.Snapshot()
		if !ok {
			return nil
		}
		observation := &models.ExecutorObservation{Runtime: "standalone", ResourceKey: target.ResourceKey, ObservedAt: time.Now().UTC(), Workspace: "unknown", Outcome: "unknown"}
		if snapshot.Status == agentruntime.AvailabilityStatusAvailable {
			observation.Outcome = "healthy"
		}
		if snapshot.Status == agentruntime.AvailabilityStatusUnavailable && snapshot.Reason == agentruntime.AvailabilityReasonAgentctlExited {
			observation.Outcome = "terminated"
			observation.Reason = "ControllerExited"
			observation.OccurredAt = snapshot.OccurredAt
		}
		return observation
	}
}
