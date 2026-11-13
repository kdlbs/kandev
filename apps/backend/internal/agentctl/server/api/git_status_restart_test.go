package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/pkg/agent"
)

const gitStatusRestartHelperEnv = "KANDEV_API_GIT_STATUS_RESTART_HELPER"

// TestGitStatusRestartHelper is a disposable agent process: it keeps the ACP
// stdin pipe open until the manager stops the lifecycle.
func TestGitStatusRestartHelper(t *testing.T) {
	if os.Getenv(gitStatusRestartHelperEnv) == "" {
		return
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
}

func newGitStatusRestartServer(t *testing.T, workDir string) *Server {
	t.Helper()
	log := newTestLogger()
	cfg := &config.InstanceConfig{WorkDir: workDir, Protocol: agent.ProtocolACP}
	manager := process.NewManager(cfg, log)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := manager.StopForTeardown(ctx); err != nil {
			t.Errorf("stop git status restart fixture: %v", err)
		}
	})
	return NewServer(cfg, manager, nil, nil, log)
}

func startGitStatusRestartAgent(t *testing.T, manager *process.Manager) {
	t.Helper()
	args := []string{os.Args[0], "-test.run=^TestGitStatusRestartHelper$"}
	err := manager.Configure(
		os.Args[0],
		args,
		true,
		map[string]string{gitStatusRestartHelperEnv: "1"},
		"",
		nil,
		false,
	)
	if err != nil {
		t.Fatalf("configure restart helper: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := manager.StartWithGeneration(ctx); err != nil {
		t.Fatalf("start restart helper: %v", err)
	}
}

func stopGitStatusRestartAgent(t *testing.T, manager *process.Manager) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := manager.Stop(ctx); err != nil {
		t.Fatalf("stop restart helper: %v", err)
	}
}

func decodeGitStatusRestart(t *testing.T, server *Server, path string) GitStatusResult {
	t.Helper()
	rec := getGitAPI(t, server, path)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s status = %d: %s", path, rec.Code, rec.Body.String())
	}
	var result GitStatusResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("%s decode: %v", path, err)
	}
	return result
}

func decodeMultiGitStatusRestart(t *testing.T, server *Server, path string) MultiRepoGitStatusResult {
	t.Helper()
	rec := getGitAPI(t, server, path)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s status = %d: %s", path, rec.Code, rec.Body.String())
	}
	var result MultiRepoGitStatusResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("%s decode: %v", path, err)
	}
	return result
}

func assertRestartGitFile(t *testing.T, files map[string]interface{}, path, marker string) {
	t.Helper()
	if marker == "" {
		t.Fatal("assertRestartGitFile requires a non-empty marker")
	}
	raw, ok := files[path]
	if !ok {
		t.Fatalf("git status files missing %q: %v", path, files)
	}
	file, ok := raw.(map[string]interface{})
	if !ok {
		t.Fatalf("file %q payload = %T", path, raw)
	}
	diff, _ := file["diff"].(string)
	if !strings.Contains(diff, marker) {
		t.Fatalf("file %q diff does not contain %q: %q", path, marker, diff)
	}
}

func TestGitStatusAfterManagerRestart(t *testing.T) {
	t.Run("single repository", func(t *testing.T) {
		repoDir := newStatusTestRepo(t, t.TempDir(), "repo")
		server := newGitStatusRestartServer(t, repoDir)
		manager := server.procMgr

		startGitStatusRestartAgent(t, manager)
		before := decodeGitStatusRestart(t, server, "/api/v1/git/status?fresh=true&details=wait")
		if !before.Success || before.StatusState != "ready" {
			t.Fatalf("initial single status = %+v", before)
		}
		beforeMulti := decodeMultiGitStatusRestart(t, server, "/api/v1/git/status/multi?fresh=true&details=wait")
		if !beforeMulti.Success || len(beforeMulti.Repos) != 1 || !beforeMulti.Repos[0].Status.Success {
			t.Fatalf("initial multi status = %+v", beforeMulti)
		}

		stopGitStatusRestartAgent(t, manager)
		writeFileAPI(t, repoDir, "README.md", "GIT_STATUS_RESTART_SINGLE\n")
		startGitStatusRestartAgent(t, manager)

		after := decodeGitStatusRestart(t, server, "/api/v1/git/status?fresh=true&details=wait")
		if !after.Success || after.StatusState != "ready" || !after.FilesComplete {
			t.Fatalf("single status after restart = %+v", after)
		}
		if after.TrackerID == "" || after.TrackerID == before.TrackerID {
			t.Fatalf("tracker identity after restart = %q, before %q", after.TrackerID, before.TrackerID)
		}
		if after.DetailState != "ready" {
			t.Fatalf("single detail state after restart = %q", after.DetailState)
		}
		assertRestartGitFile(t, after.Files, "README.md", "GIT_STATUS_RESTART_SINGLE")

		afterMulti := decodeMultiGitStatusRestart(t, server, "/api/v1/git/status/multi?fresh=true&details=wait")
		if !afterMulti.Success || len(afterMulti.Repos) != 1 {
			t.Fatalf("multi status after restart = %+v", afterMulti)
		}
		repo := afterMulti.Repos[0]
		if !repo.Status.Success || !repo.Status.FilesComplete || repo.Status.DetailState != "ready" {
			t.Fatalf("multi repository status after restart = %+v", repo.Status)
		}
		if repo.Status.TrackerID == "" || repo.Status.TrackerID == beforeMulti.Repos[0].Status.TrackerID {
			t.Fatalf("multi tracker identity after restart = %q", repo.Status.TrackerID)
		}
		assertRestartGitFile(t, repo.Status.Files, "README.md", "GIT_STATUS_RESTART_SINGLE")
	})

	t.Run("multiple repositories", func(t *testing.T) {
		taskRoot := t.TempDir()
		alpha := newStatusTestRepo(t, taskRoot, "alpha")
		beta := newStatusTestRepo(t, taskRoot, "beta")
		server := newGitStatusRestartServer(t, taskRoot)
		manager := server.procMgr

		startGitStatusRestartAgent(t, manager)
		before := decodeMultiGitStatusRestart(t, server, "/api/v1/git/status/multi?fresh=true&details=wait")
		if !before.Success || len(before.Repos) != 2 {
			t.Fatalf("initial multi status = %+v", before)
		}
		beforeTrackers := map[string]string{}
		for _, repo := range before.Repos {
			if !repo.Status.Success {
				t.Fatalf("initial %s status failed: %+v", repo.RepositoryName, repo.Status)
			}
			beforeTrackers[repo.RepositoryName] = repo.Status.TrackerID
		}

		stopGitStatusRestartAgent(t, manager)
		writeFileAPI(t, alpha, "README.md", "GIT_STATUS_RESTART_ALPHA\n")
		writeFileAPI(t, beta, "README.md", "GIT_STATUS_RESTART_BETA\n")
		startGitStatusRestartAgent(t, manager)

		after := decodeMultiGitStatusRestart(t, server, "/api/v1/git/status/multi?fresh=true&details=wait")
		if !after.Success || len(after.Repos) != 2 {
			t.Fatalf("multi status after restart = %+v", after)
		}
		markers := map[string]string{
			"alpha": "GIT_STATUS_RESTART_ALPHA",
			"beta":  "GIT_STATUS_RESTART_BETA",
		}
		for _, repo := range after.Repos {
			if !repo.Status.Success || !repo.Status.FilesComplete || repo.Status.DetailState != "ready" {
				t.Fatalf("%s status after restart = %+v", repo.RepositoryName, repo.Status)
			}
			if repo.Status.RepositoryName != repo.RepositoryName {
				t.Fatalf("status repository name = %q, want %q", repo.Status.RepositoryName, repo.RepositoryName)
			}
			if repo.Status.TrackerID == "" || repo.Status.TrackerID == beforeTrackers[repo.RepositoryName] {
				t.Fatalf("%s tracker identity after restart = %q", repo.RepositoryName, repo.Status.TrackerID)
			}
			assertRestartGitFile(t, repo.Status.Files, "README.md", markers[repo.RepositoryName])
		}
	})
}
