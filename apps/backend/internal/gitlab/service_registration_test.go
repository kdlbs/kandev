package gitlab

import (
	"context"
	"testing"
	"time"
)

func TestRegisterExistingMRByURLRequiresExactProviderRepositoryID(t *testing.T) {
	const host = "https://gitlab.acme.test"
	svc, store, client := newTaskMRLinkService(t, host)
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	setTaskMRRepositoryIdentity(t, store, "repo-1", host, "group/project")
	if _, err := store.db.Exec(`UPDATE repositories SET provider_repo_id = '42' WHERE id = 'repo-1'`); err != nil {
		t.Fatalf("set provider repository ID: %v", err)
	}
	client.SeedMR("group/project", &MR{
		IID: 17, ProjectID: 42, TargetProjectID: 42, Title: "verified", WebURL: host + "/group/project/-/merge_requests/17",
		State: "opened", HeadBranch: "feature", BaseBranch: "main", CreatedAt: time.Now().UTC(),
	})
	registered, err := svc.RegisterExistingMRByURL(context.Background(), "ws-1", "task-1", "repo-1",
		host+"/group/project/-/merge_requests/17", false, "operation-1")
	if err != nil {
		t.Fatalf("RegisterExistingMRByURL: %v", err)
	}
	if registered.ProviderRepositoryID != "42" || registered.ConnectionScope != "builtin:ws-1" ||
		registered.ComparisonPolicy != TaskMRComparisonPolicyPreserve || registered.Source != TaskMRSourceAgentRegistration {
		t.Fatalf("registered identity and policy = %+v", registered)
	}

	client.SeedMR("group/project", &MR{
		IID: 18, ProjectID: 43, TargetProjectID: 43, Title: "wrong target", WebURL: host + "/group/project/-/merge_requests/18",
		State: "opened", HeadBranch: "feature", BaseBranch: "main", CreatedAt: time.Now().UTC(),
	})
	if _, err := svc.RegisterExistingMRByURL(context.Background(), "ws-1", "task-1", "repo-1",
		host+"/group/project/-/merge_requests/18", false, "operation-2"); err != ErrTaskMRRepositoryMismatch {
		t.Fatalf("wrong provider project ID error = %v, want repository mismatch", err)
	}
	rows, err := store.ListTaskMRsByTask(context.Background(), "task-1")
	if err != nil || len(rows) != 1 {
		t.Fatalf("associations after mismatched registration = %d, err=%v; want only verified row", len(rows), err)
	}
}

func TestRegistrationKeepsAcceptedRevisionAcrossRetry(t *testing.T) {
	const host = "https://gitlab.acme.test"
	svc, store, client := newTaskMRLinkService(t, host)
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	setTaskMRRepositoryIdentity(t, store, "repo-1", host, "group/project")
	if _, err := store.db.Exec(`UPDATE repositories SET provider_repo_id = '42' WHERE id = 'repo-1'`); err != nil {
		t.Fatalf("set provider repository ID: %v", err)
	}
	client.SeedMR("group/project", &MR{
		IID: 17, ProjectID: 42, TargetProjectID: 42, Title: "retry fence",
		WebURL: host + "/group/project/-/merge_requests/17", State: "opened",
		HeadBranch: "feature", BaseBranch: "main", CreatedAt: time.Now().UTC(),
	})
	created, err := svc.RegisterExistingMRByURL(context.Background(), "ws-1", "task-1", "repo-1",
		host+"/group/project/-/merge_requests/17", false, "message-op")
	if err != nil {
		t.Fatalf("create initial association: %v", err)
	}
	detached, _, err := store.DetachTaskMRForWorkspace(context.Background(), "ws-1", created.ID)
	if err != nil {
		t.Fatalf("detach initial association: %v", err)
	}
	commit := func(ctx context.Context, registration *TaskMR, restore bool, expected int64) (*TaskMR, bool, error) {
		return store.RegisterTaskMRObservation(ctx, registration, restore, expected)
	}
	if _, err := svc.RegisterExistingMRByURLAtRevision(
		context.Background(), "ws-1", "task-1", "repo-1", host+"/group/project/-/merge_requests/17",
		created.AssociationRevision, true, "message-op", commit,
	); err != ErrTaskMRRegistrationRevisionChanged {
		t.Fatalf("retry with pre-unlink revision = %v, want revision changed", err)
	}
	restored, err := svc.RegisterExistingMRByURLAtRevision(
		context.Background(), "ws-1", "task-1", "repo-1", host+"/group/project/-/merge_requests/17",
		detached.AssociationRevision, true, "message-op", commit,
	)
	if err != nil || restored.AssociationRevision != detached.AssociationRevision+1 || restored.DetachedAt != nil {
		t.Fatalf("retry with accepted detached revision = %+v, err=%v", restored, err)
	}
}
