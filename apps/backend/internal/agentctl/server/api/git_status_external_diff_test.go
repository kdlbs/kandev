package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const workspaceHTTPHelperMarker = "--kandev-workspace-http-external-helper"
const workspaceHTTPHelperOutput = "CUSTOM WORKSPACE HTTP DIFF OUTPUT\n"

// The per-repository sentinel proves dispatch even for an instance-wide helper.
func TestWorkspaceHTTPExternalHelperProcess(t *testing.T) {
	if os.Getenv("KANDEV_TEST_WORKSPACE_HTTP_EXTERNAL_HELPER") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == workspaceHTTPHelperMarker && i+1 < len(os.Args) {
			dir, err := os.Getwd()
			require.NoError(t, err)
			path := filepath.Join(os.Args[i+1], filepath.Base(dir)+".sentinel")
			require.NoError(t, os.WriteFile(path, []byte("executed\n"), 0o600))
			fmt.Print(workspaceHTTPHelperOutput)
			os.Exit(0)
		}
	}
	t.Fatal("workspace HTTP helper invocation missing marker and sentinel directory")
}

type workspaceHTTPExternalHelper struct{ command, dir string }

func newWorkspaceHTTPExternalHelper(t *testing.T) workspaceHTTPExternalHelper {
	t.Helper()
	t.Setenv("KANDEV_TEST_WORKSPACE_HTTP_EXTERNAL_HELPER", "1")
	executable, err := os.Executable()
	require.NoError(t, err)
	dir := t.TempDir()
	quote := func(value string) string {
		return "'" + strings.ReplaceAll(filepath.ToSlash(value), "'", "'\\''") + "'"
	}
	return workspaceHTTPExternalHelper{
		command: quote(executable) + " -test.run=^TestWorkspaceHTTPExternalHelperProcess$ -- " +
			workspaceHTTPHelperMarker + " " + quote(dir),
		dir: dir,
	}
}

func (helper workspaceHTTPExternalHelper) prove(t *testing.T, repo string, env []string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "diff", "--ext-diff", "HEAD", "--", "new[ab].txt")
	cmd.Dir, cmd.Env = repo, append([]string(nil), env...)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "HTTP helper positive control: %s", output)
	require.Equal(t, workspaceHTTPHelperOutput, string(output))
	path := filepath.Join(helper.dir, filepath.Base(repo)+".sentinel")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "executed\n", string(data))
	require.NoError(t, os.Remove(path))
	t.Log("actual repository-qualified HTTP helper positive control passed")
}

func (helper workspaceHTTPExternalHelper) assertAbsent(t *testing.T, repo string) {
	t.Helper()
	_, err := os.Stat(filepath.Join(helper.dir, repo+".sentinel"))
	assert.True(t, os.IsNotExist(err), "external HTTP helper executed for %s: %v", repo, err)
}

type workspaceHTTPExternalFixture struct {
	root   string
	paths  map[string]string
	cfg    *config.InstanceConfig
	server *Server
	helper [3]workspaceHTTPExternalHelper
}

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.9, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.36, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.45, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.46
func TestGitStatusHTTPExternalHelpers(t *testing.T) {
	for _, mode := range []string{"ordinary", "configured", "environment", "both"} {
		for _, layer := range []string{"unstaged", "staged", "mixed"} {
			t.Run(mode+"/"+layer, func(t *testing.T) {
				fixture := newWorkspaceHTTPExternalFixture(t, mode, layer)
				selected := readWorkspaceHTTPExternalSelected(t, fixture, "fresh=true&details=wait")
				assertPlainHTTPStatus(t, selected, "selected", layer, fixture.paths)
				aggregate := readWorkspaceHTTPExternalAggregate(t, fixture, "fresh=true&details=wait")
				for repo, status := range aggregate {
					assertPlainHTTPStatus(t, status, repo, layer, fixture.paths)
				}
				require.Equal(t, selected, readWorkspaceHTTPExternalSelected(t, fixture, "mode=replay"))
				require.Equal(t, aggregate, readWorkspaceHTTPExternalAggregate(t, fixture, "mode=replay"))
			})
		}
	}
}

func newWorkspaceHTTPExternalFixture(t *testing.T, mode, layer string) workspaceHTTPExternalFixture {
	t.Helper()
	isolatePlainHTTPEnvironment(t)
	root := t.TempDir()
	paths := map[string]string{"new[ab].txt": "-selected", "newa.txt": "-sibling",
		"ansi.txt": "-\x1b[31mliteral\x1b[0m", "日本語.txt": "-unicode"}
	if runtime.GOOS != "windows" {
		paths["escape\x1bname.txt"] = "-escape-name"
	}
	helper := [3]workspaceHTTPExternalHelper{
		newWorkspaceHTTPExternalHelper(t), newWorkspaceHTTPExternalHelper(t), newWorkspaceHTTPExternalHelper(t),
	}
	t.Setenv("GIT_LITERAL_PATHSPECS", "1")
	setPlainHTTPEnvironmentColor(t, "always")
	env := append([]string(nil), os.Environ()...)
	for i, repo := range []string{"selected", "other"} {
		seedPlainHTTPRepo(t, root, repo, layer, paths)
		dir := filepath.Join(root, repo)
		configurePlainHTTPColor(t, dir, "diff-always")
		configureWorkspaceHTTPExternal(t, dir, mode, env, helper[i], helper[2])
	}
	if mode == "environment" || mode == "both" {
		env = append(env, "GIT_EXTERNAL_DIFF="+helper[2].command)
	}
	cfg := &config.InstanceConfig{WorkDir: root, AgentEnv: append([]string(nil), env...),
		BaseBranches: map[string]string{"selected": "main", "other": "main"}}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	require.NoError(t, err)
	manager := process.NewManager(cfg, log)
	t.Cleanup(func() { require.NoError(t, manager.StopForTeardown(context.Background())) })
	server := NewServer(cfg, manager, nil, nil, log)
	setPlainHTTPEnvironmentColor(t, "false")
	t.Setenv("GIT_EXTERNAL_DIFF", "")
	return workspaceHTTPExternalFixture{root: root, paths: paths, cfg: cfg, server: server, helper: helper}
}

func configureWorkspaceHTTPExternal(t *testing.T, dir, mode string, env []string, configured, environment workspaceHTTPExternalHelper) {
	t.Helper()
	if mode == "configured" || mode == "both" {
		runGitAPI(t, dir, "config", "diff.external", configured.command)
		configured.prove(t, dir, env)
	}
	if mode == "environment" || mode == "both" {
		if mode == "both" {
			runGitAPI(t, dir, "config", "--unset", "diff.external")
		}
		environment.prove(t, dir, append(append([]string(nil), env...), "GIT_EXTERNAL_DIFF="+environment.command))
		if mode == "both" {
			runGitAPI(t, dir, "config", "diff.external", configured.command)
		}
	}
}

func workspaceHTTPExternalReadGuard(t *testing.T, fixture workspaceHTTPExternalFixture) func() {
	t.Helper()
	before := workspaceHTTPExternalEvidence(t, fixture)
	env, live := append([]string(nil), fixture.cfg.AgentEnv...), os.Environ()
	return func() {
		t.Helper()
		for _, repo := range []string{"selected", "other"} {
			for _, helper := range fixture.helper {
				helper.assertAbsent(t, repo)
			}
		}
		assert.Equal(t, before, workspaceHTTPExternalEvidence(t, fixture))
		assert.Equal(t, env, fixture.cfg.AgentEnv)
		assert.Equal(t, live, os.Environ())
	}
}

func workspaceHTTPExternalEvidence(t *testing.T, fixture workspaceHTTPExternalFixture) map[string]string {
	t.Helper()
	evidence := plainHTTPReadEvidence(t, fixture.root, fixture.paths)
	for _, repo := range []string{"selected", "other"} {
		dir := filepath.Join(fixture.root, repo)
		data, err := os.ReadFile(filepath.Join(dir, ".git", "index"))
		require.NoError(t, err)
		evidence[repo+":raw-index"] = string(data)
		evidence[repo+":dirty"] = runGitAPI(t, dir, "--no-optional-locks", "status", "--porcelain=v1", "-z")
		evidence[repo+":patch"] = runGitAPI(t, dir, "--no-optional-locks", "diff", "--no-ext-diff", "--no-color", "--binary", "HEAD")
	}
	return evidence
}

func readWorkspaceHTTPExternalSelected(t *testing.T, fixture workspaceHTTPExternalFixture, query string) plainHTTPStatus {
	t.Helper()
	defer workspaceHTTPExternalReadGuard(t, fixture)()
	rec := getGitAPI(t, fixture.server, "/api/v1/git/status?repo=selected&"+query)
	require.Equal(t, http.StatusOK, rec.Code)
	var status plainHTTPStatus
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &status))
	return status
}

func readWorkspaceHTTPExternalAggregate(t *testing.T, fixture workspaceHTTPExternalFixture, query string) map[string]plainHTTPStatus {
	t.Helper()
	defer workspaceHTTPExternalReadGuard(t, fixture)()
	rec := getGitAPI(t, fixture.server, "/api/v1/git/status/multi?"+query)
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
	result := make(map[string]plainHTTPStatus)
	for _, repo := range aggregate.Repos {
		require.NotContains(t, result, repo.RepositoryName)
		result[repo.RepositoryName] = repo.Status
	}
	require.Contains(t, result, "selected")
	require.Contains(t, result, "other")
	return result
}
