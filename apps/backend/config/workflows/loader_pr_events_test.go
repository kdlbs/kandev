package workflows

import (
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/kandev/kandev/internal/workflow/models"
)

// TestConvertEvents_PREvents pins that built-in templates can declare the PR
// lifecycle triggers with the same generic-action allow-list as the other
// event-driven triggers.
func TestConvertEvents_PREvents(t *testing.T) {
	const yamlDoc = `
on_pr_merged:
  - type: queue_run
    config:
      reason: pr_merged
on_pr_closed:
  - type: clear_decisions
`
	var e stepEventsYAML
	if err := yaml.Unmarshal([]byte(yamlDoc), &e); err != nil {
		t.Fatalf("unmarshal yaml: %v", err)
	}
	events, err := convertEvents(e)
	if err != nil {
		t.Fatalf("convertEvents returned error: %v", err)
	}
	if len(events.OnPRMerged) != 1 || events.OnPRMerged[0].Type != models.GenericActionQueueRun {
		t.Fatalf("on_pr_merged = %#v, want one queue_run", events.OnPRMerged)
	}
	if reason, _ := events.OnPRMerged[0].Config["reason"].(string); reason != "pr_merged" {
		t.Fatalf("on_pr_merged reason = %q, want pr_merged", reason)
	}
	if len(events.OnPRClosed) != 1 || events.OnPRClosed[0].Type != models.GenericActionClearDecisions {
		t.Fatalf("on_pr_closed = %#v, want one clear_decisions", events.OnPRClosed)
	}
}

func TestConvertEvents_PREventsRejectUnknownAction(t *testing.T) {
	const yamlDoc = `
on_pr_merged:
  - type: archive_task
`
	var e stepEventsYAML
	if err := yaml.Unmarshal([]byte(yamlDoc), &e); err != nil {
		t.Fatalf("unmarshal yaml: %v", err)
	}
	if _, err := convertEvents(e); err == nil {
		t.Fatal("expected an unknown on_pr_merged action type to be rejected")
	}
}
