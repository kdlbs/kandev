package configloader

import (
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

func TestMarshalUnmarshalSettings(t *testing.T) {
	original := WorkspaceSettings{
		Name:            "test-ws",
		Description:     "A test workspace",
		ApprovalDefault: "required",
		BudgetDefault:   5000,
		TaskPrefix:      "TST",
	}

	data, err := MarshalSettings(original)
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}

	got, err := UnmarshalSettings(data)
	if err != nil {
		t.Fatalf("unmarshal settings: %v", err)
	}

	if got.Name != original.Name {
		t.Errorf("name = %q, want %q", got.Name, original.Name)
	}
	if got.Description != original.Description {
		t.Errorf("description = %q, want %q", got.Description, original.Description)
	}
	if got.ApprovalDefault != original.ApprovalDefault {
		t.Errorf("approval_default = %q, want %q", got.ApprovalDefault, original.ApprovalDefault)
	}
	if got.BudgetDefault != original.BudgetDefault {
		t.Errorf("budget_default = %d, want %d", got.BudgetDefault, original.BudgetDefault)
	}
	if got.TaskPrefix != original.TaskPrefix {
		t.Errorf("task_prefix = %q, want %q", got.TaskPrefix, original.TaskPrefix)
	}
}

func TestMarshalUnmarshalAgent(t *testing.T) {
	original := &models.AgentInstance{
		ID:                    "agent-1",
		Name:                  "ceo",
		Role:                  models.AgentRoleCEO,
		Icon:                  "crown",
		ReportsTo:             "",
		BudgetMonthlyCents:    10000,
		MaxConcurrentSessions: 3,
		DesiredSkills:         "code-review,memory",
		ExecutorPreference:    "local_docker",
	}

	data, err := MarshalAgent(original)
	if err != nil {
		t.Fatalf("marshal agent: %v", err)
	}

	got, err := UnmarshalAgent(data, "test-ws")
	if err != nil {
		t.Fatalf("unmarshal agent: %v", err)
	}

	if got.ID != original.ID {
		t.Errorf("id = %q, want %q", got.ID, original.ID)
	}
	if got.Name != original.Name {
		t.Errorf("name = %q, want %q", got.Name, original.Name)
	}
	if got.Role != original.Role {
		t.Errorf("role = %q, want %q", got.Role, original.Role)
	}
	if got.WorkspaceID != "test-ws" {
		t.Errorf("workspace_id = %q, want %q", got.WorkspaceID, "test-ws")
	}
	if got.BudgetMonthlyCents != original.BudgetMonthlyCents {
		t.Errorf("budget = %d, want %d", got.BudgetMonthlyCents, original.BudgetMonthlyCents)
	}
	if got.DesiredSkills != original.DesiredSkills {
		t.Errorf("desired_skills = %q, want %q", got.DesiredSkills, original.DesiredSkills)
	}
	if got.Status != models.AgentStatusIdle {
		t.Errorf("status = %q, want %q", got.Status, models.AgentStatusIdle)
	}
}

func TestMarshalUnmarshalProject(t *testing.T) {
	original := &models.Project{
		ID:          "proj-1",
		Name:        "api-migration",
		Description: "Migrate v1 to v2 API",
		Status:      models.ProjectStatusActive,
		Color:       "#ff0000",
		BudgetCents: 50000,
	}

	data, err := MarshalProject(original)
	if err != nil {
		t.Fatalf("marshal project: %v", err)
	}

	got, err := UnmarshalProject(data, "test-ws")
	if err != nil {
		t.Fatalf("unmarshal project: %v", err)
	}

	if got.Name != original.Name {
		t.Errorf("name = %q, want %q", got.Name, original.Name)
	}
	if got.Status != original.Status {
		t.Errorf("status = %q, want %q", got.Status, original.Status)
	}
	if got.BudgetCents != original.BudgetCents {
		t.Errorf("budget = %d, want %d", got.BudgetCents, original.BudgetCents)
	}
}

func TestUnmarshalProjectDefaultStatus(t *testing.T) {
	data := []byte("name: test\n")
	got, err := UnmarshalProject(data, "ws")
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Status != models.ProjectStatusActive {
		t.Errorf("status = %q, want %q", got.Status, models.ProjectStatusActive)
	}
}

func TestMarshalUnmarshalRoutine(t *testing.T) {
	original := &models.Routine{
		ID:                "routine-1",
		Name:              "daily-digest",
		Description:       "Send daily digest to team",
		TaskTemplate:      "Summarize today's activity",
		Status:            "active",
		ConcurrencyPolicy: "skip",
	}

	data, err := MarshalRoutine(original)
	if err != nil {
		t.Fatalf("marshal routine: %v", err)
	}

	got, err := UnmarshalRoutine(data, "test-ws")
	if err != nil {
		t.Fatalf("unmarshal routine: %v", err)
	}

	if got.Name != original.Name {
		t.Errorf("name = %q, want %q", got.Name, original.Name)
	}
	if got.ConcurrencyPolicy != original.ConcurrencyPolicy {
		t.Errorf("concurrency = %q, want %q", got.ConcurrencyPolicy, original.ConcurrencyPolicy)
	}
}

func TestUnmarshalSettingsInvalidYAML(t *testing.T) {
	_, err := UnmarshalSettings([]byte(":::invalid"))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestUnmarshalAgentInvalidYAML(t *testing.T) {
	_, err := UnmarshalAgent([]byte(":::invalid"), "ws")
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestIsYAMLFile(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"agent.yml", true},
		{"agent.yaml", true},
		{"agent.YML", true},
		{"readme.md", false},
		{"config.json", false},
	}
	for _, tt := range tests {
		if got := isYAMLFile(tt.name); got != tt.want {
			t.Errorf("isYAMLFile(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}
