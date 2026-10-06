package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type plainHTTPStatus struct {
	Success        bool                      `json:"success"`
	StatusState    string                    `json:"status_state"`
	FilesComplete  bool                      `json:"files_complete"`
	DetailState    string                    `json:"detail_state"`
	RepositoryName string                    `json:"repository_name"`
	Modified       []string                  `json:"modified"`
	Files          map[string]types.FileInfo `json:"files"`
}

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.9, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.36, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.45
func TestGitStatusHTTPPlainPatches(t *testing.T) {
	for _, color := range []string{"ordinary", "disabled", "diff-always", "ui-always", "ui-always-diff-disabled", "captured-always"} {
		for _, layer := range []string{"unstaged", "staged", "mixed"} {
			t.Run(color+"/"+layer, func(t *testing.T) {
				isolatePlainHTTPEnvironment(t)
				root := t.TempDir()
				paths := map[string]string{"new[ab].txt": "-selected", "newa.txt": "-sibling", "ansi.txt": "-\x1b[31mliteral\x1b[0m", "日本語.txt": "-unicode"}
				if runtime.GOOS != "windows" {
					paths["escape\x1bname.txt"] = "-escape-name"
				}
				for _, repo := range []string{"selected", "other"} {
					seedPlainHTTPRepo(t, root, repo, layer, paths)
					configurePlainHTTPColor(t, filepath.Join(root, repo), color)
				}
				t.Setenv("GIT_LITERAL_PATHSPECS", "1")
				if color == "captured-always" {
					setPlainHTTPEnvironmentColor(t, "always")
				}
				cfg := &config.InstanceConfig{
					WorkDir: root, AgentEnv: append([]string(nil), os.Environ()...),
					BaseBranches: map[string]string{"selected": "main", "other": "main"},
				}
				log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
				require.NoError(t, err)
				manager := process.NewManager(cfg, log)
				t.Cleanup(func() { require.NoError(t, manager.StopForTeardown(context.Background())) })
				server := NewServer(cfg, manager, nil, nil, log)
				if color == "captured-always" {
					setPlainHTTPEnvironmentColor(t, "false")
				}
				envBefore := append([]string(nil), cfg.AgentEnv...)
				liveBefore := os.Environ()
				before := plainHTTPReadEvidence(t, root, paths)
				rec := getGitAPI(t, server, "/api/v1/git/status?repo=selected&fresh=true&details=wait")
				require.Equal(t, http.StatusOK, rec.Code)
				var selected plainHTTPStatus
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &selected))
				assertPlainHTTPStatus(t, selected, "selected", layer, paths)
				require.Equal(t, before, plainHTTPReadEvidence(t, root, paths))
				rec = getGitAPI(t, server, "/api/v1/git/status/multi?fresh=true&details=wait")
				require.Equal(t, http.StatusOK, rec.Code)
				var aggregate struct {
					Success bool `json:"success"`
					Repos   []struct {
						RepositoryName string          `json:"repository_name"`
						Status         plainHTTPStatus `json:"status"`
					} `json:"repos"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &aggregate))
				require.True(t, aggregate.Success)
				require.Len(t, aggregate.Repos, 2)
				repos := make([]string, 0, 2)
				for _, result := range aggregate.Repos {
					repos = append(repos, result.RepositoryName)
					assertPlainHTTPStatus(t, result.Status, result.RepositoryName, layer, paths)
				}
				require.ElementsMatch(t, []string{"selected", "other"}, repos)
				require.Equal(t, before, plainHTTPReadEvidence(t, root, paths))
				require.Equal(t, envBefore, cfg.AgentEnv)
				require.Equal(t, liveBefore, os.Environ())
			})
		}
	}
}

func isolatePlainHTTPEnvironment(t *testing.T) {
	t.Helper()
	for _, entry := range os.Environ() {
		key, value, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "GIT_") {
			require.NoError(t, os.Unsetenv(key))
			t.Cleanup(func() { require.NoError(t, os.Setenv(key, value)) })
		}
	}
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
}

func seedPlainHTTPRepo(t *testing.T, root, repo, layer string, paths map[string]string) {
	t.Helper()
	dir := filepath.Join(root, repo)
	require.NoError(t, os.Mkdir(dir, 0o755))
	runGitAPI(t, dir, "init", "--initial-branch=main")
	runGitAPI(t, dir, "config", "user.email", "test@test.com")
	runGitAPI(t, dir, "config", "user.name", "Test User")
	runGitAPI(t, dir, "config", "core.autocrlf", "false")
	runGitAPI(t, dir, "config", "core.quotePath", "false")
	for path, tag := range paths {
		writeFileAPI(t, dir, path, plainHTTPContents(repo+tag, "base-stage", "base-worktree"))
	}
	runGitAPI(t, dir, "add", "-A")
	runGitAPI(t, dir, "commit", "-m", "Plain HTTP fixture")
	runGitAPI(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	for path, tag := range paths {
		writeFileAPI(t, dir, path, plainHTTPContents(repo+tag, "index-stage", "base-worktree"))
	}
	if layer != "unstaged" {
		runGitAPI(t, dir, "add", "-A")
	}
	if layer == "mixed" {
		for path, tag := range paths {
			writeFileAPI(t, dir, path, plainHTTPContents(repo+tag, "index-stage", "worktree-line"))
		}
	}
}

func plainHTTPContents(tag, first, second string) string {
	return first + tag + "\n" + second + tag + "\ncontext" + tag + "\n"
}

func configurePlainHTTPColor(t *testing.T, dir, color string) {
	t.Helper()
	switch color {
	case "disabled":
		runGitAPI(t, dir, "config", "color.ui", "false")
		runGitAPI(t, dir, "config", "color.diff", "false")
	case "diff-always":
		runGitAPI(t, dir, "config", "color.diff", "always")
	case "ui-always":
		runGitAPI(t, dir, "config", "color.ui", "always")
	case "ui-always-diff-disabled":
		runGitAPI(t, dir, "config", "color.ui", "always")
		runGitAPI(t, dir, "config", "color.diff", "false")
	}
}

func setPlainHTTPEnvironmentColor(t *testing.T, value string) {
	t.Helper()
	t.Setenv("GIT_CONFIG_COUNT", "2")
	t.Setenv("GIT_CONFIG_KEY_0", "color.ui")
	t.Setenv("GIT_CONFIG_VALUE_0", value)
	t.Setenv("GIT_CONFIG_KEY_1", "color.diff")
	t.Setenv("GIT_CONFIG_VALUE_1", value)
}

func plainHTTPReadEvidence(t *testing.T, root string, paths map[string]string) map[string]string {
	t.Helper()
	evidence := make(map[string]string)
	for _, repo := range []string{"selected", "other"} {
		dir := filepath.Join(root, repo)
		cfg, err := os.ReadFile(filepath.Join(dir, ".git", "config"))
		require.NoError(t, err)
		evidence[repo+":config"] = string(cfg)
		evidence[repo+":head"] = runGitAPI(t, dir, "rev-parse", "HEAD")
		evidence[repo+":refs"] = runGitAPI(t, dir, "for-each-ref", "--format=%(refname) %(objectname)")
		evidence[repo+":index"] = runGitAPI(t, dir, "ls-files", "--stage", "-z")
		for path := range paths {
			data, readErr := os.ReadFile(filepath.Join(dir, path))
			require.NoError(t, readErr)
			evidence[repo+":worktree:"+path] = string(data)
			evidence[repo+":index:"+path] = runGitAPI(t, dir, "show", ":"+path)
		}
	}
	return evidence
}

func assertPlainHTTPStatus(t *testing.T, status plainHTTPStatus, repo, layer string, paths map[string]string) {
	t.Helper()
	require.True(t, status.Success)
	require.Equal(t, "ready", status.StatusState)
	require.True(t, status.FilesComplete)
	require.Equal(t, "ready", status.DetailState)
	require.Equal(t, repo, status.RepositoryName)
	require.Len(t, status.Files, len(paths))
	require.Len(t, status.Modified, len(paths))
	for path, tag := range paths {
		file, ok := status.Files[path]
		require.True(t, ok)
		require.Equal(t, path, file.Path)
		require.Equal(t, "modified", file.Status)
		require.Equal(t, layer == "staged", file.Staged)
		require.Empty(t, file.OldPath)
		require.Equal(t, "ready", file.DiffState)
		require.Empty(t, file.DiffSkipReason)
		count := 1
		if layer == "mixed" {
			count = 2
		}
		require.Equal(t, count, file.Additions)
		require.Equal(t, count, file.Deletions)
		assertPlainHTTPPatch(t, file.Diff, path, repo+tag, layer)
		if layer != "mixed" {
			require.Nil(t, file.StagedChange)
			require.Nil(t, file.UnstagedChange)
			continue
		}
		for _, facet := range []*types.FileChangeFacet{file.StagedChange, file.UnstagedChange} {
			require.NotNil(t, facet)
			require.Equal(t, "modified", facet.Status)
			require.Equal(t, 1, facet.Additions)
			require.Equal(t, 1, facet.Deletions)
			require.Equal(t, "ready", facet.DiffState)
			require.Empty(t, facet.OldPath)
			require.Empty(t, facet.DiffSkipReason)
		}
		assertPlainHTTPPatch(t, file.StagedChange.Diff, path, repo+tag, "staged")
		assertPlainHTTPPatch(t, file.UnstagedChange.Diff, path, repo+tag, "mixed-unstaged")
	}
}

func assertPlainHTTPPatch(t *testing.T, patch, path, tag, layer string) {
	t.Helper()
	a, b := "a/"+path, "b/"+path
	if strings.Contains(path, "\x1b") {
		a = `"` + strings.ReplaceAll(a, "\x1b", `\033`) + `"`
		b = `"` + strings.ReplaceAll(b, "\x1b", `\033`) + `"`
	}
	assert.True(t, strings.HasPrefix(patch, "diff --git "+a+" "+b+"\n"), "patch header: %q", patch)
	assert.Contains(t, patch, "--- "+a+"\n+++ "+b+"\n")
	var hunk string
	switch layer {
	case "mixed":
		hunk = fmt.Sprintf("@@ -1,3 +1,3 @@\n-base-stage%s\n-base-worktree%s\n+index-stage%s\n+worktree-line%s\n context%s\n", tag, tag, tag, tag, tag)
	case "mixed-unstaged":
		hunk = fmt.Sprintf("@@ -1,3 +1,3 @@\n index-stage%s\n-base-worktree%s\n+worktree-line%s\n context%s\n", tag, tag, tag, tag)
	default:
		hunk = fmt.Sprintf("@@ -1,3 +1,3 @@\n-base-stage%s\n+index-stage%s\n base-worktree%s\n context%s\n", tag, tag, tag, tag)
	}
	assert.True(t, strings.HasSuffix(patch, hunk), "patch = %q, want exact hunk %q", patch, hunk)
	if !strings.Contains(tag, "\x1b") {
		assert.NotContains(t, patch, "\x1b[")
	}
}
