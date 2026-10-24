package profile

import (
	"regexp"
	"testing"
)

// A coordinator session can never raise or lower an action class: its bound
// tool universe holds no tool that reads or writes settings, classes,
// reviews or the policy.
func TestCoordinatorToolUniverseHasNoRaiseOrLowerTool(t *testing.T) {
	forbidden := regexp.MustCompile(`setting|class|policy|review|raise|lower|autonomy|eligib`)
	for _, name := range coordinatorToolUniverse {
		if forbidden.MatchString(name) {
			t.Errorf("coordinator tool %q can reach the raise or lower surface", name)
		}
	}
}
