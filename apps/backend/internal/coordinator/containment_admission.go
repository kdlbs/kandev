package coordinator

import (
	"context"
	"expvar"
	"sync"

	"go.uber.org/zap"
)

var containmentFailedTotal = expvar.NewMap("coordinator_containment_failed_total")

// ContainmentRecorder counts an unmet containment condition.
type ContainmentRecorder interface {
	ConditionFailed(condition string)
}

type expvarContainmentRecorder struct{}

func (expvarContainmentRecorder) ConditionFailed(condition string) {
	containmentFailedTotal.Add(condition, 1)
}

// containmentFailedCount reads the counter for one condition.
func containmentFailedCount(condition string) int64 {
	if v, ok := containmentFailedTotal.Get(condition).(*expvar.Int); ok {
		return v.Value()
	}
	return 0
}

type admissionKey struct {
	contained  bool
	firstUnmet string
}

// admissionState remembers the last admission key per coordinator id.
type admissionState struct {
	mu   sync.Mutex
	last map[string]admissionKey
}

func newAdmissionState() admissionState {
	return admissionState{last: map[string]admissionKey{}}
}

// changed stores key and reports whether it differs from the previous key for
// the coordinator. The first call after a restart never reports a change.
func (s *admissionState) changed(coordinatorID string, key admissionKey) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, seen := s.last[coordinatorID]
	s.last[coordinatorID] = key
	return seen && prev != key
}

// CheckForAdmission runs Check, counts each unmet condition and logs when the
// (Contained, first unmet condition) key changes between calls. A nil
// recorder counts on the process metric. Only admission calls it; the
// autonomy read and the settings display call Check.
func (c *ContainmentChecker) CheckForAdmission(ctx context.Context, co *Coordinator, recorder ContainmentRecorder) ContainmentResult {
	if recorder == nil {
		recorder = expvarContainmentRecorder{}
	}
	result := c.Check(ctx, co)
	for _, cond := range result.Conditions {
		if !cond.Met {
			recorder.ConditionFailed(cond.Name)
		}
	}
	if co == nil {
		return result
	}
	key := admissionKey{contained: result.Contained}
	if first, ok := result.FirstUnmet(); ok {
		key.firstUnmet = first.Name
	}
	if c.state.changed(co.ID, key) && c.log != nil {
		c.log.Info("coordinator containment changed",
			zap.String("coordinator_id", co.ID),
			zap.Bool("contained", key.contained),
			zap.String("condition", key.firstUnmet))
	}
	return result
}
