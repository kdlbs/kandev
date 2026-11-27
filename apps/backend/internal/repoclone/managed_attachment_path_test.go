package repoclone

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

type attachmentPathInspector interface {
	InspectManagedAttachmentPath(context.Context, *models.Repository, string) (bool, error)
}

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.2, AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.3
func TestManagedAttachmentPathAdmission(t *testing.T) {
	for _, scenario := range []string{"absent-current", "relative-base", "valid-current", "wrong-origin", "foreign", "absent-legacy", "valid-legacy", "git-link", "nested-refs", "alternates"} {
		t.Run(scenario, func(t *testing.T) {
			base := canonicalTempDir(t)
			if scenario == "relative-base" {
				cwd, e := os.Getwd()
				if e != nil {
					t.Fatal(e)
				}
				base, e = filepath.Rel(cwd, base)
				if e != nil {
					t.Fatal(e)
				}
			}
			c := NewCloner(Config{BasePath: base}, ProtocolHTTPS, "", logger.Default())
			inspector, ok := any(c).(attachmentPathInspector)
			if !ok {
				t.Fatal("managed attachment path admission is unavailable")
			}
			origin := canonicalTempDir(t)
			attachmentGit(t, "init", "-b", "main", origin)
			attachmentGit(t, "-C", origin, "-c", "user.name=Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "initial")
			r := &models.Repository{WorkspaceID: "ws", SourceType: "provider", Provider: "gitlab", ProviderHost: "https://gitlab.example", ProviderOwner: "team", ProviderName: "repo", RemoteURL: origin}
			current, err := c.WorkspaceProviderRepositoryPath("ws", "gitlab", r.ProviderHost, "", "", "team", "repo")
			if err != nil {
				t.Fatal(err)
			}
			path := current
			if scenario == "relative-base" {
				path, err = filepath.Abs(current)
				if err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "foreign" {
				path = filepath.Join(canonicalTempDir(t), "other")
			}
			if scenario == "absent-legacy" || scenario == "valid-legacy" {
				path, err = c.ProviderRepoPath("gitlab", r.ProviderHost, "team", "repo")
				if err != nil {
					t.Fatal(err)
				}
			}
			if scenario != "absent-current" && scenario != "absent-legacy" && scenario != "relative-base" {
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				attachmentGit(t, "clone", origin, path)
			}
			switch scenario {
			case "wrong-origin":
				attachmentGit(t, "-C", path, "remote", "set-url", "origin", "https://foreign.example/other.git")
			case "git-link":
				original := filepath.Join(path, ".git")
				target := filepath.Join(canonicalTempDir(t), "metadata")
				if err := os.Rename(original, target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, original); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			case "nested-refs":
				target := canonicalTempDir(t)
				link := filepath.Join(path, ".git", "refs", "foreign")
				if err := os.Symlink(target, link); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			case "alternates":
				if err := os.WriteFile(filepath.Join(path, ".git", "objects", "info", "alternates"), []byte(canonicalTempDir(t)), 0600); err != nil {
					t.Fatal(err)
				}
			}
			exists, err := inspector.InspectManagedAttachmentPath(context.Background(), r, path)
			valid := scenario == "absent-current" || scenario == "relative-base" || scenario == "valid-current" || scenario == "valid-legacy"
			if valid && err != nil {
				t.Fatal(err)
			}
			if !valid && err == nil {
				t.Fatal("unowned source admitted")
			}
			if valid && exists != (scenario != "absent-current" && scenario != "relative-base") {
				t.Fatalf("exists=%v", exists)
			}
		})
	}
	t.Run("changed-ancestor-before-clone", testManagedAttachmentChangedAncestor)

}

func attachmentGit(t *testing.T, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
		t.Fatalf("git: %v: %s", err, out)
	}
}

func testManagedAttachmentChangedAncestor(t *testing.T) {
	base := canonicalTempDir(t)
	c := NewCloner(Config{BasePath: base}, ProtocolHTTPS, "", logger.Default())
	r := &models.Repository{ID: "repo", WorkspaceID: "ws", SourceType: "provider", Provider: "github", ProviderHost: "https://github.com", ProviderOwner: "team", ProviderName: "repo", RemoteURL: "https://github.com/team/repo.git"}
	parent, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	ctx, err := c.ManagedAttachmentContext(parent, r)
	if err != nil {
		t.Fatal(err)
	}
	c.SetGitCredentialProvider(attachmentReplacingCredential{replace: func() {
		if err := os.Rename(base, base+".retained"); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(base, 0755); err != nil {
			t.Fatal(err)
		}
	}})
	_, err = c.EnsureWorkspaceClonedWithCredentialRequest(ctx, GitCredentialRequest{TaskID: "task", SessionID: "session", RepositoryID: "repo", WorkspaceID: "ws", Provider: "github", ProviderHost: r.ProviderHost, Owner: "team", Name: "repo", CloneURL: r.RemoteURL}, "", "")
	if err == nil || !strings.Contains(err.Error(), "path identity changed") {
		t.Fatalf("replacement was not refused before clone: %v", err)
	}
	entries, err := os.ReadDir(base)
	if err != nil || len(entries) != 0 {
		t.Fatalf("replacement root mutated: %v %v", entries, err)
	}
}

type attachmentReplacingCredential struct{ replace func() }

func (c attachmentReplacingCredential) ResolveGitCredential(context.Context, GitCredentialRequest) (string, string, error) {
	c.replace()
	return "user", "test-token", nil
}
