package github

import (
	"context"
	"errors"
	"testing"
)

func newCopyTestService(t *testing.T) *Service {
	t.Helper()
	client := NewMockClient()
	store := newTestStore(t)
	return NewService(client, AuthMethodPAT, nil, store, nil, testLogger(t))
}

func TestCopyWorkspaceSettingsToWorkspace_CopiesSettings(t *testing.T) {
	svc := newCopyTestService(t)
	ctx := context.Background()
	const src, dst = "ws-src", "ws-dst"

	if err := svc.UpsertWorkspaceSettings(ctx, &WorkspaceSettings{
		WorkspaceID:         src,
		RepoScopeMode:       RepoScopeModeRepos,
		RepoScopeRepos:      []RepoFilter{{Owner: "kdlbs", Name: "kandev"}},
		SavedPresets:        []byte(`[{"id":"p1","kind":"pr","label":"Mine"}]`),
		DefaultQueryPresets: []byte(`{"pr":[],"issue":[]}`),
	}); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	if _, err := svc.UpdateActionPresets(ctx, &UpdateActionPresetsRequest{
		WorkspaceID: src,
		PR:          &[]ActionPreset{{ID: "custom", Label: "Custom", PromptTemplate: "do {{url}}"}},
	}); err != nil {
		t.Fatalf("seed source action presets: %v", err)
	}

	got, err := svc.CopyWorkspaceSettingsToWorkspace(ctx, src, dst)
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	if got.WorkspaceID != dst || got.RepoScopeMode != RepoScopeModeRepos {
		t.Errorf("copied settings identity/scope mismatch: %+v", got)
	}
	if len(got.RepoScopeRepos) != 1 || got.RepoScopeRepos[0].Owner != "kdlbs" ||
		got.RepoScopeRepos[0].Name != "kandev" {
		t.Errorf("repo scope not copied: %+v", got.RepoScopeRepos)
	}
	if string(got.SavedPresets) != `[{"id":"p1","kind":"pr","label":"Mine"}]` {
		t.Errorf("saved presets not copied: %s", got.SavedPresets)
	}
	if string(got.DefaultQueryPresets) != `{"pr":[],"issue":[]}` {
		t.Errorf("default query presets not copied: %s", got.DefaultQueryPresets)
	}

	presets, err := svc.GetActionPresets(ctx, dst)
	if err != nil {
		t.Fatalf("get copied action presets: %v", err)
	}
	if len(presets.PR) != 1 || presets.PR[0].ID != "custom" {
		t.Errorf("action presets not copied: %+v", presets.PR)
	}
}

func TestCopyWorkspaceSettingsToWorkspace_SameWorkspace(t *testing.T) {
	svc := newCopyTestService(t)
	if _, err := svc.CopyWorkspaceSettingsToWorkspace(context.Background(), "ws-1", "ws-1"); !errors.Is(err, ErrSameWorkspace) {
		t.Fatalf("expected ErrSameWorkspace, got %v", err)
	}
}

func TestCopyWorkspaceSettingsToWorkspace_MissingIDs(t *testing.T) {
	svc := newCopyTestService(t)
	if _, err := svc.CopyWorkspaceSettingsToWorkspace(context.Background(), "", "ws-dst"); !errors.Is(err, ErrWorkspaceSettingsValidation) {
		t.Fatalf("expected ErrWorkspaceSettingsValidation for empty source, got %v", err)
	}
	if _, err := svc.CopyWorkspaceSettingsToWorkspace(context.Background(), "ws-src", ""); !errors.Is(err, ErrWorkspaceSettingsValidation) {
		t.Fatalf("expected ErrWorkspaceSettingsValidation for empty destination, got %v", err)
	}
}
