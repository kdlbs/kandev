package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAgentctlResolverKeepsHelperSelectedByPendingOlderLaunch(t *testing.T) {
	const commit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	platform := SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}
	home := t.TempDir()
	for _, version := range []string{"1.1.0", "1.2.0"} {
		if err := os.MkdirAll(filepath.Join(home, "cache", remoteHelperCacheDir, version), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	oldBundle := t.TempDir()
	oldPayload := []byte("helper selected before its Docker mount exists")
	writeResolverManifest(t, oldBundle, "1.0.0", commit, "standard", platform.String(), oldPayload)
	oldResolver := NewAgentctlResolverWithOptions(newResolverTestLogger(t), AgentctlResolverOptions{
		Version: "1.0.0", Commit: commit, BundleDir: oldBundle, HomeDir: home,
	})
	oldManifest, _, err := ReadRemoteHelperManifest(oldBundle, "1.0.0", commit)
	if err != nil {
		t.Fatal(err)
	}
	oldRecord, ok := remoteHelperForPlatform(oldManifest, platform)
	if !ok {
		t.Fatal("old manifest has no Linux helper")
	}
	oldPath, err := oldResolver.cachePath(oldManifest, oldRecord)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(oldPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldPath, oldPayload, 0o755); err != nil {
		t.Fatal(err)
	}
	oldCtx, cancelOld := context.WithCancel(context.Background())
	defer func() {
		cancelOld()
		require.Eventually(t, func() bool {
			paths, err := readActiveRemoteHelperCacheLeases(filepath.Join(home, "cache", remoteHelperCacheDir), time.Now())
			return err == nil && !slices.Contains(paths, oldPath)
		}, 5*time.Second, 10*time.Millisecond, "canceled launch must release its cache lease before temporary directory cleanup")
	}()
	if got, err := oldResolver.ResolveRemoteBinaryContext(oldCtx, platform, nil); err != nil || got != oldPath {
		t.Fatalf("old launch helper = %q, err=%v; want %q", got, err, oldPath)
	}

	newBundle := t.TempDir()
	newPayload := []byte("current helper")
	writeResolverManifest(t, newBundle, "1.3.0", commit, "standard", platform.String(), newPayload)
	newResolver := NewAgentctlResolverWithOptions(newResolverTestLogger(t), AgentctlResolverOptions{
		Version: "1.3.0", Commit: commit, BundleDir: newBundle, HomeDir: home,
	})
	newManifest, _, err := ReadRemoteHelperManifest(newBundle, "1.3.0", commit)
	if err != nil {
		t.Fatal(err)
	}
	newRecord, ok := remoteHelperForPlatform(newManifest, platform)
	if !ok {
		t.Fatal("current manifest has no Linux helper")
	}
	newPath, err := newResolver.cachePath(newManifest, newRecord)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, newPayload, 0o755); err != nil {
		t.Fatal(err)
	}
	newResolver.SetCacheMountInventory(func(context.Context) ([]string, error) {
		return nil, nil
	})
	if got, err := newResolver.ResolveRemoteBinaryContext(context.Background(), platform, nil); err != nil || got != newPath {
		t.Fatalf("current launch helper = %q, err=%v; want %q", got, err, newPath)
	}
	waitForResolverCachePrune(t, newResolver)

	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("pending older launch helper was pruned before container creation: %v", err)
	}
}
