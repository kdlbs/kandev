package api

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// @covers AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.1
func TestGitStatusHTTPReturnsRecoveredEnrichedCapture(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the Git mutation wrapper uses a POSIX shell")
	}
	fixture := newGitAPIFixture(t)
	writeFileAPI(t, fixture.repo, "http-capture-race.txt", "captured from the corrected index\n")
	statusCalls := installGitCaptureMutationShim(t, fixture.repo, "http-capture-race.txt", false)

	rec := getGitAPI(t, fixture.server, "/api/v1/git/status?fresh=true&details=wait")
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var result GitStatusResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if !result.Success || result.StatusState != "ready" || !result.FilesComplete || result.DetailState != "ready" {
		t.Fatalf("HTTP status quality = %+v, want complete enriched status", result)
	}
	file, ok := result.Files["http-capture-race.txt"].(map[string]interface{})
	if !ok || file["staged"] != true || !strings.Contains(fmt.Sprint(file["diff"]), "mutation-1") {
		t.Fatalf("HTTP recovered file = %#v, want staged detail from the correction", result.Files["http-capture-race.txt"])
	}
	if got := statusCalls(); got != 2 {
		t.Fatalf("tracked status invocations = %d, want two basic captures", got)
	}
}

// @covers AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.2
func TestGitStatusHTTPWarnsOnceAfterContinuousCaptureChanges(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the Git mutation wrapper uses a POSIX shell")
	}
	fixture := newGitAPIFixture(t)
	writeFileAPI(t, fixture.repo, "http-capture-churn.txt", "initial\n")
	log, observed := newObservedGitAPILogger(t)
	fixture.server.logger = log
	statusCalls := installGitCaptureMutationShim(t, fixture.repo, "http-capture-churn.txt", true)

	rec := getGitAPI(t, fixture.server, "/api/v1/git/status?fresh=true&details=wait")
	if rec.Code != 200 {
		t.Fatalf("status = %d, want bounded unavailable response (body %s)", rec.Code, rec.Body.String())
	}
	var result GitStatusResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if result.Success || result.StatusState != "unavailable" || result.ErrorCode != "status_unavailable" {
		t.Fatalf("unstable HTTP status = %+v, want unavailable", result)
	}
	if got := statusCalls(); got != 2 {
		t.Fatalf("tracked status invocations = %d, want two bounded basic captures", got)
	}
	warnings := observed.FilterMessage("git status capture failed due to changing repository evidence").FilterLevelExact(zapcore.WarnLevel)
	if warnings.Len() != 1 {
		t.Fatalf("unstable capture warnings = %d, want one", warnings.Len())
	}
}

// @covers AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.3
func TestGitStatusMultiRetriesOnlyTheRepositoryWithChangedEvidence(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the Git mutation wrapper uses a POSIX shell")
	}
	server, repoNames := newMultiRepoStatusServer(t)
	root := server.cfg.WorkDir
	targetRepo := filepath.Join(root, repoNames[0])
	writeFileAPI(t, targetRepo, "multi-capture-race.txt", "initial\n")
	statusCalls := installGitCaptureMutationShimForRepo(t, targetRepo, "multi-capture-race.txt", false)

	rec := getGitAPI(t, server, "/api/v1/git/status/multi?fresh=true&details=wait")
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var result MultiRepoGitStatusResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode multi status response: %v", err)
	}
	statuses := make(map[string]GitStatusResult, len(result.Repos))
	for _, repoStatus := range result.Repos {
		statuses[repoStatus.RepositoryName] = repoStatus.Status
	}
	if len(statuses) != len(repoNames) {
		t.Fatalf("multi-repository status count = %d, want %d", len(statuses), len(repoNames))
	}
	for _, repoName := range repoNames {
		status := statuses[repoName]
		if !status.Success || status.StatusState != "ready" || status.DetailState != "ready" {
			t.Fatalf("repository %q status = %+v, want complete enriched status", repoName, status)
		}
		wantCalls := 1
		if repoName == repoNames[0] {
			wantCalls = 2
			file, ok := status.Files["multi-capture-race.txt"].(map[string]interface{})
			if !ok || file["staged"] != true {
				t.Fatalf("recovered repository file = %#v, want staged file", status.Files["multi-capture-race.txt"])
			}
		}
		if got := statusCalls(filepath.Join(root, repoName)); got != wantCalls {
			t.Fatalf("repository %q tracked status captures = %d, want %d", repoName, got, wantCalls)
		}
	}
}

func newObservedGitAPILogger(t *testing.T) (*logger.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, observed := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("create observed logger: %v", err)
	}
	return log.WithFields(zap.String("component", "api-server")), observed
}

func installGitCaptureMutationShim(t *testing.T, repo, path string, continuous bool) func() int {
	t.Helper()
	countRepo := installGitCaptureMutationShimForRepo(t, repo, path, continuous)
	return func() int { return countRepo(repo) }
}

func installGitCaptureMutationShimForRepo(t *testing.T, targetRepo, path string, continuous bool) func(string) int {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("POSIX shell unavailable: %v", err)
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("find Git executable: %v", err)
	}
	binDir := t.TempDir()
	marker := filepath.Join(binDir, "mutated-once")
	calls := filepath.Join(binDir, "status-calls")
	mutations := filepath.Join(binDir, "mutation-count")
	filePath := filepath.Join(targetRepo, path)
	targetCondition := "true"
	if targetRepo != "" {
		targetCondition = fmt.Sprintf("[ \"$PWD\" = %s ]", shellQuote(targetRepo))
	}
	mutateCondition := fmt.Sprintf("[ ! -e %s ]", shellQuote(marker))
	if continuous {
		mutateCondition = "true"
	}
	script := fmt.Sprintf("#!/bin/sh\ncase \" $* \" in\n  *\" status --porcelain --untracked-files=no \"*)\n    printf '%%s\\n' \"$PWD\" >> %s\n    if %s && %s; then\n      : > %s\n      printf x >> %s\n      count=$(wc -c < %s)\n      printf 'mutation-%%s\\n' \"$count\" > %s\n      env -u GIT_INDEX_FILE %s add %s\n    fi\n    ;;\nesac\nexec %s \"$@\"\n",
		shellQuote(calls), targetCondition, mutateCondition, shellQuote(marker), shellQuote(mutations), shellQuote(mutations), shellQuote(filePath), shellQuote(realGit), shellQuote(filePath), shellQuote(realGit))
	shim := filepath.Join(binDir, "git")
	if err := os.WriteFile(shim, []byte(script), 0o755); err != nil {
		t.Fatalf("write Git command wrapper: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func(repo string) int {
		data, err := os.ReadFile(calls)
		if err != nil {
			return 0
		}
		count := 0
		for _, recordedRepo := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if recordedRepo == repo {
				count++
			}
		}
		return count
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
