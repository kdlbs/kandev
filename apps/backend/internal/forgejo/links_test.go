package forgejo

import (
	"context"
	"errors"
	"testing"
)

func createLinkTestTask(t *testing.T, store *Store, workspaceID, taskID string) {
	t.Helper()
	if _, err := store.db.Exec(`CREATE TABLE IF NOT EXISTS tasks (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL)`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO tasks (id, workspace_id) VALUES (?, ?)`, taskID, workspaceID); err != nil {
		t.Fatalf("insert task: %v", err)
	}
}

func TestStore_TaskLinksAreScopedAndUpserted(t *testing.T) {
	store := newConfigTestStore(t)
	createLinkTestTask(t, store, "workspace-a", "task-a")
	ctx := context.Background()
	if err := store.UpsertTaskIssue(ctx, &TaskIssue{TaskID: "task-a", Origin: "https://forgejo.example", Owner: "owner", Repo: "repo", IssueNumber: 7, IssueURL: "https://forgejo.example/owner/repo/issues/7", Title: "Original", State: "open"}); err != nil {
		t.Fatalf("upsert issue: %v", err)
	}
	if err := store.UpsertTaskIssue(ctx, &TaskIssue{TaskID: "task-a", Origin: "https://forgejo.example", Owner: "owner", Repo: "repo", IssueNumber: 7, IssueURL: "https://forgejo.example/owner/repo/issues/7", Title: "Updated", State: "closed"}); err != nil {
		t.Fatalf("update issue: %v", err)
	}
	issues, err := store.ListTaskIssues(ctx, "workspace-a", "task-a")
	if err != nil || len(issues) != 1 || issues[0].Title != "Updated" || issues[0].State != "closed" {
		t.Fatalf("issues = %#v, %v", issues, err)
	}
	if _, err := store.ListTaskIssues(ctx, "workspace-b", "task-a"); !errors.Is(err, ErrTaskLinkNotFound) {
		t.Fatalf("cross-workspace error = %v, want ErrTaskLinkNotFound", err)
	}
}

func TestStore_TaskPRLinksAreScopedAndUpserted(t *testing.T) {
	store := newConfigTestStore(t)
	createLinkTestTask(t, store, "workspace-a", "task-a")
	ctx := context.Background()
	pr := &TaskPR{TaskID: "task-a", Origin: "https://forgejo.example", Owner: "owner", Repo: "repo", PRNumber: 8, PRURL: "https://forgejo.example/owner/repo/pulls/8", PRTitle: "Initial", HeadBranch: "feature/a", BaseBranch: "main", State: "open"}
	if err := store.UpsertTaskPR(ctx, pr); err != nil {
		t.Fatalf("upsert PR: %v", err)
	}
	pr.PRTitle, pr.State = "Updated", "merged"
	if err := store.UpsertTaskPR(ctx, pr); err != nil {
		t.Fatalf("update PR: %v", err)
	}
	prs, err := store.ListTaskPRs(ctx, "workspace-a", "task-a")
	if err != nil || len(prs) != 1 || prs[0].PRTitle != "Updated" || prs[0].State != "merged" {
		t.Fatalf("PRs = %#v, %v", prs, err)
	}
}

func TestStore_DeleteTaskLinksIsWorkspaceScoped(t *testing.T) {
	store := newConfigTestStore(t)
	createLinkTestTask(t, store, "workspace-a", "task-a")
	ctx := context.Background()
	issue := &TaskIssue{TaskID: "task-a", Origin: "https://forgejo.example", Owner: "owner", Repo: "repo", IssueNumber: 7, IssueURL: "url", Title: "Issue", State: "open"}
	if err := store.UpsertTaskIssue(ctx, issue); err != nil {
		t.Fatal(err)
	}
	links, err := store.ListTaskIssues(ctx, "workspace-a", "task-a")
	if err != nil || len(links) != 1 {
		t.Fatalf("links=%#v err=%v", links, err)
	}
	if err := store.DeleteTaskIssue(ctx, "workspace-b", links[0].ID); !errors.Is(err, ErrTaskLinkNotFound) {
		t.Fatalf("cross-workspace delete = %v", err)
	}
	if err := store.DeleteTaskIssue(ctx, "workspace-a", links[0].ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestStore_GetTaskLinkIsWorkspaceScoped(t *testing.T) {
	store := newConfigTestStore(t)
	createLinkTestTask(t, store, "workspace-a", "task-a")
	ctx := context.Background()
	if err := store.UpsertTaskPR(ctx, &TaskPR{TaskID: "task-a", Origin: "https://forgejo.example", Owner: "owner", Repo: "repo", PRNumber: 8, PRURL: "url", PRTitle: "PR", HeadBranch: "feature", BaseBranch: "main", State: "open"}); err != nil {
		t.Fatal(err)
	}
	links, err := store.ListTaskPRs(ctx, "workspace-a", "task-a")
	if err != nil || len(links) != 1 {
		t.Fatalf("links=%#v err=%v", links, err)
	}
	got, err := store.GetTaskPRLink(ctx, "workspace-a", links[0].ID)
	if err != nil || got.PRNumber != 8 {
		t.Fatalf("link=%#v err=%v", got, err)
	}
	if _, err := store.GetTaskPRLink(ctx, "workspace-b", links[0].ID); !errors.Is(err, ErrTaskLinkNotFound) {
		t.Fatalf("cross-workspace lookup=%v", err)
	}
}

func TestStore_IssueWatchIsWorkspaceScoped(t *testing.T) {
	store := newConfigTestStore(t)
	ctx := context.Background()
	watch := &IssueWatch{WorkspaceID: "workspace-a", Owner: "owner", Repo: "repo", Enabled: true}
	if err := store.UpsertIssueWatch(ctx, watch); err != nil {
		t.Fatal(err)
	}
	watches, err := store.ListIssueWatches(ctx, "workspace-a")
	if err != nil || len(watches) != 1 || watches[0].PollIntervalSeconds != 300 {
		t.Fatalf("watches=%#v err=%v", watches, err)
	}
	if err := store.DeleteIssueWatch(ctx, "workspace-b", watches[0].ID); !errors.Is(err, ErrWatchNotFound) {
		t.Fatalf("cross-workspace delete=%v", err)
	}
}

func TestFilterWatchedIssues(t *testing.T) {
	issues := []Issue{{Number: 1, Labels: []string{"bug"}}, {Number: 2, Labels: []string{"feature"}}, {Number: 3, Labels: []string{"bug", "urgent"}}}
	filtered := filterWatchedIssues(issues, "bug, urgent")
	if len(filtered) != 2 || filtered[0].Number != 1 || filtered[1].Number != 3 {
		t.Fatalf("filtered=%#v", filtered)
	}
	if all := filterWatchedIssues(issues, ""); len(all) != 3 {
		t.Fatalf("unfiltered=%#v", all)
	}
}

func TestStore_ClaimsWatchIssueOnce(t *testing.T) {
	store := newConfigTestStore(t)
	first, err := store.ClaimIssueWatchTask(context.Background(), "watch", "owner", "repo", 7, "task-a")
	if err != nil || !first {
		t.Fatalf("first=%t err=%v", first, err)
	}
	second, err := store.ClaimIssueWatchTask(context.Background(), "watch", "owner", "repo", 7, "task-b")
	if err != nil || second {
		t.Fatalf("second=%t err=%v", second, err)
	}
}
