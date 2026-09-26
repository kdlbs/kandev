package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// TestUpdateAgentInstanceConfigFields_UpdatesOnlyOwnedColumns proves the
// column-scoped writer touches only the six columns a config import owns
// and leaves every other column - in particular status and pause_reason,
// which a concurrent writer like UpdateAgentStatusFields owns - exactly as
// they were.
func TestUpdateAgentInstanceConfigFields_UpdatesOnlyOwnedColumns(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	agent := fullAgentInstance("agent-1", "ws-1", "ada")
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("CreateAgentInstance: %v", err)
	}
	// CreateAgentInstance always writes settings='{}'; set a real value
	// through the column's real owner (UpdateAgentSettings) so this test
	// can prove UpdateAgentInstanceConfigFields leaves it alone.
	const wantSettings = `{"routing":{"tier_source":"explicit"}}`
	if err := repo.UpdateAgentSettings(ctx, agent.ID, wantSettings); err != nil {
		t.Fatalf("UpdateAgentSettings: %v", err)
	}

	fields := sqlite.AgentInstanceConfigFields{
		Role:                  "cfo",
		Icon:                  "💼",
		BudgetMonthlyCents:    9900,
		MaxConcurrentSessions: 5,
		DesiredSkills:         `["finance"]`,
		ExecutorPreference:    "sprites",
	}
	if err := repo.UpdateAgentInstanceConfigFields(ctx, agent.ID, fields); err != nil {
		t.Fatalf("UpdateAgentInstanceConfigFields: %v", err)
	}

	rows, err := repo.ListAgentInstances(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListAgentInstances: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	got := rows[0]

	// Owned columns landed.
	if string(got.Role) != fields.Role {
		t.Errorf("Role = %q, want %q", got.Role, fields.Role)
	}
	if got.Icon != fields.Icon {
		t.Errorf("Icon = %q, want %q", got.Icon, fields.Icon)
	}
	if got.BudgetMonthlyCents != fields.BudgetMonthlyCents {
		t.Errorf("BudgetMonthlyCents = %d, want %d", got.BudgetMonthlyCents, fields.BudgetMonthlyCents)
	}
	if got.MaxConcurrentSessions != fields.MaxConcurrentSessions {
		t.Errorf("MaxConcurrentSessions = %d, want %d", got.MaxConcurrentSessions, fields.MaxConcurrentSessions)
	}
	if got.DesiredSkills != fields.DesiredSkills {
		t.Errorf("DesiredSkills = %q, want %q", got.DesiredSkills, fields.DesiredSkills)
	}
	if got.ExecutorPreference != fields.ExecutorPreference {
		t.Errorf("ExecutorPreference = %q, want %q", got.ExecutorPreference, fields.ExecutorPreference)
	}

	// Everything else stayed exactly as seeded.
	if string(got.Status) != string(agent.Status) {
		t.Errorf("Status changed: got %q, want %q", got.Status, agent.Status)
	}
	if got.PauseReason != agent.PauseReason {
		t.Errorf("PauseReason changed: got %q, want %q", got.PauseReason, agent.PauseReason)
	}
	if got.Settings != wantSettings {
		t.Errorf("Settings changed: got %q, want %q", got.Settings, wantSettings)
	}
	if got.Model != agent.Model {
		t.Errorf("Model changed: got %q, want %q", got.Model, agent.Model)
	}
	if got.ReportsTo != agent.ReportsTo {
		t.Errorf("ReportsTo changed: got %q, want %q", got.ReportsTo, agent.ReportsTo)
	}
	if got.ConsecutiveFailures != agent.ConsecutiveFailures {
		t.Errorf("ConsecutiveFailures changed: got %d, want %d", got.ConsecutiveFailures, agent.ConsecutiveFailures)
	}
	if got.CooldownSec != agent.CooldownSec {
		t.Errorf("CooldownSec changed: got %d, want %d", got.CooldownSec, agent.CooldownSec)
	}
}

// TestUpdateAgentReportsTo_UpdatesOnlyReportsTo proves that the second config
// import pass cannot overwrite state that belongs to the agent runtime.
func TestUpdateAgentReportsTo_UpdatesOnlyReportsTo(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	agent := fullAgentInstance("agent-1", "ws-1", "ada")
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("CreateAgentInstance: %v", err)
	}
	const wantSettings = `{"routing":{"tier_source":"explicit"}}`
	if err := repo.UpdateAgentSettings(ctx, agent.ID, wantSettings); err != nil {
		t.Fatalf("UpdateAgentSettings: %v", err)
	}
	if err := repo.UpdateAgentStatusFields(ctx, agent.ID, "paused", "manual pause"); err != nil {
		t.Fatalf("UpdateAgentStatusFields: %v", err)
	}

	if err := repo.UpdateAgentReportsTo(ctx, agent.ID, "manager-1"); err != nil {
		t.Fatalf("UpdateAgentReportsTo: %v", err)
	}

	got, err := repo.GetAgentInstance(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetAgentInstance: %v", err)
	}
	if got.ReportsTo != "manager-1" {
		t.Errorf("ReportsTo = %q, want %q", got.ReportsTo, "manager-1")
	}
	if string(got.Status) != "paused" {
		t.Errorf("Status changed: got %q, want %q", got.Status, "paused")
	}
	if got.PauseReason != "manual pause" {
		t.Errorf("PauseReason changed: got %q, want %q", got.PauseReason, "manual pause")
	}
	if got.Settings != wantSettings {
		t.Errorf("Settings changed: got %q, want %q", got.Settings, wantSettings)
	}
	if got.Role != agent.Role {
		t.Errorf("Role changed: got %q, want %q", got.Role, agent.Role)
	}
}
