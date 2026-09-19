package github

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// multiTaskIssueStore is a TaskIssueStore fake keyed by task ID, unlike the
// single-task fakeTaskIssueStore in service_task_issue_test.go. F5's redirect
// tests need two distinct tasks resolvable at once: the observing task
// (validated before any redirect decision is made) and the workspace-group
// owner (validated by resolveEffectiveAssociationTaskID itself) — a
// single-task fake can't represent both without one lookup spuriously
// failing.
type multiTaskIssueStore struct {
	tasks map[string]*taskmodels.Task
	repos map[string][]*taskmodels.TaskRepository
}

func newMultiTaskIssueStore() *multiTaskIssueStore {
	return &multiTaskIssueStore{
		tasks: make(map[string]*taskmodels.Task),
		repos: make(map[string][]*taskmodels.TaskRepository),
	}
}

func (m *multiTaskIssueStore) addTask(task *taskmodels.Task, repositoryIDs ...string) {
	m.tasks[task.ID] = task
	repos := make([]*taskmodels.TaskRepository, 0, len(repositoryIDs))
	for _, repoID := range repositoryIDs {
		repos = append(repos, &taskmodels.TaskRepository{ID: "tr-" + task.ID + "-" + repoID, TaskID: task.ID, RepositoryID: repoID})
	}
	m.repos[task.ID] = repos
}

func (m *multiTaskIssueStore) GetTask(_ context.Context, taskID string) (*taskmodels.Task, error) {
	task, ok := m.tasks[taskID]
	if !ok {
		return nil, errors.New("task not found")
	}
	return task, nil
}

func (m *multiTaskIssueStore) ListTaskRepositories(_ context.Context, taskID string) ([]*taskmodels.TaskRepository, error) {
	task, ok := m.tasks[taskID]
	if !ok {
		return nil, errors.New("task not found")
	}
	return m.repos[task.ID], nil
}

func (m *multiTaskIssueStore) GetRepository(_ context.Context, _ string) (*taskmodels.Repository, error) {
	return nil, errors.New("not implemented")
}

func (m *multiTaskIssueStore) UpdateTaskMetadata(
	_ context.Context, _ string, _ map[string]interface{},
) (*taskmodels.Task, error) {
	return nil, errors.New("not implemented")
}

// fakeWorkspaceGroupOwnerResolver is a WorkspaceGroupOwnerResolver fake for
// resolveEffectiveAssociationTaskID's tests — a fixed answer (or error) is
// enough since the redirect logic itself, not the lookup, is under test.
type fakeWorkspaceGroupOwnerResolver struct {
	ownerTaskID string
	err         error
}

func (f *fakeWorkspaceGroupOwnerResolver) GetWorkspaceGroupOwnerTaskID(_ context.Context, _ string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.ownerTaskID, nil
}

// TestAssociatePRWithTask_RedirectsWatchSourcedToWorkspaceGroupOwner covers
// Review Round 2's F5 finding: a github_pr_watches row that already carries a
// member task's ID (whether written before this fix existed, or by any
// orchestrator producer a future change might miss) keeps feeding
// poller.go's AssociatePRWithTask(watch.TaskID, ...) forever, since nothing
// ever rewrites a watch's stored task_id. Redirecting again at this single
// private write funnel closes that gap independently of which caller or
// which stale row supplied taskID.
func TestAssociatePRWithTask_RedirectsWatchSourcedToWorkspaceGroupOwner(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)

	issueStore := newMultiTaskIssueStore()
	issueStore.addTask(&taskmodels.Task{ID: "owner1"}, "repo-canonical")
	svc.SetTaskIssueStore(issueStore)
	svc.SetWorkspaceGroupOwnerResolver(&fakeWorkspaceGroupOwnerResolver{ownerTaskID: "owner1"})

	ctx := context.Background()
	pr := &PR{
		Number: 42, RepoOwner: "org", RepoName: "repo",
		HTMLURL: "https://github.com/org/repo/pull/42",
	}

	tp, err := svc.AssociatePRWithTask(ctx, "member1", "repo-canonical", pr)
	if err != nil {
		t.Fatalf("AssociatePRWithTask: %v", err)
	}
	if tp.TaskID != "owner1" {
		t.Fatalf("TaskPR.TaskID = %q, want owner1 (the group owner)", tp.TaskID)
	}

	ownerRows, err := store.ListTaskPRsByTask(ctx, "owner1")
	if err != nil {
		t.Fatalf("list owner PR associations: %v", err)
	}
	if len(ownerRows) != 1 {
		t.Fatalf("got %d PR associations for owner1, want 1: %+v", len(ownerRows), ownerRows)
	}

	memberRows, err := store.ListTaskPRsByTask(ctx, "member1")
	if err != nil {
		t.Fatalf("list member PR associations: %v", err)
	}
	if len(memberRows) != 0 {
		t.Fatalf("got %d PR associations for member1, want 0 (redirected, not duplicated): %+v", len(memberRows), memberRows)
	}
}

// TestAssociatePRWithTask_FallsBackToObservingTask covers STEP 1b's
// fail-safe requirement for resolveEffectiveAssociationTaskID: every way the
// redirect can't be safely applied must leave the association under the
// observing task rather than silently dropping it (a redirect into a task
// validateTaskRepositoryID then rejects) or panicking on a missing
// dependency.
func TestAssociatePRWithTask_FallsBackToObservingTask(t *testing.T) {
	tests := []struct {
		name    string
		wire    func(svc *Service, issueStore *multiTaskIssueStore)
		wantErr bool
	}{
		{
			name: "no resolver wired",
			wire: func(_ *Service, issueStore *multiTaskIssueStore) {
				issueStore.addTask(&taskmodels.Task{ID: "member1"}, "repo-canonical")
			},
		},
		{
			name: "resolver errors",
			wire: func(svc *Service, issueStore *multiTaskIssueStore) {
				issueStore.addTask(&taskmodels.Task{ID: "member1"}, "repo-canonical")
				svc.SetWorkspaceGroupOwnerResolver(&fakeWorkspaceGroupOwnerResolver{err: errors.New("lookup failed")})
			},
		},
		{
			name: "owner task not found",
			wire: func(svc *Service, issueStore *multiTaskIssueStore) {
				// Only member1 is registered — GetTask("owner1") fails.
				issueStore.addTask(&taskmodels.Task{ID: "member1"}, "repo-canonical")
				svc.SetWorkspaceGroupOwnerResolver(&fakeWorkspaceGroupOwnerResolver{ownerTaskID: "owner1"})
			},
		},
		{
			name: "owner archived",
			wire: func(svc *Service, issueStore *multiTaskIssueStore) {
				issueStore.addTask(&taskmodels.Task{ID: "member1"}, "repo-canonical")
				archivedAt := time.Now().UTC()
				issueStore.addTask(&taskmodels.Task{ID: "owner1", ArchivedAt: &archivedAt}, "repo-canonical")
				svc.SetWorkspaceGroupOwnerResolver(&fakeWorkspaceGroupOwnerResolver{ownerTaskID: "owner1"})
			},
		},
		{
			name: "owner does not hold repository",
			wire: func(svc *Service, issueStore *multiTaskIssueStore) {
				issueStore.addTask(&taskmodels.Task{ID: "member1"}, "repo-canonical")
				issueStore.addTask(&taskmodels.Task{ID: "owner1"}, "repo-other")
				svc.SetWorkspaceGroupOwnerResolver(&fakeWorkspaceGroupOwnerResolver{ownerTaskID: "owner1"})
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, svc, _, store := setupPollerTest(t)
			issueStore := newMultiTaskIssueStore()
			tc.wire(svc, issueStore)
			svc.SetTaskIssueStore(issueStore)

			ctx := context.Background()
			pr := &PR{
				Number: 42, RepoOwner: "org", RepoName: "repo",
				HTMLURL: "https://github.com/org/repo/pull/42",
			}

			tp, err := svc.AssociatePRWithTask(ctx, "member1", "repo-canonical", pr)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("AssociatePRWithTask: want error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("AssociatePRWithTask: %v (association must not be silently dropped)", err)
			}
			if tp.TaskID != "member1" {
				t.Fatalf("TaskPR.TaskID = %q, want member1 (redirect unsafe, fall back to observing task)", tp.TaskID)
			}
			rows, err := store.ListTaskPRsByTask(ctx, "member1")
			if err != nil {
				t.Fatalf("list member PR associations: %v", err)
			}
			if len(rows) != 1 {
				t.Fatalf("got %d PR associations for member1, want 1: %+v", len(rows), rows)
			}
		})
	}
}

// TestAssociateExistingPRByURL_DoesNotRedirectURLLinkSource covers the F5
// scope boundary: TaskPRSourceURLLink (AssociateExistingPRByURL) is a
// deliberate cross-task action — the user explicitly chose to link a specific
// PR to the task whose card they were viewing — and must never be silently
// rerouted, even when a workspace-group redirect would otherwise apply for a
// watch-sourced write on the same task/repository pair. The resolver here
// points at a task the fake store has no record of, so if the guard were
// ever removed, the resulting redirect would surface as a hard error instead
// of a silent misattribution.
func TestAssociateExistingPRByURL_DoesNotRedirectURLLinkSource(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)

	issueStore := newMultiTaskIssueStore()
	issueStore.addTask(&taskmodels.Task{ID: "member1"}, "repo-canonical")
	// owner1 must hold repo-canonical too — otherwise resolveEffectiveAssociationTaskID
	// falls back to the observing task on its own (GetTask("owner1") errors), and the
	// no-redirect assertion below would pass even if the source==TaskPRSourceWatch
	// guard this test exists to cover were removed.
	issueStore.addTask(&taskmodels.Task{ID: "owner1"}, "repo-canonical")
	svc.SetTaskIssueStore(issueStore)
	svc.SetWorkspaceGroupOwnerResolver(&fakeWorkspaceGroupOwnerResolver{ownerTaskID: "owner1"})

	mockClient.AddPR(&PR{
		Number: 42, RepoOwner: "org", RepoName: "repo",
		HTMLURL: "https://github.com/org/repo/pull/42", HeadBranch: "feature-x", BaseBranch: "main",
	})

	ctx := context.Background()
	tp, err := svc.AssociateExistingPRByURL(ctx, "member1", "repo-canonical", "https://github.com/org/repo/pull/42")
	if err != nil {
		t.Fatalf("AssociateExistingPRByURL: %v", err)
	}
	if tp.TaskID != "member1" {
		t.Fatalf("TaskPR.TaskID = %q, want member1 (URL-link source must never redirect)", tp.TaskID)
	}

	rows, err := store.ListTaskPRsByTask(ctx, "member1")
	if err != nil {
		t.Fatalf("list member PR associations: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d PR associations for member1, want 1: %+v", len(rows), rows)
	}
}

// TestAssociatePRByURL_WatchAndAssociationAgreeOnRedirectedOwner covers
// Review Round 3's F-A finding: AssociatePRByURL created the PR watch under
// the raw, unredirected observing task, then associated under the
// workspace-group owner two lines later. createPRWatch is get-or-create and
// never re-points task_id on a hit, so the watch was permanently stuck under
// the member while the association correctly redirected to the owner. Every
// subsequent poller/batched sync call keys off watch.TaskID, which then
// resolved to zero rows for the owner and silently never synced. Both the
// watch and the association must land under the same (redirected) task.
func TestAssociatePRByURL_WatchAndAssociationAgreeOnRedirectedOwner(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)

	issueStore := newMultiTaskIssueStore()
	issueStore.addTask(&taskmodels.Task{ID: "member1"}, "repo-canonical")
	issueStore.addTask(&taskmodels.Task{ID: "owner1"}, "repo-canonical")
	svc.SetTaskIssueStore(issueStore)
	svc.SetWorkspaceGroupOwnerResolver(&fakeWorkspaceGroupOwnerResolver{ownerTaskID: "owner1"})

	mockClient.AddPR(&PR{
		Number: 42, RepoOwner: "org", RepoName: "repo",
		HTMLURL: "https://github.com/org/repo/pull/42", HeadBranch: "feature-x", BaseBranch: "main",
	})

	ctx := context.Background()
	svc.AssociatePRByURL(ctx, "session-1", "member1", "repo-canonical", "https://github.com/org/repo/pull/42", "")

	ownerAssociations, err := store.ListTaskPRsByTask(ctx, "owner1")
	if err != nil {
		t.Fatalf("list owner PR associations: %v", err)
	}
	if len(ownerAssociations) != 1 {
		t.Fatalf("got %d PR associations for owner1, want 1: %+v", len(ownerAssociations), ownerAssociations)
	}

	ownerWatches, err := store.ListPRWatchesByTask(ctx, "owner1")
	if err != nil {
		t.Fatalf("list owner PR watches: %v", err)
	}
	if len(ownerWatches) != 1 {
		t.Fatalf("got %d PR watches for owner1, want 1 (watch must be created under the same redirected task as the association): %+v", len(ownerWatches), ownerWatches)
	}

	memberWatches, err := store.ListPRWatchesByTask(ctx, "member1")
	if err != nil {
		t.Fatalf("list member PR watches: %v", err)
	}
	if len(memberWatches) != 0 {
		t.Fatalf("got %d PR watches for member1, want 0 (watch must not be stranded under the observing member task): %+v", len(memberWatches), memberWatches)
	}
}

// TestCheckSinglePRWatch_LegacyMemberWatchSyncsOwnersTaskPR covers a watch
// that already carries a member task's ID — either written before this fix
// existed, or (per apps/backend/CLAUDE.md's "PR status sync coverage") any
// numbered watch, since a watch that has found a PR is never re-pointed once
// created. Before this fix, poller.checkSinglePRWatch called
// SyncTaskPR(ctx, watch.TaskID, ...) directly, so a legacy member-attributed
// watch would look up the owner's github_task_prs row under the wrong task
// ID, find nothing, and silently no-op forever — freezing the owner's PR
// status at whatever it was when the redirect fix shipped. The watch itself
// must stay under the member (watches are never re-pointed); only the sync
// target is resolved.
func TestCheckSinglePRWatch_LegacyMemberWatchSyncsOwnersTaskPR(t *testing.T) {
	poller, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()

	issueStore := newMultiTaskIssueStore()
	issueStore.addTask(&taskmodels.Task{ID: "owner1"}, "repo-canonical")
	svc.SetTaskIssueStore(issueStore)
	svc.SetWorkspaceGroupOwnerResolver(&fakeWorkspaceGroupOwnerResolver{ownerTaskID: "owner1"})

	mockClient.AddPR(&PR{
		Number: 42, Title: "Feature PR", State: prStateMerged,
		HeadSHA: "abc123", HeadBranch: "feature-branch",
		RepoOwner: "owner", RepoName: "repo",
	})

	// The watch itself keeps the observing member's task_id — simulating
	// either a pre-fix row or one that predates the discovering session's own
	// group membership. Never re-pointed; see CreatePRWatch's get-or-create
	// semantics.
	watch := &PRWatch{
		SessionID: "sess-legacy", TaskID: "member1", RepositoryID: "repo-canonical",
		Owner: "owner", Repo: "repo", PRNumber: 42, Branch: "feature-branch",
	}
	if err := store.CreatePRWatch(ctx, withTestWorkspace(watch)); err != nil {
		t.Fatalf("create PR watch: %v", err)
	}

	// The association already correctly landed under the owner (e.g. via
	// AssociatePRByURL or CheckSessionPR's own redirect).
	if err := store.CreateTaskPR(ctx, &TaskPR{
		TaskID: "owner1", RepositoryID: "repo-canonical",
		Owner: "owner", Repo: "repo", PRNumber: 42,
		PRURL: "https://github.com/owner/repo/pull/42", PRTitle: "Feature PR",
		HeadBranch: "feature-branch", BaseBranch: "main", State: "open",
	}); err != nil {
		t.Fatalf("create task PR: %v", err)
	}

	poller.checkSinglePRWatch(ctx, watch)

	ownerTP, err := store.GetTaskPRByRepoAndNumber(ctx, "owner1", "repo-canonical", 42)
	if err != nil {
		t.Fatalf("get owner task PR: %v", err)
	}
	if ownerTP == nil {
		t.Fatal("expected owner1's task PR to still exist")
	}
	if ownerTP.State != prStateMerged {
		t.Errorf("owner1 TaskPR.State = %q, want %q (sync must land on the owner despite the watch's stale task_id)", ownerTP.State, prStateMerged)
	}
	if ownerTP.LastSyncedAt == nil {
		t.Error("expected owner1's TaskPR.LastSyncedAt to be set by the sync")
	}

	memberRows, err := store.ListTaskPRsByTask(ctx, "member1")
	if err != nil {
		t.Fatalf("list member PR associations: %v", err)
	}
	if len(memberRows) != 0 {
		t.Fatalf("got %d PR associations for member1, want 0 (sync must not create a stranded row under the observing member): %+v", len(memberRows), memberRows)
	}
}

func TestCheckSinglePRWatch_MovesLegacyMemberTaskPRToOwner(t *testing.T) {
	poller, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()

	issueStore := newMultiTaskIssueStore()
	issueStore.addTask(&taskmodels.Task{ID: "member1"}, "repo-canonical")
	issueStore.addTask(&taskmodels.Task{ID: "owner1"}, "repo-canonical")
	svc.SetTaskIssueStore(issueStore)
	svc.SetWorkspaceGroupOwnerResolver(&fakeWorkspaceGroupOwnerResolver{ownerTaskID: "owner1"})

	now := time.Now().UTC()
	mergedAt := now.Add(-time.Hour)
	mockClient.AddPR(&PR{
		Number: 42, Title: "Feature PR", State: prStateMerged,
		HeadSHA: "abc123", HeadBranch: "feature-branch", BaseBranch: "main",
		RepoOwner: "owner", RepoName: "repo", MergedAt: &mergedAt,
	})
	watch := &PRWatch{
		SessionID: "sess-legacy", TaskID: "member1", RepositoryID: "repo-canonical",
		Owner: "owner", Repo: "repo", PRNumber: 42, Branch: "feature-branch",
	}
	if err := store.CreatePRWatch(ctx, withTestWorkspace(watch)); err != nil {
		t.Fatalf("create PR watch: %v", err)
	}
	legacy := &TaskPR{
		TaskID: "member1", RepositoryID: "repo-canonical", Owner: "owner", Repo: "repo", PRNumber: 42,
		PRURL: "https://github.com/owner/repo/pull/42", PRTitle: "Feature PR",
		HeadBranch: "feature-branch", BaseBranch: "main", State: "open", CreatedAt: now.Add(-2 * time.Hour),
	}
	if err := store.CreateTaskPR(ctx, legacy); err != nil {
		t.Fatalf("create legacy member task PR: %v", err)
	}

	seen := make(chan *TaskPR, 2)
	sub, err := svc.eventBus.Subscribe(events.GitHubTaskPRUpdated, func(_ context.Context, event *bus.Event) error {
		if tp, ok := event.Data.(*TaskPR); ok {
			seen <- tp
		}
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe to task PR updates: %v", err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	poller.checkSinglePRWatch(ctx, watch)

	select {
	case eventPR := <-seen:
		if eventPR.TaskID != "owner1" {
			t.Fatalf("updated event task ID = %q, want owner1", eventPR.TaskID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no owner task PR update event published")
	}
	select {
	case eventPR := <-seen:
		t.Fatalf("unexpected second task PR update for %q", eventPR.TaskID)
	case <-time.After(100 * time.Millisecond):
	}

	ownerPR, err := store.GetTaskPRByRepoAndNumber(ctx, "owner1", "repo-canonical", 42)
	if err != nil {
		t.Fatalf("get owner task PR: %v", err)
	}
	if ownerPR == nil || ownerPR.State != prStateMerged {
		t.Fatalf("owner task PR = %+v, want merged owner row", ownerPR)
	}
	memberRows, err := store.ListTaskPRsByTask(ctx, "member1")
	if err != nil {
		t.Fatalf("list member task PRs: %v", err)
	}
	if len(memberRows) != 0 {
		t.Fatalf("member task PRs = %+v, want no active legacy member row", memberRows)
	}
}
