package lifecycle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/sftp"
)

// TestInstalledByConcurrentUploaderComparesContent covers the decision that
// lets a losing concurrent helper upload succeed: the target counts as
// installed only when it holds exactly the bytes being uploaded.
func TestInstalledByConcurrentUploaderComparesContent(t *testing.T) {
	server := newFakeSSHServer(t, nil)
	server.enableSFTP()
	client, err := sftp.NewClient(server.dial(t))
	if err != nil {
		t.Fatalf("sftp client: %v", err)
	}
	defer func() { _ = client.Close() }()

	data := []byte("agentctl helper bytes")
	target := filepath.Join(t.TempDir(), "agentctl.exe")

	if installedByConcurrentUploader(client, target, data) {
		t.Fatal("a missing target must not count as installed")
	}
	if err := os.WriteFile(target, []byte("an older helper"), 0o644); err != nil {
		t.Fatal(err)
	}
	if installedByConcurrentUploader(client, target, data) {
		t.Fatal("a target with different bytes must not count as installed")
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if !installedByConcurrentUploader(client, target, data) {
		t.Fatal("a target with the uploaded bytes must count as installed")
	}
}
