package subproc

import (
	"bytes"
	"context"
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
	"time"
)

func TestGitFinalEnvironment(t *testing.T) {
	fakeGitDir := t.TempDir()
	fakeGit := filepath.Join(fakeGitDir, "git")
	if err := os.WriteFile(fakeGit, []byte(`#!/bin/sh
printf 'GIT_TERMINAL_PROMPT=%s\n' "${GIT_TERMINAL_PROMPT-}"
printf 'GCM_INTERACTIVE=%s\n' "${GCM_INTERACTIVE-}"
printf 'GCM_GUI_PROMPT=%s\n' "${GCM_GUI_PROMPT-}"
printf 'GIT_ASKPASS=%s\n' "${GIT_ASKPASS-}"
printf 'SSH_ASKPASS=%s\n' "${SSH_ASKPASS-}"
printf 'SSH_ASKPASS_REQUIRE=%s\n' "${SSH_ASKPASS_REQUIRE-}"
printf 'GIT_SSH_COMMAND=%s\n' "${GIT_SSH_COMMAND-}"
printf 'GIT_CONFIG_COUNT=%s\n' "${GIT_CONFIG_COUNT-}"
for i in 0 1 2 3 4 5; do
  eval "key=\${GIT_CONFIG_KEY_$i-}"
  eval "value=\${GIT_CONFIG_VALUE_$i-}"
  if [ -n "$key" ]; then
    printf 'GIT_CONFIG_%s=%s\n' "$i" "$key=$value"
  fi
done
printf 'SELECTED_CREDENTIAL_DIR=%s\n' "${SELECTED_CREDENTIAL_DIR-}"
printf 'AMBIENT_ONLY=%s\n' "${AMBIENT_ONLY-}"
`), 0o700); err != nil {
		t.Fatalf("write fake git: %v", err)
	}
	t.Setenv("PATH", fakeGitDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AMBIENT_ONLY", "ambient-value")

	cmd := NewGitCommand(context.Background(), "credential", "fill")
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"SELECTED_CREDENTIAL_DIR=/selected/credentials",
		"GIT_TERMINAL_PROMPT=1",
		"GCM_INTERACTIVE=Always",
		"GCM_GUI_PROMPT=1",
		"GIT_ASKPASS=ambient-askpass",
		"SSH_ASKPASS=ambient-ssh-askpass",
		"SSH_ASKPASS_REQUIRE=force",
		"GIT_SSH_COMMAND=ssh -i /selected/key -oBatchMode=no",
		"GIT_CONFIG_COUNT=6",
		"GIT_CONFIG_KEY_0=credential.helper",
		"GIT_CONFIG_VALUE_0=",
		"GIT_CONFIG_KEY_1=credential.helper",
		"GIT_CONFIG_VALUE_1=!user-helper",
		"GIT_CONFIG_KEY_2=credential.helper",
		"GIT_CONFIG_VALUE_2=cache",
		"GIT_CONFIG_KEY_3=core.hooksPath",
		"GIT_CONFIG_VALUE_3=/selected/hooks",
		"GIT_CONFIG_KEY_4=notes.augment.mergeStrategy",
		"GIT_CONFIG_VALUE_4=union",
		"GIT_CONFIG_KEY_5=credential.https://github.com.helper",
		"GIT_CONFIG_VALUE_5=!kandev-host-gh-bridge",
	}

	out, err := RunGitCombinedOutputClass(context.Background(), GitInteractive, cmd)
	if err != nil {
		t.Fatalf("run fake git: %v\n%s", err, out)
	}
	got := gitEnvironmentLines(string(out))
	want := map[string]string{
		"GIT_TERMINAL_PROMPT":     "0",
		"GCM_INTERACTIVE":         "Never",
		"GCM_GUI_PROMPT":          "0",
		"GIT_ASKPASS":             "exit 1",
		"SSH_ASKPASS":             "exit 1",
		"SSH_ASKPASS_REQUIRE":     "never",
		"GIT_SSH_COMMAND":         "ssh -oBatchMode=yes -i /selected/key -oBatchMode=no",
		"GIT_CONFIG_COUNT":        "6",
		"GIT_CONFIG_0":            "credential.helper=",
		"GIT_CONFIG_1":            "credential.helper=!user-helper",
		"GIT_CONFIG_2":            "credential.helper=cache",
		"GIT_CONFIG_3":            "core.hooksPath=/selected/hooks",
		"GIT_CONFIG_4":            "notes.augment.mergeStrategy=union",
		"GIT_CONFIG_5":            "credential.https://github.com.helper=!kandev-host-gh-bridge",
		"SELECTED_CREDENTIAL_DIR": "/selected/credentials",
		"AMBIENT_ONLY":            "",
	}
	for key, wantValue := range want {
		if gotValue := got[key]; gotValue != wantValue {
			t.Errorf("%s = %q, want %q", key, gotValue, wantValue)
		}
	}
	if strings.Contains(string(out), "ambient-askpass") || strings.Contains(string(out), "ambient-ssh-askpass") {
		t.Fatalf("inherited interactive askpass reached Git: %s", out)
	}
}

func gitEnvironmentLines(output string) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			result[key] = value
		}
	}
	return result
}

func TestPrepareGitCommandPreservesNilAndDuplicateEnvironment(t *testing.T) {
	t.Setenv("AMBIENT_ONLY", "ambient-value")
	nilEnvCommand := exec.Command("git", "version")
	PrepareGitCommand(nilEnvCommand)
	if got, ok := environmentValue(nilEnvCommand.Env, "AMBIENT_ONLY"); !ok || got != "ambient-value" {
		t.Fatalf("nil command environment lost inherited value: %q, present=%t", got, ok)
	}

	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"SELECTED=first",
		"SELECTED=last",
		"GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_0=credential.helper",
		"GIT_CONFIG_VALUE_0=",
		"GIT_CONFIG_KEY_1=notes.augment.mergeStrategy",
		"GIT_CONFIG_VALUE_1=union",
	}
	prepared := PrepareGitEnvironment(env)
	selectedCount := 0
	for _, entry := range prepared {
		if strings.HasPrefix(entry, "SELECTED=") {
			selectedCount++
		}
	}
	if selectedCount != 1 {
		t.Fatalf("SELECTED appears %d times, want one: %v", selectedCount, prepared)
	}
	if got, ok := environmentValue(prepared, "SELECTED"); !ok || got != "last" {
		t.Fatalf("SELECTED = %q, present=%t, want last duplicate", got, ok)
	}
	if runtime.GOOS == "windows" {
		prepared = append(prepared, "selected=case-insensitive-last")
		prepared = collapseEnvironmentAssignments(prepared)
		if got, ok := environmentValue(prepared, "SELECTED"); !ok || got != "case-insensitive-last" {
			t.Fatalf("case-insensitive duplicate = %q, present=%t", got, ok)
		}
	}
}

func TestGitSSHCommandPreservation(t *testing.T) {
	tests := map[string]string{
		"ssh -i /selected/key -oBatchMode=no": "ssh -oBatchMode=yes -i /selected/key -oBatchMode=no",
		`'path with spaces/ssh' -i key`:       `'path with spaces/ssh' -oBatchMode=yes -i key`,
		"ssh -oBatchMode=yes -i key":          "ssh -oBatchMode=yes -i key",
		"env FOO=bar ssh -i key":              "ssh -oBatchMode=yes",
		"FOO=bar ssh -i key":                  "ssh -oBatchMode=yes",
		"exec ssh -i key":                     "ssh -oBatchMode=yes",
		"plink -i key":                        "ssh -oBatchMode=yes",
	}
	for command, want := range tests {
		if got := ForceGitSSHBatchMode(command); got != want {
			t.Errorf("ForceGitSSHBatchMode(%q) = %q, want %q", command, got, want)
		}
	}

	cmd := exec.Command("git", "version")
	cmd.Env = []string{"GIT_SSH=/selected/path/ssh"}
	PrepareGitCommand(cmd)
	got, ok := environmentValue(cmd.Env, "GIT_SSH_COMMAND")
	if !ok || got != "/selected/path/ssh -oBatchMode=yes" {
		t.Fatalf("GIT_SSH_COMMAND = %q, present=%t, want direct GIT_SSH path with BatchMode", got, ok)
	}
}

func TestGitRunnersApplyFinalEnvironment(t *testing.T) {
	fakeGitDir := t.TempDir()
	fakeGit := filepath.Join(fakeGitDir, "git")
	if err := os.WriteFile(fakeGit, []byte(`#!/bin/sh
printf 'GIT_TERMINAL_PROMPT=%s\n' "${GIT_TERMINAL_PROMPT-}"
printf 'GCM_INTERACTIVE=%s\n' "${GCM_INTERACTIVE-}"
printf 'GCM_GUI_PROMPT=%s\n' "${GCM_GUI_PROMPT-}"
printf 'GIT_ASKPASS=%s\n' "${GIT_ASKPASS-}"
printf 'SSH_ASKPASS=%s\n' "${SSH_ASKPASS-}"
printf 'SSH_ASKPASS_REQUIRE=%s\n' "${SSH_ASKPASS_REQUIRE-}"
`), 0o700); err != nil {
		t.Fatalf("write fake git: %v", err)
	}
	t.Setenv("PATH", fakeGitDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"GIT_TERMINAL_PROMPT=1",
		"GCM_INTERACTIVE=Always",
		"GCM_GUI_PROMPT=1",
		"GIT_ASKPASS=ambient-askpass",
		"SSH_ASKPASS=ambient-ssh-askpass",
		"SSH_ASKPASS_REQUIRE=force",
		"GIT_SSH_COMMAND=ssh -oBatchMode=no",
	}
	assertPolicy := func(t *testing.T, output []byte, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("fake git failed: %v\n%s", err, output)
		}
		got := gitEnvironmentLines(string(output))
		for key, want := range map[string]string{
			"GIT_TERMINAL_PROMPT": "0",
			"GCM_INTERACTIVE":     "Never",
			"GCM_GUI_PROMPT":      "0",
			"GIT_ASKPASS":         "exit 1",
			"SSH_ASKPASS":         "exit 1",
			"SSH_ASKPASS_REQUIRE": "never",
		} {
			if got[key] != want {
				t.Errorf("%s = %q, want %q", key, got[key], want)
			}
		}
	}

	t.Run("run", func(t *testing.T) {
		cmd := NewGitCommand(context.Background(), "version")
		cmd.Env = append([]string(nil), env...)
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		err := RunGitClass(context.Background(), GitLifecycle, cmd)
		assertPolicy(t, stdout.Bytes(), err)
	})

	t.Run("combined-output", func(t *testing.T) {
		cmd := NewGitCommand(context.Background(), "version")
		cmd.Env = append([]string(nil), env...)
		output, err := RunGitCombinedOutputClass(context.Background(), GitLifecycle, cmd)
		assertPolicy(t, output, err)
	})

	t.Run("output", func(t *testing.T) {
		cmd := NewGitCommand(context.Background(), "version")
		cmd.Env = append([]string(nil), env...)
		output, err := RunGitOutputClass(context.Background(), GitLifecycle, cmd)
		assertPolicy(t, output, err)
	})

	t.Run("combined-after-acquire", func(t *testing.T) {
		output, runErr, execErr := RunGitCombinedAfterAcquire(
			context.Background(), GitLifecycle, time.Second,
			func(execCtx context.Context) *exec.Cmd {
				cmd := NewGitCommand(execCtx, "version")
				cmd.Env = append([]string(nil), env...)
				return cmd
			},
		)
		if execErr != nil {
			t.Fatalf("execution context: %v", execErr)
		}
		assertPolicy(t, output, runErr)
	})

	t.Run("output-after-acquire", func(t *testing.T) {
		output, runErr, execErr := RunGitOutputAfterAcquire(
			context.Background(), GitLifecycle, time.Second,
			func(execCtx context.Context) *exec.Cmd {
				cmd := NewGitCommand(execCtx, "version")
				cmd.Env = append([]string(nil), env...)
				return cmd
			},
		)
		if execErr != nil {
			t.Fatalf("execution context: %v", execErr)
		}
		assertPolicy(t, output, runErr)
	})

	t.Run("after-acquire", func(t *testing.T) {
		var stdout bytes.Buffer
		runErr, execErr := RunGitAfterAcquire(
			context.Background(), GitLifecycle, time.Second,
			func(execCtx context.Context) *exec.Cmd {
				cmd := NewGitCommand(execCtx, "version")
				cmd.Env = append([]string(nil), env...)
				cmd.Stdout = &stdout
				return cmd
			},
		)
		if execErr != nil {
			t.Fatalf("execution context: %v", execErr)
		}
		assertPolicy(t, stdout.Bytes(), runErr)
	})
}

func TestGitCredentialHTTPSelectedScope(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo.git")
	runTestGit(t, root, "init", "--bare", repo)

	var authenticated int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		username, password, ok := request.BasicAuth()
		if !ok || username != "selected-user" || password != "selected-token" {
			response.Header().Set("WWW-Authenticate", `Basic realm="git-test"`)
			response.WriteHeader(http.StatusUnauthorized)
			return
		}
		atomic.AddInt32(&authenticated, 1)
		serveGitHTTPBackend(response, request, root)
	}))
	t.Cleanup(server.Close)

	host := strings.TrimPrefix(server.URL, "http://")
	environment := []string{
		"PATH=" + os.Getenv("PATH"),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=1",
		"GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_0=credential.helper",
		"GIT_CONFIG_VALUE_0=!false",
		"GIT_CONFIG_KEY_1=credential.http://" + host + ".helper",
		"GIT_CONFIG_VALUE_1=!f() { printf 'username=selected-user\\npassword=selected-token\\n'; }; f",
	}

	out, runErr, execErr := RunGitOutputAfterAcquire(
		context.Background(), GitInteractive, 5*time.Second, func(execCtx context.Context) *exec.Cmd {
			cmd := NewGitCommand(execCtx, "ls-remote", server.URL+"/repo.git")
			cmd.Env = append([]string(nil), environment...)
			return cmd
		},
	)
	if runErr != nil || execErr != nil {
		t.Fatalf("authenticated HTTP ls-remote failed: run=%v exec=%v\n%s", runErr, execErr, out)
	}
	if atomic.LoadInt32(&authenticated) == 0 {
		t.Fatal("HTTP Git fixture did not observe a request with selected credentials")
	}
	if strings.Contains(string(out), "selected-token") {
		t.Fatalf("Git output leaked selected credential: %q", out)
	}
}
func runTestGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}

func serveGitHTTPBackend(response http.ResponseWriter, request *http.Request, root string) {
	cmd := exec.Command("git", "http-backend")
	cmd.Env = append(os.Environ(),
		"GIT_PROJECT_ROOT="+root,
		"GIT_HTTP_EXPORT_ALL=1",
		"PATH_INFO="+request.URL.Path,
		"QUERY_STRING="+request.URL.RawQuery,
		"REQUEST_METHOD="+request.Method,
		"REMOTE_ADDR=127.0.0.1",
		"CONTENT_TYPE="+request.Header.Get("Content-Type"),
		"CONTENT_LENGTH="+request.Header.Get("Content-Length"),
	)
	cmd.Stdin = request.Body
	output, err := cmd.Output()
	if err != nil {
		response.WriteHeader(http.StatusBadGateway)
		_, _ = fmt.Fprintf(response, "git http-backend failed: %v", err)
		return
	}
	headerEnd := strings.Index(string(output), "\r\n\r\n")
	delimiterLength := 4
	if headerEnd < 0 {
		headerEnd = strings.Index(string(output), "\n\n")
		delimiterLength = 2
	}
	if headerEnd < 0 {
		response.WriteHeader(http.StatusBadGateway)
		return
	}
	for _, line := range strings.Split(string(output[:headerEnd]), "\n") {
		line = strings.TrimSuffix(line, "\r")
		key, value, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}
		if key == "Status" {
			var status int
			_, _ = fmt.Sscanf(value, "%d", &status)
			if status != 0 {
				response.WriteHeader(status)
			}
			continue
		}
		response.Header().Set(key, value)
	}
	_, _ = response.Write(output[headerEnd+delimiterLength:])
}
