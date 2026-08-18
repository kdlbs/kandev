package lifecycle

import (
	"context"

	"github.com/kandev/kandev/internal/common/constants"
)

// launchPhaseParentKey lets a runtime recover the deadline-free parent when a
// launch phase context is passed through its CreateInstance call. Preparation
// has its own budget and must not inherit the launch phase's remaining time.
type launchPhaseParentKey struct{}

// withLaunchPhaseTimeout starts a fresh budget for one runtime launch phase.
// The parent remains available through preparationContext for setup scripts.
func withLaunchPhaseTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	parent := preparationContext(ctx)
	phaseParent := context.WithValue(parent, launchPhaseParentKey{}, parent)
	return context.WithTimeout(phaseParent, constants.AgentLaunchTimeout)
}

// preparationContext returns the context that owns caller and manager
// cancellation, without a launch-phase deadline.
func preparationContext(ctx context.Context) context.Context {
	if parent, ok := ctx.Value(launchPhaseParentKey{}).(context.Context); ok {
		return parent
	}
	return ctx
}
