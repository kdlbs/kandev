package lifecycle

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

type resolverDeadlineTransport func(*http.Request) (*http.Response, error)

func (transport resolverDeadlineTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestAgentctlResolverLaunchDeadlineBoundsDownload(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const version = "1.2.3"
		const commit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		bundle := t.TempDir()
		payload := []byte("helper")
		writeResolverManifest(t, bundle, version, commit, "standard", "linux/amd64", payload)
		var requests atomic.Int32
		requestStarted := make(chan struct{})
		requestCanceled := make(chan struct{})
		client := &http.Client{Transport: resolverDeadlineTransport(func(r *http.Request) (*http.Response, error) {
			if requests.Add(1) == 1 {
				close(requestStarted)
				<-r.Context().Done()
				close(requestCanceled)
				return nil, r.Context().Err()
			}
			response := httptest.NewRecorder()
			writeGzip(t, response, payload)
			return response.Result(), nil
		})}
		resolver := NewAgentctlResolverWithOptions(newResolverTestLogger(t), AgentctlResolverOptions{
			Version: version, Commit: commit, BundleDir: bundle, HomeDir: t.TempDir(), HTTPClient: client,
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
		synctest.Wait()
		select {
		case <-requestCanceled:
		default:
			t.Fatal("shared helper transfer continued after its only launch waiter expired")
		}
		if _, err := resolver.ResolveRemoteBinaryContext(context.Background(), SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}, nil); err != nil {
			t.Fatalf("retry after canceled transfer: %v", err)
		}
		if got := requests.Load(); got != 2 {
			t.Fatalf("HTTP requests after retry = %d, want a fresh request", got)
		}
	})
}
