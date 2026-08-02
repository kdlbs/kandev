package backendapp

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestIsAbandonedTurnCompletion(t *testing.T) {
	startedAt := time.Date(2026, time.July, 24, 10, 0, 0, 0, time.UTC)
	if !isAbandonedTurnCompletion(map[string]interface{}{"started_at": startedAt, "completed_at": &startedAt}) {
		t.Fatal("an orphan turn completed at its start time must not notify")
	}
	completedAt := startedAt.Add(time.Second)
	if isAbandonedTurnCompletion(map[string]interface{}{"started_at": startedAt, "completed_at": &completedAt}) {
		t.Fatal("a live turn completion must remain eligible for notifications")
	}
}

func TestClarificationNotificationOccurrence(t *testing.T) {
	tests := []struct {
		name          string
		data          map[string]interface{}
		wantTaskID    string
		wantSessionID string
		wantPendingID string
		wantEligible  bool
	}{
		{
			name: "structured clarification requesting input",
			data: map[string]interface{}{
				"type": "clarification_request", "requests_input": true, "author_type": "agent",
				"task_id": "task-1", "session_id": "session-1", "metadata": map[string]interface{}{"pending_id": "pending-1"},
			},
			wantTaskID: "task-1", wantSessionID: "session-1", wantPendingID: "pending-1", wantEligible: true,
		},
		{name: "ordinary message", data: map[string]interface{}{"type": "message", "requests_input": true}, wantEligible: false},
		{name: "clarification without input request", data: map[string]interface{}{"type": "clarification_request", "requests_input": false}, wantEligible: false},
		{name: "missing metadata", data: map[string]interface{}{"type": "clarification_request", "requests_input": true}, wantEligible: false},
		{name: "wrong metadata type", data: map[string]interface{}{"type": "clarification_request", "requests_input": true, "metadata": "pending-1"}, wantEligible: false},
		{name: "empty pending ID", data: map[string]interface{}{"type": "clarification_request", "requests_input": true, "metadata": map[string]interface{}{"pending_id": ""}}, wantEligible: false},
		{name: "user authored clarification", data: map[string]interface{}{"type": "clarification_request", "requests_input": true, "author_type": "user", "task_id": "task-1", "session_id": "session-1", "metadata": map[string]interface{}{"pending_id": "pending-1"}}, wantEligible: false},
		{name: "system authored clarification", data: map[string]interface{}{"type": "clarification_request", "requests_input": true, "author_type": "system", "task_id": "task-1", "session_id": "session-1", "metadata": map[string]interface{}{"pending_id": "pending-1"}}, wantEligible: false},
		{name: "missing task ID", data: map[string]interface{}{"type": "clarification_request", "requests_input": true, "author_type": "agent", "session_id": "session-1", "metadata": map[string]interface{}{"pending_id": "pending-1"}}, wantEligible: false},
		{name: "missing session ID", data: map[string]interface{}{"type": "clarification_request", "requests_input": true, "author_type": "agent", "task_id": "task-1", "metadata": map[string]interface{}{"pending_id": "pending-1"}}, wantEligible: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taskID, sessionID, pendingID, eligible := clarificationNotificationOccurrence(tt.data)
			if taskID != tt.wantTaskID || sessionID != tt.wantSessionID || pendingID != tt.wantPendingID || eligible != tt.wantEligible {
				t.Fatalf("occurrence = (%q, %q, %q, %v), want (%q, %q, %q, %v)", taskID, sessionID, pendingID, eligible, tt.wantTaskID, tt.wantSessionID, tt.wantPendingID, tt.wantEligible)
			}
		})
	}
}

func TestCreatedChangeAssociationRouterRoutesGitLabSingleAndMultiRepo(t *testing.T) {
	var got []string
	router := createdChangeAssociationRouter{
		resolveRepositoryID: func(_ context.Context, _ string, repo string) string {
			if repo == "" {
				return "repo-primary"
			}
			return "repo-" + repo
		},
		resolveWorkspaceID: func(context.Context, string) (string, error) { return "workspace-1", nil },
		associateGitLab: func(_ context.Context, workspaceID, sessionID, taskID, repositoryID, mrURL string) error {
			got = append(got, workspaceID+"|"+sessionID+"|"+taskID+"|"+repositoryID+"|"+mrURL)
			return nil
		},
	}
	for _, repo := range []string{"", "secondary"} {
		if err := router.associate(context.Background(), "session-1", "task-1", "gitlab", "https://gitlab.example/g/r/-/merge_requests/4", "feature", repo); err != nil {
			t.Fatalf("associate repo %q: %v", repo, err)
		}
	}
	want := []string{
		"workspace-1|session-1|task-1|repo-primary|https://gitlab.example/g/r/-/merge_requests/4",
		"workspace-1|session-1|task-1|repo-secondary|https://gitlab.example/g/r/-/merge_requests/4",
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("associations = %#v", got)
	}
}

func TestCreatedChangeAssociationRouterPreservesGitHubAndMissingServices(t *testing.T) {
	var got string
	router := createdChangeAssociationRouter{
		resolveRepositoryID: func(context.Context, string, string) string { return "repo-1" },
		resolveWorkspaceID:  func(context.Context, string) (string, error) { return "workspace-1", nil },
		associateGitHub: func(
			_ context.Context, workspaceID, sessionID, taskID, repositoryID, prURL, branch string,
		) error {
			got = workspaceID + "|" + sessionID + "|" + taskID + "|" + repositoryID + "|" + prURL + "|" + branch
			return nil
		},
	}
	if err := router.associate(context.Background(), "session-1", "task-1", "github", "https://github.com/g/r/pull/2", "feature", ""); err != nil {
		t.Fatal(err)
	}
	if got != "workspace-1|session-1|task-1|repo-1|https://github.com/g/r/pull/2|feature" {
		t.Fatalf("GitHub association = %q", got)
	}
	if err := router.associate(context.Background(), "session-1", "task-1", "gitlab", "url", "feature", ""); err != nil {
		t.Fatalf("missing GitLab service should be a no-op: %v", err)
	}
}

func TestCreatedChangeAssociationRouterGitLabFailureCanRetry(t *testing.T) {
	attempts := 0
	router := createdChangeAssociationRouter{
		resolveRepositoryID: func(context.Context, string, string) string { return "repo-1" },
		resolveWorkspaceID:  func(context.Context, string) (string, error) { return "workspace-1", nil },
		associateGitLab: func(context.Context, string, string, string, string, string) error {
			attempts++
			if attempts == 1 {
				return errors.New("temporary failure containing sensitive provider output")
			}
			return nil
		},
	}
	if err := router.associate(context.Background(), "session-1", "task-1", "gitlab", "url", "feature", ""); err == nil {
		t.Fatal("first association unexpectedly succeeded")
	}
	if err := router.associate(context.Background(), "session-1", "task-1", "gitlab", "url", "feature", ""); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d", attempts)
	}
}
