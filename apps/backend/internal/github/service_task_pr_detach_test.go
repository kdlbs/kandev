package github

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestServiceDetachTaskPRAuthorizesWorkspaceAndPublishesRemoval(t *testing.T) {
	svc, store, eventBus := setupSyncTest(t)
	contract, ok := any(svc).(interface {
		DetachTaskPR(context.Context, string, string) (*TaskPR, error)
	})
	if !ok {
		t.Fatal("Service does not implement DetachTaskPR")
	}
	ctx := context.Background()
	first := &TaskPR{
		WorkspaceID: "ws-1", TaskID: "task-1", RepositoryID: "repo-1", Owner: "acme", Repo: "demo",
		PRNumber: 1, PRURL: "https://github.com/acme/demo/pull/1", PRTitle: "old", HeadBranch: "old",
		BaseBranch: "main", State: "merged", CreatedAt: time.Now().UTC(),
	}
	second := &TaskPR{
		WorkspaceID: "ws-1", TaskID: "task-1", RepositoryID: "repo-1", Owner: "acme", Repo: "demo",
		PRNumber: 2, PRURL: "https://github.com/acme/demo/pull/2", PRTitle: "new", HeadBranch: "new",
		BaseBranch: "main", State: "open", CreatedAt: time.Now().UTC().Add(time.Second),
	}
	if err := store.CreateTaskPR(ctx, first); err != nil {
		t.Fatalf("create first PR: %v", err)
	}
	if err := store.CreateTaskPR(ctx, second); err != nil {
		t.Fatalf("create second PR: %v", err)
	}
	var authorizedWorkspace string
	svc.SetWorkspaceAuthorizer(func(_ context.Context, workspaceID string) error {
		authorizedWorkspace = workspaceID
		return nil
	})

	detached, err := contract.DetachTaskPR(ctx, "ws-1", first.ID)
	if err != nil {
		t.Fatalf("detach task PR: %v", err)
	}
	if detached == nil || detached.ID != first.ID || detached.DetachedAt == nil {
		t.Fatalf("detached = %+v, want stamped first row", detached)
	}
	if authorizedWorkspace != "ws-1" {
		t.Fatalf("authorized workspace = %q, want ws-1", authorizedWorkspace)
	}
	active, err := store.ListTaskPRsByTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("list active task PRs: %v", err)
	}
	if len(active) != 1 || active[0].ID != second.ID {
		t.Fatalf("active task PRs = %+v, want sibling only", active)
	}
	if eventBus.publishedCount() != 1 {
		t.Fatalf("published events = %d, want one deletion event", eventBus.publishedCount())
	}
	eventBus.mu.Lock()
	event := eventBus.events[0]
	eventBus.mu.Unlock()
	if event.Type != "github.task_pr.deleted" {
		t.Fatalf("event type = %q, want github.task_pr.deleted", event.Type)
	}
	payload := reflect.ValueOf(event.Data)
	if payload.Kind() == reflect.Pointer {
		payload = payload.Elem()
	}
	if !payload.IsValid() || payload.FieldByName("AssociationID").String() != first.ID || payload.FieldByName("TaskID").String() != first.TaskID || payload.FieldByName("WorkspaceID").String() != first.WorkspaceID {
		t.Fatalf("event payload = %#v, want first association identity", event.Data)
	}
}

func TestServiceDetachTaskPRFailsClosedForWorkspaceMismatch(t *testing.T) {
	svc, store, _ := setupSyncTest(t)
	contract, ok := any(svc).(interface {
		DetachTaskPR(context.Context, string, string) (*TaskPR, error)
	})
	if !ok {
		t.Fatal("Service does not implement DetachTaskPR")
	}
	pr := &TaskPR{WorkspaceID: "ws-1", TaskID: "task-1", Owner: "acme", Repo: "demo", PRNumber: 1, CreatedAt: time.Now().UTC()}
	if err := store.CreateTaskPR(context.Background(), pr); err != nil {
		t.Fatalf("create PR: %v", err)
	}

	_, err := contract.DetachTaskPR(context.Background(), "ws-other", pr.ID)
	if err == nil || err.Error() != "github task PR not found" {
		t.Fatalf("error = %v, want github task PR not found", err)
	}
	active, err := store.ListTaskPRsByTask(context.Background(), pr.TaskID)
	if err != nil {
		t.Fatalf("list task PRs: %v", err)
	}
	if len(active) != 1 || active[0].ID != pr.ID {
		t.Fatalf("active task PRs = %+v, want association unchanged", active)
	}
}

func TestServiceDetachTaskPRAllowsLegacyEmptyWorkspace(t *testing.T) {
	svc, store, eventBus := setupSyncTest(t)
	ctx := context.Background()
	pr := &TaskPR{
		TaskID: "task-legacy", Owner: "acme", Repo: "demo", PRNumber: 7,
		PRURL: "https://github.com/acme/demo/pull/7", CreatedAt: time.Now().UTC(),
	}
	if err := store.CreateTaskPR(ctx, pr); err != nil {
		t.Fatalf("create legacy PR: %v", err)
	}
	svc.SetTaskIssueStore(&fakeTaskIssueStore{
		task: &taskmodels.Task{ID: "task-legacy", WorkspaceID: "ws-legacy"},
	})
	var authorizedWorkspace string
	svc.SetWorkspaceAuthorizer(func(_ context.Context, workspaceID string) error {
		authorizedWorkspace = workspaceID
		return nil
	})

	detached, err := svc.DetachTaskPR(ctx, "ws-legacy", pr.ID)
	if err != nil {
		t.Fatalf("detach legacy PR: %v", err)
	}
	if detached == nil || detached.DetachedAt == nil {
		t.Fatalf("detached legacy PR = %+v, want stamped row", detached)
	}
	if authorizedWorkspace != "ws-legacy" {
		t.Fatalf("authorized workspace = %q, want ws-legacy", authorizedWorkspace)
	}
	if eventBus.publishedCount() != 1 {
		t.Fatalf("published events = %d, want one deletion event", eventBus.publishedCount())
	}
	eventBus.mu.Lock()
	event := eventBus.events[0]
	eventBus.mu.Unlock()
	payload := reflect.ValueOf(event.Data)
	if payload.Kind() == reflect.Pointer {
		payload = payload.Elem()
	}
	if payload.FieldByName("WorkspaceID").String() != "ws-legacy" {
		t.Fatalf("event workspace = %q, want ws-legacy", payload.FieldByName("WorkspaceID").String())
	}
}

func TestServiceDetachTaskPRLegacyWorkspaceMismatchFailsClosed(t *testing.T) {
	svc, store, eventBus := setupSyncTest(t)
	ctx := context.Background()
	pr := &TaskPR{
		TaskID: "task-legacy", Owner: "acme", Repo: "demo", PRNumber: 8,
		PRURL: "https://github.com/acme/demo/pull/8", CreatedAt: time.Now().UTC(),
	}
	if err := store.CreateTaskPR(ctx, pr); err != nil {
		t.Fatalf("create legacy PR: %v", err)
	}
	svc.SetTaskIssueStore(&fakeTaskIssueStore{
		task: &taskmodels.Task{ID: "task-legacy", WorkspaceID: "ws-owner"},
	})
	authorizerCalls := 0
	svc.SetWorkspaceAuthorizer(func(context.Context, string) error {
		authorizerCalls++
		return nil
	})

	_, err := svc.DetachTaskPR(ctx, "ws-other", pr.ID)
	if !errors.Is(err, ErrTaskPRNotFound) {
		t.Fatalf("error = %v, want ErrTaskPRNotFound", err)
	}
	stored, err := store.GetTaskPRByID(ctx, pr.ID)
	if err != nil {
		t.Fatalf("reload legacy PR: %v", err)
	}
	if stored == nil || stored.DetachedAt != nil {
		t.Fatalf("stored legacy PR = %+v, want active association", stored)
	}
	if authorizerCalls != 0 {
		t.Fatalf("workspace authorizer calls = %d, want 0 on mismatch", authorizerCalls)
	}
	if eventBus.publishedCount() != 0 {
		t.Fatalf("published events = %d, want 0 on rejected detach", eventBus.publishedCount())
	}
}

func TestServiceDetachTaskPRLegacyWorkspaceRequiresTaskResolver(t *testing.T) {
	svc, store, eventBus := setupSyncTest(t)
	ctx := context.Background()
	pr := &TaskPR{
		TaskID: "task-legacy", Owner: "acme", Repo: "demo", PRNumber: 9,
		PRURL: "https://github.com/acme/demo/pull/9", CreatedAt: time.Now().UTC(),
	}
	if err := store.CreateTaskPR(ctx, pr); err != nil {
		t.Fatalf("create legacy PR: %v", err)
	}
	authorizerCalls := 0
	svc.SetWorkspaceAuthorizer(func(context.Context, string) error {
		authorizerCalls++
		return nil
	})

	_, err := svc.DetachTaskPR(ctx, "ws-supplied", pr.ID)
	if !errors.Is(err, ErrTaskPRNotFound) {
		t.Fatalf("error = %v, want ErrTaskPRNotFound", err)
	}
	stored, err := store.GetTaskPRByID(ctx, pr.ID)
	if err != nil {
		t.Fatalf("reload legacy PR: %v", err)
	}
	if stored == nil || stored.DetachedAt != nil {
		t.Fatalf("stored legacy PR = %+v, want active association", stored)
	}
	if authorizerCalls != 0 {
		t.Fatalf("workspace authorizer calls = %d, want 0 without resolver", authorizerCalls)
	}
	if eventBus.publishedCount() != 0 {
		t.Fatalf("published events = %d, want 0 on rejected detach", eventBus.publishedCount())
	}
}

func TestAssociatePRWithTaskDoesNotResurrectDetachedAssociation(t *testing.T) {
	svc, store, _ := setupSyncTest(t)
	ctx := context.Background()
	row := &TaskPR{
		WorkspaceID: "ws-1", TaskID: "task-1", RepositoryID: "repo-1", Owner: "acme", Repo: "demo",
		PRNumber: 3, PRURL: "https://github.com/acme/demo/pull/3", PRTitle: "old", HeadBranch: "old",
		BaseBranch: "main", State: "open", CreatedAt: time.Now().UTC(),
	}
	if err := store.CreateTaskPR(ctx, row); err != nil {
		t.Fatalf("create PR: %v", err)
	}
	if _, _, err := store.DetachTaskPR(ctx, row.ID); err != nil {
		t.Fatalf("detach PR: %v", err)
	}

	associated, err := svc.AssociatePRWithTask(ctx, row.TaskID, row.RepositoryID, &PR{
		Number: 3, RepoOwner: "acme", RepoName: "demo", HTMLURL: row.PRURL, Title: "old", HeadBranch: "old", BaseBranch: "main", State: "open",
	})
	if err != nil {
		t.Fatalf("associate PR: %v", err)
	}
	if associated == nil || associated.DetachedAt == nil {
		t.Fatalf("associated = %+v, want detached tombstone to remain", associated)
	}
	active, err := store.ListTaskPRsByTask(ctx, row.TaskID)
	if err != nil {
		t.Fatalf("list active PRs: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("active PRs = %+v, want no resurrected row", active)
	}
}

func TestAssociatePRWithTaskExplicitLinkRestoresDetachedAssociation(t *testing.T) {
	svc, store, eventBus := setupSyncTest(t)
	ctx := context.Background()
	row := &TaskPR{
		WorkspaceID: "ws-1", TaskID: "task-1", RepositoryID: "repo-1", Owner: "acme", Repo: "demo",
		PRNumber: 4, PRURL: "https://github.com/acme/demo/pull/4", PRTitle: "old", HeadBranch: "old",
		BaseBranch: "main", State: "open", CreatedAt: time.Now().UTC(),
	}
	if err := store.CreateTaskPR(ctx, row); err != nil {
		t.Fatalf("create PR: %v", err)
	}
	if _, _, err := store.DetachTaskPR(ctx, row.ID); err != nil {
		t.Fatalf("detach PR: %v", err)
	}

	freshPR := &PR{
		Number: 4, RepoOwner: "acme", RepoName: "demo", HTMLURL: "https://github.com/acme/demo/pull/4",
		Title: "fresh title", HeadBranch: "fresh-branch", BaseBranch: "develop", AuthorLogin: "bob",
		State: "closed", MergeableState: "dirty", Additions: 8, Deletions: 3,
	}
	associated, err := svc.associatePRWithTask(ctx, row.WorkspaceID, row.TaskID, row.RepositoryID, freshPR, true, true)
	if err != nil {
		t.Fatalf("explicitly associate PR: %v", err)
	}
	if associated == nil || associated.DetachedAt != nil {
		t.Fatalf("associated = %+v, want active restored row", associated)
	}
	if associated.PRURL != freshPR.HTMLURL || associated.PRTitle != freshPR.Title ||
		associated.HeadBranch != freshPR.HeadBranch || associated.BaseBranch != freshPR.BaseBranch ||
		associated.AuthorLogin != freshPR.AuthorLogin || associated.State != freshPR.State ||
		associated.MergeableState != freshPR.MergeableState || associated.Additions != freshPR.Additions ||
		associated.Deletions != freshPR.Deletions {
		t.Fatalf("restored PR = %+v, want fresh GitHub data", associated)
	}
	active, err := store.ListTaskPRsByTask(ctx, row.TaskID)
	if err != nil {
		t.Fatalf("list active PRs: %v", err)
	}
	if len(active) != 1 || active[0].ID != row.ID {
		t.Fatalf("active PRs = %+v, want restored row", active)
	}
	if eventBus.publishedCount() != 1 {
		t.Fatalf("published events = %d, want one restoration update", eventBus.publishedCount())
	}
}

// TestAssociatePRWithTaskExplicitLinkOnAlreadyMergedDetachedPRPopulatesOutcomeFields
// is the detached -> merged -> relink regression: a PR that was detached
// while open, then merged upstream while nobody was watching, then relinked
// via the explicit URL flow (a GetPR-based, populating fetch) must not
// surface with merged_at set and merged_by_login left NULL from before the
// detach (AC-36). RestoreTaskPR previously updated only lifecycle/display
// columns and left the five outcome fields untouched.
func TestAssociatePRWithTaskExplicitLinkOnAlreadyMergedDetachedPRPopulatesOutcomeFields(t *testing.T) {
	svc, store, _ := setupSyncTest(t)
	ctx := context.Background()
	row := &TaskPR{
		WorkspaceID: "ws-1", TaskID: "task-1", RepositoryID: "repo-1", Owner: "acme", Repo: "demo",
		PRNumber: 5, PRURL: "https://github.com/acme/demo/pull/5", PRTitle: "old", HeadBranch: "old",
		BaseBranch: "main", State: "open", CreatedAt: time.Now().UTC(),
	}
	if err := store.CreateTaskPR(ctx, row); err != nil {
		t.Fatalf("create PR: %v", err)
	}
	if _, _, err := store.DetachTaskPR(ctx, row.ID); err != nil {
		t.Fatalf("detach PR: %v", err)
	}

	mergedAt := time.Now().UTC()
	mergedPR := &PR{
		Number: 5, RepoOwner: "acme", RepoName: "demo", HTMLURL: "https://github.com/acme/demo/pull/5",
		Title: "merged while detached", HeadBranch: "old", BaseBranch: "main", AuthorLogin: "bob",
		State: prStateMerged, MergedAt: &mergedAt,
		Draft: false, ChangedFiles: 12, MergedByLogin: "carlosflorencio",
	}
	associated, err := svc.associatePRWithTask(ctx, row.WorkspaceID, row.TaskID, row.RepositoryID, mergedPR, true, true)
	if err != nil {
		t.Fatalf("explicitly associate PR: %v", err)
	}
	if associated == nil || associated.MergedAt == nil {
		t.Fatalf("associated = %+v, want merged with a timestamp", associated)
	}
	if associated.MergedByLogin == nil || *associated.MergedByLogin != "carlosflorencio" {
		t.Errorf("merged_by_login = %v, want %q (AC-36 requires non-NULL once merged_at is set)",
			associated.MergedByLogin, "carlosflorencio")
	}
	if associated.IsDraft == nil || *associated.IsDraft != false {
		t.Errorf("is_draft = %v, want false observed, not NULL", associated.IsDraft)
	}
	if associated.ChangedFiles == nil || *associated.ChangedFiles != 12 {
		t.Errorf("changed_files = %v, want 12 observed, not NULL", associated.ChangedFiles)
	}
}
