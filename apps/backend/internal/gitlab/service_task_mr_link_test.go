package gitlab

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func seedTaskMRLinkFixture(t *testing.T, store *Store, workspaceID, taskID, repositoryID string) {
	t.Helper()
	seedWorkspace(t, store, workspaceID)
	seedTask(t, store, taskID, workspaceID)
	if _, err := store.db.Exec(`CREATE TABLE IF NOT EXISTS repositories (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		provider TEXT DEFAULT '',
		provider_repo_id TEXT DEFAULT '',
		provider_host TEXT DEFAULT '',
		provider_owner TEXT DEFAULT '',
		provider_name TEXT DEFAULT '',
		remote_url TEXT DEFAULT '',
		local_path TEXT DEFAULT '',
		updated_at TIMESTAMP
	); CREATE TABLE IF NOT EXISTS task_repositories (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		repository_id TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create repository fixtures: %v", err)
	}
	if repositoryID == "" {
		return
	}
	if _, err := store.db.Exec(`INSERT INTO repositories (id, workspace_id) VALUES (?, ?)`, repositoryID, workspaceID); err != nil {
		t.Fatalf("seed repository: %v", err)
	}
	if _, err := store.db.Exec(
		`INSERT INTO task_repositories (id, task_id, repository_id) VALUES (?, ?, ?)`,
		"task-repo-"+taskID+"-"+repositoryID, taskID, repositoryID,
	); err != nil {
		t.Fatalf("seed task repository: %v", err)
	}
}

func setTaskMRRepositoryIdentity(
	t *testing.T, store *Store, repositoryID, host, projectPath string,
) {
	t.Helper()
	projectPath = strings.Trim(projectPath, "/")
	lastSlash := strings.LastIndex(projectPath, "/")
	if lastSlash <= 0 || lastSlash == len(projectPath)-1 {
		t.Fatalf("project path %q has no namespace", projectPath)
	}
	owner, name := projectPath[:lastSlash], projectPath[lastSlash+1:]
	if _, err := store.db.Exec(`UPDATE repositories
		SET provider = 'gitlab', provider_host = ?, provider_owner = ?, provider_name = ?
		WHERE id = ?`, host, owner, name, repositoryID); err != nil {
		t.Fatalf("set repository identity: %v", err)
	}
}

func setTaskMRRepositoryRemoteURL(t *testing.T, store *Store, repositoryID, remoteURL string) {
	t.Helper()
	if _, err := store.db.Exec(
		`UPDATE repositories SET remote_url = ? WHERE id = ?`, remoteURL, repositoryID,
	); err != nil {
		t.Fatalf("set repository remote_url: %v", err)
	}
}

func setTaskMRRepositoryLocalPath(t *testing.T, store *Store, repositoryID, localPath string) {
	t.Helper()
	if _, err := store.db.Exec(
		`UPDATE repositories SET local_path = ? WHERE id = ?`, localPath, repositoryID,
	); err != nil {
		t.Fatalf("set repository local_path: %v", err)
	}
}

// setTaskMRRepositoryProviderRepoID sets only provider_repo_id, leaving every
// other identity column (provider/host/owner/name/remote_url) blank. Used to
// verify hasNoDurableIdentitySignal treats provider_repo_id as a durable
// identity signal on its own, since service-layer backfills can populate it
// independently of the other provider_* columns.
func setTaskMRRepositoryProviderRepoID(t *testing.T, store *Store, repositoryID, providerRepoID string) {
	t.Helper()
	if _, err := store.db.Exec(
		`UPDATE repositories SET provider_repo_id = ? WHERE id = ?`, providerRepoID, repositoryID,
	); err != nil {
		t.Fatalf("set repository provider_repo_id: %v", err)
	}
}

func getTaskMRRepositoryRemoteURL(t *testing.T, store *Store, repositoryID string) string {
	t.Helper()
	var remoteURL string
	if err := store.db.Get(&remoteURL, `SELECT COALESCE(remote_url, '') FROM repositories WHERE id = ?`, repositoryID); err != nil {
		t.Fatalf("get repository remote_url: %v", err)
	}
	return remoteURL
}

// seedLocalGitCheckout creates a minimal local git checkout at dir whose
// origin remote is remoteURL, mirroring the on-disk shape
// resolveLocalGitOriginURL reads (a ".git" directory containing a "config"
// file with a "[remote \"origin\"]" section).
func seedLocalGitCheckout(t *testing.T, dir, remoteURL string) {
	t.Helper()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir git dir: %v", err)
	}
	config := "[core]\n\trepositoryformatversion = 0\n[remote \"origin\"]\n\turl = " + remoteURL + "\n\tfetch = +refs/heads/*:refs/remotes/origin/*\n"
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(config), 0o644); err != nil {
		t.Fatalf("write git config: %v", err)
	}
}

// seedLocalGitWorktreeCheckout creates a linked worktree checkout at
// worktreeDir: a ".git" *file* (not directory) containing a "gitdir:"
// pointer into mainGitDir/worktrees/<name>, whose own "commondir" file
// points back at mainGitDir where the origin remote is actually configured
// (mirroring what `git worktree add` produces on disk).
func seedLocalGitWorktreeCheckout(t *testing.T, worktreeDir, mainGitDir, remoteURL string) {
	t.Helper()
	if err := os.MkdirAll(mainGitDir, 0o755); err != nil {
		t.Fatalf("mkdir main git dir: %v", err)
	}
	config := "[core]\n\trepositoryformatversion = 0\n[remote \"origin\"]\n\turl = " + remoteURL + "\n\tfetch = +refs/heads/*:refs/remotes/origin/*\n"
	if err := os.WriteFile(filepath.Join(mainGitDir, "config"), []byte(config), 0o644); err != nil {
		t.Fatalf("write main git config: %v", err)
	}

	worktreeGitDir := filepath.Join(mainGitDir, "worktrees", "wt")
	if err := os.MkdirAll(worktreeGitDir, 0o755); err != nil {
		t.Fatalf("mkdir worktree git dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktreeGitDir, "commondir"), []byte("../..\n"), 0o644); err != nil {
		t.Fatalf("write commondir: %v", err)
	}

	if err := os.MkdirAll(worktreeDir, 0o755); err != nil {
		t.Fatalf("mkdir worktree dir: %v", err)
	}
	gitFile := "gitdir: " + worktreeGitDir + "\n"
	if err := os.WriteFile(filepath.Join(worktreeDir, ".git"), []byte(gitFile), 0o644); err != nil {
		t.Fatalf("write .git pointer file: %v", err)
	}
}

// setTaskMRRepositoryIdentityColumnsNull sets every provider identity column
// (provider, provider_repo_id, provider_host, provider_owner, provider_name,
// remote_url) to SQL NULL, mirroring rows persisted before those columns
// existed or by code paths that never populated them. The production schema
// declares them nullable, so ValidateTaskMRRepositoryIdentity must tolerate
// NULL scans.
func setTaskMRRepositoryIdentityColumnsNull(t *testing.T, store *Store, repositoryID string) {
	t.Helper()
	if _, err := store.db.Exec(`UPDATE repositories
		SET provider = NULL, provider_repo_id = NULL, provider_host = NULL, provider_owner = NULL,
			provider_name = NULL, remote_url = NULL
		WHERE id = ?`, repositoryID); err != nil {
		t.Fatalf("null repository identity columns: %v", err)
	}
}

func newTaskMRLinkService(t *testing.T, host string) (*Service, *Store, *MockClient) {
	t.Helper()
	store := newTestStore(t)
	client := NewMockClient(host)
	svc := NewService(host, client, AuthMethodPAT, nil, newTestLogger(t))
	svc.SetStore(store)
	svc.workspaceClients["ws-1"] = client
	return svc, store, client
}

func TestAssociateExistingMRByURLCreatesIdempotentWorkspaceScopedLink(t *testing.T) {
	const host = "https://gitlab.acme.test"
	svc, store, client := newTaskMRLinkService(t, host)
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	setTaskMRRepositoryIdentity(t, store, "repo-1", host, "group/subgroup/project")
	client.SeedMR("group/subgroup/project", &MR{
		IID: 17, Title: "MR title", WebURL: host + "/group/subgroup/project/-/merge_requests/17",
		State: "opened", HeadBranch: "feature", BaseBranch: "main", CreatedAt: time.Now().UTC(),
	})

	first, err := svc.AssociateExistingMRByURL(
		context.Background(), "ws-1", "task-1", "repo-1",
		host+"/group/subgroup/project/-/merge_requests/17?view=parallel#note_1",
	)
	if err != nil {
		t.Fatalf("AssociateExistingMRByURL: %v", err)
	}
	second, err := svc.AssociateExistingMRByURL(
		context.Background(), "ws-1", "task-1", "repo-1",
		host+"/group/subgroup/project/-/merge_requests/17",
	)
	if err != nil {
		t.Fatalf("second AssociateExistingMRByURL: %v", err)
	}
	if first.ID == "" || second.ID != first.ID {
		t.Fatalf("association IDs = %q, %q; want one stable ID", first.ID, second.ID)
	}
	if first.ProjectPath != "group/subgroup/project" || first.MRIID != 17 {
		t.Fatalf("parsed MR identity = %s!%d", first.ProjectPath, first.MRIID)
	}
	rows, err := store.ListTaskMRsByTask(context.Background(), "task-1")
	if err != nil || len(rows) != 1 {
		t.Fatalf("stored rows = %d, err = %v; want one", len(rows), err)
	}
}

// TestAssociateExistingMRByURLForSessionEnsuresWatch pins the gap the
// gateway wiring used to have: the Create-MR action and manual URL linking
// triggered from a session both have a concrete sessionID available, and
// AssociateExistingMRByURLForSession must use it to create a refresh watch
// immediately — mirroring GitHub's AssociatePRByURLForWorkspace — instead of
// leaving the MR unwatched until a later push happens to trigger
// ensureWatchForLinkedMR.
func TestAssociateExistingMRByURLForSessionEnsuresWatch(t *testing.T) {
	const host = "https://gitlab.acme.test"
	svc, store, client := newTaskMRLinkService(t, host)
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	setTaskMRRepositoryIdentity(t, store, "repo-1", host, "group/project")
	client.SeedMR("group/project", &MR{
		IID: 9, Title: "MR title", WebURL: host + "/group/project/-/merge_requests/9",
		State: "opened", HeadBranch: "feature", BaseBranch: "main", CreatedAt: time.Now().UTC(),
	})

	association, err := svc.AssociateExistingMRByURLForSession(
		context.Background(), "ws-1", "session-1", "task-1", "repo-1",
		host+"/group/project/-/merge_requests/9",
	)
	if err != nil {
		t.Fatalf("AssociateExistingMRByURLForSession: %v", err)
	}
	watch, err := store.GetMRWatchBySessionRepoAndBranch(context.Background(), "session-1", "repo-1", "feature")
	if err != nil {
		t.Fatalf("GetMRWatchBySessionRepoAndBranch: %v", err)
	}
	if watch == nil {
		t.Fatal("expected a refresh watch to be created for the session")
	}
	if watch.MRIID != 9 || watch.ProjectPath != "group/project" {
		t.Fatalf("watch = %+v, want mr_iid=9 project_path=group/project", watch)
	}
	if association.MRIID != 9 {
		t.Fatalf("association mr_iid = %d, want 9", association.MRIID)
	}

	// Calling it again for the same session/branch must not error or
	// duplicate the watch (EnsureMRWatch is idempotent).
	if _, err := svc.AssociateExistingMRByURLForSession(
		context.Background(), "ws-1", "session-1", "task-1", "repo-1",
		host+"/group/project/-/merge_requests/9",
	); err != nil {
		t.Fatalf("second AssociateExistingMRByURLForSession: %v", err)
	}
}

func TestAssociateExistingMRByURLAcceptsMRURLWithExplicitDefaultPort(t *testing.T) {
	// The workspace's configured GitLab host and the incoming MR URL should
	// match even when only one side spells out the scheme's implicit default
	// port (":443" for https), since they identify the same web origin.
	const configuredHost = "https://gitlab.acme.test:443"
	const mrURLHost = "https://gitlab.acme.test"
	svc, store, client := newTaskMRLinkService(t, configuredHost)
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	setTaskMRRepositoryIdentity(t, store, "repo-1", configuredHost, "acme/api")
	client.SeedMR("acme/api", &MR{
		IID: 21, Title: "MR", WebURL: mrURLHost + "/acme/api/-/merge_requests/21",
		State: "opened", CreatedAt: time.Now().UTC(),
	})

	if _, err := svc.AssociateExistingMRByURL(
		context.Background(), "ws-1", "task-1", "repo-1",
		mrURLHost+"/acme/api/-/merge_requests/21",
	); err != nil {
		t.Fatalf("AssociateExistingMRByURL: %v", err)
	}
}

func TestAssociateExistingMRByURLRejectsWrongHostAndCrossWorkspaceResources(t *testing.T) {
	const host = "http://gitlab.internal.test"
	svc, store, client := newTaskMRLinkService(t, host)
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	setTaskMRRepositoryIdentity(t, store, "repo-1", host, "acme/api")
	seedTaskMRLinkFixture(t, store, "ws-2", "task-2", "repo-2")
	setTaskMRRepositoryIdentity(t, store, "repo-2", host, "acme/api")
	client.SeedMR("acme/api", &MR{
		IID: 4, Title: "MR", WebURL: host + "/acme/api/-/merge_requests/4",
		State: "opened", CreatedAt: time.Now().UTC(),
	})

	_, err := svc.AssociateExistingMRByURL(
		context.Background(), "ws-1", "task-1", "repo-1",
		"https://gitlab.com/acme/api/-/merge_requests/4",
	)
	if !errors.Is(err, ErrInvalidMRURL) {
		t.Fatalf("wrong-host error = %v, want ErrInvalidMRURL", err)
	}

	_, err = svc.AssociateExistingMRByURL(
		context.Background(), "ws-1", "task-2", "repo-2",
		host+"/acme/api/-/merge_requests/4",
	)
	if !errors.Is(err, ErrTaskMRNotFound) {
		t.Fatalf("cross-workspace error = %v, want ErrTaskMRNotFound", err)
	}

	_, err = svc.AssociateExistingMRByURL(
		context.Background(), "ws-1", "task-1", "repo-2",
		host+"/acme/api/-/merge_requests/4",
	)
	if !errors.Is(err, ErrTaskMRNotFound) {
		t.Fatalf("cross-workspace repository error = %v, want ErrTaskMRNotFound", err)
	}
	rows, listErr := store.ListTaskMRsByTask(context.Background(), "task-1")
	if listErr != nil || len(rows) != 0 {
		t.Fatalf("rejected links persisted rows = %d, err = %v", len(rows), listErr)
	}
}

func TestAssociateExistingMRByURLRejectsRepositoryIdentityMismatch(t *testing.T) {
	const host = "http://gitlab.internal.test:8080"
	tests := []struct {
		name           string
		repositoryHost string
		repositoryPath string
	}{
		{name: "different GitLab host", repositoryHost: "http://other.internal.test:8080", repositoryPath: "group/subgroup/project"},
		{name: "different subgroup project", repositoryHost: host, repositoryPath: "group/other/project"},
		{name: "unknown legacy host", repositoryHost: "", repositoryPath: "group/subgroup/project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, store, client := newTaskMRLinkService(t, host)
			seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
			setTaskMRRepositoryIdentity(t, store, "repo-1", tt.repositoryHost, tt.repositoryPath)
			client.SeedMR("group/subgroup/project", &MR{
				IID: 9, Title: "MR", WebURL: host + "/group/subgroup/project/-/merge_requests/9",
				State: "opened", CreatedAt: time.Now().UTC(),
			})

			_, err := svc.AssociateExistingMRByURL(
				context.Background(), "ws-1", "task-1", "repo-1",
				host+"/group/subgroup/project/-/merge_requests/9",
			)
			if !errors.Is(err, ErrTaskMRRepositoryMismatch) {
				t.Fatalf("error = %v, want ErrTaskMRRepositoryMismatch", err)
			}
		})
	}
}

func TestAssociateExistingMRByURLAcceptsExactSelfManagedHTTPSRepositoryIdentity(t *testing.T) {
	const host = "https://gitlab.internal.test:8443"
	svc, store, client := newTaskMRLinkService(t, host)
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	setTaskMRRepositoryIdentity(t, store, "repo-1", host+"/", "group/subgroup/project")
	client.SeedMR("group/subgroup/project", &MR{
		IID: 11, Title: "MR", WebURL: host + "/group/subgroup/project/-/merge_requests/11",
		State: "opened", CreatedAt: time.Now().UTC(),
	})

	if _, err := svc.AssociateExistingMRByURL(
		context.Background(), "ws-1", "task-1", "repo-1",
		host+"/group/subgroup/project/-/merge_requests/11",
	); err != nil {
		t.Fatalf("AssociateExistingMRByURL: %v", err)
	}
}

func TestAssociateExistingMRByURLAcceptsRepositoryIdentityWithExplicitDefaultPort(t *testing.T) {
	const host = "https://gitlab.internal.test"
	svc, store, client := newTaskMRLinkService(t, host)
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	// An explicit default HTTPS port (:443) must compare equal to the
	// workspace host, which omits it.
	setTaskMRRepositoryIdentity(t, store, "repo-1", "https://gitlab.internal.test:443", "group/subgroup/project")
	client.SeedMR("group/subgroup/project", &MR{
		IID: 13, Title: "MR", WebURL: host + "/group/subgroup/project/-/merge_requests/13",
		State: "opened", CreatedAt: time.Now().UTC(),
	})

	if _, err := svc.AssociateExistingMRByURL(
		context.Background(), "ws-1", "task-1", "repo-1",
		host+"/group/subgroup/project/-/merge_requests/13",
	); err != nil {
		t.Fatalf("AssociateExistingMRByURL: %v", err)
	}
}

func TestAssociateExistingMRByURLAcceptsScpStyleRemoteWithBracketedIPv6Host(t *testing.T) {
	const host = "https://[::1]"
	svc, store, client := newTaskMRLinkService(t, host)
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	setTaskMRRepositoryRemoteURL(t, store, "repo-1", "git@[::1]:clients/socodevi/laravel/co-up.git")
	client.SeedMR("clients/socodevi/laravel/co-up", &MR{
		IID: 92, Title: "MR", WebURL: host + "/clients/socodevi/laravel/co-up/-/merge_requests/92",
		State: "opened", CreatedAt: time.Now().UTC(),
	})

	if _, err := svc.AssociateExistingMRByURL(
		context.Background(), "ws-1", "task-1", "repo-1",
		host+"/clients/socodevi/laravel/co-up/-/merge_requests/92",
	); err != nil {
		t.Fatalf("AssociateExistingMRByURL: %v", err)
	}
}

func TestAssociateExistingMRByURLAcceptsSSHURLRemoteWithIPv6Host(t *testing.T) {
	const host = "https://[::1]"
	tests := []struct {
		name      string
		remoteURL string
	}{
		{name: "no port", remoteURL: "ssh://git@[::1]/clients/socodevi/laravel/co-up.git"},
		{name: "explicit non-web port", remoteURL: "ssh://git@[::1]:2222/clients/socodevi/laravel/co-up.git"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, store, client := newTaskMRLinkService(t, host)
			seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
			setTaskMRRepositoryRemoteURL(t, store, "repo-1", tt.remoteURL)
			client.SeedMR("clients/socodevi/laravel/co-up", &MR{
				IID: 92, Title: "MR", WebURL: host + "/clients/socodevi/laravel/co-up/-/merge_requests/92",
				State: "opened", CreatedAt: time.Now().UTC(),
			})

			if _, err := svc.AssociateExistingMRByURL(
				context.Background(), "ws-1", "task-1", "repo-1",
				host+"/clients/socodevi/laravel/co-up/-/merge_requests/92",
			); err != nil {
				t.Fatalf("AssociateExistingMRByURL: %v", err)
			}
		})
	}
}

func TestAssociateExistingMRByURLAcceptsRepositoryIdentityWithSameHostDifferentScheme(t *testing.T) {
	const host = "http://gitlab.internal.test:8080"
	svc, store, client := newTaskMRLinkService(t, host)
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	setTaskMRRepositoryIdentity(t, store, "repo-1", "https://gitlab.internal.test:8080", "group/subgroup/project")
	client.SeedMR("group/subgroup/project", &MR{
		IID: 12, Title: "MR", WebURL: host + "/group/subgroup/project/-/merge_requests/12",
		State: "opened", CreatedAt: time.Now().UTC(),
	})

	if _, err := svc.AssociateExistingMRByURL(
		context.Background(), "ws-1", "task-1", "repo-1",
		host+"/group/subgroup/project/-/merge_requests/12",
	); err != nil {
		t.Fatalf("AssociateExistingMRByURL: %v", err)
	}
}

func TestAssociateExistingMRByURLAcceptsRemoteURLWhenDurableIdentityEmpty(t *testing.T) {
	// Reproduces the reported bug: a self-hosted GitLab repository added as a
	// local clone never gets a durable provider identity (only github.com and
	// gitlab.com are tagged at discovery time), but its remote_url still
	// clearly identifies the same GitLab host and nested-subgroup project.
	const host = "https://gitlab.savoirfairelinux.com"
	tests := []struct {
		name      string
		remoteURL string
	}{
		{name: "https remote", remoteURL: host + "/clients/socodevi/laravel/co-up.git"},
		{name: "https remote without .git suffix", remoteURL: host + "/clients/socodevi/laravel/co-up"},
		{name: "https remote with trailing slash", remoteURL: host + "/clients/socodevi/laravel/co-up/"},
		{name: "https remote with different case", remoteURL: "https://GitLab.SavoirFaireLinux.com/Clients/Socodevi/Laravel/Co-Up.git"},
		{name: "http remote for https workspace host", remoteURL: "http://gitlab.savoirfairelinux.com/clients/socodevi/laravel/co-up.git"},
		{name: "ssh url remote", remoteURL: "ssh://git@gitlab.savoirfairelinux.com/clients/socodevi/laravel/co-up.git"},
		{name: "scp-style remote", remoteURL: "git@gitlab.savoirfairelinux.com:clients/socodevi/laravel/co-up.git"},
		// SSH transport ports (used by hosts that put SSH on a non-standard
		// port) are unrelated to the GitLab web origin's port and must not
		// be compared against it.
		{name: "ssh url remote with explicit non-web port", remoteURL: "ssh://git@gitlab.savoirfairelinux.com:2222/clients/socodevi/laravel/co-up.git"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, store, client := newTaskMRLinkService(t, host)
			seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
			setTaskMRRepositoryRemoteURL(t, store, "repo-1", tt.remoteURL)
			client.SeedMR("clients/socodevi/laravel/co-up", &MR{
				IID: 92, Title: "MR", WebURL: host + "/clients/socodevi/laravel/co-up/-/merge_requests/92",
				State: "opened", CreatedAt: time.Now().UTC(),
			})

			if _, err := svc.AssociateExistingMRByURL(
				context.Background(), "ws-1", "task-1", "repo-1",
				host+"/clients/socodevi/laravel/co-up/-/merge_requests/92",
			); err != nil {
				t.Fatalf("AssociateExistingMRByURL: %v", err)
			}
		})
	}
}

func TestAssociateExistingMRByURLBackfillsRemoteURLFromLocalCheckoutWhenLegacyRowIsBlank(t *testing.T) {
	// Reproduces the persisted-bug shape found on a live instance: a
	// repository row created before remote_url resolution existed has BOTH
	// the durable provider identity AND remote_url blank, even though its
	// local_path checkout still has a valid origin remote. This must
	// succeed by falling back to the local git config, and must backfill
	// remote_url so subsequent calls don't need the filesystem read.
	const host = "https://gitlab.savoirfairelinux.com"
	svc, store, client := newTaskMRLinkService(t, host)
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	localPath := t.TempDir()
	seedLocalGitCheckout(t, localPath, "git@gitlab.savoirfairelinux.com:clients/socodevi/laravel/co-up.git")
	setTaskMRRepositoryLocalPath(t, store, "repo-1", localPath)
	client.SeedMR("clients/socodevi/laravel/co-up", &MR{
		IID: 92, Title: "MR", WebURL: host + "/clients/socodevi/laravel/co-up/-/merge_requests/92",
		State: "opened", CreatedAt: time.Now().UTC(),
	})

	if _, err := svc.AssociateExistingMRByURL(
		context.Background(), "ws-1", "task-1", "repo-1",
		host+"/clients/socodevi/laravel/co-up/-/merge_requests/92",
	); err != nil {
		t.Fatalf("AssociateExistingMRByURL: %v", err)
	}

	// Backfilled as a credential-free canonical "host/project" form, not the
	// raw local remote (which may embed ssh/scp syntax or credentials).
	got := getTaskMRRepositoryRemoteURL(t, store, "repo-1")
	if got != host+"/clients/socodevi/laravel/co-up" {
		t.Fatalf("remote_url not backfilled, got %q", got)
	}
}

func TestAssociateExistingMRByURLBackfillsRemoteURLFromLocalWorktreeCheckoutWhenLegacyRowIsBlank(t *testing.T) {
	// Covers the `git worktree add` on-disk shape: local_path/.git is a
	// *file* containing a "gitdir:" pointer into the main checkout's
	// "worktrees/<name>" directory, which in turn has a "commondir" file
	// pointing back at the main checkout where the origin remote actually
	// lives. resolveLocalGitDir/resolveLocalCommonGitDir must follow both
	// indirections for the legacy-row fallback to work for worktree
	// checkouts, not just plain clones.
	const host = "https://gitlab.savoirfairelinux.com"
	svc, store, client := newTaskMRLinkService(t, host)
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	root := t.TempDir()
	mainGitDir := filepath.Join(root, "main", ".git")
	worktreeDir := filepath.Join(root, "worktree")
	seedLocalGitWorktreeCheckout(
		t, worktreeDir, mainGitDir,
		"git@gitlab.savoirfairelinux.com:clients/socodevi/laravel/co-up.git",
	)
	setTaskMRRepositoryLocalPath(t, store, "repo-1", worktreeDir)
	client.SeedMR("clients/socodevi/laravel/co-up", &MR{
		IID: 92, Title: "MR", WebURL: host + "/clients/socodevi/laravel/co-up/-/merge_requests/92",
		State: "opened", CreatedAt: time.Now().UTC(),
	})

	if _, err := svc.AssociateExistingMRByURL(
		context.Background(), "ws-1", "task-1", "repo-1",
		host+"/clients/socodevi/laravel/co-up/-/merge_requests/92",
	); err != nil {
		t.Fatalf("AssociateExistingMRByURL: %v", err)
	}

	got := getTaskMRRepositoryRemoteURL(t, store, "repo-1")
	if got != host+"/clients/socodevi/laravel/co-up" {
		t.Fatalf("remote_url not backfilled, got %q", got)
	}
}

func TestAssociateExistingMRByURLRejectsLocalCheckoutPointingElsewhere(t *testing.T) {
	// A legacy blank row whose local checkout's origin points to a different
	// project must still fail closed, and must not backfill the wrong URL.
	const host = "https://gitlab.savoirfairelinux.com"
	svc, store, client := newTaskMRLinkService(t, host)
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	localPath := t.TempDir()
	seedLocalGitCheckout(t, localPath, "git@gitlab.savoirfairelinux.com:clients/socodevi/laravel/other-project.git")
	setTaskMRRepositoryLocalPath(t, store, "repo-1", localPath)
	client.SeedMR("clients/socodevi/laravel/co-up", &MR{
		IID: 92, Title: "MR", WebURL: host + "/clients/socodevi/laravel/co-up/-/merge_requests/92",
		State: "opened", CreatedAt: time.Now().UTC(),
	})

	_, err := svc.AssociateExistingMRByURL(
		context.Background(), "ws-1", "task-1", "repo-1",
		host+"/clients/socodevi/laravel/co-up/-/merge_requests/92",
	)
	if !errors.Is(err, ErrTaskMRRepositoryMismatch) {
		t.Fatalf("error = %v, want ErrTaskMRRepositoryMismatch", err)
	}
	if got := getTaskMRRepositoryRemoteURL(t, store, "repo-1"); got != "" {
		t.Fatalf("remote_url should stay blank on mismatch, got %q", got)
	}
}

func TestAssociateExistingMRByURLIgnoresLocalCheckoutWhenDurableIdentityAlreadyPointsElsewhere(t *testing.T) {
	// A repository row that already has a durable provider identity for a
	// DIFFERENT project (remote_url blank, e.g. it predates remote_url
	// resolution) must not be overridden by a coincidentally-matching local
	// checkout: the durable identity is authoritative once it exists at all.
	const host = "https://gitlab.savoirfairelinux.com"
	svc, store, client := newTaskMRLinkService(t, host)
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	setTaskMRRepositoryIdentity(t, store, "repo-1", host, "clients/socodevi/laravel/other-project")
	localPath := t.TempDir()
	seedLocalGitCheckout(t, localPath, "git@gitlab.savoirfairelinux.com:clients/socodevi/laravel/co-up.git")
	setTaskMRRepositoryLocalPath(t, store, "repo-1", localPath)
	client.SeedMR("clients/socodevi/laravel/co-up", &MR{
		IID: 92, Title: "MR", WebURL: host + "/clients/socodevi/laravel/co-up/-/merge_requests/92",
		State: "opened", CreatedAt: time.Now().UTC(),
	})

	_, err := svc.AssociateExistingMRByURL(
		context.Background(), "ws-1", "task-1", "repo-1",
		host+"/clients/socodevi/laravel/co-up/-/merge_requests/92",
	)
	if !errors.Is(err, ErrTaskMRRepositoryMismatch) {
		t.Fatalf("error = %v, want ErrTaskMRRepositoryMismatch", err)
	}
	if got := getTaskMRRepositoryRemoteURL(t, store, "repo-1"); got != "" {
		t.Fatalf("remote_url should stay blank, got %q", got)
	}
}

func TestAssociateExistingMRByURLIgnoresLocalCheckoutWhenOnlyProviderRepoIDIsSet(t *testing.T) {
	// provider_repo_id is part of the durable provider identity and can be
	// populated by service-layer backfills independently of the other
	// provider_* columns and remote_url. A row carrying only
	// provider_repo_id (every other identity column blank) already has an
	// established identity and must not fall back to the local checkout,
	// even though a naive "all other columns blank" check would treat it as
	// a fully legacy row.
	const host = "https://gitlab.savoirfairelinux.com"
	svc, store, client := newTaskMRLinkService(t, host)
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	setTaskMRRepositoryProviderRepoID(t, store, "repo-1", "12345")
	localPath := t.TempDir()
	seedLocalGitCheckout(t, localPath, "git@gitlab.savoirfairelinux.com:clients/socodevi/laravel/co-up.git")
	setTaskMRRepositoryLocalPath(t, store, "repo-1", localPath)
	client.SeedMR("clients/socodevi/laravel/co-up", &MR{
		IID: 92, Title: "MR", WebURL: host + "/clients/socodevi/laravel/co-up/-/merge_requests/92",
		State: "opened", CreatedAt: time.Now().UTC(),
	})

	_, err := svc.AssociateExistingMRByURL(
		context.Background(), "ws-1", "task-1", "repo-1",
		host+"/clients/socodevi/laravel/co-up/-/merge_requests/92",
	)
	if !errors.Is(err, ErrTaskMRRepositoryMismatch) {
		t.Fatalf("error = %v, want ErrTaskMRRepositoryMismatch", err)
	}
	if got := getTaskMRRepositoryRemoteURL(t, store, "repo-1"); got != "" {
		t.Fatalf("remote_url should stay blank, got %q", got)
	}
}

func TestAssociateExistingMRByURLBackfillsCredentialFreeRemoteURLWhenLocalOriginEmbedsCredentials(t *testing.T) {
	// A local checkout's origin can legitimately embed userinfo credentials
	// (e.g. an HTTPS remote with an inline PAT). Those must never be
	// persisted to remote_url, which is returned in repository API payloads.
	const host = "https://gitlab.savoirfairelinux.com"
	svc, store, client := newTaskMRLinkService(t, host)
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	localPath := t.TempDir()
	seedLocalGitCheckout(
		t, localPath,
		"https://alice:apikey123@gitlab.savoirfairelinux.com/clients/socodevi/laravel/co-up.git",
	)
	setTaskMRRepositoryLocalPath(t, store, "repo-1", localPath)
	client.SeedMR("clients/socodevi/laravel/co-up", &MR{
		IID: 92, Title: "MR", WebURL: host + "/clients/socodevi/laravel/co-up/-/merge_requests/92",
		State: "opened", CreatedAt: time.Now().UTC(),
	})

	if _, err := svc.AssociateExistingMRByURL(
		context.Background(), "ws-1", "task-1", "repo-1",
		host+"/clients/socodevi/laravel/co-up/-/merge_requests/92",
	); err != nil {
		t.Fatalf("AssociateExistingMRByURL: %v", err)
	}

	got := getTaskMRRepositoryRemoteURL(t, store, "repo-1")
	// Assert on the userinfo separator itself, not specific token/password
	// substrings, so this stays correct regardless of the fake credential
	// used above.
	if strings.Contains(got, "@") {
		t.Fatalf("backfilled remote_url leaked userinfo credentials: %q", got)
	}
	if got != host+"/clients/socodevi/laravel/co-up" {
		t.Fatalf("backfilled remote_url = %q, want credential-free canonical form", got)
	}
}

func TestParseLocalGitConfigOriginURLToleratesWhitespaceAndCaseVariants(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   string
	}{
		{
			name:   "canonical spacing",
			config: "[remote \"origin\"]\n\turl = https://gitlab.example.com/g/p.git\n",
			want:   "https://gitlab.example.com/g/p.git",
		},
		{
			name:   "no spaces around equals",
			config: "[remote \"origin\"]\n\turl=https://gitlab.example.com/g/p.git\n",
			want:   "https://gitlab.example.com/g/p.git",
		},
		{
			name:   "extra whitespace around equals",
			config: "[remote \"origin\"]\n\turl    =    https://gitlab.example.com/g/p.git\n",
			want:   "https://gitlab.example.com/g/p.git",
		},
		{
			name:   "uppercase key",
			config: "[remote \"origin\"]\n\tURL = https://gitlab.example.com/g/p.git\n",
			want:   "https://gitlab.example.com/g/p.git",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseLocalGitConfigOriginURL(tt.config); got != tt.want {
				t.Fatalf("parseLocalGitConfigOriginURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAssociateExistingMRByURLRejectsRemoteURLPointingElsewhere(t *testing.T) {
	const host = "https://gitlab.savoirfairelinux.com"
	svc, store, client := newTaskMRLinkService(t, host)
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	// Durable identity empty (legacy/unresolved row) and remote_url points to
	// a different project than the MR being linked.
	setTaskMRRepositoryRemoteURL(t, store, "repo-1", host+"/clients/socodevi/laravel/other-project.git")
	client.SeedMR("clients/socodevi/laravel/co-up", &MR{
		IID: 92, Title: "MR", WebURL: host + "/clients/socodevi/laravel/co-up/-/merge_requests/92",
		State: "opened", CreatedAt: time.Now().UTC(),
	})

	_, err := svc.AssociateExistingMRByURL(
		context.Background(), "ws-1", "task-1", "repo-1",
		host+"/clients/socodevi/laravel/co-up/-/merge_requests/92",
	)
	if !errors.Is(err, ErrTaskMRRepositoryMismatch) {
		t.Fatalf("error = %v, want ErrTaskMRRepositoryMismatch", err)
	}
}

func TestAssociateExistingMRByURLRejectsRemoteURLOnDifferentHost(t *testing.T) {
	const host = "https://gitlab.savoirfairelinux.com"
	svc, store, client := newTaskMRLinkService(t, host)
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	setTaskMRRepositoryRemoteURL(t, store, "repo-1", "https://gitlab.other.test/clients/socodevi/laravel/co-up.git")
	client.SeedMR("clients/socodevi/laravel/co-up", &MR{
		IID: 92, Title: "MR", WebURL: host + "/clients/socodevi/laravel/co-up/-/merge_requests/92",
		State: "opened", CreatedAt: time.Now().UTC(),
	})

	_, err := svc.AssociateExistingMRByURL(
		context.Background(), "ws-1", "task-1", "repo-1",
		host+"/clients/socodevi/laravel/co-up/-/merge_requests/92",
	)
	if !errors.Is(err, ErrTaskMRRepositoryMismatch) {
		t.Fatalf("error = %v, want ErrTaskMRRepositoryMismatch", err)
	}
}

func TestAssociateExistingMRByURLRejectsRepositoryWithNullIdentityColumns(t *testing.T) {
	// The production schema declares provider/provider_host/provider_owner/
	// provider_name/remote_url as nullable columns, so rows can legitimately
	// have SQL NULL there. ValidateTaskMRRepositoryIdentity must coalesce
	// those to empty strings and fail closed with the normal mismatch error
	// instead of an internal scan error.
	const host = "https://gitlab.savoirfairelinux.com"
	svc, store, client := newTaskMRLinkService(t, host)
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	setTaskMRRepositoryIdentityColumnsNull(t, store, "repo-1")
	client.SeedMR("clients/socodevi/laravel/co-up", &MR{
		IID: 92, Title: "MR", WebURL: host + "/clients/socodevi/laravel/co-up/-/merge_requests/92",
		State: "opened", CreatedAt: time.Now().UTC(),
	})

	_, err := svc.AssociateExistingMRByURL(
		context.Background(), "ws-1", "task-1", "repo-1",
		host+"/clients/socodevi/laravel/co-up/-/merge_requests/92",
	)
	if !errors.Is(err, ErrTaskMRRepositoryMismatch) {
		t.Fatalf("error = %v, want ErrTaskMRRepositoryMismatch", err)
	}
}

func TestAssociateExistingMRByURLInfersSoleRepositoryAndRejectsMultiRepoAmbiguity(t *testing.T) {
	const host = "https://gitlab.acme.test"
	svc, store, client := newTaskMRLinkService(t, host)
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	setTaskMRRepositoryIdentity(t, store, "repo-1", host, "acme/api")
	client.SeedMR("acme/api", &MR{
		IID: 8, Title: "MR", WebURL: host + "/acme/api/-/merge_requests/8",
		State: "opened", CreatedAt: time.Now().UTC(),
	})

	association, err := svc.AssociateExistingMRByURL(
		context.Background(), "ws-1", "task-1", "", host+"/acme/api/-/merge_requests/8",
	)
	if err != nil {
		t.Fatalf("infer sole repository: %v", err)
	}
	if association.RepositoryID != "repo-1" {
		t.Fatalf("repository_id = %q, want repo-1", association.RepositoryID)
	}

	if _, err := store.db.Exec(`INSERT INTO repositories (id, workspace_id) VALUES ('repo-2', 'ws-1');
		INSERT INTO task_repositories (id, task_id, repository_id) VALUES ('task-repo-2', 'task-1', 'repo-2')`); err != nil {
		t.Fatalf("seed second task repository: %v", err)
	}
	setTaskMRRepositoryIdentity(t, store, "repo-2", host, "acme/other")
	_, err = svc.AssociateExistingMRByURL(
		context.Background(), "ws-1", "task-1", "", host+"/acme/api/-/merge_requests/8",
	)
	if !errors.Is(err, ErrTaskMRRepositoryRequired) {
		t.Fatalf("ambiguous repository error = %v, want ErrTaskMRRepositoryRequired", err)
	}
}

func TestUnlinkTaskMRRemovesOnlySelectedAssociationAndRefreshWatch(t *testing.T) {
	svc, store, client := newTaskMRLinkService(t, DefaultHost)
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	setTaskMRRepositoryIdentity(t, store, "repo-1", DefaultHost, "acme/api")
	for iid := 1; iid <= 2; iid++ {
		client.SeedMR("acme/api", &MR{
			IID: iid, Title: "MR", WebURL: DefaultHost + "/acme/api/-/merge_requests/" + string(rune('0'+iid)),
			State: "opened", CreatedAt: time.Now().UTC(),
		})
	}
	selected, err := svc.AssociateExistingMRByURL(
		context.Background(), "ws-1", "task-1", "repo-1", DefaultHost+"/acme/api/-/merge_requests/1",
	)
	if err != nil {
		t.Fatalf("associate selected: %v", err)
	}
	_, err = svc.AssociateExistingMRByURL(
		context.Background(), "ws-1", "task-1", "repo-1", DefaultHost+"/acme/api/-/merge_requests/2",
	)
	if err != nil {
		t.Fatalf("associate retained: %v", err)
	}
	for iid := 1; iid <= 2; iid++ {
		if err := store.CreateMRWatch(context.Background(), &MRWatch{
			SessionID: "session-" + string(rune('0'+iid)), TaskID: "task-1", RepositoryID: "repo-1",
			ProjectPath: "acme/api", MRIID: iid, Branch: "feature",
		}); err != nil {
			t.Fatalf("create MR watch: %v", err)
		}
	}

	if err := svc.UnlinkTaskMR(context.Background(), "ws-1", selected.ID); err != nil {
		t.Fatalf("UnlinkTaskMR: %v", err)
	}
	rows, _ := store.ListTaskMRsByTask(context.Background(), "task-1")
	watches, _ := store.ListMRWatchesByTask(context.Background(), "task-1")
	if len(rows) != 1 || rows[0].MRIID != 2 {
		t.Fatalf("remaining associations = %+v, want only !2", rows)
	}
	if len(watches) != 1 || watches[0].MRIID != 2 {
		t.Fatalf("remaining refresh watches = %+v, want only !2", watches)
	}
}
