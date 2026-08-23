package service

import (
	"strings"
	"testing"

	workflowcfg "github.com/kandev/kandev/config/workflows"
	"github.com/kandev/kandev/internal/workflow/models"
)

// fallbackPromptPrefix is BuildPrompt's default-branch marker (prompt_builder.go).
// A shipped workflow reason that produces this prefix has lost every bit of
// task context BuildPrompt would otherwise template in.
const fallbackPromptPrefix = "You have been woken for reason:"

// reasonOccurrence names exactly where a `reason:` value was found so a
// failing assertion can point straight at the offending YAML.
type reasonOccurrence struct {
	template string
	step     string
	trigger  string
	reason   string
}

// collectReasonOccurrences walks every step of every shipped template and
// every event trigger that can carry a `reason:` config key (the four
// original triggers plus the seven Phase 2 event-driven ones), returning
// every occurrence found. This is table-driven off the actual shipped
// config rather than a hand-maintained list of known-bad reasons, so a new
// bad reason added to any template tomorrow fails this test without anyone
// updating it.
func collectReasonOccurrences(templates []*models.WorkflowTemplate) []reasonOccurrence {
	var out []reasonOccurrence
	for _, tmpl := range templates {
		for _, step := range tmpl.Steps {
			for _, a := range step.Events.OnEnter {
				if reason, ok := a.Config["reason"].(string); ok && reason != "" {
					out = append(out, reasonOccurrence{tmpl.ID, step.ID, "on_enter", reason})
				}
			}
			out = append(out, collectGenericReasons(tmpl.ID, step.ID, "on_comment", step.Events.OnComment)...)
			out = append(out, collectGenericReasons(tmpl.ID, step.ID, "on_blocker_resolved", step.Events.OnBlockerResolved)...)
			out = append(out, collectGenericReasons(tmpl.ID, step.ID, "on_children_completed", step.Events.OnChildrenCompleted)...)
			out = append(out, collectGenericReasons(tmpl.ID, step.ID, "on_approval_resolved", step.Events.OnApprovalResolved)...)
			out = append(out, collectGenericReasons(tmpl.ID, step.ID, "on_heartbeat", step.Events.OnHeartbeat)...)
			out = append(out, collectGenericReasons(tmpl.ID, step.ID, "on_budget_alert", step.Events.OnBudgetAlert)...)
			out = append(out, collectGenericReasons(tmpl.ID, step.ID, "on_agent_error", step.Events.OnAgentError)...)
		}
	}
	return out
}

func collectGenericReasons(templateID, stepID, trigger string, actions []models.GenericAction) []reasonOccurrence {
	var out []reasonOccurrence
	for _, a := range actions {
		if reason, ok := a.Config["reason"].(string); ok && reason != "" {
			out = append(out, reasonOccurrence{templateID, stepID, trigger, reason})
		}
	}
	return out
}

// TestPromptReasonContract_AllShippedReasonsResolve asserts that every
// `reason:` value emitted by a shipped workflow template resolves to a real
// BuildPrompt case instead of falling through to the bare fallback string.
// It must fail against the unmodified office-default.yml (4 of 7 reasons
// fall through) and pass once the YAML is corrected.
func TestPromptReasonContract_AllShippedReasonsResolve(t *testing.T) {
	templates, err := workflowcfg.LoadTemplates()
	if err != nil {
		t.Fatalf("LoadTemplates: %v", err)
	}

	occurrences := collectReasonOccurrences(templates)
	if len(occurrences) == 0 {
		t.Fatal("no reason: occurrences found across shipped workflow templates — test setup is broken")
	}

	for _, occ := range occurrences {
		got := BuildPrompt(&PromptContext{Reason: occ.reason})
		if strings.HasPrefix(got, fallbackPromptPrefix) {
			t.Errorf(
				"template %q step %q trigger %q: reason %q falls through to the bare fallback prompt: %q",
				occ.template, occ.step, occ.trigger, occ.reason, got,
			)
		}
	}
}

// TestPromptReasonContract_ReviewAndApprovalStagesRouteToVerdictPrompt pins
// the office-default review/approval fan-out contract: reason task_assigned
// with payload stage_type "review" or "approval" must both produce the
// reviewer verdict prompt, not the generic "start working" default prompt.
// This guards the YAML's payload-based routing so it cannot silently rot.
func TestPromptReasonContract_ReviewAndApprovalStagesRouteToVerdictPrompt(t *testing.T) {
	for _, stage := range []string{"review", "approval"} {
		pc := &PromptContext{
			Reason:    RunReasonTaskAssigned,
			StageType: stage,
			TaskID:    "T-1",
			TaskTitle: "Add auth",
		}
		got := BuildPrompt(pc)
		if !strings.Contains(got, "Submit your verdict") {
			t.Errorf("stage_type=%q: expected reviewer verdict prompt, got %q", stage, got)
		}
	}
}
