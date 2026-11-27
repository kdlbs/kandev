package orchestrator

import (
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

// classifyDynamicLaunchFailure returns the launch error the conductor routes
// on. Only the agent process and its ACP session initialization can report a
// provider failure. Workspace preparation, executor, runtime detection, and
// local agentctl errors are Kandev conditions: they stay unclassified, so the
// ordinary launch recovery owns them and no provider candidate is suspended.
func classifyDynamicLaunchFailure(err error, executionProfileID string, keywordEnabled ...bool) error {
	var classified *routingerr.Error
	if errors.As(err, &classified) {
		return err
	}
	var startup *routingerr.AgentStartupFailure
	if !errors.As(err, &startup) || startup == nil {
		return err
	}
	input := routingerr.Input{
		KeywordContext: routingerr.KeywordContext{Enabled: len(keywordEnabled) != 0 && keywordEnabled[0], Diagnostic: true, EmptyOutput: true},
		Phase:          routingerr.PhaseProcessStart,
		ProviderID:     executionProfileID,
		Stderr:         err.Error(),
	}
	classified = routingerr.Classify(input)
	if input.KeywordContext.Enabled && classified.Confidence == routingerr.ConfLow && len(input.Stderr) <= 1024 {
		if diagnostic := keywordStartupDiagnostic(startup); diagnostic != "" {
			input.Stderr = diagnostic
			classified = routingerr.Classify(input)
		}
	}
	// Unknown low-confidence startup failures are runtime errors, not
	// provider failures. Let the ordinary launch recovery own them.
	if classified.Confidence == routingerr.ConfLow {
		return err
	}
	return fmt.Errorf("%w: %v", classified, err)
}

// Only the typed startup boundary may remove its own lifecycle wrapper.
// The inner diagnostic still passes the classifier's envelope and safety guards.
func keywordStartupDiagnostic(startup *routingerr.AgentStartupFailure) string {
	if startup == nil || len(startup.Diagnostic) > 1024 {
		return ""
	}
	prefix := ""
	switch startup.Phase {
	case routingerr.PhaseProcessStart:
		prefix = "failed to start agent: "
	case routingerr.PhaseSessionInit:
		prefix = "failed to initialize ACP: "
	default:
		return ""
	}
	if !strings.HasPrefix(startup.Diagnostic, prefix) {
		return ""
	}
	return strings.TrimPrefix(startup.Diagnostic, prefix)
}
