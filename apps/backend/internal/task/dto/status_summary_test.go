package dto

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/statussummary"
)

func TestTaskDTOStatusSummaryIsBoundedAndOptional(t *testing.T) {
	without := FromTask(&models.Task{ID: "task-without-summary"})
	withoutJSON, err := json.Marshal(without)
	if err != nil {
		t.Fatalf("marshal task without summary: %v", err)
	}
	if strings.Contains(string(withoutJSON), "status_summary") {
		t.Fatalf("missing summary should remain omitted: %s", withoutJSON)
	}

	with := FromTask(&models.Task{ID: "task-with-summary"})
	with.StatusSummary = &statussummary.TaskStatusSummary{
		Revision: 1,
		ActiveError: &statussummary.ActiveErrorSummary{
			SessionID: "session-1",
			Stamp:     "error-1",
			Preview:   "safe preview",
		},
	}
	withJSON, err := json.Marshal(with)
	if err != nil {
		t.Fatalf("marshal task with summary: %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(withJSON, &decoded); err != nil {
		t.Fatalf("decode task JSON: %v", err)
	}
	if _, ok := decoded["status_summary"]; !ok {
		t.Fatalf("task JSON missing status_summary: %s", withJSON)
	}
	if _, ok := decoded["messages"]; ok {
		t.Fatal("task status summary must not carry transcript data")
	}
}
