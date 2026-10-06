package api

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
)

const externalHTTPPath = "new file mode.txt"
const externalHTTPHelperMarker = "--kandev-http-external-helper"

func TestCumulativeHTTPExternalHelperProcess(t *testing.T) {
	if os.Getenv("KANDEV_TEST_HTTP_EXTERNAL_HELPER") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == externalHTTPHelperMarker && i+1 < len(os.Args) {
			if err := os.WriteFile(os.Args[i+1], []byte("executed\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			fmt.Print("CUSTOM HTTP DIFF OUTPUT\n")
			os.Exit(0)
		}
	}
	t.Fatal("HTTP helper invocation missing marker and sentinel")
}

type externalHTTPHelper struct{ command, sentinel string }

func newExternalHTTPHelper(t *testing.T) externalHTTPHelper {
	t.Helper()
	t.Setenv("KANDEV_TEST_HTTP_EXTERNAL_HELPER", "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(t.TempDir(), "HTTP helper execution.txt")
	quote := func(value string) string {
		return "'" + strings.ReplaceAll(filepath.ToSlash(value), "'", "'\\''") + "'"
	}
	return externalHTTPHelper{
		command:  quote(executable) + " -test.run=^TestCumulativeHTTPExternalHelperProcess$ -- " + externalHTTPHelperMarker + " " + quote(sentinel),
		sentinel: sentinel,
	}
}

func externalHTTPEnvironment() []string {
	env := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GIT_") {
			env = append(env, entry)
		}
	}
	return env
}

func (h externalHTTPHelper) prove(t *testing.T, repo statusMetadataHTTPRepo, env []string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", "diff", repo.base, "--", externalHTTPPath)
	cmd.Dir = repo.dir
	cmd.Env = append(append([]string(nil), env...), "GIT_EXTERNAL_DIFF="+h.command)
	output, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "CUSTOM HTTP DIFF OUTPUT\n") {
		t.Fatalf("HTTP helper positive control: %q, err=%v", output, err)
	}
	data, err := os.ReadFile(h.sentinel)
	if err != nil || string(data) != "executed\n" {
		t.Fatalf("HTTP helper execution control: %q, err=%v", data, err)
	}
	if err := os.Remove(h.sentinel); err != nil {
		t.Fatal(err)
	}
}

func (h externalHTTPHelper) assertAbsent(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(h.sentinel); !os.IsNotExist(err) {
		t.Errorf("external HTTP helper executed during built-in read: %v", err)
	}
}

func externalHTTPState(t *testing.T, repo string) string {
	t.Helper()
	configBytes, err := os.ReadFile(filepath.Join(repo, ".git", "config"))
	if err != nil {
		t.Fatal(err)
	}
	state := string(configBytes) + runGitAPI(t, repo, "rev-parse", "HEAD") + runGitAPI(t, repo, "show-ref") +
		runGitAPI(t, repo, "ls-files", "--stage", "-z") + runGitAPI(t, repo, "--no-optional-locks", "status", "--porcelain=v1", "-z") +
		runGitAPI(t, repo, "diff", "--no-ext-diff", "--no-color", "--binary", "HEAD")
	for _, path := range strings.Split(runGitAPI(t, repo, "ls-files", "-z"), "\x00") {
		if path == "" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(repo, path))
		if err != nil {
			t.Fatal(err)
		}
		state += path + "\x00" + string(data) + "\x00"
	}
	return state
}

// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.3
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.8
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.9
func TestCumulativeDiffExternalHelpersHTTP(t *testing.T) {
	for _, mode := range []string{"plain", "configured", "environment", "both"} {
		t.Run(mode, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("XDG_CONFIG_HOME", home)
			root := t.TempDir()
			alpha := seedStatusMetadataHTTPRepo(t, root, "alpha", externalHTTPPath, false)
			beta := seedStatusMetadataHTTPRepo(t, root, "beta", externalHTTPPath, false)
			configured, environment := newExternalHTTPHelper(t), newExternalHTTPHelper(t)
			env := externalHTTPEnvironment()
			configured.prove(t, alpha, env)
			environment.prove(t, beta, env)
			if mode == "environment" || mode == "both" {
				env = append(env, "GIT_EXTERNAL_DIFF="+environment.command)
			}
			for _, repo := range []statusMetadataHTTPRepo{alpha, beta} {
				if mode == "configured" || mode == "both" {
					runGitAPI(t, repo.dir, "config", "diff.external", configured.command)
				}
				before := externalHTTPState(t, repo.dir)
				t.Cleanup(func() {
					if externalHTTPState(t, repo.dir) != before {
						t.Errorf("HTTP comparison changed repository %s", repo.name)
					}
				})
			}
			log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
			if err != nil {
				t.Fatal(err)
			}
			cfg := &config.InstanceConfig{WorkDir: root, AgentEnv: append([]string(nil), env...), BaseBranches: map[string]string{"alpha": "main", "beta": "main"}}
			manager := process.NewManager(cfg, log)
			t.Cleanup(func() { _ = manager.StopForTeardown(context.Background()) })
			server := NewServer(cfg, manager, nil, nil, log)
			t.Cleanup(func() {
				configured.assertAbsent(t)
				environment.assertAbsent(t)
				if !reflect.DeepEqual(cfg.AgentEnv, env) {
					t.Error("HTTP comparison changed captured helper environment")
				}
			})
			for _, repo := range []statusMetadataHTTPRepo{alpha, beta} {
				t.Run(repo.name, func(t *testing.T) { assertExternalHTTPSelected(t, server, repo) })
			}
			t.Run("aggregate", func(t *testing.T) {
				var result process.CumulativeDiffResult
				readStatusMetadataHTTP(t, server, "/api/v1/git/cumulative-diff?base="+alpha.base, &result)
				if !result.Success || len(result.Files) != 2 || result.TotalCommits != 2 || result.TruncatedFilesCount != 0 {
					t.Fatalf("aggregate cumulative = %+v", result)
				}
				for _, repo := range []statusMetadataHTTPRepo{alpha, beta} {
					patch := runGitAPI(t, repo.dir, "diff", "--no-ext-diff", "--no-color", "--src-prefix=a/", "--dst-prefix=b/", repo.base)
					entry := assertStatusMetadataHTTPFile(t, result.Files, repo.name+"\x00"+externalHTTPPath, externalHTTPPath, "modified", 2, 1, patch)
					if entry["repository_name"] != repo.name || entry["base_ref"] != repo.base {
						t.Errorf("aggregate repository identity = %#v", entry)
					}
					if _, present := entry["is_submodule"]; present {
						t.Errorf("ordinary repository marked as submodule: %#v", entry)
					}
				}
			})
			// The single-repository route shares the captured environment contract.
			singleCfg := &config.InstanceConfig{WorkDir: alpha.dir, AgentEnv: append([]string(nil), env...)}
			singleManager := process.NewManager(singleCfg, log)
			t.Cleanup(func() { _ = singleManager.StopForTeardown(context.Background()) })
			singleServer := NewServer(singleCfg, singleManager, nil, nil, log)
			t.Run("single", func(t *testing.T) {
				var result process.CumulativeDiffResult
				readStatusMetadataHTTP(t, singleServer, "/api/v1/git/cumulative-diff?base="+alpha.base, &result)
				assertExternalHTTPResult(t, result, alpha)
			})
		})
	}
}

func assertExternalHTTPSelected(t *testing.T, server *Server, repo statusMetadataHTTPRepo) {
	t.Helper()
	patch := runGitAPI(t, repo.dir, "diff", "--no-ext-diff", "--no-color", "--src-prefix=a/", "--dst-prefix=b/", repo.base)
	t.Run("commit_control", func(t *testing.T) {
		var result process.CommitDiffResult
		readStatusMetadataHTTP(t, server, "/api/v1/git/commit/"+repo.head+"?repo="+repo.name, &result)
		if !result.Success || result.CommitSHA != repo.head || len(result.Files) != 1 || result.FilesChanged != 1 || result.Insertions != 2 || result.Deletions != 1 {
			t.Fatalf("selected commit control = %+v", result)
		}
		assertStatusMetadataHTTPFile(t, result.Files, externalHTTPPath, externalHTTPPath, "modified", 2, 1, patch)
	})
	t.Run("cumulative", func(t *testing.T) {
		var result process.CumulativeDiffResult
		readStatusMetadataHTTP(t, server, "/api/v1/git/cumulative-diff?base="+repo.base+"&repo="+repo.name, &result)
		assertExternalHTTPResult(t, result, repo)
	})
}

func assertExternalHTTPResult(t *testing.T, result process.CumulativeDiffResult, repo statusMetadataHTTPRepo) {
	t.Helper()
	if !result.Success || len(result.Files) != 1 || result.BaseCommit != repo.base || result.HeadCommit != repo.head || result.TotalCommits != 1 || result.TruncatedFilesCount != 0 {
		t.Fatalf("selected cumulative = %+v", result)
	}
	patch := runGitAPI(t, repo.dir, "diff", "--no-ext-diff", "--no-color", "--src-prefix=a/", "--dst-prefix=b/", repo.base)
	assertStatusMetadataHTTPFile(t, result.Files, externalHTTPPath, externalHTTPPath, "modified", 2, 1, patch)
}
