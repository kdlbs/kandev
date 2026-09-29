package lifecycle

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

func newResolverTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	return log
}

func TestAgentctlResolverManifestDownloadsExactHelperAndReusesCache(t *testing.T) {
	const version = "1.2.3"
	const commit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	payload := []byte("verified linux helper")
	bundle := t.TempDir()
	home := t.TempDir()
	writeResolverManifest(t, bundle, version, commit, "standard", "linux/amd64", payload)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if got, want := r.URL.Path, "/releases/download/v1.2.3/agentctl-linux-amd64.gz"; got != want {
			t.Errorf("request path = %q, want %q", got, want)
			http.NotFound(w, r)
			return
		}
		writeGzip(t, w, payload)
	}))
	defer server.Close()

	options := AgentctlResolverOptions{
		Version: version, Commit: commit, BundleDir: bundle, HomeDir: home,
		ReleaseBaseURL: server.URL + "/releases/download",
	}
	resolver := NewAgentctlResolverWithOptions(newResolverTestLogger(t), options)
	platform := SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}
	path, err := resolver.ResolveRemoteBinaryContext(context.Background(), platform, nil)
	if err != nil {
		t.Fatalf("ResolveRemoteBinaryContext: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(payload) {
		t.Fatalf("cached helper = %q, err=%v", got, err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	resolver = NewAgentctlResolverWithOptions(newResolverTestLogger(t), options)
	if _, err := resolver.ResolveRemoteBinaryContext(context.Background(), platform, nil); err != nil {
		t.Fatalf("ResolveRemoteBinaryContext cache hit: %v", err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("download requests = %d, want 1", got)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("cached helper mode = %v, err=%v; want executable", info, err)
	}
}

func TestAgentctlResolverManifestUsesTaggedReleaseIdentityOnceInURL(t *testing.T) {
	const version = "v1.2.3"
	const commit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	payload := []byte("verified tagged linux helper")
	bundle := t.TempDir()
	home := t.TempDir()
	writeResolverManifest(t, bundle, version, commit, "standard", "linux/amd64", payload)
	var requestPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		writeGzip(t, w, payload)
	}))
	defer server.Close()

	resolver := NewAgentctlResolverWithOptions(newResolverTestLogger(t), AgentctlResolverOptions{
		Version: version, Commit: commit, BundleDir: bundle, HomeDir: home,
		ReleaseBaseURL: server.URL + "/releases/download",
	})
	if _, err := resolver.ResolveRemoteBinaryContext(context.Background(), SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}, nil); err != nil {
		t.Fatalf("ResolveRemoteBinaryContext: %v", err)
	}
	if want := "/releases/download/v1.2.3/agentctl-linux-amd64.gz"; requestPath != want {
		t.Fatalf("request path = %q, want %q", requestPath, want)
	}
}

func TestAgentctlResolverManifestNeverUsesHostBinaryWhenFetchFails(t *testing.T) {
	const version = "1.2.3"
	const commit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	bundle := t.TempDir()
	writeResolverManifest(t, bundle, version, commit, "standard", "linux/amd64", []byte("helper"))
	if err := os.MkdirAll(filepath.Join(bundle, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "bin", "agentctl"), []byte("host binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	resolver := NewAgentctlResolverWithOptions(newResolverTestLogger(t), AgentctlResolverOptions{
		Version: version, Commit: commit, BundleDir: bundle, HomeDir: t.TempDir(),
		ReleaseBaseURL: "http://127.0.0.1:1/releases/download",
	})
	if path, err := resolver.ResolveRemoteBinaryContext(context.Background(), SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}, nil); err == nil {
		t.Fatalf("resolved %q after fetch failure, want error", path)
	}
}

func TestAgentctlResolverManifestEmitsTypedFailureProgress(t *testing.T) {
	const version = "1.2.3"
	const commit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	bundle := t.TempDir()
	writeResolverManifest(t, bundle, version, commit, "standard", "linux/amd64", []byte("helper"))
	resolver := NewAgentctlResolverWithOptions(newResolverTestLogger(t), AgentctlResolverOptions{
		Version: version, Commit: commit, BundleDir: bundle, HomeDir: t.TempDir(),
		ReleaseBaseURL: "http://127.0.0.1:1/releases/download", DownloadTimeout: time.Second,
	})
	var got []PrepareStep
	_, err := resolver.ResolveRemoteBinaryContext(context.Background(), SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}, func(step PrepareStep, _, _ int) {
		got = append(got, step)
	})
	if err == nil {
		t.Fatal("expected fetch error")
	}
	if len(got) != 2 || got[0].Kind != PrepareStepKindRemoteHelperDownload || got[0].Status != PrepareStepRunning ||
		got[1].Status != PrepareStepFailed || got[1].RemotePlatform != "linux/amd64" || got[1].Name != "" {
		t.Fatalf("progress = %#v, want typed running and failed events", got)
	}
}

func TestAgentctlResolverFullManifestUsesBundledHelperOffline(t *testing.T) {
	const version = "1.2.3"
	const commit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	payload := []byte("bundled linux helper")
	bundle := t.TempDir()
	writeResolverManifest(t, bundle, version, commit, "full", "linux/amd64", payload)
	helperPath := filepath.Join(bundle, "bin", "agentctl-linux-amd64")
	if err := os.WriteFile(helperPath, payload, 0o755); err != nil {
		t.Fatal(err)
	}
	resolver := NewAgentctlResolverWithOptions(newResolverTestLogger(t), AgentctlResolverOptions{
		Version: version, Commit: commit, BundleDir: bundle, HomeDir: t.TempDir(),
		ReleaseBaseURL: "http://127.0.0.1:1/releases/download",
	})
	got, err := resolver.ResolveRemoteBinaryContext(context.Background(), SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}, nil)
	if err != nil || got != helperPath {
		t.Fatalf("full bundle resolution = %q, %v; want %q", got, err, helperPath)
	}
}

func TestAgentctlResolverSingleflightsConcurrentDownloads(t *testing.T) {
	const version = "1.2.3"
	const commit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	payload := []byte("one shared download")
	bundle := t.TempDir()
	home := t.TempDir()
	writeResolverManifest(t, bundle, version, commit, "standard", "linux/amd64", payload)
	compressed := gzipBytes(t, payload)
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		entered <- struct{}{}
		<-release
		_, _ = w.Write(compressed)
	}))
	defer server.Close()
	resolver := NewAgentctlResolverWithOptions(newResolverTestLogger(t), AgentctlResolverOptions{
		Version: version, Commit: commit, BundleDir: bundle, HomeDir: home,
		ReleaseBaseURL: server.URL + "/releases/download",
	})
	platform := SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}
	results := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := resolver.ResolveRemoteBinaryContext(context.Background(), platform, nil)
			results <- err
		}()
	}
	<-entered
	close(release)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("ResolveRemoteBinaryContext: %v", err)
		}
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("download requests = %d, want 1", got)
	}
}

type observedDoneContext struct {
	context.Context
	observed chan struct{}
	once     sync.Once
}

func (c *observedDoneContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.observed) })
	return c.Context.Done()
}

func TestAgentctlResolverSharedDownloadSurvivesFirstWaiterCancellation(t *testing.T) {
	const version = "1.2.3"
	const commit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	payload := []byte("shared helper survives first waiter")
	bundle, home := t.TempDir(), t.TempDir()
	writeResolverManifest(t, bundle, version, commit, "standard", "linux/amd64", payload)
	compressed := gzipBytes(t, payload)
	requestStarted := make(chan struct{})
	allowResponse := make(chan struct{})
	requestCanceled := make(chan struct{})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		close(requestStarted)
		select {
		case <-r.Context().Done():
			close(requestCanceled)
		case <-allowResponse:
			_, _ = w.Write(compressed)
		}
	}))
	defer server.Close()
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(allowResponse) }) }
	defer release()
	resolver := NewAgentctlResolverWithOptions(newResolverTestLogger(t), AgentctlResolverOptions{
		Version: version, Commit: commit, BundleDir: bundle, HomeDir: home,
		ReleaseBaseURL: server.URL,
	})
	platform := SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}

	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstResult := make(chan error, 1)
	go func() {
		_, err := resolver.ResolveRemoteBinaryContext(firstCtx, platform, nil)
		firstResult <- err
	}()
	<-requestStarted

	secondJoined := make(chan struct{})
	secondResult := make(chan error, 1)
	secondCtx := &observedDoneContext{Context: context.Background(), observed: secondJoined}
	go func() {
		_, err := resolver.ResolveRemoteBinaryContext(secondCtx, platform, nil)
		secondResult <- err
	}()
	<-secondJoined

	cancelFirst()
	if err := <-firstResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("first caller error = %v, want context cancellation", err)
	}
	release()
	if err := <-secondResult; err != nil {
		t.Fatalf("second live caller failed after first caller canceled: %v", err)
	}
	select {
	case <-requestCanceled:
		t.Fatal("shared HTTP request was canceled with the first caller")
	default:
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("HTTP requests = %d, want one shared transfer", got)
	}
}

func TestSSHCheckAgentctlCachedUsesStandardManifestWithoutDownloading(t *testing.T) {
	const version = "1.2.3"
	const commit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	platform := SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}
	payload := []byte("expected remote helper")
	bundle, home := t.TempDir(), t.TempDir()
	writeResolverManifest(t, bundle, version, commit, "standard", platform.String(), payload)
	var requests atomic.Int32
	assetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "release asset endpoint unavailable", http.StatusServiceUnavailable)
	}))
	defer assetServer.Close()
	resolver := NewAgentctlResolverWithOptions(newResolverTestLogger(t), AgentctlResolverOptions{
		Version: version, Commit: commit, BundleDir: bundle, HomeDir: home,
		ReleaseBaseURL: assetServer.URL,
	})
	digest := sha256.Sum256(payload)
	server := newFakeSSHServer(t, newSSHScriptedHandler(t,
		sshScriptRule{match: remoteHomeCommand, result: sshOut("/home/kandev")},
		sshScriptRule{match: "cat '/home/kandev/.kandev/bin/agentctl.sha256'", result: sshOut(hex.EncodeToString(digest[:]) + "\n")},
	).handle)

	cached, err := SSHCheckAgentctlCached(context.Background(), server.dial(t), resolver, platform)
	if err != nil {
		t.Fatalf("SSHCheckAgentctlCached: %v", err)
	}
	if !cached {
		t.Fatal("manifest-matching remote helper must report cached")
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("asset requests = %d, want no helper download during connection test", got)
	}
	cacheEntries, err := filepath.Glob(filepath.Join(home, "cache", remoteHelperCacheDir, version, "linux-amd64", "*", "agentctl"))
	if err != nil || len(cacheEntries) != 0 {
		t.Fatalf("cache entries after SSH connection test = %v, err=%v; want an empty cache", cacheEntries, err)
	}
}

func TestAgentctlResolverPackagedDesktopBundleUsesPreseededCache(t *testing.T) {
	if os.Getenv("KANDEV_DESKTOP_SMOKE") != "1" {
		t.Skip("run by the release-shaped Desktop launcher smoke")
	}
	bundle := os.Getenv("KANDEV_DESKTOP_SMOKE_BUNDLE_DIR")
	home := os.Getenv("KANDEV_DESKTOP_SMOKE_HOME_DIR")
	version := os.Getenv("KANDEV_DESKTOP_SMOKE_VERSION")
	commit := os.Getenv("KANDEV_DESKTOP_SMOKE_COMMIT")
	if bundle == "" || home == "" || version == "" || commit == "" {
		t.Fatal("release-shaped Desktop smoke environment is incomplete")
	}
	if got := os.Getenv("KANDEV_BUNDLE_DIR"); got != bundle {
		t.Fatalf("KANDEV_BUNDLE_DIR = %q, want release-shaped Desktop bundle %q", got, bundle)
	}
	t.Setenv("KANDEV_HOME_DIR", home)
	platform := SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}
	for _, envName := range agentctlBinaryEnvNames(platform) {
		if value := os.Getenv(envName); value != "" {
			t.Fatalf("%s=%q must be empty to prove manifest-based cache selection", envName, value)
		}
		t.Setenv(envName, "")
	}

	manifest, exists, err := ReadRemoteHelperManifest(bundle, version, commit)
	if err != nil || !exists {
		t.Fatalf("ReadRemoteHelperManifest = (%v, %v), want a valid packaged manifest", exists, err)
	}
	if manifest.Variant != RemoteHelperVariantStandard {
		t.Fatalf("Desktop manifest variant = %q, want standard", manifest.Variant)
	}
	record, ok := remoteHelperForPlatform(manifest, platform)
	if !ok {
		t.Fatalf("Desktop manifest has no helper for %s", platform.String())
	}
	wantPath := filepath.Join(home, "cache", remoteHelperCacheDir, version, "linux-amd64", record.SHA256, "agentctl")
	if valid, err := validateCachedRemoteHelper(wantPath, record); err != nil || !valid {
		t.Fatalf("preseeded helper cache is invalid: valid=%v err=%v", valid, err)
	}

	var requests atomic.Int32
	assetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "release asset endpoint is blocked", http.StatusServiceUnavailable)
	}))
	defer assetServer.Close()
	resolver := NewAgentctlResolverWithOptions(newResolverTestLogger(t), AgentctlResolverOptions{
		Version: version, Commit: commit, HomeDir: home, ReleaseBaseURL: assetServer.URL,
	})
	got, err := resolver.ResolveRemoteBinaryContext(context.Background(), platform, nil)
	if err != nil {
		t.Fatalf("ResolveRemoteBinaryContext: %v", err)
	}
	if got != wantPath {
		t.Fatalf("resolved helper = %q, want verified Desktop cache entry %q", got, wantPath)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("release asset requests = %d, want no download for a verified preseeded helper", got)
	}
}

func TestAgentctlResolverRejectsBadDownloadWithoutPublishingCacheFile(t *testing.T) {
	const version = "1.2.3"
	const commit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	expected := []byte("expected helper")
	actual := []byte("truncated helper")
	bundle := t.TempDir()
	home := t.TempDir()
	writeResolverManifest(t, bundle, version, commit, "standard", "linux/amd64", expected)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { writeGzip(t, w, actual) }))
	defer server.Close()
	resolver := NewAgentctlResolverWithOptions(newResolverTestLogger(t), AgentctlResolverOptions{
		Version: version, Commit: commit, BundleDir: bundle, HomeDir: home, ReleaseBaseURL: server.URL,
	})
	if path, err := resolver.ResolveRemoteBinaryContext(context.Background(), SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}, nil); err == nil {
		t.Fatalf("resolved %q with mismatched digest", path)
	}
	cacheRoot := filepath.Join(home, "cache", remoteHelperCacheDir, version)
	if _, err := os.Stat(cacheRoot); err != nil {
		t.Fatalf("cache directory: %v", err)
	}
	files, err := filepath.Glob(filepath.Join(cacheRoot, "linux-amd64", "*", "agentctl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("published invalid cache files: %v", files)
	}
	temps, err := filepath.Glob(filepath.Join(cacheRoot, "linux-amd64", "*", ".agentctl-download-*"))
	if err != nil || len(temps) != 0 {
		t.Fatalf("temporary cache files = %v, err=%v", temps, err)
	}
}

func TestAgentctlResolverCacheIsolatedByReleaseVersion(t *testing.T) {
	const commit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	payload := []byte("same platform helper")
	root, home := t.TempDir(), t.TempDir()
	platform := SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}
	for _, version := range []string{"1.2.3", "1.2.4"} {
		bundle := filepath.Join(root, version)
		if err := os.MkdirAll(bundle, 0o755); err != nil {
			t.Fatal(err)
		}
		writeResolverManifest(t, bundle, version, commit, "standard", "linux/amd64", payload)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { writeGzip(t, w, payload) }))
		resolver := NewAgentctlResolverWithOptions(newResolverTestLogger(t), AgentctlResolverOptions{
			Version: version, Commit: commit, BundleDir: bundle, HomeDir: home, ReleaseBaseURL: server.URL,
		})
		if _, err := resolver.ResolveRemoteBinaryContext(context.Background(), platform, nil); err != nil {
			t.Fatalf("resolve %s: %v", version, err)
		}
		server.Close()
	}
	for _, version := range []string{"1.2.3", "1.2.4"} {
		matches, err := filepath.Glob(filepath.Join(home, "cache", remoteHelperCacheDir, version, "linux-amd64", "*", "agentctl"))
		if err != nil || len(matches) != 1 {
			t.Fatalf("cached release %s files = %v, err=%v", version, matches, err)
		}
	}
}

func TestAgentctlResolverPrunesOldCacheOnVerifiedHelperResolution(t *testing.T) {
	const version = "1.4.0"
	const commit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	payload := []byte("current cached helper")
	bundle, home := t.TempDir(), t.TempDir()
	writeResolverManifest(t, bundle, version, commit, "standard", "linux/amd64", payload)
	manifest, _, err := ReadRemoteHelperManifest(bundle, version, commit)
	if err != nil {
		t.Fatal(err)
	}
	record, _ := remoteHelperForPlatform(manifest, SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"})
	currentPath := filepath.Join(home, "cache", remoteHelperCacheDir, version, "linux-amd64", record.SHA256, "agentctl")
	if err := os.MkdirAll(filepath.Dir(currentPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(currentPath, payload, 0o755); err != nil {
		t.Fatal(err)
	}
	stalePath := filepath.Join(home, "cache", remoteHelperCacheDir, "1.1.0", "linux-amd64", "old-digest", "agentctl")
	if err := os.MkdirAll(filepath.Dir(stalePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stalePath, []byte("old helper"), 0o755); err != nil {
		t.Fatal(err)
	}
	pinnedPath := filepath.Join(home, "cache", remoteHelperCacheDir, "1.0.0", "linux-amd64", "mounted-digest", "agentctl")
	if err := os.MkdirAll(filepath.Dir(pinnedPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pinnedPath, []byte("mounted helper"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"1.2.0", "1.3.0"} {
		if err := os.MkdirAll(filepath.Join(home, "cache", remoteHelperCacheDir, version), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	resolver := NewAgentctlResolverWithOptions(newResolverTestLogger(t), AgentctlResolverOptions{
		Version: version, Commit: commit, BundleDir: bundle, HomeDir: home,
	})
	inventoryCalled := false
	resolver.SetCacheMountInventory(func(context.Context) ([]string, error) {
		inventoryCalled = true
		return []string{pinnedPath}, nil
	})
	if _, err := resolver.ResolveRemoteBinaryContext(context.Background(), SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}, nil); err != nil {
		t.Fatalf("ResolveRemoteBinaryContext: %v", err)
	}
	waitForResolverCachePrune(t, resolver)
	if !inventoryCalled {
		t.Fatal("verified non-Docker helper resolution did not request the shared mount inventory")
	}
	if _, err := os.Stat(filepath.Join(home, "cache", remoteHelperCacheDir, "1.1.0")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unpinned stale cache version remains after resolution: %v", err)
	}
	if _, err := os.Stat(pinnedPath); err != nil {
		t.Fatalf("helper mounted by a reconnectable container was removed: %v", err)
	}
}

func TestAgentctlResolverCachePruneDoesNotDelayDeadlineBoundLaunch(t *testing.T) {
	const version = "1.4.0"
	const commit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	platform := SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}
	payload := []byte("current cached helper")
	bundle, home := t.TempDir(), t.TempDir()
	writeResolverManifest(t, bundle, version, commit, "standard", platform.String(), payload)
	manifest, _, err := ReadRemoteHelperManifest(bundle, version, commit)
	if err != nil {
		t.Fatal(err)
	}
	record, _ := remoteHelperForPlatform(manifest, platform)
	cachePath := filepath.Join(home, "cache", remoteHelperCacheDir, version, "linux-amd64", record.SHA256, "agentctl")
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachePath, payload, 0o755); err != nil {
		t.Fatal(err)
	}

	resolver := NewAgentctlResolverWithOptions(newResolverTestLogger(t), AgentctlResolverOptions{
		Version: version, Commit: commit, BundleDir: bundle, HomeDir: home,
	})
	inventoryStarted := make(chan struct{})
	inventoryFinished := make(chan struct{})
	releaseInventory := make(chan struct{})
	var releaseOnce sync.Once
	unblockInventory := func() { releaseOnce.Do(func() { close(releaseInventory) }) }
	t.Cleanup(unblockInventory)
	deadlineSeen := make(chan bool, 1)
	resolver.SetCacheMountInventory(func(ctx context.Context) ([]string, error) {
		_, hasDeadline := ctx.Deadline()
		deadlineSeen <- hasDeadline
		close(inventoryStarted)
		defer close(inventoryFinished)
		select {
		case <-releaseInventory:
			return nil, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	type resolution struct {
		path string
		err  error
	}
	resolved := make(chan resolution, 1)
	go func() {
		path, err := resolver.ResolveRemoteBinaryContext(ctx, platform, nil)
		resolved <- resolution{path: path, err: err}
	}()
	select {
	case <-inventoryStarted:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("cache prune did not start its mount inventory")
	}
	if !<-deadlineSeen {
		cancel()
		t.Fatal("background mount inventory has no time bound")
	}
	select {
	case got := <-resolved:
		if got.err != nil || got.path != cachePath {
			t.Fatalf("resolution = %q, %v; want %q", got.path, got.err, cachePath)
		}
	case <-ctx.Done():
		t.Fatal("helper resolution waited for background cache cleanup until the launch deadline")
	}
	cancel()
	select {
	case <-inventoryFinished:
		t.Fatal("caller cancellation unexpectedly canceled the owned background sweep")
	default:
	}
	unblockInventory()
	waitForResolverCachePrune(t, resolver)
}

func waitForResolverCachePrune(t *testing.T, resolver *AgentctlResolver) {
	t.Helper()
	resolver.cachePruneMu.Lock()
	done := resolver.cachePruneDone
	resolver.cachePruneMu.Unlock()
	if done == nil {
		t.Fatal("verified helper resolution did not schedule cache pruning")
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("background helper cache pruning did not finish")
	}
}

func TestAgentctlResolverDefersPruningWhenMountInventoryIsUncertain(t *testing.T) {
	const version = "1.4.0"
	const commit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	payload := []byte("current cached helper")
	bundle, home := t.TempDir(), t.TempDir()
	writeResolverManifest(t, bundle, version, commit, "standard", "linux/amd64", payload)
	manifest, _, err := ReadRemoteHelperManifest(bundle, version, commit)
	if err != nil {
		t.Fatal(err)
	}
	record, _ := remoteHelperForPlatform(manifest, SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"})
	currentPath := filepath.Join(home, "cache", remoteHelperCacheDir, version, "linux-amd64", record.SHA256, "agentctl")
	if err := os.MkdirAll(filepath.Dir(currentPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(currentPath, payload, 0o755); err != nil {
		t.Fatal(err)
	}
	oldVersion := filepath.Join(home, "cache", remoteHelperCacheDir, "1.0.0")
	if err := os.MkdirAll(oldVersion, 0o755); err != nil {
		t.Fatal(err)
	}
	resolver := NewAgentctlResolverWithOptions(newResolverTestLogger(t), AgentctlResolverOptions{
		Version: version, Commit: commit, BundleDir: bundle, HomeDir: home,
	})
	resolver.SetCacheMountInventory(func(context.Context) ([]string, error) {
		return nil, errors.New("Docker inventory is unavailable")
	})
	if _, err := resolver.ResolveRemoteBinaryContext(context.Background(), SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}, nil); err != nil {
		t.Fatalf("ResolveRemoteBinaryContext: %v", err)
	}
	waitForResolverCachePrune(t, resolver)
	if _, err := os.Stat(oldVersion); err != nil {
		t.Fatalf("old helper cache was pruned with an uncertain mount inventory: %v", err)
	}
}

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
	defer cancelOld()
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

func TestAgentctlResolverLaunchDeadlineBoundsDownload(t *testing.T) {
	const version = "1.2.3"
	const commit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	bundle := t.TempDir()
	payload := []byte("helper")
	writeResolverManifest(t, bundle, version, commit, "standard", "linux/amd64", payload)
	var requests atomic.Int32
	requestStarted := make(chan struct{})
	requestCanceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			close(requestStarted)
			<-r.Context().Done()
			close(requestCanceled)
			return
		}
		writeGzip(t, w, payload)
	}))
	defer server.Close()
	resolver := NewAgentctlResolverWithOptions(newResolverTestLogger(t), AgentctlResolverOptions{
		Version: version, Commit: commit, BundleDir: bundle, HomeDir: t.TempDir(), ReleaseBaseURL: server.URL,
		DownloadTimeout: 250 * time.Millisecond,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	var steps []PrepareStep
	_, err := resolver.ResolveRemoteBinaryContext(ctx, SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}, func(step PrepareStep, _, _ int) {
		steps = append(steps, step)
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("resolve error = %v, want launch deadline", err)
	}
	if len(steps) != 2 || steps[1].FailureCode != "timeout" || steps[1].Status != PrepareStepFailed {
		t.Fatalf("download progress = %#v, want timeout failure", steps)
	}
	select {
	case <-requestStarted:
	default:
		t.Fatal("helper transfer did not start")
	}
	select {
	case <-requestCanceled:
	case <-time.After(150 * time.Millisecond):
		t.Fatal("shared helper transfer continued after its only launch waiter expired")
	}
	if _, err := resolver.ResolveRemoteBinaryContext(context.Background(), SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}, nil); err != nil {
		t.Fatalf("retry after canceled transfer: %v", err)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("HTTP requests after retry = %d, want a fresh request", got)
	}
}

func TestAgentctlResolverDoesNotUseCorruptCacheEntry(t *testing.T) {
	const version = "1.2.3"
	const commit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	payload := []byte("verified helper")
	bundle, home := t.TempDir(), t.TempDir()
	writeResolverManifest(t, bundle, version, commit, "standard", "linux/amd64", payload)
	manifest, _, err := ReadRemoteHelperManifest(bundle, version, commit)
	if err != nil {
		t.Fatal(err)
	}
	resolver := NewAgentctlResolverWithOptions(newResolverTestLogger(t), AgentctlResolverOptions{
		Version: version, Commit: commit, BundleDir: bundle, HomeDir: home,
		ReleaseBaseURL: "http://127.0.0.1:1/releases/download",
	})
	record, _ := remoteHelperForPlatform(manifest, SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"})
	path, err := resolver.cachePath(manifest, record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("stale native helper"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := resolver.ResolveRemoteBinaryContext(context.Background(), SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}, nil); err == nil {
		t.Fatalf("resolved %q from corrupt cache, want download failure", got)
	}
}

func TestAgentctlResolverCrossProcessCacheWritesAreAtomic(t *testing.T) {
	const version = "1.2.3"
	const commit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	payload := []byte("atomic cross-process helper")
	bundle, home := t.TempDir(), t.TempDir()
	writeResolverManifest(t, bundle, version, commit, "standard", "linux/amd64", payload)
	compressed := gzipBytes(t, payload)
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		entered <- struct{}{}
		<-release
		_, _ = w.Write(compressed)
	}))
	defer server.Close()
	commands := make([]*exec.Cmd, 0, 2)
	outputs := make([][]byte, 2)
	for range 2 {
		command := exec.Command(os.Args[0], "-test.run=^TestAgentctlResolverCrossProcessCacheWriteHelper$")
		command.Env = append(os.Environ(),
			"KANDEV_TEST_REMOTE_HELPER_SUBPROCESS=1",
			"KANDEV_TEST_REMOTE_HELPER_BUNDLE="+bundle,
			"KANDEV_TEST_REMOTE_HELPER_HOME="+home,
			"KANDEV_TEST_REMOTE_HELPER_URL="+server.URL,
			"KANDEV_AGENTCTL_LINUX_AMD64_BINARY=",
			"KANDEV_AGENTCTL_LINUX_BINARY=",
		)
		commands = append(commands, command)
	}
	var wg sync.WaitGroup
	for i, command := range commands {
		wg.Add(1)
		go func(i int, command *exec.Cmd) {
			defer wg.Done()
			outputs[i], _ = command.CombinedOutput()
		}(i, command)
	}
	<-entered
	<-entered
	close(release)
	wg.Wait()
	for i, output := range outputs {
		if commands[i].ProcessState == nil || !commands[i].ProcessState.Success() {
			t.Fatalf("resolver subprocess %d failed: %s", i, output)
		}
	}
	manifest, _, err := ReadRemoteHelperManifest(bundle, version, commit)
	if err != nil {
		t.Fatal(err)
	}
	record, _ := remoteHelperForPlatform(manifest, SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"})
	cachePath := filepath.Join(home, "cache", remoteHelperCacheDir, version, "linux-amd64", record.SHA256, "agentctl")
	got, err := os.ReadFile(cachePath)
	if err != nil || string(got) != string(payload) {
		t.Fatalf("cross-process cache = %q, err=%v", got, err)
	}
	temps, err := filepath.Glob(filepath.Join(filepath.Dir(cachePath), ".agentctl-download-*"))
	if err != nil || len(temps) != 0 {
		t.Fatalf("temporary download files = %v, err=%v", temps, err)
	}
}

func TestAgentctlResolverCrossProcessCacheWriteHelper(t *testing.T) {
	if os.Getenv("KANDEV_TEST_REMOTE_HELPER_SUBPROCESS") != "1" {
		t.Skip("subprocess-only test helper")
	}
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	resolver := NewAgentctlResolverWithOptions(newResolverTestLogger(t), AgentctlResolverOptions{
		Version: "1.2.3", Commit: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		BundleDir:      os.Getenv("KANDEV_TEST_REMOTE_HELPER_BUNDLE"),
		HomeDir:        os.Getenv("KANDEV_TEST_REMOTE_HELPER_HOME"),
		ReleaseBaseURL: os.Getenv("KANDEV_TEST_REMOTE_HELPER_URL"),
		HTTPClient:     client,
	})
	if _, err := resolver.ResolveRemoteBinaryContext(context.Background(), SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}, nil); err != nil {
		t.Fatal(err)
	}
}

func writeResolverManifest(t *testing.T, root, version, commit, variant, platform string, payload []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	manifest := testRemoteHelperManifest(variant)
	manifest.Version, manifest.Commit = version, commit
	for i := range manifest.Helpers {
		if manifest.Helpers[i].Platform == platform {
			manifest.Helpers[i].SHA256 = hex.EncodeToString(sum[:])
			manifest.Helpers[i].SizeBytes = int64(len(payload))
		}
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, RemoteHelperManifestName), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeGzip(t *testing.T, w io.Writer, data []byte) {
	t.Helper()
	writer := gzip.NewWriter(w)
	if _, err := writer.Write(data); err != nil {
		t.Errorf("gzip write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Errorf("gzip close: %v", err)
	}
}

func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	writeGzip(t, &buf, data)
	return buf.Bytes()
}

func TestAgentctlResolverRemoteBinaryUsesPlatformEnvOverride(t *testing.T) {
	tmp := t.TempDir()
	helper := filepath.Join(tmp, "agentctl-darwin-arm64")
	if err := os.WriteFile(helper, []byte("stub"), 0o755); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	t.Setenv("KANDEV_AGENTCTL_DARWIN_ARM64_BINARY", helper)

	resolver := NewAgentctlResolver(newResolverTestLogger(t))
	got, err := resolver.ResolveRemoteBinary(SSHRemotePlatform{GOOS: "darwin", GOARCH: "arm64"})
	if err != nil {
		t.Fatalf("ResolveRemoteBinary: %v", err)
	}
	if got != helper {
		t.Fatalf("ResolveRemoteBinary = %q, want %q", got, helper)
	}
}

func TestAgentctlResolverLinuxAMD64KeepsLegacyEnvOverride(t *testing.T) {
	tmp := t.TempDir()
	helper := filepath.Join(tmp, "agentctl-linux-amd64")
	if err := os.WriteFile(helper, []byte("stub"), 0o755); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	t.Setenv("KANDEV_AGENTCTL_LINUX_BINARY", helper)

	resolver := NewAgentctlResolver(newResolverTestLogger(t))
	got, err := resolver.ResolveRemoteBinary(SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"})
	if err != nil {
		t.Fatalf("ResolveRemoteBinary: %v", err)
	}
	if got != helper {
		t.Fatalf("ResolveRemoteBinary = %q, want %q", got, helper)
	}
}

func TestAgentctlResolverLinuxAMD64PrefersPrimaryEnvOverride(t *testing.T) {
	tmp := t.TempDir()
	primary := filepath.Join(tmp, "agentctl-linux-amd64-primary")
	legacy := filepath.Join(tmp, "agentctl-linux-amd64-legacy")
	for _, helper := range []string{primary, legacy} {
		if err := os.WriteFile(helper, []byte("stub"), 0o755); err != nil {
			t.Fatalf("write helper %s: %v", helper, err)
		}
	}
	t.Setenv("KANDEV_AGENTCTL_LINUX_AMD64_BINARY", primary)
	t.Setenv("KANDEV_AGENTCTL_LINUX_BINARY", legacy)

	resolver := NewAgentctlResolver(newResolverTestLogger(t))
	got, err := resolver.ResolveRemoteBinary(SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"})
	if err != nil {
		t.Fatalf("ResolveRemoteBinary: %v", err)
	}
	if got != primary {
		t.Fatalf("ResolveRemoteBinary = %q, want primary %q", got, primary)
	}
}

func TestAgentctlResolverLinuxAMD64BadPrimaryEnvDoesNotUseLegacyFallback(t *testing.T) {
	tmp := t.TempDir()
	primary := filepath.Join(tmp, "missing-agentctl")
	legacy := filepath.Join(tmp, "agentctl-linux-amd64-legacy")
	if err := os.WriteFile(legacy, []byte("stub"), 0o755); err != nil {
		t.Fatalf("write legacy helper: %v", err)
	}
	t.Setenv("KANDEV_AGENTCTL_LINUX_AMD64_BINARY", primary)
	t.Setenv("KANDEV_AGENTCTL_LINUX_BINARY", legacy)

	resolver := NewAgentctlResolver(newResolverTestLogger(t))
	got, err := resolver.ResolveRemoteBinary(SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"})
	if err == nil {
		t.Fatalf("ResolveRemoteBinary = %q, want error", got)
	}
	for _, want := range []string{"KANDEV_AGENTCTL_LINUX_AMD64_BINARY", primary} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want substring %q", err.Error(), want)
		}
	}
}

func TestAgentctlResolverRemoteBinaryNotFoundErrorNamesPlatformAndEnv(t *testing.T) {
	t.Setenv("KANDEV_AGENTCTL_DARWIN_AMD64_BINARY", "")

	resolver := NewAgentctlResolver(newResolverTestLogger(t))
	_, err := resolver.ResolveRemoteBinary(SSHRemotePlatform{GOOS: "darwin", GOARCH: "amd64"})
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"darwin/amd64", "KANDEV_AGENTCTL_DARWIN_AMD64_BINARY"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want substring %q", err.Error(), want)
		}
	}
}

func TestAgentctlBinaryCandidatesIncludesRemoteHelpersAndHostFallback(t *testing.T) {
	exeDir := filepath.Join("tmp", "kandev", "bin")
	platform := SSHRemotePlatform{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}

	got := agentctlBinaryCandidates(exeDir, platform)
	name := agentctlBinaryName(platform)
	want := []string{
		filepath.Join(exeDir, name),
		filepath.Join(exeDir, "..", "build", name),
		filepath.Join(exeDir, "..", "bin", name),
		filepath.Join(exeDir, "agentctl"),
		filepath.Join(exeDir, "..", "build", "agentctl"),
		filepath.Join(exeDir, "..", "bin", "agentctl"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("agentctlBinaryCandidates = %#v, want %#v", got, want)
	}
}

func TestAgentctlBinaryCandidatesOmitsHostFallbackForDifferentPlatform(t *testing.T) {
	exeDir := filepath.Join("tmp", "kandev", "bin")
	platform := SSHRemotePlatform{GOOS: "darwin", GOARCH: "arm64"}
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		platform = SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}
	}

	got := agentctlBinaryCandidates(exeDir, platform)
	name := agentctlBinaryName(platform)
	want := []string{
		filepath.Join(exeDir, name),
		filepath.Join(exeDir, "..", "build", name),
		filepath.Join(exeDir, "..", "bin", name),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("agentctlBinaryCandidates = %#v, want %#v", got, want)
	}
}
