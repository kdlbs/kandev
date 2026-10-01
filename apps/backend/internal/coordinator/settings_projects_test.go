package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func projectsFixture(t *testing.T) (*Store, *Coordinator, *Service, *fakeProjects) {
	t.Helper()
	store, c, _, svc := phase2ApproveFixture(t)
	svc.phase31 = true
	p := newFakeProjects()
	p.addRepo("repo-a", "Alpha")
	p.addRepo("repo-b", "beta")
	p.addRepo("repo-d", "Delta")
	p.addSet("set-1", "Payments", "repo-a", "repo-b")
	svc.SetProjectReader(p)
	return store, c, svc, p
}

func projectsBody(scope string, noRepo bool, entries ...ProjectEntry) string {
	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		parts = append(parts, fmt.Sprintf(`{"kind":%q,"id":%q}`, e.Kind, e.ID))
	}
	return fmt.Sprintf(`{"projects":{"scope":%q,"include_no_repository":%t,"entries":[%s]}}`, scope, noRepo, strings.Join(parts, ","))
}

func revisionOf(t *testing.T, store *Store, id string) int {
	t.Helper()
	c, err := store.GetCoordinatorByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return c.PolicyRevision
}

func TestSaveSettings_ProjectsValidationCodes(t *testing.T) {
	store, c, svc, _ := projectsFixture(t)
	many := make([]ProjectEntry, 0, 51)
	for i := 0; i < 51; i++ {
		many = append(many, repoEntry(fmt.Sprintf("r%02d", i)))
	}
	cases := []struct{ name, body, code string }{
		{"not an object", `{"projects":[]}`, codeInvalidProjects},
		{"missing scope", `{"projects":{}}`, codeInvalidProjects},
		{"bad scope", `{"projects":{"scope":"some"}}`, codeInvalidProjects},
		{"bad kind", `{"projects":{"scope":"selected","entries":[{"kind":"x","id":"a"}]}}`, codeInvalidProjects},
		{"blank id", `{"projects":{"scope":"selected","entries":[{"kind":"repository","id":" "}]}}`, codeInvalidProjects},
		{"bad toggle", `{"projects":{"scope":"selected","include_no_repository":"yes"}}`, codeInvalidProjects},
		{"empty", projectsBody("selected", false), codeProjectsEmpty},
		{"too many", projectsBody("selected", false, many...), codeProjectsTooMany},
		{"duplicate", projectsBody("selected", false, repoEntry("repo-a"), repoEntry("repo-a")), codeProjectsDuplicate},
		{"foreign", projectsBody("selected", false, repoEntry("repo-x")), codeProjectsForeignEntry},
		{"foreign set", projectsBody("selected", false, setEntry("set-x")), codeProjectsForeignEntry},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.SaveSettings(context.Background(), c.WorkspaceID, c.ID, []byte(tc.body))
			if got := settingsCode(t, err); got != tc.code {
				t.Fatalf("code = %q, want %q", got, tc.code)
			}
			if rev := revisionOf(t, store, c.ID); rev != 0 {
				t.Fatalf("a refused save raised the revision to %d", rev)
			}
		})
	}
}

func TestSaveSettings_ProjectsChangeArchivesAndRaisesRevisionOnce(t *testing.T) {
	store, c, svc, _ := projectsFixture(t)
	got := mustSave(t, svc, c.WorkspaceID, c.ID, projectsBody("selected", true, setEntry("set-1"), repoEntry("repo-d")))
	if got.PolicyRevision != 1 {
		t.Fatalf("revision = %d, want 1", got.PolicyRevision)
	}
	if got.Projects == nil || got.Projects.Scope != "selected" || !got.Projects.IncludeNoRepository ||
		strings.Join(got.Projects.RepositoryIDs, ",") != "repo-a,repo-b,repo-d" {
		t.Fatalf("projects = %+v", got.Projects)
	}
	if got.ProjectsConfig == nil || len(got.ProjectsConfig.Entries) != 2 || got.ProjectsConfig.Entries[0].Kind != projectKindSet {
		t.Fatalf("projects_config = %+v", got.ProjectsConfig)
	}
	set, err := store.LoadWatchSet(context.Background(), store.ro, c.ID)
	if err != nil || !set.Projects.Selected || !set.Projects.IncludeNoRepo {
		t.Fatalf("stored = %+v, %v", set.Projects, err)
	}

	again := mustSave(t, svc, c.WorkspaceID, c.ID, projectsBody("selected", true, repoEntry("repo-d"), setEntry("set-1")))
	if again.PolicyRevision != 1 {
		t.Fatalf("an equal body raised the revision to %d", again.PolicyRevision)
	}
}

func TestSaveSettings_ProjectsSwitchToAllKeepsEntriesAndToggleIsNotAChange(t *testing.T) {
	store, c, svc, _ := projectsFixture(t)
	mustSave(t, svc, c.WorkspaceID, c.ID, projectsBody("selected", true, repoEntry("repo-a")))
	got := mustSave(t, svc, c.WorkspaceID, c.ID, `{"projects":{"scope":"all","include_no_repository":false,"entries":[{"kind":"repository","id":"repo-b"}]}}`)
	if got.PolicyRevision != 2 || got.Projects != nil {
		t.Fatalf("got revision %d projects %+v, want 2 and no enforcement member", got.PolicyRevision, got.Projects)
	}
	state, err := store.loadProjectState(context.Background(), store.ro, c.ID)
	if err != nil || state.scope != "all" || !state.includeNoRepo || len(state.entries) != 1 || state.entries[0].ID != "repo-a" {
		t.Fatalf("state = %+v, %v; want the previous entries and toggle kept", state, err)
	}
	same := mustSave(t, svc, c.WorkspaceID, c.ID, `{"projects":{"scope":"all","include_no_repository":true}}`)
	if same.PolicyRevision != 2 {
		t.Fatalf("a toggle-only change under all raised the revision to %d", same.PolicyRevision)
	}
}

func TestSaveSettings_StaleStoredEntryResentIsDropped(t *testing.T) {
	store, c, svc, p := projectsFixture(t)
	mustSave(t, svc, c.WorkspaceID, c.ID, projectsBody("selected", false, repoEntry("repo-a"), repoEntry("repo-d")))
	p.repos = p.repos[:2]
	got := mustSave(t, svc, c.WorkspaceID, c.ID, projectsBody("selected", false, repoEntry("repo-a"), repoEntry("repo-b"), repoEntry("repo-d")))
	if got.PolicyRevision != 2 {
		t.Fatalf("revision = %d, want 2", got.PolicyRevision)
	}
	state, _ := store.loadProjectState(context.Background(), store.ro, c.ID)
	if len(state.entries) != 2 || state.entries[0].ID != "repo-a" || state.entries[1].ID != "repo-b" {
		t.Fatalf("entries = %+v, want the stale repo-d dropped", state.entries)
	}
	p.repos = nil
	_, err := svc.SaveSettings(context.Background(), c.WorkspaceID, c.ID, []byte(projectsBody("selected", false, repoEntry("repo-a"))))
	if code := settingsCode(t, err); code != codeProjectsEmpty {
		t.Fatalf("a body whose only entry is stored but gone: code %q, want %q", code, codeProjectsEmpty)
	}
	_, err = svc.SaveSettings(context.Background(), c.WorkspaceID, c.ID, []byte(projectsBody("selected", false, repoEntry("repo-d"))))
	if code := settingsCode(t, err); code != codeProjectsForeignEntry {
		t.Fatalf("an entry neither live nor stored: code %q, want %q", code, codeProjectsForeignEntry)
	}
}

func TestSaveSettings_ProjectsMemberIsIgnoredWithPhase31Off(t *testing.T) {
	store, c, svc, _ := projectsFixture(t)
	svc.phase31 = false
	got := mustSave(t, svc, c.WorkspaceID, c.ID, projectsBody("selected", true, repoEntry("repo-a")))
	if got.PolicyRevision != 0 || got.ProjectsConfig != nil {
		t.Fatalf("got %+v, want nothing written and no editor member", got)
	}
	state, _ := store.loadProjectState(context.Background(), store.ro, c.ID)
	if state.scope != "all" {
		t.Fatalf("scope = %q, want all", state.scope)
	}
}

func TestSaveSettings_EveryProjectReadsNothing(t *testing.T) {
	_, c, svc, p := projectsFixture(t)
	p.failSets, p.failRepos = errBoom, errBoom
	mustSave(t, svc, c.WorkspaceID, c.ID, `{"projects":{"scope":"all","entries":[{"kind":"bogus","id":""}]}}`)
	if n := p.readCount(); n != 0 {
		t.Fatalf("reads = %d, want 0", n)
	}
}

func TestGetSettings_ProjectsReadShapes(t *testing.T) {
	_, c, svc, p := projectsFixture(t)
	ctx := context.Background()
	raw := func(v any) string { b, _ := json.Marshal(v); return string(b) }

	got, err := svc.GetSettings(ctx, c.WorkspaceID, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Projects != nil || raw(got.ProjectsConfig) != `{"scope":"all","entries":[],"include_no_repository":false}` {
		t.Fatalf("all: projects %+v config %s", got.Projects, raw(got.ProjectsConfig))
	}

	mustSave(t, svc, c.WorkspaceID, c.ID, projectsBody("selected", false, setEntry("set-1")))
	p.failSets = errBoom
	got, _ = svc.GetSettings(ctx, c.WorkspaceID, c.ID)
	if raw(got.Projects) != `{"scope":"selected","repository_ids":null,"include_no_repository":false}` {
		t.Fatalf("a failed listing must read as null ids: %s", raw(got.Projects))
	}

	svc.phase31 = false
	got, _ = svc.GetSettings(ctx, c.WorkspaceID, c.ID)
	if got.ProjectsConfig != nil || got.Projects == nil {
		t.Fatalf("flag off: config %+v projects %+v; the enforcement member stays", got.ProjectsConfig, got.Projects)
	}
}

func TestWatchProjectsView_NamesAreOrderedAndOmittedOnAFailedListing(t *testing.T) {
	store, c, svc, p := projectsFixture(t)
	projectScope(t, store, c.ID, true, setEntry("set-1"), repoEntry("repo-d"), repoEntry("repo-b"), repoEntry("repo-a"))
	found, _ := store.GetCoordinatorByID(context.Background(), c.ID)
	view, err := svc.WatchProjectsView(context.Background(), found)
	if err != nil {
		t.Fatal(err)
	}
	dto := view.(*WatchProjectsDTO)
	if dto.Names == nil || strings.Join(*dto.Names, ",") != "Payments,Alpha,beta,Delta" {
		t.Fatalf("names = %v", dto.Names)
	}
	p.failRepos = errBoom
	view, _ = svc.WatchProjectsView(context.Background(), found)
	if dto = view.(*WatchProjectsDTO); dto.Names != nil || dto.RepositoryIDs != nil {
		t.Fatalf("a failed listing must omit names and null the ids: %+v", dto)
	}
}

func TestWatchProjectsView_AllIsPresentOnlyWithPhase31(t *testing.T) {
	store, c, svc, _ := projectsFixture(t)
	found, _ := store.GetCoordinatorByID(context.Background(), c.ID)
	view, _ := svc.WatchProjectsView(context.Background(), found)
	if b, _ := json.Marshal(view); string(b) != `{"scope":"all"}` {
		t.Fatalf("view = %s", b)
	}
	svc.phase31 = false
	if view, _ = svc.WatchProjectsView(context.Background(), found); view != nil {
		t.Fatalf("view = %v, want none with the flag off", view)
	}
}

func TestProjectDeleted_RemovesEntriesPublishesOnceAndKeepsRevision(t *testing.T) {
	store, c, svc, _ := projectsFixture(t)
	mustSave(t, svc, c.WorkspaceID, c.ID, projectsBody("selected", false, setEntry("set-1"), repoEntry("repo-a")))
	rev := revisionOf(t, store, c.ID)
	ctx := context.Background()
	if err := svc.ProjectDeleted(ctx, projectKindSet, "set-1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.ProjectDeleted(ctx, projectKindSet, "set-1"); err != nil {
		t.Fatal(err)
	}
	state, _ := store.loadProjectState(ctx, store.ro, c.ID)
	if len(state.entries) != 1 || state.entries[0].ID != "repo-a" || state.scope != "selected" {
		t.Fatalf("state = %+v", state)
	}
	if got := revisionOf(t, store, c.ID); got != rev {
		t.Fatalf("revision = %d, want %d: the cascade is not a manager save", got, rev)
	}
	if err := svc.ProjectDeleted(ctx, projectKindRepository, "repo-a"); err != nil {
		t.Fatal(err)
	}
	state, _ = store.loadProjectState(ctx, store.ro, c.ID)
	if len(state.entries) != 0 || state.scope != "selected" {
		t.Fatalf("an emptied list must stay selected and watch nothing: %+v", state)
	}
}

func TestSetup_ProjectsMemberIsValidatedAndStoredAtomically(t *testing.T) {
	svc, store := setupService(t)
	svc.phase31 = true
	p := newFakeProjects()
	p.addRepo("repo-a", "Alpha")
	svc.SetProjectReader(p)
	ctx := context.Background()
	wantStep := func(projects, code string) {
		t.Helper()
		_, err := svc.CreateSetup(ctx, testWorkspaceID, []byte(setupBody(map[string]string{"projects": projects})))
		se, ok := err.(*SettingsError)
		if !ok || se.Step != setupStepProjects || se.Code != code {
			t.Fatalf("%s: err = %#v, want %s on the projects step", projects, err, code)
		}
		if n := setupCountRows(t, store, "coordinators"); n != 0 {
			t.Fatalf("a refused setup left %d coordinators", n)
		}
	}
	wantStep(`{"scope":"selected"}`, codeProjectsEmpty)
	wantStep(`{"scope":"selected","entries":[{"kind":"repository","id":"nope"}]}`, codeProjectsForeignEntry)
	wantStep(`{"scope":"bogus"}`, codeInvalidProjects)

	created, err := svc.CreateSetup(ctx, testWorkspaceID, []byte(setupBody(map[string]string{
		"projects": `{"scope":"selected","include_no_repository":true,"entries":[{"kind":"repository","id":"repo-a"}]}`})))
	if err != nil {
		t.Fatal(err)
	}
	state, _ := store.loadProjectState(ctx, store.ro, created.ID)
	if state.scope != "selected" || !state.includeNoRepo || len(state.entries) != 1 || created.PolicyRevision != 1 {
		t.Fatalf("state = %+v revision %d", state, created.PolicyRevision)
	}

	plain, err := svc.CreateSetup(ctx, testWorkspaceID, []byte(setupBody(nil)))
	if err != nil {
		t.Fatal(err)
	}
	if st, _ := store.loadProjectState(ctx, store.ro, plain.ID); st.scope != "all" {
		t.Fatalf("a setup without projects must keep every project: %+v", st)
	}
}

func TestSetup_ProjectsMemberIsIgnoredWithPhase31Off(t *testing.T) {
	svc, store := setupService(t)
	created, err := svc.CreateSetup(context.Background(), testWorkspaceID, []byte(setupBody(map[string]string{"projects": `{"scope":"selected"}`})))
	if err != nil {
		t.Fatal(err)
	}
	if st, _ := store.loadProjectState(context.Background(), store.ro, created.ID); st.scope != "all" {
		t.Fatalf("scope = %q, want all", st.scope)
	}
}
