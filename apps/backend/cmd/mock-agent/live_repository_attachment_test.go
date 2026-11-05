package main

import (
	"strings"
	"testing"
)

func TestLiveRepositoryAttachment(t *testing.T) {
	if _, ok := scenarioRegistry[liveRepositoryAttachmentScenarioPrefix]; !ok {
		t.Fatalf("scenario %q is not registered", liveRepositoryAttachmentScenarioPrefix)
	}

	tests := []struct {
		name    string
		raw     string
		wantErr string
		want    liveRepositoryAttachmentArgs
	}{
		{
			name: "local repository defaults the base branch",
			raw:  `{"local_path":"/tmp/workspace/secondary"}`,
			want: liveRepositoryAttachmentArgs{LocalPath: "/tmp/workspace/secondary", BaseBranch: "main"},
		},
		{
			name: "registered repository keeps explicit branch",
			raw:  `{"repository_id":"repo-2","base_branch":"develop","checkout_branch":"feature/x"}`,
			want: liveRepositoryAttachmentArgs{RepositoryID: "repo-2", BaseBranch: "develop", CheckoutBranch: "feature/x"},
		},
		{name: "missing arguments", raw: "", wantErr: "missing JSON arguments"},
		{name: "invalid JSON", raw: "{", wantErr: "invalid JSON arguments"},
		{name: "missing locator", raw: `{"base_branch":"main"}`, wantErr: "repository_id or local_path is required"},
		{
			name:    "two locators",
			raw:     `{"repository_id":"repo-2","local_path":"/tmp/secondary"}`,
			wantErr: "pass only one repository locator",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseLiveRepositoryAttachmentArgs(test.raw)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error = %v, want %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse args: %v", err)
			}
			if got != test.want {
				t.Fatalf("args = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestLiveRepositoryAttachmentWorkspacePrefixUsesToolResponse(t *testing.T) {
	result := `{"task_workspace_path":"/tasks/demo","worktree_path":"/tasks/demo/secondary-repository-main"}`
	if got := attachmentWorkspacePrefix(result, "fallback"); got != "secondary-repository-main" {
		t.Fatalf("attachmentWorkspacePrefix() = %q, want tool response prefix", got)
	}
	if got := attachmentWorkspacePrefix("{", "fallback"); got != "fallback" {
		t.Fatalf("attachmentWorkspacePrefix() = %q, want fallback for malformed response", got)
	}
	if got := attachmentWorkspacePrefix(
		`{"task_workspace_path":"/tasks/demo","worktree_path":"/elsewhere/repo"}`,
		"fallback",
	); got != "fallback" {
		t.Fatalf("attachmentWorkspacePrefix() = %q, want fallback outside the workspace", got)
	}
}

func TestLiveRepositoryAttachmentDerivesWorkspaceDirectoryName(t *testing.T) {
	tests := []struct {
		name string
		args liveRepositoryAttachmentArgs
		want string
	}{
		{name: "local path", args: liveRepositoryAttachmentArgs{LocalPath: "/workspace/secondary/"}, want: "secondary"},
		{name: "repository id", args: liveRepositoryAttachmentArgs{RepositoryID: "repo-2"}, want: "secondary"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.args.attachmentDirName(); got != test.want {
				t.Fatalf("attachmentDirName() = %q, want %q", got, test.want)
			}
		})
	}
}
