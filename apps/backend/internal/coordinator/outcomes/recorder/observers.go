package recorder

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/coordinator"
)

// Observers fans one decision out to several observers. A panic in one is
// counted and logged and never reaches the others or the decision.
type Observers struct {
	log  *zap.Logger
	list []namedObserver
}

type namedObserver struct {
	label string
	obs   coordinator.DecisionObserver
}

// NewObservers builds a fan-out of the grader queue and the override capture.
func NewObservers(log *zap.Logger, q *Queue, c *Capture) *Observers {
	if log == nil {
		log = zap.NewNop()
	}
	return &Observers{log: log, list: []namedObserver{{ObserverGrader, q}, {ObserverOverride, c}}}
}

// OnDecision implements coordinator.DecisionObserver.
func (o *Observers) OnDecision(ctx context.Context, ev coordinator.DecisionEvent) {
	for _, n := range o.list {
		o.call(ctx, n, ev)
	}
}

func (o *Observers) call(ctx context.Context, n namedObserver, ev coordinator.DecisionEvent) {
	defer func() {
		if r := recover(); r != nil {
			bump(observerPanicTotal, n.label)
			o.log.Error("coordinator outcomes observer panicked", zap.String("observer", n.label), zap.Any("panic", r))
		}
	}()
	n.obs.OnDecision(ctx, ev)
}
