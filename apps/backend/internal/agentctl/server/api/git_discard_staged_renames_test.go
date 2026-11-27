package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.9, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.44, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.47
func TestHandleGitDiscardStagedRenames(t *testing.T) {
	for _, edit := range []string{"pure", "mixed"} {
		t.Run(edit, func(t *testing.T) {
			isolatePlainHTTPEnvironment(t)
			root := t.TempDir()
			want := map[string]map[string]discardAPIState{}
			evidence := map[string][]string{}
			for _, repo := range []string{"selected", "other"} {
				dir := seedRenameDiscardAPIRepo(t, root, repo, edit)
				evidence[repo] = renameDiscardAPIEvidence(t, dir)
				want[repo] = map[string]discardAPIState{}
				for _, path := range []string{"old.txt", "new.txt", "neighbor.txt", "ordinary.txt", "added.txt", "untracked.txt"} {
					want[repo][path] = readDiscardAPIState(t, dir, path)
				}
			}
			server := newDiscardAPIServer(t, root, "0", "0")
			beforeEnvironment := append([]string(nil), server.cfg.AgentEnv...)
			beforeAmbient := os.Environ()
			before := readRenameDiscardAPIStatus(t, server)
			file, ok := before.Files["new.txt"]
			require.True(t, ok, "actual registered status must expose the destination")
			require.Equal(t, "old.txt", file.OldPath)
			if edit == "mixed" {
				require.NotNil(t, file.StagedChange)
				require.Equal(t, "renamed", file.StagedChange.Status)
				require.Equal(t, "old.txt", file.StagedChange.OldPath)
			} else {
				require.Equal(t, "renamed", file.Status)
			}
			assertDiscardAPIState(t, root, want)
			want["selected"]["old.txt"] = discardAPIState{indexed: true, exists: true,
				index: strings.Repeat("selected committed source\n", 40), worktree: strings.Repeat("selected committed source\n", 40)}
			want["selected"]["new.txt"] = discardAPIState{}
			want["selected"]["ordinary.txt"] = discardAPIState{indexed: true, exists: true, index: "selected ordinary HEAD\n", worktree: "selected ordinary HEAD\n"}
			want["selected"]["added.txt"], want["selected"]["untracked.txt"] = discardAPIState{}, discardAPIState{}
			rec := postGitAPI(t, server, "/api/v1/git/discard", GitDiscardRequest{Repo: "selected", Paths: []string{"new.txt", "ordinary.txt", "added.txt", "untracked.txt"}})
			require.Equal(t, http.StatusOK, rec.Code)
			result := decodeGitOperationResult(t, rec)
			assert.True(t, result.Success, "result=%+v", result)
			assert.Empty(t, result.Error)
			assertDiscardAPIState(t, root, want)
			after := readRenameDiscardAPIStatus(t, server)
			assert.NotContains(t, after.Files, "old.txt", "restored source must not retain a staged deletion")
			assert.NotContains(t, after.Files, "new.txt")
			for repo, before := range evidence {
				assert.Equal(t, before, renameDiscardAPIEvidence(t, filepath.Join(root, repo)))
			}
			assert.Equal(t, beforeEnvironment, server.cfg.AgentEnv)
			assert.Equal(t, beforeAmbient, os.Environ())
		})
	}
}

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.44, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.47
func TestHandleGitDiscardStagedRenameRefusals(t *testing.T) {
	isolatePlainHTTPEnvironment(t)
	root := t.TempDir()
	want := map[string]map[string]discardAPIState{}
	for _, repo := range []string{"selected", "other"} {
		dir := seedRenameDiscardAPIRepo(t, root, repo, "mixed")
		writeFileAPI(t, dir, "old.txt", repo+" recreated source\n")
		want[repo] = map[string]discardAPIState{}
		for _, path := range []string{"old.txt", "new.txt", "neighbor.txt", "ordinary.txt", "added.txt", "untracked.txt"} {
			want[repo][path] = readDiscardAPIState(t, dir, path)
		}
	}
	server := newDiscardAPIServer(t, root, "1", "1")
	for _, req := range []GitDiscardRequest{
		{Repo: "selected", Paths: []string{"new.txt", "ordinary.txt", "added.txt", "untracked.txt"}},
		{Repo: "selected", Paths: []string{"ordinary.txt", ""}}, {Repo: "selected"},
		{Repo: "selected", Paths: []string{}}, {Repo: "../other", Paths: []string{"new.txt"}},
		{Repo: "missing", Paths: []string{"new.txt"}},
	} {
		rec := postGitAPI(t, server, "/api/v1/git/discard", req)
		result := decodeGitOperationResult(t, rec)
		assert.False(t, result.Success, "request=%+v result=%+v", req, result)
		assert.NotEmpty(t, result.Error)
		assertDiscardAPIState(t, root, want)
	}
}

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.44, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.47
func TestHandleGitDiscardStagedRenameRelativeSelections(t *testing.T) {
	for _, tc := range []struct {
		name     string
		paths    []string
		occupied bool
		refused  bool
	}{
		{name: "relative", paths: []string{"./new.txt", "ordinary.txt", "added.txt", "untracked.txt"}},
		{name: "duplicate-endpoints", paths: []string{"./new.txt", "new.txt", "././new.txt", "./old.txt", "old.txt", "ordinary.txt", "added.txt", "untracked.txt"}},
		{name: "occupied-source", paths: []string{"./new.txt", "ordinary.txt", "added.txt", "untracked.txt"}, occupied: true, refused: true},
		{name: "raw-rejected-before-cleaning", paths: []string{"invalid\x00/../new.txt"}, refused: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolatePlainHTTPEnvironment(t)
			root := t.TempDir()
			want := map[string]map[string]discardAPIState{}
			evidence := map[string][]string{}
			for _, repo := range []string{"selected", "other"} {
				dir := seedRenameDiscardAPIRepo(t, root, repo, "mixed")
				if tc.occupied {
					writeFileAPI(t, dir, "old.txt", repo+" recreated source\n")
				}
				evidence[repo] = renameDiscardAPIEvidence(t, dir)
				want[repo] = map[string]discardAPIState{}
				for _, path := range []string{"old.txt", "new.txt", "neighbor.txt", "ordinary.txt", "added.txt", "untracked.txt"} {
					want[repo][path] = readDiscardAPIState(t, dir, path)
				}
			}
			server := newDiscardAPIServer(t, root, "1", "1")
			beforeEnvironment, beforeAmbient := append([]string(nil), server.cfg.AgentEnv...), os.Environ()
			before := readRenameDiscardAPIStatus(t, server)
			require.Contains(t, before.Files, "new.txt")
			require.NotNil(t, before.Files["new.txt"].StagedChange)
			require.Equal(t, "renamed", before.Files["new.txt"].StagedChange.Status)
			require.Equal(t, "old.txt", before.Files["new.txt"].StagedChange.OldPath)
			assertDiscardAPIState(t, root, want)
			if !tc.refused {
				want["selected"]["old.txt"] = discardAPIState{indexed: true, exists: true,
					index: strings.Repeat("selected committed source\n", 40), worktree: strings.Repeat("selected committed source\n", 40)}
				want["selected"]["ordinary.txt"] = discardAPIState{indexed: true, exists: true,
					index: "selected ordinary HEAD\n", worktree: "selected ordinary HEAD\n"}
				for _, path := range []string{"new.txt", "added.txt", "untracked.txt"} {
					want["selected"][path] = discardAPIState{}
				}
			}
			rec := postGitAPI(t, server, "/api/v1/git/discard", GitDiscardRequest{Repo: "selected", Paths: tc.paths})
			require.Equal(t, http.StatusOK, rec.Code)
			result := decodeGitOperationResult(t, rec)
			assert.Equal(t, !tc.refused, result.Success, "result=%+v", result)
			if tc.refused {
				assert.NotEmpty(t, result.Error)
			} else {
				assert.Empty(t, result.Error)
				after := readRenameDiscardAPIStatus(t, server)
				assert.NotContains(t, after.Files, "old.txt", "no residual staged source deletion")
				assert.NotContains(t, after.Files, "new.txt")
			}
			assertDiscardAPIState(t, root, want)
			for repo, before := range evidence {
				assert.Equal(t, before, renameDiscardAPIEvidence(t, filepath.Join(root, repo)))
			}
			assert.Equal(t, beforeEnvironment, server.cfg.AgentEnv)
			assert.Equal(t, beforeAmbient, os.Environ())
		})
	}
}

func seedRenameDiscardAPIRepo(t *testing.T, root, repo, edit string) string {
	t.Helper()
	dir := filepath.Join(root, repo)
	require.NoError(t, os.Mkdir(dir, 0o755))
	runGitAPI(t, dir, "init", "--initial-branch=main")
	runGitAPI(t, dir, "config", "user.email", "test@test.com")
	runGitAPI(t, dir, "config", "user.name", "HTTP Rename Test")
	runGitAPI(t, dir, "config", "core.autocrlf", "false")
	content := strings.Repeat(repo+" committed source\n", 40)
	writeFileAPI(t, dir, "old.txt", content)
	writeFileAPI(t, dir, "neighbor.txt", repo+" neighbor HEAD\n")
	writeFileAPI(t, dir, "ordinary.txt", repo+" ordinary HEAD\n")
	runGitAPI(t, dir, "add", "-A")
	runGitAPI(t, dir, "commit", "-m", "independent registered rename fixture")
	runGitAPI(t, dir, "mv", "--", "old.txt", "new.txt")
	if edit == "mixed" {
		writeFileAPI(t, dir, "new.txt", content+"staged edit\n")
		runGitAPI(t, dir, "add", "--", "new.txt")
		writeFileAPI(t, dir, "new.txt", content+"staged edit\nworking edit\n")
	}
	for _, path := range []string{"neighbor.txt", "ordinary.txt", "added.txt"} {
		writeFileAPI(t, dir, path, repo+"/"+path+" staged\n")
		runGitAPI(t, dir, "add", "--", path)
		writeFileAPI(t, dir, path, repo+"/"+path+" worktree\n")
	}
	writeFileAPI(t, dir, "untracked.txt", repo+" untracked\n")
	return dir
}

func readRenameDiscardAPIStatus(t *testing.T, server *Server) plainHTTPStatus {
	t.Helper()
	rec := getGitAPI(t, server, "/api/v1/git/status?repo=selected&fresh=true&details=wait")
	require.Equal(t, http.StatusOK, rec.Code)
	var status plainHTTPStatus
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &status))
	require.True(t, status.Success)
	require.True(t, status.FilesComplete)
	require.Equal(t, "selected", status.RepositoryName)
	return status
}

func renameDiscardAPIEvidence(t *testing.T, dir string) []string {
	t.Helper()
	config, err := os.ReadFile(filepath.Join(dir, ".git", "config"))
	require.NoError(t, err)
	return []string{runGitAPI(t, dir, "rev-parse", "HEAD"), runGitAPI(t, dir, "symbolic-ref", "HEAD"),
		runGitAPI(t, dir, "for-each-ref", "--format=%(refname) %(objectname)"), string(config)}
}
