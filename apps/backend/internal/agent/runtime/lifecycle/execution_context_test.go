package lifecycle

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/common/constants"
)

func TestCoalescedExecutionContextDoesNotConsumeLaunchBudgetDuringPreparation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		manager := &Manager{stopCh: make(chan struct{})}
		sharedCtx, cancel := manager.coalescedExecutionContext(context.Background())
		defer cancel()

		if _, ok := sharedCtx.Deadline(); ok {
			t.Fatal("coalesced execution context must not have a launch deadline")
		}

		// Simulate environment creation before the runtime launch phase begins.
		<-time.After(constants.SetupScriptTimeout)
		launchCtx, launchCancel := withLaunchPhaseTimeout(sharedCtx)
		defer launchCancel()

		deadline, ok := launchCtx.Deadline()
		if !ok {
			t.Fatal("launch phase context must have a deadline")
		}
		if got := time.Until(deadline); got != constants.AgentLaunchTimeout {
			t.Fatalf("launch phase budget = %s, want %s", got, constants.AgentLaunchTimeout)
		}
	})
}

func TestPreparationContextRetainsItsIndependentBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		base := context.Background()
		launchCtx, launchCancel := withLaunchPhaseTimeout(base)
		defer launchCancel()

		prepareParent := preparationContext(launchCtx)
		if _, ok := prepareParent.Deadline(); ok {
			t.Fatal("preparation parent must not inherit the launch deadline")
		}

		prepareCtx, prepareCancel := context.WithTimeout(prepareParent, constants.SetupScriptTimeout)
		defer prepareCancel()
		deadline, ok := prepareCtx.Deadline()
		if !ok {
			t.Fatal("preparation context must have a deadline")
		}
		if got := time.Until(deadline); got != constants.SetupScriptTimeout {
			t.Fatalf("preparation budget = %s, want %s", got, constants.SetupScriptTimeout)
		}
	})
}
