package scheduler_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/routing"
	"github.com/kandev/kandev/internal/office/scheduler"
)

type suspendedCircuitError struct{}

func (suspendedCircuitError) Error() string                    { return "suspended" }
func (suspendedCircuitError) CircuitSuspendedUntil() time.Time { return time.Time{} }

func TestRoutingPropagatesCircuitSuspension(t *testing.T) {
	repo := newTestRepoSched(t)
	seedRoutingConfig(t, repo, []routing.ProviderID{"claude-acp"})
	starter := newFakeTaskStarter()
	starter.failFor["claude-acp"] = suspendedCircuitError{}
	ss := buildScheduler(t, repo, starter)
	run := seedRun(t, repo, `{"task_id":"suspended-task"}`)
	for i := 0; i <= scheduler.MaxAttemptsPerRun; i++ {
		launched, parked, err := ss.DispatchWithRouting(context.Background(), run, makeAgent(), scheduler.LaunchContext{})
		var suspended suspendedCircuitError
		if launched || parked || !errors.As(err, &suspended) {
			t.Fatalf("attempt %d: launched=%v parked=%v error=%v", i, launched, parked, err)
		}
	}
}
