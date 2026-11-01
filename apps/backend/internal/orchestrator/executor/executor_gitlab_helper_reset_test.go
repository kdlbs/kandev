package executor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func gitConfigPairs(t *testing.T, env map[string]string) [][2]string {
	t.Helper()
	count, err := strconv.Atoi(env["GIT_CONFIG_COUNT"])
	if err != nil {
		t.Fatalf("GIT_CONFIG_COUNT = %q", env["GIT_CONFIG_COUNT"])
	}
	pairs := make([][2]string, 0, count)
	for i := 0; i < count; i++ {
		key, keyOK := env["GIT_CONFIG_KEY_"+strconv.Itoa(i)]
		value, valueOK := env["GIT_CONFIG_VALUE_"+strconv.Itoa(i)]
		if !keyOK || !valueOK {
			t.Fatalf("GIT_CONFIG index %d not contiguous: %#v", i, env)
		}
		pairs = append(pairs, [2]string{key, value})
	}
	if _, extra := env["GIT_CONFIG_KEY_"+strconv.Itoa(count)]; extra {
		t.Fatalf("GIT_CONFIG entries beyond count %d: %#v", count, env)
	}
	return pairs
}

func newGitLabHelperExecutor(t *testing.T, host string) *Executor {
	t.Helper()
	e := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	e.SetGitLabCredentialResolver(&fakeGitLabCredentialResolver{byWorkspace: map[string]struct{ host, token string }{
		"ws": {host: host, token: "glpat-secret"},
	}})
	return e
}

func TestInjectGitLabWorkspaceCredentialsResetsHelperListBeforeTokenHelper(t *testing.T) {
	e := newGitLabHelperExecutor(t, "https://gitlab.example")
	req := &LaunchAgentRequest{WorkspaceID: "ws", Env: map[string]string{
		"GIT_CONFIG_COUNT":   "1",
		"GIT_CONFIG_KEY_0":   "safe.directory",
		"GIT_CONFIG_VALUE_0": "*",
	}}

	e.injectGitLabWorkspaceCredentials(context.Background(), req)

	got := gitConfigPairs(t, req.Env)
	want := [][2]string{
		{"safe.directory", "*"},
		{"credential.https://gitlab.example.helper", ""},
		{"credential.https://gitlab.example.helper", gitLabCredentialHelper},
	}
	if len(got) != len(want) {
		t.Fatalf("entries = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("entry %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestInjectGitLabWorkspaceCredentialsIsIdempotentAndDropsStaleReset(t *testing.T) {
	e := newGitLabHelperExecutor(t, "https://gitlab.example")
	req := &LaunchAgentRequest{WorkspaceID: "ws", Env: map[string]string{}}
	e.injectGitLabWorkspaceCredentials(context.Background(), req)
	e.injectGitLabWorkspaceCredentials(context.Background(), req)

	got := gitConfigPairs(t, req.Env)
	if len(got) != 2 || got[0][1] != "" || got[1][1] != gitLabCredentialHelper {
		t.Fatalf("re-injection entries = %#v", got)
	}

	// Workspace without credentials: both the reset and the helper go away.
	e.SetGitLabCredentialResolver(&fakeGitLabCredentialResolver{byWorkspace: map[string]struct{ host, token string }{}})
	e.injectGitLabWorkspaceCredentials(context.Background(), req)
	if _, ok := req.Env["GIT_CONFIG_COUNT"]; ok {
		t.Fatalf("stale GitLab helper entries remain: %#v", req.Env)
	}
}

// A system-level helper that would block (osxkeychain on macOS) must never run
// for the GitLab origin once core has injected its token helper.
func TestInjectGitLabWorkspaceCredentialsSkipsSystemHelper(t *testing.T) {
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "system-helper-ran")
	script := filepath.Join(dir, "system-helper.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	systemConfig := filepath.Join(dir, "gitconfig")
	if err := os.WriteFile(systemConfig, []byte("[credential]\n\thelper = "+script+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	e := newGitLabHelperExecutor(t, "https://gitlab.example")
	req := &LaunchAgentRequest{WorkspaceID: "ws", Env: map[string]string{}}
	e.injectGitLabWorkspaceCredentials(context.Background(), req)

	cmd := exec.Command(gitBin, "credential", "fill")
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + dir,
		"GIT_CONFIG_SYSTEM=" + systemConfig,
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_TERMINAL_PROMPT=0",
		envGitLabToken + "=" + req.Env[envGitLabToken],
	}
	for k, v := range req.Env {
		if strings.HasPrefix(k, "GIT_CONFIG_") {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	cmd.Stdin = strings.NewReader("protocol=https\nhost=gitlab.example\n\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git credential fill: %v\n%s", err, out)
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Fatal("system credential helper ran before the token helper")
	}
	if !strings.Contains(string(out), "username=oauth2") || !strings.Contains(string(out), "password=glpat-secret") {
		t.Fatalf("token helper did not answer: %q", out)
	}
}
