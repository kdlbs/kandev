package service

import (
	"context"
	"os/exec"
	"testing"

	"github.com/kandev/kandev/internal/repoclone"
	"github.com/kandev/kandev/internal/task/models"
)

func TestCredentialFreeCloneOrigin(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr string
	}{
		{name: "https", input: "https://github.com/acme/api.git", want: "https://github.com/acme/api.git"},
		{name: "ssh url", input: "ssh://git@github.com/acme/api.git", want: "ssh://git@github.com/acme/api.git"},
		{name: "scp ssh", input: "git@github.com:acme/api.git", want: "git@github.com:acme/api.git"},
		{name: "https password", input: "https://user:secret@example.com/acme/api.git", wantErr: "origin_credentials"},
		{name: "https user", input: "https://user@example.com/acme/api.git", wantErr: "origin_credentials"},
		{name: "file origin", input: "file:///tmp/api.git", wantErr: "unsupported_origin"},
		{name: "plain path", input: "/tmp/api", wantErr: "unsupported_origin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := credentialFreeCloneOrigin(tt.input)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("credentialFreeCloneOrigin(%q) error = %v, want %q", tt.input, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("credentialFreeCloneOrigin(%q): %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("credentialFreeCloneOrigin(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeRemoteOriginBranch(t *testing.T) {
	t.Parallel()
	if got, err := normalizeRemoteOriginBranch("origin/feature/api"); err != nil || got != "feature/api" {
		t.Fatalf("normalizeRemoteOriginBranch(origin/feature/api) = %q, %v", got, err)
	}
	if _, err := normalizeRemoteOriginBranch("--upload-pack=evil"); err == nil {
		t.Fatal("normalizeRemoteOriginBranch accepted an unsafe branch")
	}
}

func TestRemoteOriginDefaultBranch(t *testing.T) {
	t.Parallel()
	branches := []Branch{{Name: "develop"}, {Name: "main"}, {Name: "release"}}
	if got := remoteOriginDefaultBranch(branches, "release"); got != "release" {
		t.Fatalf("current branch default = %q, want release", got)
	}
	if got := remoteOriginDefaultBranch(branches, "feature/local"); got != "main" {
		t.Fatalf("preferred branch default = %q, want main", got)
	}
	if got := remoteOriginDefaultBranch([]Branch{{Name: "release"}}, "feature/local"); got != "release" {
		t.Fatalf("first branch default = %q, want release", got)
	}
}

func TestInspectLocalRepositoryCloneSourceWithoutOrigin(t *testing.T) {
	t.Parallel()
	path := t.TempDir()
	runGitTestCommand(t, path, "init", "--quiet")
	result, err := inspectLocalRepositoryCloneSourcePath(context.Background(), path)
	if err != nil {
		t.Fatalf("inspectLocalRepositoryCloneSourcePath: %v", err)
	}
	if result.Ready || result.Origin != "" || result.Reason != "missing_origin" {
		t.Fatalf("inspection = %+v, want missing origin and unavailable", result)
	}
}

type recordingRemoteOriginBranchLister struct {
	names   []string
	request repoclone.GitCredentialRequest
	err     error
}

func (l *recordingRemoteOriginBranchLister) ListLocalOriginBranches(
	_ context.Context, _ string, request repoclone.GitCredentialRequest,
) ([]string, error) {
	l.request = request
	return l.names, l.err
}

func TestInspectLocalRepositoryCloneSourceUsesWorkspaceOriginLister(t *testing.T) {
	t.Parallel()
	path := t.TempDir()
	runGitTestCommand(t, path, "init", "--quiet")
	runGitTestCommand(t, path, "remote", "add", "origin", "https://github.com/acme/api.git")
	lister := &recordingRemoteOriginBranchLister{names: []string{"main", "feature/api", "--unsafe"}}
	request := cloneCredentialRequest("workspace-1", &models.Repository{
		ID:             "repo-1",
		Provider:       "github",
		ProviderHost:   "https://github.com",
		ProviderOwner:  "acme",
		ProviderName:   "api",
		ProviderScope:  "scope-1",
		ProviderRepoID: "provider-1",
	})

	result, err := inspectLocalRepositoryCloneSourcePathWithLister(
		context.Background(), path, lister, request,
	)
	if err != nil {
		t.Fatalf("inspectLocalRepositoryCloneSourcePathWithLister: %v", err)
	}
	if !result.Ready || result.Origin != "https://github.com/acme/api.git" {
		t.Fatalf("inspection = %+v, want ready GitHub origin", result)
	}
	if len(result.Branches) != 2 || result.Branches[0].Name != "feature/api" || result.Branches[1].Name != "main" {
		t.Fatalf("branches = %+v, want sorted safe origin branches", result.Branches)
	}
	if lister.request != request {
		t.Fatalf("credential request = %+v, want %+v", lister.request, request)
	}
}

func TestInspectLocalRepositoryCloneSourceListerFailureIsUnavailable(t *testing.T) {
	t.Parallel()
	path := t.TempDir()
	runGitTestCommand(t, path, "init", "--quiet")
	runGitTestCommand(t, path, "remote", "add", "origin", "https://github.com/acme/api.git")
	lister := &recordingRemoteOriginBranchLister{err: context.DeadlineExceeded}

	result, err := inspectLocalRepositoryCloneSourcePathWithLister(
		context.Background(), path, lister, repoclone.GitCredentialRequest{WorkspaceID: "workspace-1"},
	)
	if err != nil {
		t.Fatalf("inspectLocalRepositoryCloneSourcePathWithLister: %v", err)
	}
	if result.Ready || result.Reason != remoteOriginUnavailableReason {
		t.Fatalf("inspection = %+v, want unavailable origin", result)
	}
}

func runGitTestCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}
