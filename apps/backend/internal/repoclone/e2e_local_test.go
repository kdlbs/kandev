package repoclone

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

func localGitRemote(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "rig.git")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q", "--bare", dir}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}
	return "file://" + dir
}

func e2eLocalRequest(remote string) GitCredentialRequest {
	return GitCredentialRequest{WorkspaceID: "workspace", Provider: ProviderE2ELocal, ProviderHost: "http://localhost", CloneURL: remote, Owner: "local", Name: "rig"}
}

func TestE2ELocalRemoteIsClonedWithoutCredentialsUnderTheE2EProfile(t *testing.T) {
	t.Setenv(e2eMockEnv, "true")
	remote := localGitRemote(t)
	cloner := NewCloner(Config{BasePath: t.TempDir()}, ProtocolHTTPS, "", logger.Default())
	path, err := cloner.EnsureWorkspaceClonedWithCredentialRequest(context.Background(), e2eLocalRequest(remote), "", "")
	if err != nil {
		t.Fatalf("EnsureWorkspaceClonedWithCredentialRequest() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
		t.Fatalf("clone target has no .git: %v", err)
	}
}

func TestE2ELocalRemoteIsRefusedOutsideTheE2EProfile(t *testing.T) {
	t.Setenv(e2eMockEnv, "")
	remote := localGitRemote(t)
	cloner := NewCloner(Config{BasePath: t.TempDir()}, ProtocolHTTPS, "", logger.Default())
	if _, err := cloner.EnsureWorkspaceClonedWithCredentialRequest(context.Background(), e2eLocalRequest(remote), "", ""); err == nil {
		t.Fatal("a file remote was cloned outside the e2e profile")
	}
}

func TestE2ELocalRemoteRejectsOtherProvidersAndHosts(t *testing.T) {
	t.Setenv(e2eMockEnv, "true")
	if _, ok := E2ELocalRemote("github", "file:///tmp/rig.git"); ok {
		t.Fatal("a real provider matched")
	}
	for _, raw := range []string{"http://intranet.example/rig.git", "file://host/tmp/rig.git", "ssh://localhost/rig.git"} {
		if _, ok := E2ELocalRemote(ProviderE2ELocal, raw); ok {
			t.Fatalf("E2ELocalRemote(%q) matched", raw)
		}
	}
}
