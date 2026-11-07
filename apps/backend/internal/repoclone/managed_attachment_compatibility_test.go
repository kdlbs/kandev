package repoclone

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.2, AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.3
func TestManagedAttachmentPathCompatibility(t *testing.T) {
	for _, scenario := range []string{"alias-missing", "alias-existing", "base-link", "descendant-link", "newline-existing", "space-existing", "scope-provider-legacy", "scope-owner-legacy", "scope-unpaired"} {
		t.Run(scenario, func(t *testing.T) {
			root := canonicalTempDir(t)
			base := filepath.Join(root, "clones")
			if strings.HasPrefix(scenario, "alias-") {
				physical := filepath.Join(root, "physical")
				mustAttachmentDirectory(t, physical)
				alias := filepath.Join(root, "alias")
				attachmentSymlink(t, physical, alias)
				base = filepath.Join(alias, "clones")
			}
			if scenario == "base-link" {
				physical := filepath.Join(root, "physical")
				mustAttachmentDirectory(t, physical)
				attachmentSymlink(t, physical, base)
			}
			c := NewCloner(Config{BasePath: base}, ProtocolHTTPS, "", logger.Default())
			origin := canonicalTempDir(t)
			attachmentGit(t, "init", "-b", "main", origin)
			attachmentGit(t, "-C", origin, "-c", "user.name=Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "initial")
			r := &models.Repository{WorkspaceID: "ws", SourceType: "provider", Provider: "gitlab", ProviderHost: "https://gitlab.example", ProviderOwner: "team", ProviderName: "repo", RemoteURL: origin}
			if scenario == "newline-existing" {
				r.ProviderName = "repo \n"
			}
			if scenario == "space-existing" {
				r.ProviderName = "repo \t"
			}
			path, err := c.WorkspaceProviderRepositoryPath(r.WorkspaceID, r.Provider, r.ProviderHost, "", "", r.ProviderOwner, r.ProviderName)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "scope-provider-legacy" {
				path, err = c.ProviderRepoPath(r.Provider, r.ProviderHost, r.ProviderOwner, r.ProviderName)
			}
			if scenario == "scope-owner-legacy" {
				path, err = c.RepoPath(r.ProviderOwner, r.ProviderName)
			}
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "descendant-link" {
				mustAttachmentDirectory(t, base)
				foreign := filepath.Join(root, "foreign")
				mustAttachmentDirectory(t, foreign)
				attachmentSymlink(t, foreign, filepath.Join(base, "workspaces"))
			}
			if scenario == "newline-existing" || scenario == "space-existing" {
				probeAttachmentPathName(t, root, r.ProviderName)
			}
			if scenario != "alias-missing" {
				mustAttachmentDirectory(t, filepath.Dir(path))
				attachmentGit(t, "clone", origin, path)
				info, err := os.Lstat(path)
				if err != nil {
					t.Fatal(err)
				}
				if info.Name() != r.ProviderName && (scenario == "newline-existing" || scenario == "space-existing") {
					t.Skip("filesystem normalizes trailing whitespace")
				}
			}
			if strings.HasPrefix(scenario, "scope-") {
				r.ProviderScope = "connection"
				r.ProviderRepoID = "project"
			}
			if scenario == "scope-unpaired" {
				r.ProviderRepoID = ""
			}
			r.LocalPath = path
			// The fixture checkout and this unrelated sentinel must survive refusal.
			sentinel := filepath.Join(root, "sentinel")
			if err := os.WriteFile(sentinel, []byte("preserve"), 0600); err != nil {
				t.Fatal(err)
			}
			exists, err := c.InspectManagedAttachmentPath(context.Background(), r, path)
			refused := strings.HasPrefix(scenario, "scope-") || scenario == "base-link" || scenario == "descendant-link"
			if refused && err == nil {
				t.Fatal("redirected or scoped legacy checkout admitted")
			}
			if !refused && (err != nil || exists != (scenario != "alias-missing")) {
				t.Fatalf("exact configured checkout admission: exists=%v err=%v path=%q", exists, err, path)
			}
			if got, err := os.ReadFile(sentinel); err != nil || string(got) != "preserve" {
				t.Fatalf("sentinel changed: %q %v", got, err)
			}
			if scenario != "alias-missing" && runAttachmentGitOutput(t, path, "rev-parse", "HEAD") != runAttachmentGitOutput(t, origin, "rev-parse", "HEAD") {
				t.Fatal("checkout content changed")
			}
		})
	}
}

func mustAttachmentDirectory(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
}
func attachmentSymlink(t *testing.T, target, path string) {
	t.Helper()
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
}

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.2
func TestManagedAttachmentAliasIdentityDuringAuthentication(t *testing.T) {
	for _, existingBase := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing-base", true: "existing-base"}[existingBase], func(t *testing.T) {
			root := canonicalTempDir(t)
			physical := filepath.Join(root, "physical")
			mustAttachmentDirectory(t, physical)
			alias := filepath.Join(root, "alias")
			attachmentSymlink(t, physical, alias)
			base := filepath.Join(alias, "clones")
			if existingBase {
				mustAttachmentDirectory(t, base)
			}
			c := NewCloner(Config{BasePath: base}, ProtocolHTTPS, "", logger.Default())
			r := &models.Repository{ID: "repo", WorkspaceID: "ws", SourceType: "provider", Provider: "github", ProviderHost: "https://github.com", ProviderOwner: "team", ProviderName: "repo", RemoteURL: "https://github.com/team/repo.git"}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			t.Cleanup(cancel)
			scoped, err := c.ManagedAttachmentContext(ctx, r)
			if err != nil {
				t.Fatal(err)
			}
			replaced := false
			c.SetGitCredentialProvider(attachmentReplacingCredential{replace: func() {
				replaced = true
				// Keep the alias inode unchanged while replacing its physical target.
				if err := os.Rename(physical, physical+".retained"); err != nil {
					t.Fatal(err)
				}
				mustAttachmentDirectory(t, physical)
				if err := os.WriteFile(filepath.Join(physical, "sentinel"), []byte("preserve"), 0600); err != nil {
					t.Fatal(err)
				}
			}})
			_, err = c.EnsureWorkspaceClonedWithCredentialRequest(scoped, GitCredentialRequest{TaskID: "task", SessionID: "session", RepositoryID: r.ID, WorkspaceID: r.WorkspaceID, Provider: r.Provider, ProviderHost: r.ProviderHost, Owner: r.ProviderOwner, Name: r.ProviderName, CloneURL: r.RemoteURL}, "", "")
			if !replaced || err == nil || (!strings.Contains(err.Error(), "path identity changed") && !errors.Is(err, os.ErrNotExist)) {
				t.Fatalf("physical alias target replacement not refused: %v", err)
			}
			entries, err := os.ReadDir(physical)
			if err != nil || len(entries) != 1 || entries[0].Name() != "sentinel" {
				t.Fatalf("replacement target mutated: %v %v", entries, err)
			}
			if got, err := os.ReadFile(filepath.Join(physical, "sentinel")); err != nil || string(got) != "preserve" {
				t.Fatal("replacement sentinel changed")
			}
			if _, err := os.Lstat(filepath.Join(physical, "clones")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("replacement clone base created: %v", err)
			}
		})
	}
}

func runAttachmentGitOutput(t *testing.T, path string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", path}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git: %v: %s", err, out)
	}
	return string(out)
}

func probeAttachmentPathName(t *testing.T, root, name string) {
	t.Helper()
	probe := filepath.Join(root, name)
	if err := os.Mkdir(probe, 0755); err != nil {
		t.Skipf("filesystem cannot create exact whitespace name: %v", err)
	}
	info, err := os.Lstat(probe)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(probe); err != nil {
		t.Fatal(err)
	}
	if info.Name() != name {
		t.Skip("filesystem normalizes trailing whitespace")
	}
}
