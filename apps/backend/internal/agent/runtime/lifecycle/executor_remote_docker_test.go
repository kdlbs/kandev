package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRemoteDockerExecutor_CreateInstanceIsImplemented replaces the assertion
// that this runtime reports itself unimplemented. The stub it guarded is gone:
// a launch now fails on its own configuration, not on the runtime's existence.
func TestRemoteDockerExecutor_CreateInstanceIsImplemented(t *testing.T) {
	executor := NewRemoteDockerExecutor(newTestLogger())
	_, err := executor.CreateInstance(context.Background(), &ExecutorCreateRequest{InstanceID: "instance-1"})
	if err == nil {
		t.Fatal("CreateInstance with an empty request = nil error, want a configuration error")
	}
	if strings.Contains(err.Error(), "not yet implemented") {
		t.Fatalf("CreateInstance still reports itself unimplemented: %v", err)
	}
}

func TestRemoteDockerExecutorUsesConfiguredResolverForManifestBundle(t *testing.T) {
	const version = "1.2.3"
	const commit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	payload := []byte("verified remote Docker helper")
	bundle := t.TempDir()
	home := t.TempDir()
	writeResolverManifest(t, bundle, version, commit, RemoteHelperVariantStandard, "linux/amd64", payload)
	t.Setenv("KANDEV_BUNDLE_DIR", bundle)

	manifest, exists, err := ReadRemoteHelperManifest(bundle, version, commit)
	if err != nil || !exists {
		t.Fatalf("ReadRemoteHelperManifest = (%v, %v), want the configured release manifest", exists, err)
	}
	record, ok := remoteHelperForPlatform(manifest, SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"})
	if !ok {
		t.Fatal("release manifest has no linux/amd64 helper")
	}
	cachePath := filepath.Join(home, "cache", remoteHelperCacheDir, version, "linux-amd64", record.SHA256, "agentctl")
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachePath, payload, 0o755); err != nil {
		t.Fatal(err)
	}

	resolver := NewAgentctlResolverWithOptions(newTestLogger(), AgentctlResolverOptions{
		Version: version, Commit: commit, BundleDir: bundle, HomeDir: home,
		ReleaseBaseURL: "http://127.0.0.1:1/releases/download",
	})
	executor := NewRemoteDockerExecutor(newTestLogger(), resolver)
	writer := &recordingArchiveWriter{}
	inputs := executor.newRemoteContainerInputs(
		writer,
		SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"},
		NewCommandBuilder(),
	)

	if err := inputs.DeliverHelpers(context.Background(), "container-1"); err != nil {
		t.Fatalf("DeliverHelpers: %v", err)
	}
	helper, ok := entryByName(writer.entries(t), "usr/local/bin/agentctl")
	if !ok || helper.body != string(payload) {
		t.Fatalf("delivered helper = (%q, %v), want the verified cached payload", helper.body, ok)
	}
}
