package gitlab

import (
	"context"
	"testing"
)

func TestService_GetActionPresetsOrDefault_FallsBackToDefaults(t *testing.T) {
	svc := newServiceWithStore(t)
	got, err := svc.GetActionPresetsOrDefault(context.Background(), "ws-new")
	if err != nil {
		t.Fatalf("GetActionPresetsOrDefault: %v", err)
	}
	if got == nil {
		t.Fatalf("expected non-nil presets")
	}
	if len(got.MR) == 0 || len(got.Issue) == 0 {
		t.Fatalf("expected defaults to be injected when none stored, got %+v", got)
	}
	if got.MR[0].ID == "" {
		t.Fatalf("default presets missing ID: %+v", got.MR[0])
	}
}

func TestService_UpdateActionPresets_PartialMerge(t *testing.T) {
	svc := newServiceWithStore(t)
	ctx := context.Background()
	// First write only MR presets.
	mrPresets := []ActionPreset{{ID: "x", Label: "Custom", PromptTemplate: "do {{url}}"}}
	if _, err := svc.UpdateActionPresets(ctx, &UpdateActionPresetsRequest{
		WorkspaceID: "ws-1",
		MR:          &mrPresets,
	}); err != nil {
		t.Fatalf("UpdateActionPresets: %v", err)
	}
	// Read back: MR should be custom, Issue should still be defaults (from
	// GetActionPresetsOrDefault).
	got, err := svc.GetActionPresetsOrDefault(ctx, "ws-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.MR) != 1 || got.MR[0].ID != "x" {
		t.Fatalf("MR presets not persisted: %+v", got.MR)
	}
	if len(got.Issue) == 0 {
		t.Fatalf("Issue presets should fallback to defaults when empty")
	}
}

func TestServiceUpdateActionPresetsNormalizesAndDropsBlankEntries(t *testing.T) {
	svc := newServiceWithStore(t)
	ctx := context.Background()
	mrPresets := []ActionPreset{
		{Label: "  Review carefully  ", Hint: "  inspect  ", Icon: " eye ", PromptTemplate: "  Review {{url}}  "},
		{ID: "blank-label", Label: " ", PromptTemplate: "do work"},
		{ID: "blank-prompt", Label: "No prompt", PromptTemplate: "  "},
	}

	got, err := svc.UpdateActionPresets(ctx, &UpdateActionPresetsRequest{WorkspaceID: "ws-1", MR: &mrPresets})
	if err != nil {
		t.Fatalf("UpdateActionPresets: %v", err)
	}
	if len(got.MR) != 1 {
		t.Fatalf("normalized MR presets = %+v, want one valid entry", got.MR)
	}
	preset := got.MR[0]
	if preset.ID == "" || preset.Label != "Review carefully" || preset.Hint != "inspect" || preset.Icon != "eye" || preset.PromptTemplate != "Review {{url}}" {
		t.Fatalf("normalized preset = %+v", preset)
	}
}

func TestService_ResetActionPresets(t *testing.T) {
	svc := newServiceWithStore(t)
	ctx := context.Background()
	custom := []ActionPreset{{ID: "x", Label: "x"}}
	if _, err := svc.UpdateActionPresets(ctx, &UpdateActionPresetsRequest{
		WorkspaceID: "ws-1",
		MR:          &custom,
	}); err != nil {
		t.Fatalf("UpdateActionPresets: %v", err)
	}
	got, err := svc.ResetActionPresets(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ResetActionPresets: %v", err)
	}
	if len(got.MR) == 0 || got.MR[0].ID == "x" {
		t.Fatalf("reset should restore defaults, got %+v", got.MR)
	}
}

func newServiceWithStore(t *testing.T) *Service {
	t.Helper()
	store := newTestStore(t)
	log := newTestLogger(t)
	svc := NewService("https://gitlab.com", NewNoopClient("https://gitlab.com"), AuthMethodNone, nil, log)
	svc.SetStore(store)
	return svc
}
