package service

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveExplicitLocalRepositoryPath_AcceptsReciprocalSubmoduleMetadata(t *testing.T) {
	parent := canonicalTempDir(t)
	repoPath := filepath.Join(parent, "module")
	gitDir := filepath.Join(parent, "metadata", "modules", "module")
	worktree, err := filepath.Rel(gitDir, repoPath)
	if err != nil {
		t.Fatalf("Rel worktree: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "heads"), 0o755); err != nil {
		t.Fatalf("MkdirAll git metadata: %v", err)
	}
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("MkdirAll repo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, ".git"), []byte("gitdir: "+gitDir+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte("[Core]\n  WorkTree = "+worktree+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o600); err != nil {
		t.Fatalf("WriteFile HEAD: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "refs", "heads", "main"), []byte("0000000000000000000000000000000000000000\n"), 0o600); err != nil {
		t.Fatalf("WriteFile main ref: %v", err)
	}

	resolved, branch, err := resolveExplicitLocalRepositoryPath(repoPath)
	if err != nil {
		t.Fatalf("resolveExplicitLocalRepositoryPath: %v", err)
	}
	if resolved != repoPath {
		t.Fatalf("resolved path = %q, want %q", resolved, repoPath)
	}
	if branch != "main" {
		t.Fatalf("default branch = %q, want main", branch)
	}
}

func TestResolveExplicitLocalRepositoryPath_AcceptsGitQuotedSubmoduleWorktree(t *testing.T) {
	parent := canonicalTempDir(t)
	sourcePath := filepath.Join(parent, "source")
	superprojectPath := filepath.Join(parent, "superproject")
	for _, repositoryPath := range []string{sourcePath, superprojectPath} {
		if err := os.MkdirAll(repositoryPath, 0o755); err != nil {
			t.Fatalf("MkdirAll %q: %v", repositoryPath, err)
		}
		gitCommand(t, repositoryPath, "init", "-b", "main")
		gitCommand(t, repositoryPath, "config", "user.email", "test@example.com")
		gitCommand(t, repositoryPath, "config", "user.name", "Test User")
	}
	if err := os.WriteFile(filepath.Join(sourcePath, "README.md"), []byte("source\n"), 0o600); err != nil {
		t.Fatalf("WriteFile source README: %v", err)
	}
	gitCommand(t, sourcePath, "add", "README.md")
	gitCommand(t, sourcePath, "commit", "-m", "initial commit")

	moduleName := "module#hash\"quote"
	modulePath := filepath.Join(superprojectPath, moduleName)
	gitCommand(t, superprojectPath, "-c", "protocol.file.allow=always", "submodule", "add", sourcePath, moduleName)
	gitDir, err := resolveGitDir(modulePath)
	if err != nil {
		t.Fatalf("resolveGitDir: %v", err)
	}
	config, err := os.ReadFile(filepath.Join(gitDir, "config"))
	if err != nil {
		t.Fatalf("ReadFile submodule config: %v", err)
	}
	if !strings.Contains(string(config), "worktree = \"") {
		t.Fatalf("Git-generated config does not quote core.worktree: %s", config)
	}
	if !strings.Contains(string(config), `\"`) {
		t.Fatalf("Git-generated config does not escape core.worktree quote: %s", config)
	}

	resolved, _, err := resolveExplicitLocalRepositoryPath(modulePath)
	if err != nil {
		t.Fatalf("resolveExplicitLocalRepositoryPath: %v", err)
	}
	want, err := filepath.EvalSymlinks(modulePath)
	if err != nil {
		t.Fatalf("EvalSymlinks module path: %v", err)
	}
	if resolved != want {
		t.Fatalf("resolved path = %q, want %q", resolved, want)
	}
}

func gitCommand(t *testing.T, repositoryPath string, args ...string) {
	t.Helper()
	commandArgs := append([]string{"-C", repositoryPath}, args...)
	output, err := exec.Command("git", commandArgs...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

func TestParseGitConfigCoreWorktree(t *testing.T) {
	for _, test := range []struct {
		name, config, want string
		wantErr            bool
	}{
		{"case and whitespace", "[Core]\n WorkTree = ../module \n", "../module", false},
		{"compact", "[core]\nworktree=module\n", "module", false},
		{"quoted value with inline comment", "[core]\nworktree = \"../module#hash\" ; keep hash\n", "../module#hash", false},
		{"unquoted value with inline comment", "[core]\nworktree = ../module#hash\n", "../module", false},
		{"partially quoted value", "[core]\nworktree = \"../module\"suffix\n", "../modulesuffix", false},
		{"duplicate uses effective last value", "[core]\nworktree=selected\nworktree=other\n", "other", false},
		{"outside core", "worktree=wrong\n[other]\nworktree=also-wrong\n", "", true},
		{"empty", "[core]\nworktree = \n", "", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseGitConfigCoreWorktree(test.config)
			if test.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("parse = %q, %v; want %q, nil", got, err, test.want)
			}
		})
	}
}

func TestResolveExplicitLocalRepositoryPath_RejectsMismatchedSubmoduleWorktree(t *testing.T) {
	parent := canonicalTempDir(t)
	repoPath := filepath.Join(parent, "module")
	gitDir := filepath.Join(parent, "metadata", "modules", "module")
	other := filepath.Join(parent, "other")
	worktree, err := filepath.Rel(gitDir, other)
	if err != nil {
		t.Fatal(err)
	}
	selectedWorktree, err := filepath.Rel(gitDir, repoPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "heads"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}
	for path, value := range map[string]string{filepath.Join(repoPath, ".git"): "gitdir: " + gitDir, filepath.Join(gitDir, "config"): "[core]\nworktree=" + selectedWorktree + "\nworktree=" + worktree, filepath.Join(gitDir, "HEAD"): "ref: refs/heads/main"} {
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	_, _, err = resolveExplicitLocalRepositoryPath(repoPath)
	if !errors.Is(err, ErrInvalidRepositoryPath) {
		t.Fatalf("error = %v", err)
	}
}

func TestResolveExplicitLocalRepositoryPath_RejectsSubmoduleAlternateConfigSources(t *testing.T) {
	for _, test := range []struct {
		name             string
		config           string
		additionalConfig map[string]string
	}{
		{
			name:   "include override",
			config: "[core]\nworktree = %s\n[include]\npath = override.conf\n",
			additionalConfig: map[string]string{
				"override.conf": "[core]\nworktree = %s\n",
			},
		},
		{
			name:   "include override with header comment",
			config: "[core]\nworktree = %s\n[include] # load override\npath = override.conf\n",
			additionalConfig: map[string]string{
				"override.conf": "[core]\nworktree = %s\n",
			},
		},
		{
			name:   "worktree config override",
			config: "[core]\nworktree = %s\n[extensions]\nworktreeConfig = true\n",
			additionalConfig: map[string]string{
				"config.worktree": "[core]\nworktree = %s\n",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			parent := canonicalTempDir(t)
			repoPath := filepath.Join(parent, "module")
			gitDir := filepath.Join(parent, "metadata", "modules", "module")
			other := filepath.Join(parent, "other")
			selectedWorktree, err := filepath.Rel(gitDir, repoPath)
			if err != nil {
				t.Fatal(err)
			}
			otherWorktree, err := filepath.Rel(gitDir, other)
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{repoPath, other, filepath.Join(gitDir, "refs", "heads")} {
				if err := os.MkdirAll(path, 0o755); err != nil {
					t.Fatalf("MkdirAll %q: %v", path, err)
				}
			}
			if err := os.WriteFile(filepath.Join(repoPath, ".git"), []byte("gitdir: "+gitDir), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(fmt.Sprintf(test.config, selectedWorktree)), 0o600); err != nil {
				t.Fatal(err)
			}
			for name, config := range test.additionalConfig {
				if err := os.WriteFile(filepath.Join(gitDir, name), []byte(fmt.Sprintf(config, otherWorktree)), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(gitDir, "refs", "heads", "main"), []byte("0000000000000000000000000000000000000000"), 0o600); err != nil {
				t.Fatal(err)
			}

			_, _, err = resolveExplicitLocalRepositoryPath(repoPath)
			if !errors.Is(err, ErrInvalidRepositoryPath) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
