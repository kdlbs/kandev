package repoclone

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.1, AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.2
func TestManagedAttachmentGitPathCompatibility(t *testing.T) {
	for _, scenario := range []string{"relative-common-dir", "space", "newline", "parent-alias"} {
		t.Run(scenario, func(t *testing.T) {
			root := canonicalTempDir(t)
			base := filepath.Join(root, "clones")
			if scenario == "parent-alias" {
				physical := filepath.Join(root, "physical")
				mustAttachmentDirectory(t, physical)
				alias := filepath.Join(root, "alias")
				attachmentSymlink(t, physical, alias)
				base = filepath.Join(alias, "clones")
			}
			name := "repo"
			if scenario == "space" {
				name += " \t"
			}
			if scenario == "newline" {
				name += " \n"
			}
			probeAttachmentPathName(t, root, name)
			cloner := NewCloner(Config{BasePath: base}, ProtocolHTTPS, "", logger.Default())
			path, err := cloner.WorkspaceRepoPath("ws", "github", "team", name)
			if err != nil {
				t.Fatal(err)
			}
			attachmentGit(t, "init", "-b", "main", path)
			origin := "https://github.com/team/repo.git"
			attachmentGit(t, "-C", path, "remote", "add", "origin", origin)
			repo := &models.Repository{WorkspaceID: "ws", SourceType: "provider", Provider: "github", ProviderHost: "https://github.com", ProviderOwner: "team", ProviderName: name, RemoteURL: origin, LocalPath: path}
			installLegacyAttachmentGit(t)

			exists, err := cloner.InspectManagedAttachmentPath(context.Background(), repo, path)
			if err != nil || !exists {
				t.Fatalf("valid managed clone rejected with pre-2.31 Git: exists=%v err=%v", exists, err)
			}
			if repo.LocalPath != path {
				t.Fatalf("registered lexical path changed: got %q want %q", repo.LocalPath, path)
			}
		})
	}
}

func installLegacyAttachmentGit(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("legacy Git shell wrapper requires a POSIX shell")
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	quotedGit := "'" + strings.ReplaceAll(realGit, "'", "'\\''") + "'"
	// Git 2.30 echoes unknown rev-parse options and exits zero with the real path.
	script := "#!/bin/sh\n" +
		"if [ \"$3\" = rev-parse ] && [ \"$4\" = --path-format=absolute ]; then\n" +
		"  printf '%s\\n' --path-format=absolute\n" +
		"  exec " + quotedGit + " -C \"$2\" rev-parse \"$5\"\n" +
		"fi\n" +
		"exec " + quotedGit + " \"$@\"\n"
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}
