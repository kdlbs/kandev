package dto

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/statussummary"
)

func TestTaskDTOStatusSummaryPreservesProcessExitStatus(t *testing.T) {
	exitCode := 1
	task := FromTask(&models.Task{ID: "task-process-exit"})
	task.StatusSummary = &statussummary.TaskStatusSummary{
		Revision: 1,
		ActiveError: &statussummary.ActiveErrorSummary{
			SessionID: "session-1",
			Stamp:     "failure-stamp",
			Preview:   models.AgentErrorCauseProcessExitedDetail,
			Causes: []models.AgentErrorCause{{
				Operation: models.AgentErrorCauseOperationResume,
				Code:      models.AgentErrorCauseCodeProcessExited,
				Reason:    models.AgentErrorCauseReasonMiseConfigUntrusted,
				ExitCode:  &exitCode,
			}},
		},
	}
	encoded, err := json.Marshal(task)
	if err != nil {
		t.Fatalf("marshal task DTO: %v", err)
	}
	var payload struct {
		StatusSummary struct {
			ActiveError struct {
				Causes []models.AgentErrorCause `json:"causes"`
			} `json:"active_error"`
		} `json:"status_summary"`
	}
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode task DTO: %v", err)
	}
	if len(payload.StatusSummary.ActiveError.Causes) != 1 {
		t.Fatalf("active error causes = %#v", payload.StatusSummary.ActiveError.Causes)
	}
	cause := payload.StatusSummary.ActiveError.Causes[0]
	if cause.Code != models.AgentErrorCauseCodeProcessExited ||
		cause.Reason != models.AgentErrorCauseReasonMiseConfigUntrusted ||
		cause.ExitCode == nil || *cause.ExitCode != 1 {
		t.Fatalf("process exit cause = %#v", cause)
	}
}
