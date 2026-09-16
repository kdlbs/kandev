package orchestrator

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
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
				"manual change-request fixup",
				"sibling review",
				"historical auto-fix instructions",
				"Tool availability or enabled automation settings alone do not establish this obligation",
				"report_change_request_auto_fix_outcome_kandev exactly once",
			}
			for _, want := range assertions {
				if !strings.Contains(prompt, want) {
					t.Errorf("protocol does not contain %q: %s", want, visible)
				}
			}
		})
	}
}

func TestCIAutomationOutcomeToolUsesCurrentKandevCatalog(t *testing.T) {
	currentSession := func(currentSource streams.MCPServerSource, currentTools []string, previousTools []string) *models.TaskSession {
		toSummaries := func(names []string) []streams.MCPToolSummary {
			tools := make([]streams.MCPToolSummary, 0, len(names))
			for _, name := range names {
				tools = append(tools, streams.MCPToolSummary{Name: name})
			}
			return tools
		}
		current := streams.MCPAttachmentAttempt{
			AttemptID: "current-attempt",
			Servers: []streams.MCPServerAttachment{{
				Name: "kandev", Source: currentSource, Tools: toSummaries(currentTools),
			}},
		}
		history := streams.MCPAttachmentHistory{
			Version: streams.MCPAttachmentSchemaVersion,
			Current: current,
		}
		if len(previousTools) > 0 {
			history.Previous = []streams.MCPAttachmentAttempt{{
				AttemptID: "previous-attempt",
				Servers: []streams.MCPServerAttachment{{
					Name: "kandev", Source: streams.MCPServerSourceKandev, Tools: toSummaries(previousTools),
				}},
			}}
		}
		return &models.TaskSession{Metadata: map[string]interface{}{
			models.SessionMetaKeyMCPAttachmentState: history,
		}}
	}

	tests := []struct {
		name        string
		session     *models.TaskSession
		wantTool    string
		wantCatalog bool
	}{
		{
			name:        "current neutral wins",
			session:     currentSession(streams.MCPServerSourceKandev, []string{ciAutomationLegacyOutcomeTool, ciAutomationNeutralOutcomeTool}, nil),
			wantTool:    ciAutomationNeutralOutcomeTool,
			wantCatalog: true,
		},
		{
			name:        "current legacy is retained",
			session:     currentSession(streams.MCPServerSourceKandev, []string{ciAutomationLegacyOutcomeTool}, nil),
			wantTool:    ciAutomationLegacyOutcomeTool,
			wantCatalog: true,
		},
		{
			name:        "previous attempt is ignored",
			session:     currentSession(streams.MCPServerSourceKandev, nil, []string{ciAutomationNeutralOutcomeTool}),
			wantCatalog: false,
		},
		{
			name:        "foreign source is ignored",
			session:     currentSession(streams.MCPServerSourceProfile, []string{ciAutomationNeutralOutcomeTool}, nil),
			wantCatalog: false,
		},
		{
			name: "missing current server is ignored",
			session: &models.TaskSession{Metadata: map[string]interface{}{
				models.SessionMetaKeyMCPAttachmentState: streams.MCPAttachmentHistory{
					Version: streams.MCPAttachmentSchemaVersion,
					Current: streams.MCPAttachmentAttempt{AttemptID: "current-attempt"},
				},
			}},
			wantCatalog: false,
		},
		{
			name:        "malformed history is ignored",
			session:     &models.TaskSession{Metadata: map[string]interface{}{models.SessionMetaKeyMCPAttachmentState: map[string]interface{}{"version": "bad"}}},
			wantCatalog: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTool, gotCatalog := ciAutomationOutcomeToolForSession(tt.session)
			if gotTool != tt.wantTool || gotCatalog != tt.wantCatalog {
				t.Fatalf("ciAutomationOutcomeToolForSession() = (%q, %v), want (%q, %v)", gotTool, gotCatalog, tt.wantTool, tt.wantCatalog)
			}
		})
	}
}

func TestReplaceCIAutoFixOutcomeProtocolPreservesPromptText(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		t.Run(map[bool]string{false: "structured", true: "passthrough"}[passthrough], func(t *testing.T) {
			content := ciAutomationAppendOutcomeProtocolForTool("edited task prompt", passthrough, ciAutomationLegacyOutcomeTool)
			updated, changed := replaceCIAutoFixOutcomeProtocol(content, ciAutomationLegacyOutcomeTool, ciAutomationNeutralOutcomeTool)
			if !changed {
				t.Fatal("replaceCIAutoFixOutcomeProtocol() reported no replacement")
			}
			if !strings.Contains(updated, "edited task prompt") || !strings.Contains(updated, ciAutomationNeutralOutcomeTool) {
				t.Fatalf("updated content lost prompt or new tool: %s", updated)
			}
			if strings.Contains(updated, ciAutomationLegacyOutcomeTool) {
				t.Fatalf("updated content retained old tool: %s", updated)
			}
		})
	}

	unchanged, changed := replaceCIAutoFixOutcomeProtocol("user text with no protocol", ciAutomationLegacyOutcomeTool, ciAutomationNeutralOutcomeTool)
	if changed || unchanged != "user text with no protocol" {
		t.Fatalf("unrecognized protocol result = (%q, %v)", unchanged, changed)
	}
}

func TestReplaceCIAutoFixOutcomeProtocolRecognizesHistoricalQueuedBlock(t *testing.T) {
	// Keep this fixture independent from the current protocol builder. It is
	// copied from the pre-provider-neutral runtime so this test covers the
	// actual queued content that migration must recognize.
	const historicalProtocol = `Kandev PR auto-fix outcome protocol:
This instruction applies only to the current Kandev-dispatched auto-fix turn that received this protocol. It expires when this turn ends.
Before this turn ends, call report_pr_auto_fix_outcome_kandev exactly once with one of these outcomes:
- action_taken: you made or requested a concrete provider-visible change and want Kandev to wait for CI or PR progress.
- non_actionable: the current feedback does not identify a change this task can make.
- blocked: a concrete change is needed, but an external condition prevents it. Include a short reason.
Do not report an outcome for manual PR fixup, sibling review messages, or historical auto-fix instructions. Tool availability or enabled automation settings alone do not establish this obligation.
Do not claim action_taken from a plan or an attempted command alone. If the tool is unavailable, continue the repair work and explain that the outcome could not be recorded.`

	for _, wrapped := range []bool{false, true} {
		content := "edited queued prompt"
		if wrapped {
			content += "\n\n" + sysprompt.Wrap(historicalProtocol)
		} else {
			content += "\n\n" + historicalProtocol
		}
		updated, changed := replaceCIAutoFixOutcomeProtocol(content, ciAutomationLegacyOutcomeTool, ciAutomationNeutralOutcomeTool)
		if !changed {
			t.Fatalf("historical %v protocol was not replaced", wrapped)
		}
		if !strings.Contains(updated, "edited queued prompt") || !strings.Contains(updated, ciAutomationNeutralOutcomeTool) {
			t.Fatalf("historical %v replacement lost prompt or new tool: %s", wrapped, updated)
		}
		if strings.Contains(updated, "Kandev PR auto-fix outcome protocol:") || strings.Contains(updated, ciAutomationLegacyOutcomeTool) {
			t.Fatalf("historical %v replacement retained the old protocol: %s", wrapped, updated)
		}
	}
}
