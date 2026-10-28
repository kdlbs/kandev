package github

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestRegistrationCreatesObservationAndPreservesActiveAuthority(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.db.Exec(`INSERT INTO tasks (id, workspace_id) VALUES ('task-registration', 'ws-registration')`); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	registration := registrationTaskPR("task-registration", "repo-1", "100", 17)
	created, changed, err := store.RegisterTaskPRObservation(ctx, registration, false, 0)
	if err != nil {
		t.Fatalf("RegisterTaskPRObservation: %v", err)
	}
	if !changed || created == nil || created.ComparisonPolicy != TaskPRComparisonPolicyPreserve ||
		created.AssociationRevision != 1 || created.Source != TaskPRSourceAgentRegistration {
		t.Fatalf("created registration = %+v, changed=%v", created, changed)
	}
	options, err := store.GetTaskPRAutomationOptions(ctx, created.TaskID, created.RepositoryID, created.PRNumber)
	if err != nil {
		t.Fatalf("read automation defaults: %v", err)
	}
	if options.AutoFixEnabled || options.AutoMergeEnabled || options.PromptOnReviewRequested || options.PromptOnMerged || options.PromptOnClosed {
		t.Fatalf("registration enabled automation: %+v", options)
	}

	trueValue := true
	if _, err := store.UpdateTaskPRAutomationOptions(ctx, created.TaskID, created.RepositoryID, created.PRNumber,
		TaskPRAutomationOptionsPatch{AutoFixEnabled: &trueValue}, false); err != nil {
		t.Fatalf("enable existing authority: %v", err)
	}
	activeRetry := registrationTaskPR("task-registration", "repo-1", "100", 17)
	activeRetry.PRTitle = "new remote title"
	active, changed, err := store.RegisterTaskPRObservation(ctx, activeRetry, false, created.AssociationRevision)
	if err != nil {
		t.Fatalf("repeat registration: %v", err)
	}
	if changed || active.ID != created.ID || active.PRTitle != created.PRTitle {
		t.Fatalf("active registration retry changed committed state: %+v, changed=%v", active, changed)
	}
	options, err = store.GetTaskPRAutomationOptions(ctx, created.TaskID, created.RepositoryID, created.PRNumber)
	if err != nil || !options.AutoFixEnabled {
		t.Fatalf("repeat registration did not preserve existing authority: options=%+v err=%v", options, err)
	}

	detached, transitioned, err := store.DetachTaskPR(ctx, created.ID)
	if err != nil || !transitioned || detached.AssociationRevision != 2 {
		t.Fatalf("detach registration: row=%+v transitioned=%v err=%v", detached, transitioned, err)
	}
	if _, _, err := store.RegisterTaskPRObservation(ctx, registrationTaskPR("task-registration", "repo-1", "100", 17), true, created.AssociationRevision); err != ErrTaskPRRegistrationRevisionChanged {
		t.Fatalf("stale explicit restore error = %v, want revision changed", err)
	}
	if _, _, err := store.RegisterTaskPRObservation(ctx, registrationTaskPR("task-registration", "repo-1", "100", 17), false, detached.AssociationRevision); err != ErrTaskPRRegistrationDetached {
		t.Fatalf("implicit restore error = %v, want detached", err)
	}
	restored, changed, err := store.RegisterTaskPRObservation(ctx, registrationTaskPR("task-registration", "repo-1", "100", 17), true, detached.AssociationRevision)
	if err != nil || !changed || restored.AssociationRevision != 3 || restored.DetachedAt != nil || restored.ComparisonPolicy != TaskPRComparisonPolicyPreserve {
		t.Fatalf("explicit restore: row=%+v changed=%v err=%v", restored, changed, err)
	}
	options, err = store.GetTaskPRAutomationOptions(ctx, created.TaskID, created.RepositoryID, created.PRNumber)
	if err != nil || options.AutoFixEnabled || options.AutoMergeEnabled || options.PromptOnReviewRequested || options.PromptOnMerged || options.PromptOnClosed {
		t.Fatalf("restored registration inherited automation: options=%+v err=%v", options, err)
	}
}

func TestReplaceTaskPRReturnsPersistedAssociationRevision(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	created, err := store.ReplaceTaskPR(ctx, &TaskPR{
		ID: "task-pr-revision", TaskID: "task-pr-revision", RepositoryID: "repo-1",
		Owner: "acme", Repo: "demo", PRNumber: 17, State: "open", CreatedAt: time.Now().UTC(),
	}, &PRStatus{})
	if err != nil {
		t.Fatalf("create task PR: %v", err)
	}
	if created.AssociationRevision != 1 {
		t.Fatalf("created event revision = %d, want persisted default 1", created.AssociationRevision)
	}

	refreshed, err := store.ReplaceTaskPR(ctx, &TaskPR{
		ID: created.ID, TaskID: created.TaskID, RepositoryID: created.RepositoryID,
		Owner: created.Owner, Repo: created.Repo, PRNumber: created.PRNumber,
		State: "closed", CreatedAt: created.CreatedAt,
	}, &PRStatus{})
	if err != nil {
		t.Fatalf("refresh task PR: %v", err)
	}
	if refreshed.AssociationRevision != created.AssociationRevision {
		t.Fatalf("refreshed event revision = %d, want preserved revision %d", refreshed.AssociationRevision, created.AssociationRevision)
	}
	stored, err := store.GetTaskPRByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("read refreshed task PR: %v", err)
	}
	if stored.AssociationRevision != refreshed.AssociationRevision || stored.State != "closed" {
		t.Fatalf("stored task PR = %+v, want revision %d and closed state", stored, refreshed.AssociationRevision)
	}
}

func TestRegistrationLegacyIdentityRepair(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.db.Exec(`INSERT INTO tasks (id, workspace_id) VALUES ('task-legacy-registration', 'ws-registration')`); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	legacy := &TaskPR{
		ID: "legacy-association", TaskID: "task-legacy-registration", RepositoryID: "repo-1",
		Owner: "acme", Repo: "demo", PRNumber: 7, PRURL: "https://github.com/acme/demo/pull/7",
		CreatedAt: time.Now().UTC(),
	}
	if err := store.CreateTaskPR(ctx, legacy); err != nil {
		t.Fatalf("seed legacy association: %v", err)
	}
	registration := registrationTaskPR(legacy.TaskID, legacy.RepositoryID, "different-provider-id", legacy.PRNumber)
	if _, _, err := store.RegisterTaskPRObservation(ctx, registration, true, 1); err == nil {
		t.Fatal("registration repaired a legacy identity from the current path without verification")
	}
	stored, err := store.GetTaskPRByID(ctx, legacy.ID)
	if err != nil || stored == nil || stored.ProviderRepositoryID != "" {
		t.Fatalf("legacy association identity changed: stored=%+v err=%v", stored, err)
	}
}

func TestRegistrationImmutableIdentity(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.db.Exec(`INSERT INTO tasks (id, workspace_id) VALUES ('task-registration-canonical', 'ws-registration')`); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	first, changed, err := store.RegisterTaskPRObservation(ctx,
		registrationTaskPR("task-registration-canonical", "repo-old", "provider-repo-42", 17), false, 0)
	if err != nil || !changed {
		t.Fatalf("create initial association: row=%+v changed=%v err=%v", first, changed, err)
	}

	// A new local repository binding can point to the same immutable remote
	// repository after a rename or remove-and-reattach operation.
	rebound := registrationTaskPR("task-registration-canonical", "repo-new", "provider-repo-42", 17)
	rebound.Owner = "renamed"
	rebound.Repo = "demo"
	rebound.RepositoryPath = "renamed/demo"
	second, changed, err := store.RegisterTaskPRObservation(ctx, rebound, false, first.AssociationRevision)
	if err != nil {
		t.Fatalf("register through rebound repository: %v", err)
	}
	if changed || second == nil || second.ID != first.ID || second.RepositoryID != first.RepositoryID {
		t.Fatalf("canonical association was duplicated or rebound: first=%+v second=%+v changed=%v", first, second, changed)
	}
	var count int
	if err := store.db.Get(&count, `SELECT COUNT(*) FROM github_task_prs WHERE task_id = ? AND provider_repository_id = ? AND pr_number = ?`,
		"task-registration-canonical", "provider-repo-42", 17); err != nil || count != 1 {
		t.Fatalf("canonical association count = %d, err=%v, want 1", count, err)
	}
	if _, _, err := store.DetachTaskPR(ctx, first.ID); err != nil {
		t.Fatalf("detach original repository association: %v", err)
	}
	reusedPath := registrationTaskPR("task-registration-canonical", "repo-reused", "provider-repo-99", 17)
	reused, changed, err := store.RegisterTaskPRObservation(ctx, reusedPath, false, 0)
	if err != nil || !changed || reused.ID == first.ID || reused.DetachedAt != nil {
		t.Fatalf("path reuse inherited a different repository association: row=%+v changed=%v err=%v", reused, changed, err)
	}
}

func TestRegistrationConcurrentRestoreUsesRevisionFence(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.db.Exec(`INSERT INTO tasks (id, workspace_id) VALUES ('task-registration-race', 'ws-registration')`); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	created, changed, err := store.RegisterTaskPRObservation(ctx,
		registrationTaskPR("task-registration-race", "repo-registration-race", "provider-repo-race", 29), false, 0)
	if err != nil || !changed || created.AssociationRevision != 1 {
		t.Fatalf("create registration: row=%+v changed=%v err=%v", created, changed, err)
	}
	detached, changed, err := store.DetachTaskPR(ctx, created.ID)
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
			_, restored, restoreErr := store.RegisterTaskPRObservation(ctx,
				registrationTaskPR("task-registration-race", "repo-registration-race", "provider-repo-race", 29), true, 2)
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
		if result.err != nil && result.err != ErrTaskPRRegistrationRevisionChanged {
			t.Fatalf("concurrent restore failed unexpectedly: %v", result.err)
		}
		if result.changed {
			changedCount++
		}
	}
	stored, err := store.GetTaskPRByID(ctx, created.ID)
	if err != nil || changedCount != 1 || stored.AssociationRevision != 3 || stored.DetachedAt != nil {
		t.Fatalf("concurrent restore result: changed=%d stored=%+v err=%v", changedCount, stored, err)
	}
}

func registrationTaskPR(taskID, repositoryID, providerRepositoryID string, number int) *TaskPR {
	now := time.Now().UTC()
	return &TaskPR{
		WorkspaceID: "ws-registration", TaskID: taskID, RepositoryID: repositoryID,
		Owner: "acme", Repo: "demo", PRNumber: number,
		PRURL: "https://github.com/acme/demo/pull/" + strconv.Itoa(number), PRTitle: "registration",
		HeadBranch: "feature", BaseBranch: "main", HeadSHA: "head-sha", AuthorLogin: "agent",
		State: "open", CreatedAt: now, Source: TaskPRSourceAgentRegistration,
		ProviderHost: "https://github.com", ConnectionScope: "builtin:ws-registration",
		ProviderRepositoryID: providerRepositoryID, RepositoryPath: "acme/demo",
		ComparisonPolicy: TaskPRComparisonPolicyPreserve, AssociationRevision: 1,
	}
}
