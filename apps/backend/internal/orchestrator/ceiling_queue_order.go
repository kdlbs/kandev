package orchestrator

import (
	"sort"
	"time"

	"github.com/kandev/kandev/internal/system/sessioncapacity"
	"github.com/kandev/kandev/internal/task/models"
)

func (s *Service) orderCeilingDeferredTasks(tasks []*models.Task) []*models.Task {
	if s.sessionCeiling == nil {
		return tasks
	}
	c := s.sessionCeiling
	c.mu.Lock()
	all := c.policy == sessioncapacity.PolicyAll
	c.mu.Unlock()
	if !all {
		return tasks
	}
	return oldestCeilingTasks(tasks)
}

type ceilingQueueArrival struct {
	task *models.Task
	id   string
	at   time.Time
}

// A missing legacy timestamp has no invented age. Known arrivals come first,
// then records without a usable timestamp reach the existing replay validation.
// Equal recorded times retain deterministic ID order without a second counter.
func oldestCeilingTasks(tasks []*models.Task) []*models.Task {
	arrivals := make([]ceilingQueueArrival, 0, len(tasks))
	for _, task := range tasks {
		arrival := ceilingQueueArrival{task: task}
		if task != nil {
			arrival.id = task.ID
			record, _ := task.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
			if deferral, err := models.ReadCeilingDeferral(record); err == nil {
				arrival.at = deferral.QueuedAt
			}
		}
		arrivals = append(arrivals, arrival)
	}
	sort.SliceStable(arrivals, func(i, j int) bool {
		left, right := arrivals[i], arrivals[j]
		if left.at.IsZero() != right.at.IsZero() {
			return !left.at.IsZero()
		}
		if left.at.Equal(right.at) {
			return left.id < right.id
		}
		return left.at.Before(right.at)
	})
	ordered := make([]*models.Task, 0, len(arrivals))
	for _, arrival := range arrivals {
		ordered = append(ordered, arrival.task)
	}
	return ordered
}
