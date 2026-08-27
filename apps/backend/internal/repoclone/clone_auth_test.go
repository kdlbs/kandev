package repoclone

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

func TestGitCmdKeepsCredentialOutOfArgumentsAndEnvironment(t *testing.T) {
	t.Parallel()
	cloneURL := "https://dev.azure.com/acme/p/_git/r"
	cmd := exec.CommandContext(context.Background(), "git", "clone", "--", cloneURL)
	configureTestGitCommand(t, cmd, &cloneAuth{
		origin: "https://dev.azure.com", username: "kandev", password: "secret-pat",
	})
	if strings.Contains(strings.Join(cmd.Args, " "), "secret-pat") {
		t.Fatal("credential leaked into command arguments")
	}
	if joined := strings.Join(cmd.Env, "\n"); strings.Contains(joined, "secret-pat") ||
		!strings.Contains(joined, "GIT_CONFIG_KEY_1=credential.https://dev.azure.com.helper") {
		t.Fatalf("credential environment = %s", joined)
	}
}

func TestEnsureWorkspaceClonedWithBasicAuthKeepsCredentialScopedToGitChild(t *testing.T) {
	tests := []struct {
		name   string
		cancel bool
	}{
		{name: "git failure"},
		{name: "context cancellation", cancel: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			binDir := canonicalTempDir(t)
			capturePath := filepath.Join(canonicalTempDir(t), "git-env")
			fakeGit := "#!/bin/sh\nprintf '%s\\n%s' \"$GIT_CONFIG_KEY_1\" \"$GIT_CONFIG_VALUE_1\" > \"$CAPTURE_PATH\"\n" +
				"helper=${GIT_CONFIG_VALUE_1#!}\n\"$helper\" get > \"$CAPTURE_PATH.helper\"\n" +
				"if [ \"$BLOCK_GIT\" = 1 ]; then exec sleep 10; fi\nexit 1\n"
			if err := os.WriteFile(filepath.Join(binDir, "git"), []byte(fakeGit), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("CAPTURE_PATH", capturePath)
			t.Setenv("GIT_CONFIG_VALUE_0", "")
			if tc.cancel {
				t.Setenv("BLOCK_GIT", "1")
			}
			cloner := NewCloner(Config{BasePath: canonicalTempDir(t)}, ProtocolHTTPS, "", logger.Default())
			ctx := context.Background()
			var cancel context.CancelFunc
			if tc.cancel {
				// Generous enough for a cold shell start (macOS is slow) so the
				// fake git reliably writes CAPTURE_PATH before entering its long
				// sleep; the deadline then cancels the blocked clone mid-run.
				ctx, cancel = context.WithTimeout(ctx, 750*time.Millisecond)
				defer cancel()
			}
			targetPath, err := cloner.EnsureWorkspaceClonedWithBasicAuth(
				ctx, "workspace-a", "azure_devops", "", "https://dev.azure.com/acme/p/_git/r",
				"p", "r", "kandev", "secret-pat",
			)
			if err == nil {
				t.Fatal("expected git clone error")
			}
			if tc.cancel && ctx.Err() != context.DeadlineExceeded {
				t.Fatalf("expected cancelled clone context, got %v", ctx.Err())
			}
			wantPath := filepath.Join("workspaces", "workspace-a", "azure_devops", "p", "r")
			if !strings.Contains(targetPath, wantPath) {
				t.Fatalf("authenticated clone path = %q, want workspace-isolated path containing %q", targetPath, wantPath)
			}
			captured, readErr := os.ReadFile(capturePath)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if strings.Contains(string(captured), "secret-pat") {
				t.Fatal("credential leaked into Git child environment")
			}
			expectedScope := "credential.https://dev.azure.com.helper"
			if !strings.Contains(string(captured), expectedScope) {
				t.Fatal("credential helper was not scoped to the authenticated repository host")
			}
			helperOutput, helperErr := os.ReadFile(capturePath + ".helper")
			if helperErr != nil || string(helperOutput) != "username=kandev\npassword=secret-pat\n" {
				t.Fatalf("credential helper output = %q, error = %v", helperOutput, helperErr)
			}
			if os.Getenv("GIT_CONFIG_VALUE_0") != "" {
				t.Fatal("credential escaped into the parent process environment")
			}
		})
	}
}

func TestAuthenticatedCloneDoesNotLeavePromisorCheckout(t *testing.T) {
	binDir := t.TempDir()
	capturePath := filepath.Join(t.TempDir(), "git-args")
	fakeGit := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$CAPTURE_PATH\"\nexit 1\n"
	if err := os.WriteFile(filepath.Join(binDir, "git"), []byte(fakeGit), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CAPTURE_PATH", capturePath)

	cloner := NewCloner(Config{BasePath: t.TempDir()}, ProtocolHTTPS, "", logger.Default())
	_, _ = cloner.EnsureWorkspaceClonedWithBasicAuth(
		context.Background(), "workspace-a", "bitbucket", "https://bitbucket.org",
		"https://bitbucket.org/acme/repository.git", "acme", "repository",
		"x-token-auth", "transient-token",
	)
	args, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(args), "--filter=blob:none") {
		t.Fatalf("authenticated clone used a promisor filter: %s", args)
	}
}

func TestScopedWorkspaceRefreshUsesOnlyAuthenticatedOrigin(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	capturePath := filepath.Join(root, "git-refresh")
	fakeGit := `#!/bin/sh
if [ "$3" = "config" ]; then printf '%s\n' "https://bitbucket.org/acme/repository.git"; exit 0; fi
if [ "$3" = "remote" ]; then exit 0; fi
	printf '%s\n' "$@" >> "$CAPTURE_PATH.args"
printf '%s\n' "$GIT_CONFIG_KEY_1" > "$CAPTURE_PATH.scope"
helper=${GIT_CONFIG_VALUE_1#!}
"$helper" get > "$CAPTURE_PATH.helper"
`
	if err := os.WriteFile(filepath.Join(binDir, "git"), []byte(fakeGit), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CAPTURE_PATH", capturePath)

	basePath := filepath.Join(root, "repos")
	cloner := NewCloner(Config{BasePath: basePath}, ProtocolHTTPS, "", logger.Default())
	credentials := &recordingCredentialProvider{password: "transient-token"}
	cloner.SetGitCredentialProvider(credentials)
	request := GitCredentialRequest{
		WorkspaceID: "workspace-a", TaskID: "task-a", SessionID: "session-a", RepositoryID: "repo-a",
		Provider: "bitbucket", ProviderHost: "https://bitbucket.org",
		CloneURL: "https://bitbucket.org/acme/repository.git", Owner: "acme", Name: "repository",
		CheckoutBranch: "feature/pr", PRNumber: 42,
	}
	repositoryPath, err := cloner.WorkspaceProviderRepoPath(
		request.WorkspaceID, request.Provider, request.ProviderHost, request.Owner, request.Name,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repositoryPath, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := cloner.RefreshWorkspaceRepositoryWithCredentialRequest(
		context.Background(), request, repositoryPath, "", "",
	); err != nil {
		t.Fatalf("RefreshWorkspaceRepositoryWithCredentialRequest(): %v", err)
	}
	args, err := os.ReadFile(capturePath + ".args")
	if err != nil {
		t.Fatal(err)
	}
	if got := string(args); !strings.Contains(got, "fetch\n--prune\n--force\n--no-tags\norigin\n") ||
		!strings.Contains(got, "pull/42/head:refs/remotes/origin/pr/42\n") || strings.Contains(got, "--all") {
		t.Fatalf("refresh args = %q", got)
	}
	scope, err := os.ReadFile(capturePath + ".scope")
	if err != nil {
		t.Fatal(err)
	}
	if string(scope) != "credential.https://bitbucket.org.helper\n" {
		t.Fatalf("credential scope = %q", scope)
	}
	helper, err := os.ReadFile(capturePath + ".helper")
	if err != nil {
		t.Fatal(err)
	}
	if string(helper) != "username=x-access-token\npassword=transient-token\n" {
		t.Fatalf("helper output = %q", helper)
	}
	if credentials.request.TaskID != "task-a" || credentials.request.SessionID != "session-a" ||
		credentials.request.RepositoryID != "repo-a" {
		t.Fatalf("credential request lost exact scope: %+v", credentials.request)
	}
}

func TestScopedWorkspaceBasicAuthRefreshUsesExactCheckout(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	capturePath := filepath.Join(root, "git-basic-refresh")
	fakeGit := `#!/bin/sh
if [ "$3" = "config" ]; then printf '%s\n' "https://dev.azure.com/acme/p/_git/repository"; exit 0; fi
if [ "$3" = "remote" ]; then exit 0; fi
printf '%s\n' "$@" > "$CAPTURE_PATH.args"
printf '%s\n' "$GIT_CONFIG_KEY_1" > "$CAPTURE_PATH.scope"
helper=${GIT_CONFIG_VALUE_1#!}
"$helper" get > "$CAPTURE_PATH.helper"
`
	if err := os.WriteFile(filepath.Join(binDir, "git"), []byte(fakeGit), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CAPTURE_PATH", capturePath)

	basePath := filepath.Join(root, "repos")
	cloner := NewCloner(Config{BasePath: basePath}, ProtocolHTTPS, "", logger.Default())
	repositoryPath, err := cloner.WorkspaceProviderRepoPath(
		"workspace-a", "azure_devops", "https://dev.azure.com/acme", "p", "repository",
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repositoryPath, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	type basicAuthRefresher interface {
		RefreshWorkspaceRepositoryWithBasicAuth(
			context.Context, string, string, string, string, string, string, string, string, string,
		) error
	}
	refresher, ok := any(cloner).(basicAuthRefresher)
	if !ok {
		t.Fatal("Cloner does not expose strict basic-auth refresh")
	}
	if err := refresher.RefreshWorkspaceRepositoryWithBasicAuth(
		context.Background(), "workspace-a", "azure_devops", "https://dev.azure.com/acme",
		"https://dev.azure.com/acme/p/_git/repository", "p", "repository", repositoryPath,
		"kandev", "secret-pat",
	); err != nil {
		t.Fatalf("RefreshWorkspaceRepositoryWithBasicAuth(): %v", err)
	}
	args, err := os.ReadFile(capturePath + ".args")
	if err != nil {
		t.Fatal(err)
	}
	if got := string(args); !strings.Contains(got, "fetch\n--prune\n--force\n--no-tags\norigin\n") {
		t.Fatalf("refresh args = %q", got)
	}
	scope, err := os.ReadFile(capturePath + ".scope")
	if err != nil {
		t.Fatal(err)
	}
	if string(scope) != "credential.https://dev.azure.com.helper\n" {
		t.Fatalf("credential scope = %q", scope)
	}
	helper, err := os.ReadFile(capturePath + ".helper")
	if err != nil {
		t.Fatal(err)
	}
	if string(helper) != "username=kandev\npassword=secret-pat\n" {
		t.Fatalf("helper output = %q", helper)
	}
}

// canonicalTempDir returns t.TempDir() with symlinks resolved so the Git child
// and the assertions agree on paths (macOS /var -> /private/var).
func canonicalTempDir(t *testing.T) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}
