package gitlab

import (
	"context"
	"errors"
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
		provider TEXT NOT NULL DEFAULT '',
		provider_host TEXT NOT NULL DEFAULT '',
		provider_owner TEXT NOT NULL DEFAULT '',
		provider_name TEXT NOT NULL DEFAULT ''
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
		{name: "same hostname with HTTPS origin", repositoryHost: "https://gitlab.internal.test:8080", repositoryPath: "group/subgroup/project"},
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
