package process

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func prepareGitLabPRRepo(t *testing.T, remoteURL string) (*GitOperator, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell wrapper test is Unix-only")
	}
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	runGit(t, repoDir, "checkout", "-b", "feature/gitlab-rest")
	writeFile(t, repoDir, "gitlab-rest.txt", "gitlab rest\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "add gitlab rest change")
	scriptDir := t.TempDir()
	writeGitRemoteWrapper(t, scriptDir, remoteURL)
	t.Setenv("PATH", scriptDir)
	return NewGitOperator(repoDir, newTestLogger(t), nil), scriptDir
}

func TestGitOperatorCreatePR_GitLabRESTUsesJSONAndProjectDefaultBranch(t *testing.T) {
	var createBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("PRIVATE-TOKEN"); got != "workspace-token" {
			t.Errorf("PRIVATE-TOKEN = %q", got)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/group%2Fwidgets":
			_ = json.NewEncoder(w).Encode(map[string]any{"default_branch": "develop"})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/merge_requests"):
			_ = json.NewEncoder(w).Encode([]any{})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/merge_requests"):
			if err := json.NewDecoder(r.Body).Decode(&createBody); err != nil {
				t.Errorf("decode request: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"web_url": serverURL(r) + "/group/widgets/-/merge_requests/7",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	op, _ := prepareGitLabPRRepo(t, server.URL+"/group/widgets.git")
	t.Setenv("KANDEV_GITLAB_HOST", server.URL)
	t.Setenv("GITLAB_TOKEN", "workspace-token")
	result, err := op.CreatePR(context.Background(), "Quoted \"title\"\nline", "Body \"value\"", "", true)
	if err != nil {
		t.Fatalf("CreatePR: %v", err)
	}
	if !result.Success || result.Provider != "gitlab" {
		t.Fatalf("result = %+v", result)
	}
	if createBody["target_branch"] != "develop" || createBody["source_branch"] != "feature/gitlab-rest" {
		t.Fatalf("branch body = %#v", createBody)
	}
	if createBody["title"] != "Draft: Quoted \"title\"\nline" {
		t.Fatalf("title body = %#v", createBody["title"])
	}
}

func TestGitOperatorCreatePR_GitLabRESTRetryReusesExistingMR(t *testing.T) {
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/group%2Fwidgets":
			_ = json.NewEncoder(w).Encode(map[string]any{"default_branch": "main"})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/merge_requests"):
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"web_url":       serverURL(r) + "/group/widgets/-/merge_requests/9",
				"target_branch": "main",
			}})
		case r.Method == http.MethodPost:
			posts.Add(1)
			http.Error(w, "duplicate", http.StatusConflict)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	op, _ := prepareGitLabPRRepo(t, "ssh://git@"+strings.TrimPrefix(server.URL, "http://")+"/group/widgets.git")
	t.Setenv("KANDEV_GITLAB_HOST", server.URL)
	t.Setenv("GITLAB_TOKEN", "workspace-token")
	result, err := op.CreatePR(context.Background(), "Retry", "Body", "main", false)
	if err != nil || !result.Success || !strings.HasSuffix(result.PRURL, "/merge_requests/9") {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if posts.Load() != 0 {
		t.Fatalf("create requests = %d, want 0", posts.Load())
	}
}

func TestGitOperatorCreatePR_DoesNotSendGitLabTokenToMismatchedHost(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	op, _ := prepareGitLabPRRepo(t, "https://gitlab.com/acme/widgets.git")
	t.Setenv("KANDEV_GITLAB_HOST", server.URL)
	t.Setenv("GITLAB_TOKEN", "must-not-leak")
	result, err := op.CreatePR(context.Background(), "Title", "Body", "main", false)
	if err != nil {
		t.Fatalf("CreatePR: %v", err)
	}
	if result.Success || !strings.Contains(strings.ToLower(result.Error), "host") {
		t.Fatalf("result = %+v", result)
	}
	if calls.Load() != 0 {
		t.Fatalf("configured host received %d requests", calls.Load())
	}
}

func TestGitOperatorCreatePRValidatesGitLabOriginBeforePush(t *testing.T) {
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("find git: %v", err)
	}
	op, scriptDir := prepareGitLabPRRepo(t, "https://gitlab.com/acme/widgets.git")
	pushMarker := filepath.Join(scriptDir, "push-called")
	writeExecutable(t, filepath.Join(scriptDir, "git"), fmt.Sprintf(`#!/bin/sh
if [ "$1" = "remote" ] && [ "$2" = "get-url" ]; then
  printf 'https://gitlab.com/acme/widgets.git\n'
  exit 0
fi
if [ "$1" = "push" ]; then
  touch %q
  exit 0
fi
exec %q "$@"
`, pushMarker, realGit))
	t.Setenv(gitLabHostEnv, "https://gitlab.internal")
	t.Setenv(gitLabTokenEnv, "must-not-leak")

	result, createErr := op.CreatePR(context.Background(), "Title", "Body", "main", false)
	if createErr != nil {
		t.Fatalf("CreatePR: %v", createErr)
	}
	if result.Success || !strings.Contains(strings.ToLower(result.Error), "host") {
		t.Fatalf("result = %+v", result)
	}
	if _, statErr := os.Stat(pushMarker); !os.IsNotExist(statErr) {
		t.Fatalf("push ran before origin validation: %v", statErr)
	}
}

func TestSanitizeGitPushOutputRedactsCredentialBearingURLsAndTokens(t *testing.T) {
	t.Setenv(gitLabTokenEnv, "process-token")
	op := NewGitOperator(t.TempDir(), newTestLogger(t), nil)
	op.setEnvironmentProvider(func() []string { return []string{gitLabTokenEnv + "=workspace-token"} })
	got := op.sanitizeGitPushOutput(
		"remote: https://oauth2:embedded-token@gitlab.example/g/r.git process-token workspace-token",
	)
	for _, secret := range []string{"oauth2", "embedded-token", "process-token", "workspace-token"} {
		if strings.Contains(got, secret) {
			t.Fatalf("sanitized push output contains %q: %q", secret, got)
		}
	}
}

func TestGitOperatorCreatePRReturnsOnlySanitizedPushFailure(t *testing.T) {
	op, scriptDir := prepareGitLabPRRepo(t, "https://gitlab.com/acme/widgets.git")
	writeExecutable(t, filepath.Join(scriptDir, "git"), `#!/bin/sh
if [ "$1" = "remote" ] && [ "$2" = "get-url" ]; then
  printf 'https://gitlab.com/acme/widgets.git\n'
  exit 0
fi
if [ "$1" = "push" ]; then
  printf 'fatal: https://oauth2:embedded-token@gitlab.com/acme/widgets.git workspace-token\n' >&2
  exit 1
fi
exit 9
`)
	t.Setenv(gitLabTokenEnv, "workspace-token")

	result, err := op.CreatePR(context.Background(), "Sensitive title", "Sensitive body", "main", false)
	if err != nil {
		t.Fatalf("CreatePR: %v", err)
	}
	combined := result.Error + "\n" + result.Output
	for _, secret := range []string{"oauth2", "embedded-token", "workspace-token", "Sensitive title", "Sensitive body"} {
		if strings.Contains(combined, secret) {
			t.Fatalf("push failure contains %q: %+v", secret, result)
		}
	}
}

func serverURL(r *http.Request) string {
	return fmt.Sprintf("http://%s", r.Host)
}

func TestGitLabFailureSanitizationRedactsSensitiveValues(t *testing.T) {
	t.Setenv("GITLAB_TOKEN", "secret-token")
	got := sanitizePRFailure(
		`request for https://oauth2:secret-token@gitlab.example failed: secret-token Sensitive title`,
		"Sensitive title",
	)
	for _, secret := range []string{"secret-token", "Sensitive title", "oauth2:"} {
		if strings.Contains(got, secret) {
			t.Fatalf("sanitized error still contains %q: %q", secret, got)
		}
	}
	if !strings.Contains(got, redactedLogValue) {
		t.Fatalf("sanitized error = %q", got)
	}
}

func TestParseGitLabRepoInfoSupportsSelfManagedSSHAndSubgroups(t *testing.T) {
	info, err := parseGitLabRepoInfo(
		"git@gitlab.internal:group/subgroup/widgets.git",
		"http://gitlab.internal:8080",
	)
	if err != nil {
		t.Fatalf("parseGitLabRepoInfo: %v", err)
	}
	if info.Origin != "http://gitlab.internal:8080" || info.ProjectPath != "group/subgroup/widgets" {
		t.Fatalf("info = %#v", info)
	}
}

func TestParseGitLabRepoInfoRejectsHTTPOriginSchemeOrPortMismatch(t *testing.T) {
	for _, remote := range []string{
		"http://gitlab.internal/group/widgets.git",
		"https://gitlab.internal:8443/group/widgets.git",
	} {
		if _, err := parseGitLabRepoInfo(remote, "https://gitlab.internal"); err == nil {
			t.Fatalf("accepted mismatched remote %q", remote)
		}
	}
}

func TestGitOperatorCreatePRRejectsMismatchedRESTWebURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode([]any{})
		case http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"web_url": "https://attacker.example/group/widgets/-/merge_requests/7",
			})
		}
	}))
	t.Cleanup(server.Close)
	op, _ := prepareGitLabPRRepo(t, server.URL+"/group/widgets.git")
	t.Setenv(gitLabHostEnv, server.URL)
	t.Setenv(gitLabTokenEnv, "workspace-token")

	result, err := op.CreatePR(context.Background(), "Title", "Body", "main", false)
	if err != nil {
		t.Fatalf("CreatePR: %v", err)
	}
	if result.Success || !result.BranchPushed || result.PRURL != "" || result.Output != "" ||
		result.Error != "branch was pushed; retry merge request creation" {
		t.Fatalf("result = %+v", result)
	}
	if strings.Contains(result.Error, "attacker.example") {
		t.Fatalf("partial result leaked provider details: %+v", result)
	}
}

func TestFinalizePRCreationAfterPushReturnsGenericProviderAwarePartialState(t *testing.T) {
	for _, test := range []struct {
		provider prProvider
		want     string
	}{
		{provider: prProviderGitHub, want: "branch was pushed; retry pull request creation"},
		{provider: prProviderAzureRepos, want: "branch was pushed; retry pull request creation"},
		{provider: prProviderGitLab, want: "branch was pushed; retry merge request creation"},
	} {
		result, err := finalizePRCreationAfterPush(&PRCreateResult{
			Provider: string(test.provider),
			Output:   "provider output containing secret-token",
			Error:    "provider failure containing secret-token",
		}, errors.New("transport failure containing secret-token"))
		if err != nil {
			t.Fatalf("finalize %s: %v", test.provider, err)
		}
		if result.Success || !result.BranchPushed || result.Output != "" || result.PRURL != "" || result.Error != test.want {
			t.Fatalf("finalize %s = %+v", test.provider, result)
		}
		if strings.Contains(fmt.Sprintf("%+v", result), "secret-token") {
			t.Fatalf("finalize %s leaked provider details: %+v", test.provider, result)
		}
	}
}

func TestGitOperatorCreatePRGlabUsesProjectDefaultBeforeExistingLookup(t *testing.T) {
	op, scriptDir := prepareGitLabPRRepo(t, "git@gitlab.com:group/widgets.git")
	writeExecutable(t, filepath.Join(scriptDir, "glab"), `#!/bin/sh
if [ "$1" = "api" ]; then
  printf '{"default_branch":"develop"}\n'
  exit 0
fi
if [ "$1" = "mr" ] && [ "$2" = "list" ]; then
  case " $* " in
    *" --target-branch develop "*) ;;
    *) exit 9 ;;
  esac
  printf '[{"web_url":"https://gitlab.com/group/widgets/-/merge_requests/1","target_branch":"main"},{"web_url":"https://gitlab.com/group/widgets/-/merge_requests/2","target_branch":"develop"}]\n'
  exit 0
fi
exit 8
`)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	result, err := op.CreatePR(context.Background(), "Title", "Body", "", false)
	if err != nil {
		t.Fatalf("CreatePR: %v", err)
	}
	if !result.Success || result.PRURL != "https://gitlab.com/group/widgets/-/merge_requests/2" {
		t.Fatalf("result = %+v", result)
	}
}
