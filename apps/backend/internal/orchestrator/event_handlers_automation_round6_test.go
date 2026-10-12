package orchestrator

import (
	"encoding/json"
	"time"

	"github.com/kandev/kandev/internal/automation"
)

func retrySnapshotForTest(run *automation.AutomationRun, a *automation.Automation) string {
	triggerID := run.TriggerID
	if triggerID == "" {
		triggerID = "retry-trigger"
	}
	snapshot, _ := json.Marshal(automation.RetryLaunchConfigSnapshot{
		Version:            automation.RetryLaunchConfigVersion,
		AutomationID:       a.ID,
		WorkspaceID:        a.WorkspaceID,
		Name:               a.Name,
		WorkflowID:         a.WorkflowID,
		WorkflowStepID:     a.WorkflowStepID,
		AgentProfileID:     a.AgentProfileID,
		ExecutorProfileID:  a.ExecutorProfileID,
		Prompt:             a.Prompt,
		TaskTitleTemplate:  a.TaskTitleTemplate,
		TaskMode:           a.TaskMode,
		RepositoryMode:     a.RepositoryMode,
		ContinuationPolicy: a.ContinuationPolicy,
		TriggerID:          triggerID,
		TriggerType:        run.TriggerType,
		TriggerData:        json.RawMessage(`{"projection_version":1}`),
		ResolvedPrompt:     a.Prompt,
		ResolvedTitle:      a.Name,
		ResolvedTriggerAt:  time.Now().UTC(),
		RetryPolicy:        a.RetryPolicy,
	})
	return string(snapshot)
}
