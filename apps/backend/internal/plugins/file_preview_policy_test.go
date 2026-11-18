package plugins

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// @covers AC-PLUGINS-FILE-VIEWERS-006.1
func TestFilePreviewPolicyScopeAndPersistence(t *testing.T) {
	dir := t.TempDir()
	policies := newFilePreviewPolicies(dir)
	scope := filePreviewScope{UserID: "alice", WorkspaceID: "workspace", InstallationID: "install", Digest: "v1"}
	policy, err := policies.get(scope)
	if err != nil || policy.Mode != "ask" || policy.Revision != 0 {
		t.Fatalf("default: %+v %v", policy, err)
	}
	saved, err := policies.set(scope, "allow", 0)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := newFilePreviewPolicies(dir).get(scope)
	if err != nil || loaded != saved || loaded.Mode != "allow" {
		t.Fatalf("reload: %+v %v", loaded, err)
	}
	for _, other := range []filePreviewScope{
		{UserID: "bob", WorkspaceID: "workspace", InstallationID: "install", Digest: "v1"},
		{UserID: "alice", WorkspaceID: "other", InstallationID: "install", Digest: "v1"},
		{UserID: "alice", WorkspaceID: "workspace", InstallationID: "new", Digest: "v1"},
		{UserID: "alice", WorkspaceID: "workspace", InstallationID: "install", Digest: "v2"},
	} {
		got, err := policies.get(other)
		if err != nil || got.Mode != "ask" {
			t.Fatalf("foreign scope inherited allowance: %+v %v", got, err)
		}
	}
}

// @covers AC-PLUGINS-FILE-VIEWERS-006.6
func TestFilePreviewPolicyStaleWriteAndCorruption(t *testing.T) {
	dir := t.TempDir()
	policies := newFilePreviewPolicies(dir)
	scope := filePreviewScope{UserID: "alice", WorkspaceID: "w", InstallationID: "i", Digest: "d"}
	saved, err := policies.set(scope, "allow", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := policies.set(scope, "deny", 0); err == nil {
		t.Fatal("accepted stale policy write")
	}
	if _, err := policies.set(scope, "unexpected", saved.Revision); err == nil {
		t.Fatal("accepted unknown policy")
	}
	if err := os.WriteFile(filepath.Join(dir, "file-preview-policies.json"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := policies.get(scope); err == nil {
		t.Fatal("storage error granted access")
	}
}

func TestFilePreviewPolicyUpgradeThenRollbackDoesNotRestoreAllow(t *testing.T) {
	policies := newFilePreviewPolicies(t.TempDir())
	scope := filePreviewScope{UserID: "user", WorkspaceID: "w", InstallationID: "i", Digest: "v1"}
	if _, err := policies.set(scope, "allow", 0); err != nil {
		t.Fatal(err)
	}
	scope.Digest = "v2"
	if _, err := policies.get(scope); err != nil {
		t.Fatal(err)
	}
	scope.Digest = "v1"
	got, err := policies.get(scope)
	if err != nil || got.Mode != "ask" {
		t.Fatalf("rollback revived access: %+v %v", got, err)
	}
}

func TestFilePreviewPolicyRejectsOldDigestWriteWithoutRead(t *testing.T) {
	policies := newFilePreviewPolicies(t.TempDir())
	scope := filePreviewScope{UserID: "user", WorkspaceID: "w", InstallationID: "i", Digest: "v1"}
	old, err := policies.set(scope, filePreviewAllow, 0)
	if err != nil {
		t.Fatal(err)
	}
	scope.Digest = "v2"
	if _, err := policies.set(scope, filePreviewAllow, old.Revision); !errors.Is(err, ErrApprovalRevisionConflict) {
		t.Fatalf("old digest write accepted: %v", err)
	}
	current, err := policies.get(scope)
	if err != nil || current.Mode != filePreviewAsk || current.Revision <= old.Revision {
		t.Fatalf("digest reset: %+v %v", current, err)
	}
	if _, err := policies.set(scope, filePreviewAllow, current.Revision); err != nil {
		t.Fatalf("fresh decision rejected: %v", err)
	}
}
