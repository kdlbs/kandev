package github

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
)

func TestWorkspaceScopeUnionRoundTrip(t *testing.T) {
	store := newTestStore(t)
	settings := &WorkspaceSettings{
		WorkspaceID:   "ws-scope-union",
		RepoScopeMode: "selected",
		RepoScopeOrgs: []string{" Acme "},
		RepoScopeRepos: []RepoFilter{
			{Owner: "Outside", Name: "Tool"},
		},
	}
	if err := store.UpsertWorkspaceSettings(context.Background(), settings); err != nil {
		t.Fatalf("UpsertWorkspaceSettings() error = %v", err)
	}

	got, err := store.GetWorkspaceSettings(context.Background(), settings.WorkspaceID)
	if err != nil {
		t.Fatalf("GetWorkspaceSettings() error = %v", err)
	}
	if got.RepoScopeMode != "selected" || len(got.RepoScopeOrgs) != 1 || got.RepoScopeOrgs[0] != "Acme" || len(got.RepoScopeRepos) != 1 || got.RepoScopeRepos[0] != (RepoFilter{Owner: "Outside", Name: "Tool"}) {
		t.Fatalf("round-tripped scope = %+v, want both selected grants", got)
	}
	for _, repository := range []struct {
		owner string
		repo  string
		want  bool
	}{
		{owner: "acme", repo: "service", want: true},
		{owner: "outside", repo: "tool", want: true},
		{owner: "outside", repo: "other", want: false},
	} {
		if allowed := repositoryInWorkspaceScope(got, repository.owner, repository.repo); allowed != repository.want {
			t.Errorf("repositoryInWorkspaceScope(%q, %q) = %v, want %v", repository.owner, repository.repo, allowed, repository.want)
		}
	}
}

func TestWorkspaceScopeUnionPatchCompatibility(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	orgs := []string{"Acme"}
	repos := []RepoFilter{{Owner: "outside", Name: "tool"}}
	updated, err := store.PatchWorkspaceSettings(ctx, &UpdateWorkspaceSettingsRequest{
		WorkspaceID: "ws-scope-patch", RepoScopeMode: stringPtr("selected"),
		RepoScopeOrgs: &orgs, RepoScopeRepos: &repos,
	})
	if err != nil {
		t.Fatalf("mixed selected patch: %v", err)
	}
	if updated.RepoScopeMode != "selected" || len(updated.RepoScopeOrgs) != 1 || len(updated.RepoScopeRepos) != 1 {
		t.Fatalf("mixed selected settings = %+v", updated)
	}

	updated, err = store.PatchWorkspaceSettings(ctx, &UpdateWorkspaceSettingsRequest{
		WorkspaceID: "ws-scope-patch", RepoScopeMode: stringPtr("selected"),
	})
	if err != nil {
		t.Fatalf("selected mode patch: %v", err)
	}
	if updated.RepoScopeMode != "selected" || len(updated.RepoScopeOrgs) != 1 || updated.RepoScopeOrgs[0] != "Acme" || len(updated.RepoScopeRepos) != 1 {
		t.Fatalf("omitted selected lists were not preserved: %+v", updated)
	}

	emptyOrgs := []string{}
	updated, err = store.PatchWorkspaceSettings(ctx, &UpdateWorkspaceSettingsRequest{
		WorkspaceID: "ws-scope-patch", RepoScopeOrgs: &emptyOrgs,
	})
	if err != nil {
		t.Fatalf("empty list patch: %v", err)
	}
	if len(updated.RepoScopeOrgs) != 0 || len(updated.RepoScopeRepos) != 1 {
		t.Fatalf("explicit empty list did not clear owners while preserving repositories: %+v", updated)
	}

	var nullRequest UpdateWorkspaceSettingsRequest
	if err := json.Unmarshal([]byte(`{"workspace_id":"ws-scope-patch","repo_scope_repos":null}`), &nullRequest); err != nil {
		t.Fatalf("decode null list request: %v", err)
	}
	if nullRequest.RepoScopeRepos != nil {
		t.Fatalf("null repository list = %#v, want omitted", nullRequest.RepoScopeRepos)
	}

	updated, err = store.PatchWorkspaceSettings(ctx, &UpdateWorkspaceSettingsRequest{
		WorkspaceID: "ws-scope-patch", RepoScopeMode: stringPtr("orgs"), RepoScopeOrgs: &orgs,
	})
	if err != nil {
		t.Fatalf("legacy organizations patch: %v", err)
	}
	if updated.RepoScopeMode != "selected" || len(updated.RepoScopeOrgs) != 1 || updated.RepoScopeOrgs[0] != "Acme" || len(updated.RepoScopeRepos) != 0 {
		t.Fatalf("legacy organizations canonicalized to %+v", updated)
	}

	invalid := "unknown"
	if _, err := store.PatchWorkspaceSettings(ctx, &UpdateWorkspaceSettingsRequest{WorkspaceID: "ws-scope-patch", RepoScopeMode: &invalid}); err == nil {
		t.Fatal("unknown mode patch was accepted")
	}
	stored, err := store.GetWorkspaceSettings(ctx, "ws-scope-patch")
	if err != nil {
		t.Fatalf("read after invalid patch: %v", err)
	}
	if stored.RepoScopeMode != "selected" || len(stored.RepoScopeOrgs) != 1 || len(stored.RepoScopeRepos) != 0 {
		t.Fatalf("invalid patch changed existing policy: %+v", stored)
	}

	all := "all"
	_, err = store.PatchWorkspaceSettings(ctx, &UpdateWorkspaceSettingsRequest{
		WorkspaceID: "ws-scope-patch", RepoScopeMode: &all, RepoScopeOrgs: &orgs,
	})
	if err == nil {
		t.Fatal("all-mode patch with a nonempty list was accepted")
	}
	_, err = store.PatchWorkspaceSettings(ctx, &UpdateWorkspaceSettingsRequest{
		WorkspaceID: "ws-all-scope-patch", RepoScopeMode: &all,
	})
	if err != nil {
		t.Fatalf("set all mode: %v", err)
	}
	_, err = store.PatchWorkspaceSettings(ctx, &UpdateWorkspaceSettingsRequest{
		WorkspaceID: "ws-all-scope-patch", RepoScopeOrgs: &orgs,
	})
	if err == nil {
		t.Fatal("nonempty list patch under all mode was accepted")
	}
	stored, err = store.GetWorkspaceSettings(ctx, "ws-all-scope-patch")
	if err != nil {
		t.Fatalf("read all-mode settings: %v", err)
	}
	if stored.RepoScopeMode != "all" || len(stored.RepoScopeOrgs) != 0 {
		t.Fatalf("failed list patch changed all-mode policy: %+v", stored)
	}
}

func TestWorkspaceScopeUnionMigration(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	created := time.Now().UTC()
	seed := []struct {
		workspaceID string
		mode        string
		orgs        string
		repos       string
	}{
		{"legacy-orgs", "ORGS", `["Acme"]`, `[{"owner":"stale","name":"repo"}]`},
		{"legacy-repos", "repos", `["stale"]`, `[{"owner":"Outside","name":"Tool"}]`},
		{"all", "ALL", `["stale"]`, `[{"owner":"stale","name":"repo"}]`},
		{"empty-mode", "", `["stale"]`, `[{"owner":"stale","name":"repo"}]`},
		{"selected", "SELECTED", `["Acme"]`, `[{"owner":"Outside","name":"Tool"}]`},
		{"unsupported", "future-policy", `["Acme"]`, `[{"owner":"Outside","name":"Tool"}]`},
	}
	for _, row := range seed {
		if _, err := store.db.ExecContext(ctx, `INSERT INTO github_workspace_settings (workspace_id, repo_scope_mode, repo_scope_orgs, repo_scope_repos, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, row.workspaceID, row.mode, row.orgs, row.repos, created, created); err != nil {
			t.Fatalf("seed %s: %v", row.workspaceID, err)
		}
	}
	if _, err := NewStore(store.db, store.ro); err != nil {
		t.Fatalf("first store reopen with scope migration: %v", err)
	}
	if _, err := NewStore(store.db, store.ro); err != nil {
		t.Fatalf("second store reopen with scope migration: %v", err)
	}
	want := map[string]struct {
		mode  string
		orgs  string
		repos string
	}{
		"legacy-orgs":  {"selected", `["Acme"]`, `[]`},
		"legacy-repos": {"selected", `[]`, `[{"owner":"Outside","name":"Tool"}]`},
		"all":          {"all", `[]`, `[]`},
		"empty-mode":   {"all", `[]`, `[]`},
		"selected":     {"selected", `["Acme"]`, `[{"owner":"Outside","name":"Tool"}]`},
		"unsupported":  {"future-policy", `["Acme"]`, `[{"owner":"Outside","name":"Tool"}]`},
	}
	for id, expected := range want {
		var got struct {
			Mode  string `db:"repo_scope_mode"`
			Orgs  string `db:"repo_scope_orgs"`
			Repos string `db:"repo_scope_repos"`
		}
		if err := store.ro.GetContext(ctx, &got, `SELECT repo_scope_mode, repo_scope_orgs, repo_scope_repos FROM github_workspace_settings WHERE workspace_id = ?`, id); err != nil {
			t.Fatalf("read %s after migration: %v", id, err)
		}
		if got.Mode != expected.mode || got.Orgs != expected.orgs || got.Repos != expected.repos {
			t.Errorf("migrated %s = %+v, want %+v", id, got, expected)
		}
	}
	unsupported, err := store.GetWorkspaceSettings(ctx, "unsupported")
	if err != nil {
		t.Fatalf("read unsupported stored mode: %v", err)
	}
	if repositoryInWorkspaceScope(unsupported, "Acme", "service") {
		t.Fatal("unsupported stored mode widened access")
	}

	if _, err := store.db.ExecContext(ctx, `INSERT INTO github_workspace_settings (workspace_id, repo_scope_mode, repo_scope_orgs, repo_scope_repos, created_at, updated_at) VALUES ('failure', 'orgs', '["kept"]', '[{"owner":"stale","name":"repo"}]', ?, ?)`, created, created); err != nil {
		t.Fatalf("seed transaction failure row: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `CREATE TRIGGER reject_repo_scope_migration BEFORE UPDATE ON github_workspace_settings WHEN OLD.workspace_id = 'failure' BEGIN SELECT RAISE(ABORT, 'migration blocked'); END`); err != nil {
		t.Fatalf("create migration failure trigger: %v", err)
	}
	if err := store.migrateWorkspaceRepoScopes(); err == nil {
		t.Fatal("migration failure trigger was ignored")
	}
	var mode, orgsJSON, reposJSON string
	if err := store.ro.QueryRowContext(ctx, `SELECT repo_scope_mode, repo_scope_orgs, repo_scope_repos FROM github_workspace_settings WHERE workspace_id = 'failure'`).Scan(&mode, &orgsJSON, &reposJSON); err != nil {
		t.Fatalf("read transaction failure row: %v", err)
	}
	if mode != "orgs" || orgsJSON != `["kept"]` || reposJSON != `[{"owner":"stale","name":"repo"}]` {
		t.Fatalf("failed migration changed row: mode=%s orgs=%s repos=%s", mode, orgsJSON, reposJSON)
	}
}

func TestPostgresWorkspaceScopeUnion(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := database.Exec(`CREATE TABLE workspaces (id TEXT PRIMARY KEY); CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, archived_at TIMESTAMPTZ)`); err != nil {
		t.Fatalf("create prerequisite tables: %v", err)
	}
	store, err := NewStore(database, database)
	if err != nil {
		t.Fatalf("initialize GitHub store: %v", err)
	}
	now := time.Now().UTC()
	if _, err := database.Exec(`INSERT INTO github_workspace_settings (workspace_id, repo_scope_mode, repo_scope_orgs, repo_scope_repos, created_at, updated_at) VALUES ('scope-postgres', 'orgs', '["Acme"]', '[{"owner":"stale","name":"repo"}]', $1, $1) ON CONFLICT (workspace_id) DO UPDATE SET repo_scope_mode = EXCLUDED.repo_scope_mode, repo_scope_orgs = EXCLUDED.repo_scope_orgs, repo_scope_repos = EXCLUDED.repo_scope_repos`, now); err != nil {
		t.Fatalf("seed legacy settings: %v", err)
	}
	if err := store.migrateWorkspaceRepoScopes(); err != nil {
		t.Fatalf("migrate PostgreSQL scope: %v", err)
	}
	migrated, err := store.GetWorkspaceSettings(t.Context(), "scope-postgres")
	if err != nil {
		t.Fatalf("read migrated PostgreSQL scope: %v", err)
	}
	if migrated.RepoScopeMode != "selected" ||
		len(migrated.RepoScopeOrgs) != 1 ||
		migrated.RepoScopeOrgs[0] != "Acme" ||
		len(migrated.RepoScopeRepos) != 0 {
		t.Fatalf("migrated PostgreSQL scope = %+v, want selected with Acme and no repositories", migrated)
	}
	mode := "selected"
	orgs := []string{"Acme"}
	repos := []RepoFilter{{Owner: "Outside", Name: "Tool"}}
	updated, err := store.PatchWorkspaceSettings(t.Context(), &UpdateWorkspaceSettingsRequest{
		WorkspaceID: "scope-postgres", RepoScopeMode: &mode, RepoScopeOrgs: &orgs, RepoScopeRepos: &repos,
	})
	if err != nil {
		t.Fatalf("patch PostgreSQL union scope: %v", err)
	}
	if updated.RepoScopeMode != "selected" || len(updated.RepoScopeOrgs) != 1 || len(updated.RepoScopeRepos) != 1 {
		t.Fatalf("PostgreSQL union scope = %+v", updated)
	}
}

func stringPtr(value string) *string { return &value }
