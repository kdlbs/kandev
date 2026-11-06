package lifecycle

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAgentctlResolverRechecksCacheAfterDelayedMiss(t *testing.T) {
	const version = "1.2.3"
	const commit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	payload := []byte("one shared download")
	bundle, home := t.TempDir(), t.TempDir()
	writeResolverManifest(t, bundle, version, commit, "standard", "linux/amd64", payload)
	compressed := gzipBytes(t, payload)
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
		_, _ = w.Write(compressed)
	}))
	defer server.Close()
	var releaseOnce, continueOnce, observedOnce sync.Once
	releaseResponse := func() { releaseOnce.Do(func() { close(release) }) }
	continueSecond := make(chan struct{})
	unblockSecond := func() { continueOnce.Do(func() { close(continueSecond) }) }
	defer releaseResponse()
	defer unblockSecond()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resolver := NewAgentctlResolverWithOptions(newResolverTestLogger(t), AgentctlResolverOptions{
		Version: version, Commit: commit, BundleDir: bundle, HomeDir: home,
		ReleaseBaseURL: server.URL + "/releases/download",
	})
	platform := SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}
	wait := func(ch <-chan struct{}, label string) {
		t.Helper()
		select {
		case <-ch:
		case <-time.After(10 * time.Second):
			t.Fatalf("timed out waiting for %s", label)
		}
	}
	waitResult := func(ch <-chan error, label string) {
		t.Helper()
		select {
		case err := <-ch:
			if err != nil {
				t.Fatalf("%s: %v", label, err)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("timed out waiting for %s", label)
		}
	}
	firstResult := make(chan error, 1)
	go func() { _, err := resolver.ResolveRemoteBinaryContext(ctx, platform, nil); firstResult <- err }()
	wait(entered, "first download")
	secondCacheMiss := make(chan struct{})
	secondResult := make(chan error, 1)
	go func() {
		_, err := resolver.ResolveRemoteBinaryContext(ctx, platform, func(step PrepareStep, _, _ int) {
			if step.Status == PrepareStepRunning {
				observedOnce.Do(func() { close(secondCacheMiss); <-continueSecond })
			}
		})
		secondResult <- err
	}()
	wait(secondCacheMiss, "second cache miss")
	releaseResponse()
	waitResult(firstResult, "first resolver")
	unblockSecond()
	waitResult(secondResult, "second resolver")
	if got := requests.Load(); got != 1 {
		t.Fatalf("download requests = %d, want 1", got)
	}
}
