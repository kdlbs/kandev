package controller

import (
	"context"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"testing"
)

// @covers AC-AGENTS-SESSION-SETTING-SEMANTICS-001.4, .5, .7
func TestPiThinkingProfileSaveAndClear(t *testing.T) {
	ctrl, repo := newSQLiteBackedController(t)
	ctx := context.Background()
	if err := repo.CreateAgent(ctx, &models.Agent{ID: "pi", Name: "pi-acp"}); err != nil {
		t.Fatal(err)
	}
	saved := &models.AgentProfile{AgentID: "pi", Name: "Planning", Model: "cursor-agent/gemini-4-argon-high", Mode: "medium", RequireExactModel: true}
	if err := repo.CreateAgentProfile(ctx, saved); err != nil {
		t.Fatal(err)
	}
	name := "Renamed"
	if _, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{ID: saved.ID, Name: &name}); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.GetAgentProfile(ctx, saved.ID)
	if err != nil || stored.Mode != "medium" {
		t.Fatalf("omitted selection changed: %+v %v", stored, err)
	}
	options := map[string]string{"thought_level": "off", "custom": "keep"}
	updated, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{ID: saved.ID, ConfigOptions: &options})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Mode != "" || updated.ConfigOptions["thought_level"] != "off" {
		t.Fatalf("duplicate alias survived: %+v", updated)
	}
	empty := ""
	options = map[string]string{"custom": "keep"}
	if _, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{ID: saved.ID, Mode: &empty, ConfigOptions: &options}); err != nil {
		t.Fatal(err)
	}
	stored, err = repo.GetAgentProfile(ctx, saved.ID)
	if err != nil || stored.Mode != "" || stored.ConfigOptions["thought_level"] != "" || stored.ConfigOptions["custom"] != "keep" || stored.Model != saved.Model || !stored.RequireExactModel {
		t.Fatalf("default clear lost intent: %+v %v", stored, err)
	}
}
