package automation

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
)

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.10
// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-005.6
// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-008.4
// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-009.6
func TestExportPerPRSettings(t *testing.T) {
	svc, _, _, workflowLookup, stepLookup, _ := resolveTestFixture(t)
	wsLookup := &fakeExportWorkspaceLookup{exists: map[string]bool{"ws-1": true}}
	svc.SetExportWorkspaceLookup(wsLookup)
	workflowLookup.workflows["wf-1"] = &taskmodels.Workflow{ID: "wf-1", Name: "PR review"}
	stepLookup.steps["step-start"] = &workflowmodels.WorkflowStep{ID: "step-start", WorkflowID: "wf-1", Name: "Triage"}
	stepLookup.steps["step-review"] = &workflowmodels.WorkflowStep{ID: "step-review", WorkflowID: "wf-1", Name: "Review"}

	req := perPRRequest("per-pr")
	req.WorkflowStepID = "step-start"
	req.FollowUpPrompt = "Review only {{pr.previous_head_sha}}..{{pr.head_sha}}"
	req.PRContinuationStepID = "step-review"
	req.PRMergedAction = PREndActionArchive
	req.PRClosedAction = PREndActionDelete
	_, err := svc.CreateAutomation(context.Background(), req)
	require.NoError(t, err)
	plain := perPRRequest("plain")
	plain.ContinuationPolicy = ContinuationPolicyNewTask
	_, err = svc.CreateAutomation(context.Background(), plain)
	require.NoError(t, err)

	body, err := svc.ExportAutomationsDocument(context.Background(), "ws-1")
	require.NoError(t, err)
	var doc struct {
		Automations []map[string]any `yaml:"automations"`
	}
	require.NoError(t, yaml.Unmarshal(body, &doc))
	require.Len(t, doc.Automations, 2)
	perPR, other := doc.Automations[0], doc.Automations[1]
	require.Equal(t, "per-pr", perPR["name"])
	require.Equal(t, "reuse_per_pr", perPR["continuation_policy"])
	require.Equal(t, "archive", perPR["pr_merged_action"])
	require.Equal(t, "delete", perPR["pr_closed_action"])
	require.Equal(t, req.FollowUpPrompt, perPR["follow_up_prompt"])
	require.Equal(t, map[string]any{"name": "PR review", "step": "Triage", "continuation_step": "Review"}, perPR["workflow"])

	require.NotContains(t, other, "pr_merged_action", "end-of-PR actions are emitted only for reuse_per_pr")
	require.NotContains(t, other, "follow_up_prompt")

	text := string(body)
	order := []string{"continuation_policy:", "pr_merged_action:", "pr_closed_action:", "task_mode:"}
	last := -1
	for _, key := range order {
		idx := strings.Index(text, key)
		require.Greater(t, idx, last, "key order: %s", key)
		last = idx
	}
	require.Less(t, strings.Index(text, "  step: Triage"), strings.Index(text, "continuation_step: Review"))
}
