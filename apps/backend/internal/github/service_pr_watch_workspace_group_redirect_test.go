package github

import (
	"context"
	"errors"
	"testing"
	"time"

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
