package backendapp

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/github"
)

func newStatusSummaryTestStore(t *testing.T) *github.Store {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "github.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	if _, err := sqlxDB.Exec(`CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT, archived_at DATETIME)`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	if _, err := sqlxDB.Exec(`CREATE TABLE workspaces (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("create workspaces table: %v", err)
	}
	store, err := github.NewStore(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store
}

func TestTaskStatusSummaryPRKeyMatchesLiveEventIdentity(t *testing.T) {
	tests := []struct {
		name string
		pr   *github.TaskPR
		want string
	}{
		{
			name: "repository and number",
			pr:   &github.TaskPR{ID: "association-1", RepositoryID: "repo-a", PRNumber: 42, PRURL: "https://example.test/42"},
			want: "repo-a#42",
		},
		{
			name: "legacy URL",
			pr:   &github.TaskPR{ID: "association-2", PRNumber: 42, PRURL: "https://example.test/42"},
			want: "https://example.test/42",
		},
		{
			name: "repository only",
			pr:   &github.TaskPR{ID: "association-3", RepositoryID: "repo-a"},
			want: "repo-a",
		},
		{
			name: "number only",
			pr:   &github.TaskPR{ID: "association-4", PRNumber: 42},
			want: "#42",
		},
		{
			name: "association without source identity",
			pr:   &github.TaskPR{ID: "association-5"},
			want: "",
		},
		{
			name: "nil pull request",
			want: "",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := taskStatusSummaryPRKey(test.pr); got != test.want {
				t.Fatalf("taskStatusSummaryPRKey() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestTaskStatusRuntimeProvidersTreatNilOrchestratorAsAbsent(t *testing.T) {
	activityProvider, countQueuedPrompts := taskStatusRuntimeProviders(nil)
	if activityProvider != nil {
		t.Fatalf("activity provider = %#v, want nil", activityProvider)
	}
	if countQueuedPrompts != nil {
		t.Fatal("queued prompt counter should be nil without an orchestrator")
	}
}

func TestGitHubTaskStatusSummaryPRReaderPreservesMergeQueueState(t *testing.T) {
	ctx := context.Background()
	store := newStatusSummaryTestStore(t)
	queuePR := &github.TaskPR{
		TaskID: "task-queue-summary", RepositoryID: "repo-queue-summary", PRNumber: 42,
		PRURL: "https://example.test/42", State: "open", CreatedAt: time.Now().UTC(),
		MergeQueueState: "queued",
	}
	if err := store.CreateTaskPR(ctx, queuePR); err != nil {
		t.Fatalf("CreateTaskPR: %v", err)
	}

	reader := &githubTaskStatusSummaryPRReader{
		gh: github.NewService(nil, "", nil, store, nil, nil),
	}
	result, err := reader.ListTaskStatusSummaryPullRequests(ctx, []string{queuePR.TaskID})
	if err != nil {
		t.Fatalf("ListTaskStatusSummaryPullRequests: %v", err)
	}
	inputs := result[queuePR.TaskID]
	if len(inputs) != 1 {
		t.Fatalf("summary inputs = %+v, want one input", inputs)
	}
	if inputs[0].MergeQueueState != queuePR.MergeQueueState {
		t.Fatalf("MergeQueueState = %q, want %q", inputs[0].MergeQueueState, queuePR.MergeQueueState)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.10
func TestGitHubTaskStatusSummaryPRReaderIncludesPerPRAutomationFlags(t *testing.T) {
	ctx := context.Background()
	store := newStatusSummaryTestStore(t)
	queuePR := &github.TaskPR{
		TaskID: "task-automation-summary", RepositoryID: "repo-automation-summary", PRNumber: 42,
		PRURL: "https://example.test/42", State: "open", CreatedAt: time.Now().UTC(),
	}
	if err := store.CreateTaskPR(ctx, queuePR); err != nil {
		t.Fatalf("CreateTaskPR: %v", err)
	}
	autoFix := true
	autoMerge := true
	if _, err := store.UpdateTaskPRAutomationOptions(ctx, queuePR.TaskID, queuePR.RepositoryID, queuePR.PRNumber,
		github.TaskPRAutomationOptionsPatch{AutoFixEnabled: &autoFix, AutoMergeEnabled: &autoMerge}, false,
	); err != nil {
		t.Fatalf("UpdateTaskPRAutomationOptions: %v", err)
	}

	reader := &githubTaskStatusSummaryPRReader{
		gh: github.NewService(nil, "", nil, store, nil, nil),
	}
	result, err := reader.ListTaskStatusSummaryPullRequests(ctx, []string{queuePR.TaskID})
	if err != nil {
		t.Fatalf("ListTaskStatusSummaryPullRequests: %v", err)
	}
	inputs := result[queuePR.TaskID]
	if len(inputs) != 1 {
		t.Fatalf("summary inputs = %+v, want one input", inputs)
	}
	if !inputs[0].AutoFixEnabled || !inputs[0].AutoMergeEnabled {
		t.Fatalf("automation flags = %+v, want both enabled", inputs[0])
	}
}
