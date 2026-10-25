package linear

import (
	"context"
	"errors"
	"testing"
)

type pluginWritebackClient struct {
	*fakeClient
	commentID    string
	commentIDArg string
	comment      string
	commentErr   error
}

func (c *pluginWritebackClient) AddComment(_ context.Context, issueID, body string) (string, error) {
	c.commentIDArg = issueID
	c.comment = body
	return c.commentID, c.commentErr
}

func TestPluginWriteback(t *testing.T) {
	ctx := context.Background()
	fixture := newSvcFixture(t)
	const workspaceID = "workspace-plugin-writeback"
	if err := fixture.store.UpsertConfigForWorkspace(ctx, workspaceID, &LinearConfig{
		AuthMethod: AuthMethodAPIKey, DefaultTeamKey: "ENG",
	}); err != nil {
		t.Fatalf("seed workspace config: %v", err)
	}
	if err := fixture.secrets.Set(ctx, SecretKeyForWorkspace(workspaceID), "Linear API key", "workspace-secret"); err != nil {
		t.Fatalf("seed workspace secret: %v", err)
	}
	if _, err := fixture.store.db.ExecContext(ctx, `
		INSERT INTO linear_issue_watches (id, workspace_id, workflow_id, workflow_step_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		"watch-plugin-writeback", workspaceID, "workflow-1", "step-1"); err != nil {
		t.Fatalf("seed Linear issue watch: %v", err)
	}
	if _, err := fixture.store.ReserveIssueWatchTask(ctx, "watch-plugin-writeback", "ENG-12", "https://linear.app/acme/issue/ENG-12"); err != nil {
		t.Fatalf("reserve Linear source link: %v", err)
	}
	if err := fixture.store.AssignIssueWatchTaskID(ctx, "watch-plugin-writeback", "ENG-12", "task-linear-12"); err != nil {
		t.Fatalf("assign Linear source link: %v", err)
	}
	client := &pluginWritebackClient{fakeClient: &fakeClient{}, commentID: "comment-1"}
	var receivedSecret string
	fixture.svc.clientFn = func(_ *LinearConfig, secret string) Client {
		receivedSecret = secret
		return client
	}
	client.getIssueFn = func(identifier string) (*LinearIssue, error) {
		return &LinearIssue{ID: "issue-uuid-1", Identifier: identifier, TeamKey: "ENG"}, nil
	}

	issue, err := fixture.svc.GetIssueForWorkspace(ctx, workspaceID, "ENG-12")
	if err != nil {
		t.Fatalf("GetIssueForWorkspace: %v", err)
	}
	if issue.ID != "issue-uuid-1" || issue.Identifier != "ENG-12" || receivedSecret != "workspace-secret" {
		t.Fatalf("workspace read used the wrong issue or credential: issue=%+v secret=%q", issue, receivedSecret)
	}
	link, err := fixture.svc.GetTaskIssueLink(ctx, "task-linear-12")
	if err != nil {
		t.Fatalf("GetTaskIssueLink: %v", err)
	}
	if link.WorkspaceID != workspaceID || link.IssueIdentifier != "ENG-12" {
		t.Fatalf("task source link = %+v", link)
	}
	if _, err := fixture.svc.GetTaskIssueLink(ctx, "task-without-link"); !errors.Is(err, ErrSourceLinkNotFound) {
		t.Fatalf("missing task link error = %v, want ErrSourceLinkNotFound", err)
	}
	commentID, err := fixture.svc.AddCommentForWorkspace(ctx, workspaceID, issue.ID, "Progress: tests pass")
	if err != nil {
		t.Fatalf("AddCommentForWorkspace: %v", err)
	}
	if commentID != "comment-1" || client.commentIDArg != issue.ID || client.comment != "Progress: tests pass" {
		t.Fatalf("comment request not forwarded: id=%q issue=%q body=%q", commentID, client.commentIDArg, client.comment)
	}
	if err := fixture.svc.SetIssueStateForWorkspace(ctx, workspaceID, issue.ID, "state-done"); err != nil {
		t.Fatalf("SetIssueStateForWorkspace: %v", err)
	}
	if len(client.transitionLog) != 1 || client.transitionLog[0] != "issue-uuid-1:state-done" {
		t.Fatalf("state request not forwarded: %+v", client.transitionLog)
	}

	if _, err := fixture.svc.GetIssueForWorkspace(ctx, "workspace-without-credentials", "ENG-12"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("missing workspace credentials error = %v, want ErrNotConfigured", err)
	}
}
