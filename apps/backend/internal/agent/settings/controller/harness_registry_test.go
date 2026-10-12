package controller

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type harnessRegistryTransport func(*http.Request) (*http.Response, error)

func (f harnessRegistryTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// AC-AGENTS-RUNTIME-UPDATES-003.11: this request never invokes npm.
func TestHarnessStableLatestUsesTrustedHTTPSRegistryWithoutNPM(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	client := &http.Client{Transport: harnessRegistryTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "https://registry.npmjs.org/@oh-my-pi%2Fpi-coding-agent" {
			t.Errorf("registry URL = %s", req.URL.String())
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"dist-tags":{"latest":"1.2.3"}}`)), Header: make(http.Header)}, nil
	})}
	updater := &hostRuntimeUpdater{httpClient: client}
	got, err := updater.ResolveHarnessLatest(context.Background(), "@oh-my-pi/pi-coding-agent")
	if err != nil {
		t.Fatal(err)
	}
	if got != "1.2.3" {
		t.Errorf("stable latest = %q", got)
	}
}

func TestHarnessLatestRejectsOversizedPackument(t *testing.T) {
	client := &http.Client{Transport: harnessRegistryTransport(func(*http.Request) (*http.Response, error) {
		body := strings.NewReader(strings.Repeat(" ", harnessPackumentMaxBytes+1))
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(body), Header: make(http.Header)}, nil
	})}
	updater := &hostRuntimeUpdater{httpClient: client}
	if _, err := updater.ResolveHarnessLatest(context.Background(), "@oh-my-pi/pi-coding-agent"); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized metadata error = %v", err)
	}
}

func TestHarnessLatestLookupDeadlineAndCallerCancellation(t *testing.T) {
	requests := make(chan *http.Request, 1)
	client := &http.Client{Transport: harnessRegistryTransport(func(req *http.Request) (*http.Response, error) {
		requests <- req
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}
	ctrl := &Controller{runtimeUpdater: &hostRuntimeUpdater{httpClient: client}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	result := make(chan error, 1)
	go func() {
		_, err := ctrl.resolveHarnessLatest(ctx, "@oh-my-pi/pi-coding-agent")
		result <- err
	}()
	var req *http.Request
	select {
	case req = <-requests:
	case <-time.After(time.Second):
		t.Fatal("registry request did not start")
	}
	deadline, ok := req.Context().Deadline()
	if !ok {
		t.Fatal("registry request has no deadline")
	}
	if remaining := time.Until(deadline); remaining <= 0 || remaining > runtimeUpdateStatusLookupTimeout {
		t.Fatalf("registry deadline remaining = %v, want within %v", remaining, runtimeUpdateStatusLookupTimeout)
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("lookup error = %v, want caller cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked registry request did not terminate on caller cancellation")
	}
}
