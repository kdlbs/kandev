package controller

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/models"
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

func TestPiThinkingProfileLegacyModePatch(t *testing.T) {
	ctrl, repo := newSQLiteBackedController(t)
	ctx := context.Background()
	if err := repo.CreateAgent(ctx, &models.Agent{ID: "pi", Name: "pi-acp"}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		mode    string
		options *map[string]string
		want    string
	}{
		{name: "replace", mode: "high", want: "high"},
		{name: "clear"},
		{name: "submitted config wins", mode: "high", options: &map[string]string{"thought_level": "off", "custom": "keep"}, want: "off"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profile := &models.AgentProfile{
				AgentID: "pi", Name: tc.name, Model: "cursor-agent/gemini-4-argon-high", RequireExactModel: true,
				ConfigOptions: map[string]string{"thought_level": "medium", "custom": "keep"},
			}
			if err := repo.CreateAgentProfile(ctx, profile); err != nil {
				t.Fatal(err)
			}
			if _, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{ID: profile.ID, Mode: &tc.mode, ConfigOptions: tc.options}); err != nil {
				t.Fatal(err)
			}
			stored, err := repo.GetAgentProfile(ctx, profile.ID)
			if err != nil {
				t.Fatal(err)
			}
			if level, present := stored.ConfigOptions["thought_level"]; level != tc.want || present != (tc.want != "") || stored.Mode != "" {
				t.Errorf("stored thinking = %q (present %t), mode = %q; want thinking %q and no legacy mode", level, present, stored.Mode, tc.want)
			}
			if stored.ConfigOptions["custom"] != "keep" || stored.Model != profile.Model || !stored.RequireExactModel {
				t.Errorf("patch changed unrelated profile settings: %+v", stored)
			}
		})
	}
}

func TestPiThinkingProfileCreateCanonicalizesLegacyMode(t *testing.T) {
	ctrl, repo := newSQLiteBackedController(t)
	ctx := context.Background()
	if err := ctrl.agentRegistry.Register(agents.NewPiACP()); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateAgent(ctx, &models.Agent{ID: "pi", Name: "pi-acp"}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		options map[string]string
		want    string
	}{
		{name: "legacy only", want: "medium"},
		{name: "canonical wins", options: map[string]string{"thought_level": "off"}, want: "off"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profile, err := ctrl.CreateProfile(ctx, CreateProfileRequest{
				AgentID: "pi", Name: tc.name, Mode: "medium", ConfigOptions: tc.options,
			})
			if err != nil {
				t.Fatal(err)
			}
			stored, err := repo.GetAgentProfile(ctx, profile.ID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.Mode != "" || stored.ConfigOptions["thought_level"] != tc.want {
				t.Fatalf("stored mode = %q, options = %v", stored.Mode, stored.ConfigOptions)
			}
		})
	}
}
