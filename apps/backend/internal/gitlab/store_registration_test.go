package gitlab

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestRegistrationCreatesObservationAndSoftDetachFence(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTaskMRLinkFixture(t, store, "workspace-registration", "task-registration", "repo-registration")
	setTaskMRRepositoryIdentity(t, store, "repo-registration", "https://gitlab.example.test", "group/project")
	if _, err := store.db.Exec(`UPDATE repositories SET provider_repo_id = '42' WHERE id = 'repo-registration'`); err != nil {
		t.Fatalf("set immutable provider repository ID: %v", err)
	}
	registration := registrationTaskMR("task-registration", "repo-registration", "42", 9)
	created, changed, err := store.RegisterTaskMRObservation(ctx, registration, false, 0)
	if err != nil {
		t.Fatalf("RegisterTaskMRObservation: %v", err)
	}
	if !changed || created.ComparisonPolicy != TaskMRComparisonPolicyPreserve || created.AssociationRevision != 1 ||
		created.Source != TaskMRSourceAgentRegistration || created.DetachedAt != nil {
		t.Fatalf("created registration = %+v, changed=%v", created, changed)
	}
	options, err := store.GetTaskMRAutomationOptionsForMR(ctx, created.TaskID, MRIdentity{
		RepositoryID: created.RepositoryID, ProjectPath: created.ProjectPath, MRIID: created.MRIID,
	})
	if err != nil || options.AutoFixEnabled || options.AutoMergeEnabled || options.PromptOnReviewRequested || options.PromptOnMerged || options.PromptOnClosed {
		t.Fatalf("registration automation options = %+v, err=%v", options, err)
	}

	detached, changed, err := store.DetachTaskMRForWorkspace(ctx, "workspace-registration", created.ID)
	if err != nil || !changed || detached.DetachedAt == nil || detached.AssociationRevision != 2 {
		t.Fatalf("detach registration = %+v, changed=%v err=%v", detached, changed, err)
	}
	if _, _, err := store.RegisterTaskMRObservation(ctx, registrationTaskMR("task-registration", "repo-registration", "42", 9), true, 1); err != ErrTaskMRRegistrationRevisionChanged {
		t.Fatalf("stale restore error = %v, want revision changed", err)
	}
	restored, changed, err := store.RegisterTaskMRObservation(ctx, registrationTaskMR("task-registration", "repo-registration", "42", 9), true, 2)
	if err != nil || !changed || restored.AssociationRevision != 3 || restored.DetachedAt != nil {
		t.Fatalf("restore registration = %+v, changed=%v err=%v", restored, changed, err)
	}
}

func TestRegistrationLegacyIdentityRepair(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTaskMRLinkFixture(t, store, "workspace-registration", "task-registration", "repo-registration")
	legacy := registrationTaskMR("task-registration", "repo-registration", "", 9)
	legacy.ProviderRepositoryID = ""
	if err := store.UpsertTaskMR(ctx, legacy); err != nil {
		t.Fatalf("seed legacy association: %v", err)
	}
	if _, _, err := store.RegisterTaskMRObservation(ctx, registrationTaskMR("task-registration", "repo-registration", "42", 9), true, 1); err != ErrTaskMRRegistrationIdentityUnresolved {
		t.Fatalf("legacy identity registration error = %v, want unresolved", err)
	}
	stored, err := store.GetTaskMRIncludingDetached(ctx, "task-registration", "repo-registration", "group/project", 9)
	if err != nil || stored == nil || stored.ProviderRepositoryID != "" {
		t.Fatalf("legacy association was rebound: stored=%+v err=%v", stored, err)
	}
}

func TestRegistrationImmutableIdentity(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTaskMRLinkFixture(t, store, "workspace-registration", "task-registration-identity", "repo-old")
	setTaskMRRepositoryIdentity(t, store, "repo-old", "https://gitlab.example.test", "group/project")
	if _, err := store.db.Exec(`UPDATE repositories SET provider_repo_id = '42' WHERE id = 'repo-old'`); err != nil {
		t.Fatalf("set original provider repository ID: %v", err)
	}
	first, changed, err := store.RegisterTaskMRObservation(ctx,
		registrationTaskMR("task-registration-identity", "repo-old", "42", 29), false, 0)
	if err != nil || !changed {
		t.Fatalf("create initial association: row=%+v changed=%v err=%v", first, changed, err)
	}
	if _, err := store.db.Exec(`INSERT INTO repositories (id, workspace_id, provider, provider_repo_id, provider_host, provider_owner, provider_name) VALUES (?, ?, 'gitlab', '42', ?, 'renamed', 'project')`,
		"repo-renamed", "workspace-registration", "https://gitlab.example.test"); err != nil {
		t.Fatalf("create rebound repository: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO task_repositories (id, task_id, repository_id) VALUES (?, ?, ?)`,
		"task-repo-registration-identity", "task-registration-identity", "repo-renamed"); err != nil {
		t.Fatalf("attach rebound repository: %v", err)
	}
	rebound := registrationTaskMR("task-registration-identity", "repo-renamed", "42", 29)
	rebound.RepositoryPath = "renamed/project"
	rebound.ProjectPath = "renamed/project"
	rebound.Host = "https://gitlab.example.test"
	second, changed, err := store.RegisterTaskMRObservation(ctx, rebound, false, first.AssociationRevision)
	if err != nil || changed || second == nil || second.ID != first.ID || second.RepositoryID != first.RepositoryID {
		t.Fatalf("canonical identity was duplicated or rebound: first=%+v second=%+v changed=%v err=%v", first, second, changed, err)
	}
	if _, _, err := store.DetachTaskMRForWorkspace(ctx, "workspace-registration", first.ID); err != nil {
		t.Fatalf("detach original association: %v", err)
	}
	if _, err := store.db.Exec(`UPDATE repositories SET provider_repo_id = '99' WHERE id = 'repo-renamed'`); err != nil {
		t.Fatalf("reuse path with different immutable identity: %v", err)
	}
	reused := registrationTaskMR("task-registration-identity", "repo-renamed", "99", 29)
	reused.RepositoryPath = "renamed/project"
	reused.ProjectPath = "renamed/project"
	replacement, changed, err := store.RegisterTaskMRObservation(ctx, reused, false, 0)
	if err != nil || !changed || replacement == nil || replacement.ID == first.ID || replacement.DetachedAt != nil {
		t.Fatalf("path reuse inherited a different repository association: replacement=%+v changed=%v err=%v", replacement, changed, err)
	}
}

func TestRegistrationConcurrentRestoreUsesRevisionFence(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTaskMRLinkFixture(t, store, "workspace-registration", "task-registration-race", "repo-registration")
	setTaskMRRepositoryIdentity(t, store, "repo-registration", "https://gitlab.example.test", "group/project")
	if _, err := store.db.Exec(`UPDATE repositories SET provider_repo_id = '42' WHERE id = 'repo-registration'`); err != nil {
		t.Fatalf("set immutable provider repository ID: %v", err)
	}
	created, changed, err := store.RegisterTaskMRObservation(ctx,
		registrationTaskMR("task-registration-race", "repo-registration", "42", 29), false, 0)
	if err != nil || !changed || created.AssociationRevision != 1 {
		t.Fatalf("create registration: row=%+v changed=%v err=%v", created, changed, err)
	}
	detached, changed, err := store.DetachTaskMRForWorkspace(ctx, "workspace-registration", created.ID)
	if err != nil || !changed || detached.AssociationRevision != 2 {
		t.Fatalf("detach registration: row=%+v changed=%v err=%v", detached, changed, err)
	}

	start := make(chan struct{})
	results := make(chan struct {
		changed bool
		err     error
	}, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			_, restored, restoreErr := store.RegisterTaskMRObservation(ctx,
				registrationTaskMR("task-registration-race", "repo-registration", "42", 29), true, 2)
			results <- struct {
				changed bool
				err     error
			}{restored, restoreErr}
		}()
	}
	close(start)
	workers.Wait()
	close(results)

	changedCount := 0
	for result := range results {
		if result.err != nil && result.err != ErrTaskMRRegistrationRevisionChanged {
			t.Fatalf("concurrent restore failed unexpectedly: %v", result.err)
		}
		if result.changed {
			changedCount++
		}
	}
	stored, err := store.GetTaskMRIncludingDetached(ctx, "task-registration-race", "repo-registration", "group/project", 29)
	if err != nil || changedCount != 1 || stored == nil || stored.AssociationRevision != 3 || stored.DetachedAt != nil {
		t.Fatalf("concurrent restore result: changed=%d stored=%+v err=%v", changedCount, stored, err)
	}
}

func registrationTaskMR(taskID, repositoryID, providerRepositoryID string, iid int) *TaskMR {
	now := time.Now().UTC()
	return &TaskMR{
		TaskID: taskID, RepositoryID: repositoryID, Host: "https://gitlab.example.test",
		ProjectPath: "group/project", MRIID: iid, MRURL: "https://gitlab.example.test/group/project/-/merge_requests/9",
		MRTitle: "registration", HeadBranch: "feature", HeadSHA: "head", BaseBranch: "main",
		State: "opened", CreatedAt: now, UpdatedAt: now, Source: TaskMRSourceAgentRegistration,
		ProviderRepositoryID: providerRepositoryID, ConnectionScope: "builtin:workspace-registration",
		RepositoryPath: "group/project", ComparisonPolicy: TaskMRComparisonPolicyPreserve,
		AssociationRevision: 1, FirstOriginKind: "agent_registration", FirstOriginID: "registration-1",
	}
}
