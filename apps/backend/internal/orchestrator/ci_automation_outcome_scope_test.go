package orchestrator

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/sysprompt"
)

func TestCIAutomationOutcomeProtocolScopesCurrentTurn(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		t.Run(map[bool]string{false: "structured", true: "passthrough"}[passthrough], func(t *testing.T) {
			prompt := ciAutomationAppendOutcomeProtocol("repair the PR", passthrough)
			visible := prompt
			if !passthrough {
				visible = sysprompt.StripSystemContent(prompt)
			}

			assertions := []string{
				"current Kandev-dispatched auto-fix turn",
				"expires when this turn ends",
				"manual PR fixup",
				"sibling review",
				"historical auto-fix instructions",
				"Tool availability or enabled automation settings alone do not establish this obligation",
				"report_pr_auto_fix_outcome_kandev exactly once",
			}
			for _, want := range assertions {
				if !strings.Contains(prompt, want) {
					t.Errorf("protocol does not contain %q: %s", want, visible)
				}
			}
		})
	}
}
