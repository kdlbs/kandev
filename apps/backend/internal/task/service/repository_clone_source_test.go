package service

import (
	"context"
	"os/exec"
	"testing"
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

func runGitTestCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}
