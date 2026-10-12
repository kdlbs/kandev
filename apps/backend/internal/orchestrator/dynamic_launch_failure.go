package orchestrator

import (
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

// classifyDynamicLaunchFailure returns the launch error the conductor routes
// on. Only the agent process and its ACP session initialization can report a
// provider failure. Workspace preparation, executor, runtime detection, and
// local agentctl errors are Kandev conditions: they stay unclassified, so the
// ordinary launch recovery owns them and no provider candidate is suspended.
func classifyDynamicLaunchFailure(err error, executionProfileID string) error {
	var classified *routingerr.Error
	if errors.As(err, &classified) {
		return err
	}
	var startup *routingerr.AgentStartupFailure
	if !errors.As(err, &startup) || startup == nil {
		return err
	}
	classified = routingerr.Classify(routingerr.Input{
		Phase:      routingerr.PhaseProcessStart,
		ProviderID: executionProfileID,
		Stderr:     err.Error(),
	})
	// Unknown low-confidence startup failures are runtime errors, not
	// provider failures. Let the ordinary launch recovery own them.
	if classified.Confidence == routingerr.ConfLow {
		return err
	}
	return fmt.Errorf("%w: %v", classified, err)
}
