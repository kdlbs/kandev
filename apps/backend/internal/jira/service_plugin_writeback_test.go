package jira

import (
	"context"
	"errors"
	"testing"
)

type pluginWritebackClient struct {
	*fakeClient
	commentID  string
	commentKey string
	comment    string
	commentErr error
}

func (c *pluginWritebackClient) AddComment(_ context.Context, issueKey, body string) (string, error) {
	c.commentKey = issueKey
	c.comment = body
	return c.commentID, c.commentErr
}

func TestPluginWriteback(t *testing.T) {
	ctx := context.Background()
	fixture := newSvcFixture(t)
	const workspaceID = "workspace-plugin-writeback"
	if err := fixture.store.UpsertConfigForWorkspace(ctx, workspaceID, &JiraConfig{
		SiteURL: "https://example.atlassian.net", Email: "agent@example.com",
		AuthMethod: AuthMethodAPIToken, InstanceType: InstanceTypeCloud,
	}); err != nil {
		t.Fatalf("seed workspace config: %v", err)
	}
	if err := fixture.secrets.Set(ctx, SecretKeyForWorkspace(workspaceID), "Jira token", "workspace-secret"); err != nil {
		t.Fatalf("seed workspace secret: %v", err)
	}
	if _, err := fixture.store.db.ExecContext(ctx, `
		INSERT INTO jira_issue_watches (id, workspace_id, workflow_id, workflow_step_id, jql, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		"watch-plugin-writeback", workspaceID, "workflow-1", "step-1", "project = ENG"); err != nil {
		t.Fatalf("seed Jira issue watch: %v", err)
	}
	if _, err := fixture.store.ReserveIssueWatchTask(ctx, "watch-plugin-writeback", "ENG-12", "https://example.atlassian.net/browse/ENG-12"); err != nil {
		t.Fatalf("reserve Jira source link: %v", err)
	}
	if err := fixture.store.AssignIssueWatchTaskID(ctx, "watch-plugin-writeback", "ENG-12", "task-jira-12"); err != nil {
		t.Fatalf("assign Jira source link: %v", err)
	}
	client := &pluginWritebackClient{fakeClient: &fakeClient{}, commentID: "comment-1"}
	var receivedSecret string
	fixture.svc.clientFn = func(_ *JiraConfig, secret string) Client {
		receivedSecret = secret
		return client
	}

	ticket, err := fixture.svc.GetTicketForWorkspace(ctx, workspaceID, "ENG-12")
	if err != nil {
		t.Fatalf("GetTicketForWorkspace: %v", err)
	}
	if ticket.Key != "ENG-12" || receivedSecret != "workspace-secret" {
		t.Fatalf("workspace read used the wrong issue or credential: ticket=%+v secret=%q", ticket, receivedSecret)
	}
	link, err := fixture.svc.GetTaskIssueLink(ctx, "task-jira-12")
	if err != nil {
		t.Fatalf("GetTaskIssueLink: %v", err)
	}
	if link.WorkspaceID != workspaceID || link.IssueKey != "ENG-12" {
		t.Fatalf("task source link = %+v", link)
	}
	if _, err := fixture.svc.GetTaskIssueLink(ctx, "task-without-link"); !errors.Is(err, ErrSourceLinkNotFound) {
		t.Fatalf("missing task link error = %v, want ErrSourceLinkNotFound", err)
	}
	commentID, err := fixture.svc.AddCommentForWorkspace(ctx, workspaceID, ticket.Key, "Progress: tests pass")
	if err != nil {
		t.Fatalf("AddCommentForWorkspace: %v", err)
	}
	if commentID != "comment-1" || client.commentKey != ticket.Key || client.comment != "Progress: tests pass" {
		t.Fatalf("comment request not forwarded: id=%q key=%q body=%q", commentID, client.commentKey, client.comment)
	}
	if err := fixture.svc.DoTransitionForWorkspace(ctx, workspaceID, ticket.Key, "transition-2"); err != nil {
		t.Fatalf("DoTransitionForWorkspace: %v", err)
	}
	if len(client.transitionLog) != 1 || client.transitionLog[0] != "ENG-12:transition-2" {
		t.Fatalf("transition request not forwarded: %+v", client.transitionLog)
	}

	if _, err := fixture.svc.GetTicketForWorkspace(ctx, "workspace-without-credentials", "ENG-12"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("missing workspace credentials error = %v, want ErrNotConfigured", err)
	}
}
