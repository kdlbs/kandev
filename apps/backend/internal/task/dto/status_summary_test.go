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

func TestTaskDTOStatusSummaryInvalidationIsExplicit(t *testing.T) {
	var task TaskDTO
	if err := json.Unmarshal([]byte(`{"id":"task-1","status_summary_invalidated":true}`), &task); err != nil {
		t.Fatalf("decode task invalidation: %v", err)
	}
	encoded, err := json.Marshal(task)
	if err != nil {
		t.Fatalf("encode task invalidation: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decode task JSON: %v", err)
	}
	if invalidated, _ := decoded["status_summary_invalidated"].(bool); !invalidated {
		t.Fatalf("task JSON missing explicit status-summary invalidation: %s", encoded)
	}
}

func TestEnrichTaskStatusSummaryDistinguishesInvalidationFromOmission(t *testing.T) {
	tests := []struct {
		name        string
		summaries   map[string]*statussummary.TaskStatusSummary
		wantSummary bool
		wantInvalid bool
	}{
		{name: "omitted", summaries: map[string]*statussummary.TaskStatusSummary{}},
		{name: "invalidated", summaries: map[string]*statussummary.TaskStatusSummary{"task-1": nil}, wantInvalid: true},
		{name: "carried", summaries: map[string]*statussummary.TaskStatusSummary{
			"task-1": {Revision: 3},
		}, wantSummary: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			task := TaskDTO{ID: "task-1"}
			EnrichTaskStatusSummary(&task, task.ID, test.summaries)
			if (task.StatusSummary != nil) != test.wantSummary ||
				task.StatusSummaryInvalidated != test.wantInvalid {
				t.Fatalf("summary = %+v, invalidated = %v", task.StatusSummary, task.StatusSummaryInvalidated)
			}
		})
	}
}
