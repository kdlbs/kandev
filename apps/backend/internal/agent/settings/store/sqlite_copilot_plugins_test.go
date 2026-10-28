package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

func TestCopilotPluginSelectionRoundTrip(t *testing.T) {
	repo := newFreshRepo(t)
	ctx := context.Background()
	agent := &models.Agent{Name: "copilot-acp"}
	if err := repo.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	profile := &models.AgentProfile{
		AgentID:                agent.ID,
		Name:                   "plugins",
		CopilotSelectedPlugins: []string{"installed:copilot-plugins/spark", "live:fixture-market/harness"},
	}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	got, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("read created profile: %v", err)
	}
	if !reflect.DeepEqual(got.CopilotSelectedPlugins, profile.CopilotSelectedPlugins) {
		t.Fatalf("created plugins = %#v, want %#v", got.CopilotSelectedPlugins, profile.CopilotSelectedPlugins)
	}

	got.CopilotSelectedPlugins = []string{}
	if err := repo.UpdateAgentProfile(ctx, got); err != nil {
		t.Fatalf("clear plugins: %v", err)
	}
	got, err = repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("read cleared profile: %v", err)
	}
	if got.CopilotSelectedPlugins == nil || len(got.CopilotSelectedPlugins) != 0 {
		t.Fatalf("explicit empty plugins = %#v, want []", got.CopilotSelectedPlugins)
	}
}

func TestCopilotPluginSelectionConcurrentOmission(t *testing.T) {
	repo := newFreshRepo(t)
	ctx := context.Background()
	agent := &models.Agent{Name: "copilot-acp"}
	if err := repo.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	profile := &models.AgentProfile{AgentID: agent.ID, Name: "plugins", CopilotSelectedPlugins: []string{}}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	// Editor A reads the profile before editor B commits a selection.
	stale, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("read stale profile: %v", err)
	}
	selection := []string{"installed:copilot-plugins/spark"}
	committed := *stale
	committed.CopilotSelectedPlugins = selection
	if err := repo.UpdateAgentProfileWithSelectionIntent(ctx, &committed, nil, &selection); err != nil {
		t.Fatalf("commit explicit selection: %v", err)
	}
	if !reflect.DeepEqual(committed.CopilotSelectedPlugins, selection) {
		t.Fatalf("committed response = %#v, want %#v", committed.CopilotSelectedPlugins, selection)
	}

	// Editor A saves an unrelated field with an omitted selection. The
	// committed explicit selection must survive and be returned.
	stale.Name = "renamed"
	if err := repo.UpdateAgentProfileWithSelectionIntent(ctx, stale, nil, nil); err != nil {
		t.Fatalf("save omitted selection: %v", err)
	}
	if !reflect.DeepEqual(stale.CopilotSelectedPlugins, selection) {
		t.Fatalf("omitted update response = %#v, want %#v", stale.CopilotSelectedPlugins, selection)
	}
	got, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("read final profile: %v", err)
	}
	if got.Name != "renamed" || !reflect.DeepEqual(got.CopilotSelectedPlugins, selection) {
		t.Fatalf("final profile = %#v, want renamed with committed plugins", got)
	}
}

func TestCopilotPluginSelectionMigration(t *testing.T) {
	db := newLegacyDB(t)
	ctx := context.Background()
	if _, err := db.Exec(`INSERT INTO agents (id, name, created_at, updated_at) VALUES ('a1', 'copilot-acp', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	repo, err := newSQLiteRepository(db, db, nil, false)
	if err != nil {
		t.Fatalf("newSQLiteRepository: %v", err)
	}
	profile := &models.AgentProfile{
		AgentID: "a1", Name: "Migrated", CopilotSelectedPlugins: []string{"installed:m/p"},
	}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("create migrated profile: %v", err)
	}
	got, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("read migrated profile: %v", err)
	}
	if !reflect.DeepEqual(got.CopilotSelectedPlugins, profile.CopilotSelectedPlugins) {
		t.Fatalf("migrated selection = %#v, want %#v", got.CopilotSelectedPlugins, profile.CopilotSelectedPlugins)
	}
}
