package repoclone

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

func TestGitCredentialRequestCarriesExactTaskScope(t *testing.T) {
	t.Parallel()

	requestType := reflect.TypeOf(GitCredentialRequest{})
	for _, field := range []string{"TaskID", "SessionID", "RepositoryID", "ProviderScope", "ProviderRepositoryID"} {
		if _, found := requestType.FieldByName(field); !found {
			t.Errorf("GitCredentialRequest is missing %s", field)
		}
	}
}

type exactScopeWorkspaceCloner interface {
	EnsureWorkspaceClonedWithCredentialRequest(
		context.Context, GitCredentialRequest, string, string,
	) (string, error)
	RefreshWorkspaceRepositoryWithCredentialRequest(
		context.Context, GitCredentialRequest, string, string, string,
	) error
}

func TestClonerExposesExactScopeWorkspaceClone(t *testing.T) {
	t.Parallel()

	cloner := NewCloner(Config{BasePath: t.TempDir()}, ProtocolHTTPS, "", logger.Default())
	if _, supported := any(cloner).(exactScopeWorkspaceCloner); !supported {
		t.Fatal("Cloner does not expose exact-scope workspace clone and refresh operations")
	}
}

func TestClonerBuildCloneURLUsesCurrentProtocol(t *testing.T) {
	resolver := &mutableGitProtocolResolver{protocol: ProtocolHTTPS}
	cloner := NewClonerWithProtocolResolver(
		Config{BasePath: t.TempDir()}, resolver, "", logger.Default(),
	)

	got, err := cloner.BuildCloneURLWithHost(
		context.Background(), "github", "https://github.com", "acme", "widgets",
	)
	if err != nil {
		t.Fatalf("BuildCloneURLWithHost() initial error = %v", err)
	}
	if got != "https://github.com/acme/widgets.git" {
		t.Fatalf("initial clone URL = %q, want HTTPS", got)
	}

	resolver.protocol = ProtocolSSH
	got, err = cloner.BuildCloneURLWithHost(
		context.Background(), "github", "https://github.com", "acme", "widgets",
	)
	if err != nil {
		t.Fatalf("BuildCloneURLWithHost() updated error = %v", err)
	}
	if got != "git@github.com:acme/widgets.git" {
		t.Fatalf("updated clone URL = %q, want SSH", got)
	}
}

type mutableGitProtocolResolver struct {
	protocol string
}

func (r *mutableGitProtocolResolver) ResolveGitProtocol(context.Context, string) string {
	return r.protocol
}

func TestProviderRepoPathSeparatesProviderHosts(t *testing.T) {
	t.Parallel()

	cloner := NewCloner(Config{}, ProtocolHTTPS, t.TempDir(), logger.Default())
	gitHubPath, err := cloner.ProviderRepoPath("github", "https://github.com", "group", "project")
	if err != nil {
		t.Fatalf("GitHub ProviderRepoPath: %v", err)
	}
	gitLabPath, err := cloner.ProviderRepoPath("gitlab", "https://gitlab.com", "group", "project")
	if err != nil {
		t.Fatalf("GitLab ProviderRepoPath: %v", err)
	}
	selfManagedPath, err := cloner.ProviderRepoPath("gitlab", "https://gitlab.internal:8443", "group", "project")
	if err != nil {
		t.Fatalf("self-managed GitLab ProviderRepoPath: %v", err)
	}

	if gitHubPath == gitLabPath || gitLabPath == selfManagedPath || gitHubPath == selfManagedPath {
		t.Fatalf("provider paths collided: github=%q gitlab=%q self-managed=%q", gitHubPath, gitLabPath, selfManagedPath)
	}
	if !strings.Contains(selfManagedPath, filepath.Join("gitlab", "gitlab.internal-8443")) {
		t.Fatalf("self-managed path %q does not contain normalized provider and host", selfManagedPath)
	}
}

func TestClone_PreservesNonDefaultRemoteBranches(t *testing.T) {
	t.Parallel()

	originPath := initBareRepoWithReleaseBranch(t)
	targetPath := filepath.Join(t.TempDir(), "clone")

	cloner := NewCloner(Config{}, ProtocolSSH, t.TempDir(), logger.Default())
	if err := cloner.clone(context.Background(), originPath, targetPath, nil); err != nil {
		t.Fatalf("clone() unexpected error: %v", err)
	}

	if !gitRefExists(t, targetPath, "refs/remotes/origin/release") {
		t.Fatal("expected cloned repo to contain origin/release for downstream worktree base branches")
	}
}

func TestSetOriginURL(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repoPath := t.TempDir()
	runGit(t, repoPath, "init", "--quiet")
	runGit(t, repoPath, "remote", "add", "origin", "https://github.com/acme/widgets.git")

	cloner := NewCloner(Config{}, ProtocolSSH, t.TempDir(), logger.Default())
	if err := cloner.SetOriginURL(context.Background(), repoPath, "git@github.com:acme/widgets.git"); err != nil {
		t.Fatalf("SetOriginURL() error = %v", err)
	}
	if got := strings.TrimSpace(runGit(t, repoPath, "remote", "get-url", "origin")); got != "git@github.com:acme/widgets.git" {
		t.Fatalf("origin = %q, want SSH URL", got)
	}

	if err := cloner.SetOriginURL(context.Background(), "", "https://github.com/acme/widgets.git"); err == nil {
		t.Fatal("SetOriginURL() error = nil, want missing repository path error")
	}
	if err := cloner.SetOriginURL(context.Background(), repoPath, ""); err == nil {
		t.Fatal("SetOriginURL() error = nil, want missing origin URL error")
	}
}

func TestSetOriginURLSkipsWriteWhenOriginAlreadyMatches(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repoPath := t.TempDir()
	runGit(t, repoPath, "init", "--quiet")
	const origin = "https://github.com/acme/widgets.git"
	runGit(t, repoPath, "remote", "add", "origin", origin)
	if err := os.WriteFile(filepath.Join(repoPath, ".git", "config.lock"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(filepath.Join(repoPath, ".git", "config.lock")) })

	cloner := NewCloner(Config{}, ProtocolHTTPS, t.TempDir(), logger.Default())
	if err := cloner.SetOriginURL(context.Background(), repoPath, origin); err != nil {
		t.Fatalf("SetOriginURL() with an already-matching origin = %v, want no-op success", err)
	}
}

func TestSetOriginURLComparesStoredOriginBeforeInsteadOfExpansion(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repoPath := t.TempDir()
	runGit(t, repoPath, "init", "--quiet")
	const origin = "https://github.com/acme/widgets.git"
	runGit(t, repoPath, "remote", "add", "origin", origin)
	runGit(t, repoPath, "config", "--local", "url.git@github.com:.insteadOf", "https://github.com/")
	if got := strings.TrimSpace(runGit(t, repoPath, "remote", "get-url", "origin")); got != "git@github.com:acme/widgets.git" {
		t.Fatalf("expanded origin = %q, want SSH URL", got)
	}
	if err := os.WriteFile(filepath.Join(repoPath, ".git", "config.lock"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(filepath.Join(repoPath, ".git", "config.lock")) })

	cloner := NewCloner(Config{}, ProtocolHTTPS, t.TempDir(), logger.Default())
	if err := cloner.SetOriginURL(context.Background(), repoPath, origin); err != nil {
		t.Fatalf("SetOriginURL() with insteadOf rewrite = %v, want no-op success", err)
	}
}

func TestSetOriginURLIncludesBoundedGitDiagnostic(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repoPath := t.TempDir()
	runGit(t, repoPath, "init", "--quiet")
	runGit(t, repoPath, "remote", "add", "origin", "https://github.com/acme/widgets.git")
	if err := os.WriteFile(filepath.Join(repoPath, ".git", "config.lock"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(filepath.Join(repoPath, ".git", "config.lock")) })

	cloner := NewCloner(Config{}, ProtocolSSH, t.TempDir(), logger.Default())
	err := cloner.SetOriginURL(context.Background(), repoPath, "git@github.com:acme/widgets.git")
	if err == nil || !strings.Contains(err.Error(), "could not lock config file") {
		t.Fatalf("SetOriginURL() error = %v, want Git lock diagnostic", err)
	}
	if strings.TrimSpace(err.Error()) == "set repository origin: exit status 128" {
		t.Fatalf("SetOriginURL() discarded Git diagnostic: %v", err)
	}
}

func TestSetOriginURLClassifiesDubiousOwnership(t *testing.T) {
	binDir := t.TempDir()
	gitPath := filepath.Join(binDir, "git")
	if err := os.WriteFile(gitPath, []byte("#!/bin/sh\nprintf '%s\\n' 'fatal: detected dubious ownership in repository at /var/lib/kandev/repos/acme/widgets' >&2\nexit 128\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cloner := NewCloner(Config{}, ProtocolSSH, t.TempDir(), logger.Default())
	err := cloner.SetOriginURL(context.Background(), "/var/lib/kandev/repos/acme/widgets", "git@github.com:acme/widgets.git")
	if err == nil || !errors.Is(err, ErrRepositoryOwnershipMismatch) {
		t.Fatalf("SetOriginURL() error = %v, want ownership mismatch", err)
	}
	if !strings.Contains(err.Error(), "service account") || strings.Contains(err.Error(), "safe.directory") {
		t.Fatalf("ownership diagnostic = %v, want service-account guidance without safe.directory workaround", err)
	}
}

func TestRedactCloneOutputRedactsCredentialsAndBoundsOutput(t *testing.T) {
	output := "fatal: https://alice:secret@example.com/acme/widgets.git password=another-secret Authorization: Bearer synthetic-secret " + strings.Repeat("x", maxGitDiagnosticBytes+100)
	redacted := redactCloneOutput(output, "another-secret")
	if strings.Contains(redacted, "secret") || strings.Contains(redacted, "another-secret") {
		t.Fatalf("redactCloneOutput() leaked credential: %q", redacted)
	}
	if len(redacted) > maxGitDiagnosticBytes {
		t.Fatalf("redactCloneOutput() length = %d, want <= %d", len(redacted), maxGitDiagnosticBytes)
	}
}

func TestSetOriginURLSerializesConcurrentUpdates(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repoPath := t.TempDir()
	runGit(t, repoPath, "init", "--quiet")
	runGit(t, repoPath, "remote", "add", "origin", "https://github.com/acme/widgets.git")
	cloner := NewCloner(Config{}, ProtocolSSH, t.TempDir(), logger.Default())

	const workers = 32
	start := make(chan struct{})
	errs := make(chan error, workers)
	var group sync.WaitGroup
	group.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer group.Done()
			<-start
			errs <- cloner.SetOriginURL(context.Background(), repoPath, "git@github.com:acme/widgets.git")
		}()
	}
	close(start)
	group.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent SetOriginURL() error = %v", err)
		}
	}
}

func TestWorkspaceRepoPathIsolatesManagedClones(t *testing.T) {
	t.Parallel()

	basePath := t.TempDir()
	cloner := NewCloner(Config{BasePath: basePath}, ProtocolSSH, "", logger.Default())
	first, err := cloner.WorkspaceRepoPath("workspace-a", "github", "acme", "private")
	if err != nil {
		t.Fatalf("WorkspaceRepoPath(workspace-a): %v", err)
	}
	second, err := cloner.WorkspaceRepoPath("workspace-b", "github", "acme", "private")
	if err != nil {
		t.Fatalf("WorkspaceRepoPath(workspace-b): %v", err)
	}
	if first == second {
		t.Fatalf("workspace clone paths must differ, both were %q", first)
	}
	if !cloner.ShouldRecloneForWorkspace("workspace-b", first) {
		t.Fatal("workspace-b must not reuse workspace-a's managed clone")
	}
	if cloner.ShouldRecloneForWorkspace("workspace-a", first) {
		t.Fatal("workspace-a should reuse its own managed clone")
	}
	legacy := filepath.Join(basePath, "acme", "private")
	if !cloner.ShouldRecloneForWorkspace("workspace-a", legacy) {
		t.Fatal("legacy shared managed clone must be rematerialized")
	}
}

func TestWorkspaceProviderRepoPathSeparatesSelfManagedOrigins(t *testing.T) {
	t.Parallel()

	basePath := t.TempDir()
	cloner := NewCloner(Config{BasePath: basePath}, ProtocolHTTPS, "", logger.Default())
	first, err := cloner.WorkspaceProviderRepoPath(
		"workspace-a", "gitlab", "https://gitlab.first.internal", "acme", "api",
	)
	if err != nil {
		t.Fatalf("first WorkspaceProviderRepoPath: %v", err)
	}
	second, err := cloner.WorkspaceProviderRepoPath(
		"workspace-a", "gitlab", "https://gitlab.second.internal", "acme", "api",
	)
	if err != nil {
		t.Fatalf("second WorkspaceProviderRepoPath: %v", err)
	}
	defaultOrigin, err := cloner.WorkspaceProviderRepoPath(
		"workspace-a", "gitlab", "https://gitlab.com", "acme", "api",
	)
	if err != nil {
		t.Fatalf("default WorkspaceProviderRepoPath: %v", err)
	}
	if first == second {
		t.Fatalf("self-managed origins collided at %q", first)
	}
	if !strings.Contains(first, filepath.Join("gitlab", "gitlab.first.internal", "acme", "api")) {
		t.Fatalf("first path did not retain normalized origin: %q", first)
	}
	wantDefault := filepath.Join(basePath, "workspaces", "workspace-a", "gitlab", "acme", "api")
	if defaultOrigin != wantDefault {
		t.Fatalf("default origin path = %q, want %q", defaultOrigin, wantDefault)
	}
}

func TestWorkspaceProviderRepositoryPathSeparatesOpaqueScopesAndImmutableIDs(t *testing.T) {
	t.Parallel()
	cloner := NewCloner(Config{BasePath: t.TempDir()}, ProtocolHTTPS, "", nil)

	first, err := cloner.WorkspaceProviderRepositoryPath(
		"workspace-1", "bitbucket", "https://forge.example.test",
		"https://forge.example.test/dc-a", "repository-42", "TEAM", "widgets",
	)
	if err != nil {
		t.Fatal(err)
	}
	otherScope, err := cloner.WorkspaceProviderRepositoryPath(
		"workspace-1", "bitbucket", "https://forge.example.test",
		"https://forge.example.test/dc-b", "repository-42", "TEAM", "widgets",
	)
	if err != nil {
		t.Fatal(err)
	}
	otherRepository, err := cloner.WorkspaceProviderRepositoryPath(
		"workspace-1", "bitbucket", "https://forge.example.test",
		"https://forge.example.test/dc-a", "repository-99", "RENAMED", "renamed",
	)
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := cloner.WorkspaceProviderRepositoryPath(
		"workspace-1", "bitbucket", "https://forge.example.test",
		"https://forge.example.test/dc-a", "repository-42", "RENAMED", "renamed",
	)
	if err != nil {
		t.Fatal(err)
	}
	if first == otherScope || first == otherRepository {
		t.Fatalf("scoped clone paths collided: first=%q scope=%q repository=%q", first, otherScope, otherRepository)
	}
	if renamed != first {
		t.Fatalf("mutable owner/name changed immutable clone path: first=%q renamed=%q", first, renamed)
	}
	opaqueVariant, err := cloner.WorkspaceProviderRepositoryPath(
		"workspace-1", "bitbucket", "https://forge.example.test",
		"https://forge.example.test/dc-a", " repository-42 ", "TEAM", "widgets",
	)
	if err != nil {
		t.Fatal(err)
	}
	if opaqueVariant == first {
		t.Fatal("opaque provider repository IDs must not be trimmed before hashing")
	}
}

func TestEnsureClonedAtPathRejectsExistingCheckoutWithDifferentOrigin(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	target := filepath.Join(root, "checkout")
	runGit(t, root, "init", target)
	runGit(t, target, "remote", "add", "origin", "https://forge.example.test/dc-a/scm/TEAM/widgets.git")

	cloner := NewCloner(Config{BasePath: root}, ProtocolHTTPS, "", nil)
	_, err := cloner.ensureClonedAtPathWithOriginVerification(
		context.Background(), "https://forge.example.test/dc-b/scm/TEAM/widgets.git", target, nil, true,
	)
	if !errors.Is(err, ErrManagedCloneOriginMismatch) {
		t.Fatalf("ensureClonedAtPath error = %v, want ErrManagedCloneOriginMismatch", err)
	}
}

func TestEnsureClonedAtPathOriginMismatchDoesNotExposeConfiguredCredentials(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	target := filepath.Join(root, "checkout")
	runGit(t, root, "init", target)
	runGit(t, target, "remote", "add", "origin", "https://user:secret-token@forge.example.test/scm/TEAM/widgets.git")

	cloner := NewCloner(Config{BasePath: root}, ProtocolHTTPS, "", nil)
	_, err := cloner.ensureClonedAtPathWithOriginVerification(
		context.Background(), "https://forge.example.test/scm/TEAM/widgets.git", target, nil, true,
	)
	if !errors.Is(err, ErrManagedCloneOriginMismatch) {
		t.Fatalf("ensureClonedAtPath error = %v, want ErrManagedCloneOriginMismatch", err)
	}
	if strings.Contains(err.Error(), "secret-token") || strings.Contains(err.Error(), "user:") {
		t.Fatalf("origin mismatch exposed credentials: %v", err)
	}
}

func TestWorkspaceRepoPathRejectsTraversal(t *testing.T) {
	t.Parallel()

	cloner := NewCloner(Config{BasePath: t.TempDir()}, ProtocolHTTPS, "", logger.Default())
	for _, testCase := range []struct {
		name, workspaceID, owner, repo string
	}{
		{name: "workspace", workspaceID: "../outside", owner: "acme", repo: "private"},
		{name: "owner", workspaceID: "workspace", owner: "../outside", repo: "private"},
		{name: "repo", workspaceID: "workspace", owner: "acme", repo: "../outside"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := cloner.WorkspaceRepoPath(testCase.workspaceID, "github", testCase.owner, testCase.repo); err == nil {
				t.Fatal("WorkspaceRepoPath() expected traversal error")
			}
		})
	}
}

func TestWorkspaceRepoPathSupportsNestedProviderOwner(t *testing.T) {
	t.Parallel()

	basePath := t.TempDir()
	cloner := NewCloner(Config{BasePath: basePath}, ProtocolHTTPS, "", logger.Default())
	path, err := cloner.WorkspaceRepoPath("workspace-a", "gitlab", "group/subgroup", "repository")
	if err != nil {
		t.Fatalf("WorkspaceRepoPath() unexpected error: %v", err)
	}
	want := filepath.Join(basePath, "workspaces", "workspace-a", "gitlab", "group", "subgroup", "repository")
	if path != want {
		t.Fatalf("WorkspaceRepoPath() = %q, want %q", path, want)
	}
}

func TestEnsureWorkspaceClonedUsesSelectedCredentialWithoutAmbientFallback(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	capturePath := filepath.Join(root, "capture")
	gitPath := filepath.Join(binDir, "git")
	script := `#!/bin/sh
printf '%s\n' "$@" > "$KANDEV_TEST_CAPTURE.args"
helper=${GIT_CONFIG_VALUE_1#!}
"$helper" get > "$KANDEV_TEST_CAPTURE.helper"
printf '%s\n' "$GH_TOKEN|$GITHUB_TOKEN|$GIT_CONFIG_GLOBAL|$GIT_CONFIG_NOSYSTEM|$KANDEV_REPOCLONE_GITHUB_USERNAME|$KANDEV_REPOCLONE_GITHUB_TOKEN|$GIT_CONFIG_VALUE_1" > "$KANDEV_TEST_CAPTURE.env"
`
	if err := os.WriteFile(gitPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake git: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("KANDEV_TEST_CAPTURE", capturePath)
	t.Setenv("GH_TOKEN", "ambient-gh-token")
	t.Setenv("GITHUB_TOKEN", "ambient-github-token")
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "credential.helper")
	t.Setenv("GIT_CONFIG_VALUE_0", "!malicious-helper")

	credentials := &recordingCredentialProvider{password: "workspace-token"}
	cloner := NewCloner(Config{BasePath: filepath.Join(root, "repos")}, ProtocolSSH, "", logger.Default())
	cloner.SetGitCredentialProvider(credentials)
	target, err := cloner.EnsureWorkspaceCloned(
		context.Background(), "workspace-a", "github", "git@github.com:acme/private.git", "acme", "private",
	)
	if err != nil {
		t.Fatalf("EnsureWorkspaceCloned(): %v", err)
	}
	if credentials.workspaceID != "workspace-a" {
		t.Fatalf("credential workspace = %q, want workspace-a", credentials.workspaceID)
	}
	if !strings.Contains(target, filepath.Join("workspaces", "workspace-a", "github", "acme", "private")) {
		t.Fatalf("target path %q is not workspace isolated", target)
	}
	args := readTestFile(t, capturePath+".args")
	if !strings.Contains(args, "https://github.com/acme/private.git") {
		t.Fatalf("git args do not contain credential-compatible HTTPS URL: %s", args)
	}
	if !strings.Contains(args, "--\nhttps://github.com/acme/private.git") {
		t.Fatalf("git args do not terminate clone options before the URL: %s", args)
	}
	if strings.Contains(args, "workspace-token") || strings.Contains(args, "ambient-") {
		t.Fatalf("git args leaked credential material: %s", args)
	}
	env := strings.TrimSpace(readTestFile(t, capturePath+".env"))
	parts := strings.Split(env, "|")
	wantParts := []string{"", "", os.DevNull, "1", "", ""}
	if len(parts) != 7 || strings.Join(parts[:6], "\x00") != strings.Join(wantParts, "\x00") ||
		!strings.HasPrefix(parts[6], "!") {
		t.Fatalf("git auth environment = %#v", parts)
	}
	if helper := readTestFile(t, capturePath+".helper"); helper != "username=x-access-token\npassword=workspace-token\n" {
		t.Fatalf("credential helper output = %q", helper)
	}
	if strings.Contains(env, "workspace-token") {
		t.Fatalf("git environment leaked workspace token: %s", env)
	}
	cmd := exec.CommandContext(context.Background(), "git", "version")
	configureTestGitCommand(t, cmd, &cloneAuth{
		origin: "https://github.com", username: "x-access-token", password: "workspace-token",
	})
	assertUniqueGitConfigEnv(t, cmd.Env)
}

func TestManagedGitCommandExecutesWithCompleteConfigAndNoAmbientAuth(t *testing.T) {
	t.Setenv("GH_TOKEN", "ambient-gh-token")
	t.Setenv("GITHUB_TOKEN", "ambient-github-token")
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "credential.helper")
	t.Setenv("GIT_CONFIG_VALUE_0", "!printf 'password=ambient-token\\n'")

	cmd := exec.CommandContext(context.Background(), "git", "credential", "fill")
	configureTestGitCommand(t, cmd, &cloneAuth{
		origin: "https://github.com", username: "workspace-user", password: "workspace-token",
	})
	cmd.Stdin = strings.NewReader("protocol=https\nhost=github.com\npath=acme/private.git\n\n")

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("managed git credential command failed: %v\n%s", err, out)
	}
	got := string(out)
	if !strings.Contains(got, "username=workspace-user") || !strings.Contains(got, "password=workspace-token") {
		t.Fatalf("managed credential output = %q", got)
	}
	if strings.Contains(got, "ambient-token") || envValue(cmd.Env, "GH_TOKEN") != "" ||
		envValue(cmd.Env, "GITHUB_TOKEN") != "" {
		t.Fatalf("ambient authentication reached managed git: output=%q env=%v", got, cmd.Env)
	}
}

func TestEnsureWorkspaceClonedRequiresExplicitGitHubCredential(t *testing.T) {
	t.Parallel()

	cloner := NewCloner(Config{BasePath: t.TempDir()}, ProtocolHTTPS, "", logger.Default())
	_, err := cloner.EnsureWorkspaceCloned(
		context.Background(), "workspace-a", "github", "https://github.com/acme/private.git", "acme", "private",
	)
	if !errors.Is(err, ErrWorkspaceCredentialUnavailable) {
		t.Fatalf("EnsureWorkspaceCloned() error = %v, want credential unavailable", err)
	}
}

func TestWorkspaceCloneAuthPreservesNonGitHubURL(t *testing.T) {
	t.Parallel()

	cloner := NewCloner(Config{BasePath: t.TempDir()}, ProtocolSSH, "", logger.Default())
	want := "git@ssh.dev.azure.com:v3/acme/Platform/api"
	got, auth, err := cloner.workspaceCloneAuth(
		context.Background(), "workspace-a", "azure_devops", "", want, "Platform", "api", "", "",
	)
	if err != nil {
		t.Fatalf("workspaceCloneAuth() unexpected error: %v", err)
	}
	if got != want || auth != nil {
		t.Fatalf("workspaceCloneAuth() = (%q, %#v), want (%q, nil)", got, auth, want)
	}
}

func TestWorkspaceCloneAuthPrefersDeclaredGitHubHTTPSURL(t *testing.T) {
	cloner := NewCloner(Config{BasePath: t.TempDir()}, ProtocolSSH, "", logger.Default())
	cloner.SetGitCredentialProvider(&recordingCredentialProvider{password: "workspace-token"})
	declared := "https://github.enterprise.example/scm/ENG/widgets.git"

	cloneURL, auth, err := cloner.workspaceCloneAuth(
		context.Background(), "workspace-a", "github", "https://github.enterprise.example", declared, "acme", "widgets", "", "",
	)
	if err != nil {
		t.Fatalf("workspaceCloneAuth(): %v", err)
	}
	if cloneURL != declared || auth == nil || auth.origin != "https://github.enterprise.example" {
		t.Fatalf("workspace clone auth = (%q, %#v)", cloneURL, auth)
	}
}

func TestWorkspaceCloneAuthRejectsMismatchedGitHubURLBeforeCredentialUse(t *testing.T) {
	cloner := NewCloner(Config{BasePath: t.TempDir()}, ProtocolHTTPS, "", logger.Default())
	cloner.SetGitCredentialProvider(&recordingCredentialProvider{password: "workspace-token"})

	cloneURL, auth, err := cloner.workspaceCloneAuth(
		context.Background(), "workspace-a", "github", "https://github.com",
		"https://attacker.example/acme/widgets.git", "acme", "widgets", "", "",
	)
	if err != nil {
		t.Fatalf("workspaceCloneAuth(): %v", err)
	}
	if cloneURL != "https://github.com/acme/widgets.git" || auth == nil || auth.origin != "https://github.com" {
		t.Fatalf("workspace clone auth = (%q, %#v)", cloneURL, auth)
	}
}

type recordingCredentialProvider struct {
	workspaceID string
	request     GitCredentialRequest
	password    string
}

func (p *recordingCredentialProvider) ResolveGitCredential(
	_ context.Context, request GitCredentialRequest,
) (string, string, error) {
	p.workspaceID = request.WorkspaceID
	p.request = request
	return "x-access-token", p.password, nil
}

func TestWorkspaceCloneAuthResolvesPluginProviderCredentialForExactOrigin(t *testing.T) {
	credentials := &recordingCredentialProvider{password: "plugin-token"}
	cloner := NewCloner(Config{BasePath: t.TempDir()}, ProtocolHTTPS, "", logger.Default())
	cloner.SetGitCredentialProvider(credentials)
	cloneURL := "https://bitbucket.example/context/scm/ENG/widgets.git"

	gotURL, auth, err := cloner.workspaceCloneAuth(
		context.Background(), "workspace-a", "bitbucket", "https://bitbucket.example/context",
		cloneURL, "ENG", "widgets", "", "",
	)
	if err != nil {
		t.Fatalf("workspaceCloneAuth(): %v", err)
	}
	if gotURL != cloneURL || auth == nil || auth.username != "x-access-token" || auth.password != "plugin-token" {
		t.Fatalf("workspace clone auth = (%q, %#v)", gotURL, auth)
	}
	if credentials.request.Provider != "bitbucket" || credentials.request.ProviderHost != "https://bitbucket.example/context" ||
		credentials.request.CloneURL != cloneURL || credentials.request.WorkspaceID != "workspace-a" {
		t.Fatalf("credential request = %+v", credentials.request)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

func assertUniqueGitConfigEnv(t *testing.T, env []string) {
	t.Helper()
	counts := make(map[string]int)
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if key == "GIT_CONFIG_COUNT" || strings.HasPrefix(key, "GIT_CONFIG_KEY_") ||
			strings.HasPrefix(key, "GIT_CONFIG_VALUE_") {
			counts[key]++
		}
	}
	for key, count := range counts {
		if count != 1 {
			t.Fatalf("%s occurs %d times in git environment", key, count)
		}
	}
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}

func TestGitCmdBindsGitLabCredentialToExactOrigin(t *testing.T) {
	auth, err := credentialAuth(
		"https://gitlab.internal/group/repo.git", "https://gitlab.internal", "workspace-token",
	)
	if err != nil {
		t.Fatalf("credentialAuth: %v", err)
	}
	cmd := exec.CommandContext(context.Background(), "git", "version")
	configureTestGitCommand(t, cmd, auth)
	joined := strings.Join(cmd.Env, "\n")
	if strings.Contains(joined, "workspace-token") ||
		!strings.Contains(joined, "GIT_CONFIG_KEY_1=credential.https://gitlab.internal.helper") {
		t.Fatalf("credential env = %s", joined)
	}
	if _, err := credentialAuth(
		"https://gitlab.com/group/repo.git", "https://gitlab.internal", "must-not-leak",
	); err == nil {
		t.Fatal("expected cross-host credential binding to fail")
	}
}

func configureTestGitCommand(t *testing.T, cmd *exec.Cmd, auth *cloneAuth) {
	t.Helper()
	cleanup, err := configureGitCommand(cmd, auth)
	if err != nil {
		t.Fatalf("configureGitCommand(): %v", err)
	}
	t.Cleanup(cleanup)
}

func TestGitCmdUsesSSHAuthForMatchingWorkspaceGitLabHost(t *testing.T) {
	for _, cloneURL := range []string{
		"git@gitlab.internal:group/repo.git",
		"ssh://git@gitlab.internal:2222/group/repo.git",
	} {
		auth, err := credentialAuth(cloneURL, "https://gitlab.internal:8443", "workspace-token")
		if err != nil {
			t.Fatalf("credentialAuth(%q): %v", cloneURL, err)
		}
		if auth != nil {
			t.Fatalf("SSH clone received HTTP credentials: %#v", auth)
		}
	}
}

func TestGitCmdRejectsSSHCloneForDifferentWorkspaceGitLabHost(t *testing.T) {
	for _, cloneURL := range []string{
		"git@gitlab.other:group/repo.git",
		"ssh://git@gitlab.other:2222/group/repo.git",
	} {
		if _, err := credentialAuth(cloneURL, "https://gitlab.internal", "must-not-leak"); err == nil {
			t.Fatalf("gitCmd accepted mismatched SSH clone %q", cloneURL)
		}
	}
}

func initBareRepoWithReleaseBranch(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	originPath := filepath.Join(root, "origin.git")
	workPath := filepath.Join(root, "work")

	runGit(t, root, "init", "--bare", "-b", "main", originPath)
	runGit(t, root, "clone", originPath, workPath)
	runGit(t, workPath, "config", "user.email", "test@example.com")
	runGit(t, workPath, "config", "user.name", "Test User")

	readmePath := filepath.Join(workPath, "README.md")
	if err := os.WriteFile(readmePath, []byte("main\n"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	runGit(t, workPath, "add", "README.md")
	runGit(t, workPath, "commit", "-m", "main commit")
	runGit(t, workPath, "push", "origin", "main")

	runGit(t, workPath, "checkout", "-b", "release")
	if err := os.WriteFile(readmePath, []byte("release\n"), 0o644); err != nil {
		t.Fatalf("write release README.md: %v", err)
	}
	runGit(t, workPath, "commit", "-am", "release commit")
	runGit(t, workPath, "push", "origin", "release")

	return originPath
}

func gitRefExists(t *testing.T, repoPath, ref string) bool {
	t.Helper()

	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", ref)
	cmd.Dir = repoPath
	return cmd.Run() == nil
}

func runGit(t *testing.T, repoPath string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
	return string(output)
}
