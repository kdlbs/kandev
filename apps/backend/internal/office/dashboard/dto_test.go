package dashboard

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

func TestNewDashboardResponseUsesPublicJSONShape(t *testing.T) {
	resp := NewDashboardResponse(&models.DashboardData{
		AgentCount:         2,
		RunningCount:       1,
		PausedCount:        1,
		ErrorCount:         0,
		TasksInProgress:    3,
		OpenTasks:          4,
		BlockedTasks:       5,
		MonthSpendSubcents: 123,
		PendingApprovals:   6,
		TaskCount:          7,
		SkillCount:         8,
		RoutineCount:       9,
		RunActivity: []models.RunActivityDay{{
			Date:      "2026-06-16",
			Succeeded: 2,
			Failed:    1,
			Other:     0,
		}},
		TaskBreakdown: models.TaskBreakdown{Open: 1, InProgress: 2, Blocked: 3, Done: 4},
		RecentTasks: []models.RecentTask{{
			ID:         "task-1",
			Identifier: "KAN-1",
			Title:      "Ship dashboard",
			Status:     stateCompleted,
			UpdatedAt:  "2026-06-16T12:00:00Z",
		}},
	}, nil)

	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal response: %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal response: %v", err)
	}
	for _, key := range []string{
		"agent_count",
		"task_count",
		"skill_count",
		"routine_count",
		"run_activity",
		"task_breakdown",
		"recent_tasks",
		"agent_summaries",
	} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("expected %q in dashboard response: %s", key, raw)
		}
	}

	var recent []RecentTaskDTO
	if err := json.Unmarshal(decoded["recent_tasks"], &recent); err != nil {
		t.Fatalf("Unmarshal recent_tasks: %v", err)
	}
	if len(recent) != 1 || recent[0].Status != statusDoneLowercase {
		t.Fatalf("recent task status = %#v, want %q", recent, statusDoneLowercase)
	}

	var summaries []AgentSummary
	if err := json.Unmarshal(decoded["agent_summaries"], &summaries); err != nil {
		t.Fatalf("Unmarshal agent_summaries: %v", err)
	}
	if summaries == nil {
		t.Fatal("agent_summaries should encode as an empty array, not null")
	}
}
