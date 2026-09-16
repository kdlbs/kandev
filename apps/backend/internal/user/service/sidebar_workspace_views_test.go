package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/user/models"
	"github.com/kandev/kandev/internal/user/store"
)

func sidebarService(t *testing.T) (*Service, *casFakeRepo, *[]string) {
	t.Helper()
	repo := newCASFakeRepo(&models.UserSettings{UserID: store.DefaultUserID, SidebarViews: []models.SidebarView{{ID: "legacy", Name: "Original"}}, SidebarActiveViewID: "legacy"})
	svc := newCASService(repo, nil)
	ids := []string{"a", "b"}
	svc.SetSidebarWorkspaceAccess(func(context.Context) ([]string, error) { return ids, nil })
	return svc, repo, &ids
}

func sidebarPatch(t *testing.T, raw string) *UpdateUserSettingsRequest {
	t.Helper()
	var req UpdateUserSettingsRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatal(err)
	}
	return &req
}

func TestSidebarWorkspaceMigrationAndDefaults(t *testing.T) {
	svc, repo, ids := sidebarService(t)
	got, err := svc.GetUserSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.SidebarWorkspaceVersion != 1 || len(got.SidebarViewsByWorkspace) != 2 {
		t.Fatalf("missing workspace migration: %+v", got.SidebarViewsByWorkspace)
	}
	got.SidebarViewsByWorkspace["a"].Views[0].Name = "Changed"
	if got.SidebarViewsByWorkspace["b"].Views[0].Name != "Original" {
		t.Fatal("migrated views alias each other")
	}
	*ids = append(*ids, "c")
	got, err = svc.GetUserSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.SidebarViewsByWorkspace["c"].ActiveViewID != store.DefaultSidebarViewID {
		t.Fatal("later workspace did not use defaults")
	}
	if repo.snapshot().SidebarViewsByWorkspace["a"].Views[0].Name != "Original" {
		t.Fatal("read mutated persisted state")
	}
}

func TestSidebarWorkspacePatchIsolation(t *testing.T) {
	svc, repo, _ := sidebarService(t)
	if _, err := svc.GetUserSettings(context.Background()); err != nil {
		t.Fatal(err)
	}
	req := sidebarPatch(t, `{"SidebarViewState":{"workspace_id":"a","views":[{"id":"new","name":"Only A"}],"active_view_id":"new","draft":null}}`)
	got, err := svc.UpdateUserSettings(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if got.SidebarViewsByWorkspace["a"].ActiveViewID != "new" {
		t.Fatal("scoped patch ignored")
	}
	if got.SidebarViewsByWorkspace["b"].Views[0].Name != "Original" {
		t.Fatal("other workspace changed")
	}
	before := repo.snapshot().Revision
	_, err = svc.UpdateUserSettings(context.Background(), sidebarPatch(t, `{"SidebarViewState":{"workspace_id":"inaccessible","views":[]}}`))
	if !errors.Is(err, ErrValidation) || repo.snapshot().Revision != before {
		t.Fatalf("invalid scope persisted: %v", err)
	}
	_, err = svc.UpdateUserSettings(context.Background(), &UpdateUserSettingsRequest{SidebarViews: ptr(store.DefaultSidebarViews())})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("legacy mutation accepted: %v", err)
	}
}

func TestSidebarWorkspaceLegacyPatchUsesExplicitWorkspaceID(t *testing.T) {
	svc, repo, _ := sidebarService(t)
	if _, err := svc.GetUserSettings(context.Background()); err != nil {
		t.Fatal(err)
	}
	workspaceID := "a"
	views := []models.SidebarView{{ID: "scoped", Name: "Scoped"}}
	got, err := svc.UpdateUserSettings(context.Background(), &UpdateUserSettingsRequest{
		WorkspaceID:  &workspaceID,
		SidebarViews: &views,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.SidebarViewsByWorkspace[workspaceID].Views[0].Name != "Scoped" {
		t.Fatalf("workspace-scoped legacy patch was ignored: %+v", got.SidebarViewsByWorkspace)
	}
	if got.SidebarViewsByWorkspace["b"].Views[0].Name != "Original" {
		t.Fatalf("legacy patch changed another workspace: %+v", got.SidebarViewsByWorkspace)
	}
	if repo.snapshot().SidebarViews[0].Name != "Original" {
		t.Fatalf("legacy global state changed: %+v", repo.snapshot().SidebarViews)
	}
}

func TestUpdateUserSettingsReturnsCommittedSettingsWhenSidebarProjectionFails(t *testing.T) {
	repo := newCASFakeRepo(&models.UserSettings{
		UserID:                  store.DefaultUserID,
		SidebarWorkspaceVersion: 1,
		SidebarViewsByWorkspace: map[string]models.SidebarWorkspaceState{
			"a": {
				Views:        store.DefaultSidebarViews(),
				ActiveViewID: store.DefaultSidebarViewID,
			},
		},
	})
	eventBus := &casEventBus{}
	svc := newCASService(repo, eventBus)
	accessCalls := 0
	svc.SetSidebarWorkspaceAccess(func(context.Context) ([]string, error) {
		accessCalls++
		if accessCalls == 1 {
			return []string{"a"}, nil
		}
		return nil, errors.New("workspace read failed")
	})

	got, err := svc.UpdateUserSettings(context.Background(), &UpdateUserSettingsRequest{DefaultEditorID: ptr("editor")})
	if err != nil {
		t.Fatalf("UpdateUserSettings: %v", err)
	}
	if got == nil || got.DefaultEditorID != "editor" {
		t.Fatalf("updated settings = %+v, want committed editor", got)
	}
	if got.SidebarViewsByWorkspace != nil {
		t.Fatalf("response exposed an unprojected sidebar state: %#v", got.SidebarViewsByWorkspace)
	}
	if eventBus.count() != 1 {
		t.Fatalf("events = %d, want one event despite projection failure", eventBus.count())
	}
	eventData, ok := eventBus.events[0].Data.(map[string]interface{})
	if !ok {
		t.Fatalf("event data = %T, want map[string]interface{}", eventBus.events[0].Data)
	}
	if _, exists := eventData["sidebar_views_by_workspace"]; exists {
		t.Fatalf("event exposed an unprojected sidebar state: %#v", eventData["sidebar_views_by_workspace"])
	}
}

func TestSidebarWorkspaceMigrationFailureDoesNotMarkComplete(t *testing.T) {
	svc, repo, _ := sidebarService(t)
	svc.SetSidebarWorkspaceAccess(func(context.Context) ([]string, error) { return nil, errors.New("workspace read failed") })
	if _, err := svc.GetUserSettings(context.Background()); err == nil {
		t.Fatal("missing workspace error")
	}
	if repo.snapshot().SidebarWorkspaceVersion != 0 {
		t.Fatal("failed migration marked complete")
	}
	svc.SetSidebarWorkspaceAccess(func(context.Context) ([]string, error) { return []string{"a"}, nil })
	repo.forceConflictCount = 3
	if _, err := svc.GetUserSettings(context.Background()); !errors.Is(err, ErrUserSettingsConflict) {
		t.Fatalf("missing CAS error: %v", err)
	}
	if repo.snapshot().SidebarWorkspaceVersion != 0 {
		t.Fatal("failed CAS marked complete")
	}
}

func TestSidebarWorkspacePatchDraftAndReferences(t *testing.T) {
	svc, _, _ := sidebarService(t)
	for _, raw := range []string{
		`{"SidebarViewState":{"workspace_id":"a","draft":{"base_view_id":"legacy","filters":[],"sort":{"key":"title","direction":"asc"},"group":"none"}}}`,
		`{"SidebarViewState":{"workspace_id":"a","views":[{"id":"legacy","name":"Renamed"}]}}`,
	} {
		got, err := svc.UpdateUserSettings(context.Background(), sidebarPatch(t, raw))
		if err != nil {
			t.Fatal(err)
		}
		if got.SidebarViewsByWorkspace["a"].Draft == nil {
			t.Fatal("omitted draft was cleared")
		}
	}
	got, err := svc.UpdateUserSettings(context.Background(), sidebarPatch(t, `{"SidebarViewState":{"workspace_id":"a","views":[],"draft":null}}`))
	if err != nil {
		t.Fatal(err)
	}
	entry := got.SidebarViewsByWorkspace["a"]
	if entry.ActiveViewID != store.DefaultSidebarViewID || entry.Draft != nil {
		t.Fatalf("invalid defaults: %+v", entry)
	}
	_, err = svc.UpdateUserSettings(context.Background(), sidebarPatch(t, `{"SidebarViewState":{"workspace_id":"a","draft":{"base_view_id":"foreign"}}}`))
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("foreign draft accepted: %v", err)
	}
}

func TestSidebarWorkspaceConcurrentWritesPreserveBothEntries(t *testing.T) {
	svc, repo, _ := sidebarService(t)
	if _, err := svc.GetUserSettings(context.Background()); err != nil {
		t.Fatal(err)
	}
	repo.blockNextUpsert = true
	repo.releaseUpsert = make(chan struct{})
	repo.upsertStarted = make(chan struct{}, 4)
	done := make(chan error, 1)
	go func() {
		_, err := svc.UpdateUserSettings(context.Background(), sidebarPatch(t, `{"SidebarViewState":{"workspace_id":"a","views":[{"id":"legacy","name":"A edited"}]}}`))
		done <- err
	}()
	<-repo.upsertStarted
	_, err := svc.UpdateUserSettings(context.Background(), sidebarPatch(t, `{"SidebarViewState":{"workspace_id":"b","views":[{"id":"legacy","name":"B edited"}]}}`))
	close(repo.releaseUpsert)
	if err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	entries := repo.snapshot().SidebarViewsByWorkspace
	if entries["a"].Views[0].Name != "A edited" || entries["b"].Views[0].Name != "B edited" {
		t.Fatalf("concurrent update lost: %+v", entries)
	}
}
