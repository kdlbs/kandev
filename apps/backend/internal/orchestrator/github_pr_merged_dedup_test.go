package orchestrator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/automation"
)

// recordSuccessRun writes the task_created row: this is the "the run really
// happened" record, so github_pr_merged must consume its dedup key here —
// otherwise the same merged PR could refire the automation indefinitely.
// recordFailedRun's and maybeSkipForConcurrencyCap's dedup-key blanking is
// covered elsewhere (event_handlers_automation_test.go, scheduler_test.go);
// this is the missing "successful/task-created paths consume it" half of
// that same review request — recordSuccessRun had no direct test at all.
func TestRecordSuccessRun_GitHubPRMerged_ConsumesDedupKey(t *testing.T) {
	autoSvc := &stubAutomationService{}
	svc := &Service{logger: testLogger()}
	svc.SetAutomationService(autoSvc)

	evt := &automation.AutomationTriggeredEvent{
		AutomationID: "a-1",
		TriggerID:    "trg-1",
		TriggerType:  automation.TriggerTypeGitHubPRMerged,
		DedupKey:     "pr_merged:task-1:acme/api#42",
	}
	err := svc.recordSuccessRun(context.Background(), evt, "task-created-1")
	require.NoError(t, err)

	require.Len(t, autoSvc.runs, 1)
	require.Equal(t, "pr_merged:task-1:acme/api#42", autoSvc.runs[0].DedupKey,
		"a successful task-created row must consume the dedup key so a duplicate PR-merged event does not refire")
	require.Equal(t, automation.RunStatusTaskCreated, autoSvc.runs[0].Status)
	require.Equal(t, "task-created-1", autoSvc.runs[0].TaskID)
}
