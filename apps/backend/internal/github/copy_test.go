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

	got, err := svc.CopyWorkspaceSettingsToWorkspace(ctx, src, dst)
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	if got.WorkspaceID != dst || got.RepoScopeMode != RepoScopeModeRepos {
		t.Errorf("copied settings identity/scope mismatch: %+v", got)
	}
	if len(got.RepoScopeRepos) != 1 || got.RepoScopeRepos[0].Owner != "kdlbs" {
		t.Errorf("repo scope not copied: %+v", got.RepoScopeRepos)
	}
	if string(got.SavedPresets) != `[{"id":"p1","kind":"pr","label":"Mine"}]` {
		t.Errorf("saved presets not copied: %s", got.SavedPresets)
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
		t.Fatalf("expected ErrWorkspaceSettingsValidation, got %v", err)
	}
}
