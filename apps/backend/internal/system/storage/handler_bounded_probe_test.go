package storage

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGetStorageDiskBoundsBlockedRootDiscovery(t *testing.T) {
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer func() {
		releaseOnce.Do(func() { close(release) })
		waitForDiskProbesToDrain(t)
	}()
	timeout := 30 * time.Millisecond
	handler := NewHandler(HandlerConfig{
		DiskPath: "/home",
		DiskCapacity: func(context.Context, string) (DiskCapacity, error) {
			return measuredDiskCapacity(), nil
		},
		DiskRoots: func(context.Context) ([]DiskRootCandidate, error) {
			<-release
			return nil, errors.New("slow root resolver")
		},
	})
	handler.diskProbeTimeout = timeout
	router := newTestRouter(handler)

	started := time.Now()
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/system/storage/disk", nil))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		releaseOnce.Do(func() { close(release) })
		<-done
		t.Fatal("disk response waited for blocked root discovery")
	}
	releaseOnce.Do(func() { close(release) })
	if elapsed := time.Since(started); elapsed > 150*time.Millisecond {
		t.Fatalf("response took %s, want bounded root-discovery time", elapsed)
	}
	var body DiskCapacityResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Available || len(body.TemporaryRoots) != 0 || body.TemporaryRootsWarning == "" {
		t.Fatalf("response = %#v, want healthy home and unavailable temporary discovery", body)
	}
}

func TestGetStorageDiskKeepsSiblingCapacityWhenOneRootProbeBlocks(t *testing.T) {
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer func() {
		releaseOnce.Do(func() { close(release) })
		waitForDiskProbesToDrain(t)
	}()
	handler := NewHandler(HandlerConfig{
		DiskPath: "/home",
		DiskCapacity: func(_ context.Context, path string) (DiskCapacity, error) {
			if path == "/tmp-slow" {
				<-release
				return measuredDiskCapacity(), nil
			}
			return measuredDiskCapacity(), nil
		},
		DiskRoots: func(context.Context) ([]DiskRootCandidate, error) {
			return []DiskRootCandidate{{RequestedPath: "/tmp-slow"}, {RequestedPath: "/tmp-healthy"}}, nil
		},
	})
	handler.diskProbeTimeout = 30 * time.Millisecond
	router := newTestRouter(handler)
	response := httptest.NewRecorder()
	started := time.Now()
	done := make(chan struct{})
	go func() {
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/system/storage/disk", nil))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		releaseOnce.Do(func() { close(release) })
		<-done
		t.Fatal("disk response waited for blocked candidate capacity")
	}
	releaseOnce.Do(func() { close(release) })
	if elapsed := time.Since(started); elapsed > 150*time.Millisecond {
		t.Fatalf("response took %s, want bounded candidate capacity probe", elapsed)
	}
	var body DiskCapacityResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.TemporaryRoots) != 2 || body.TemporaryRoots[0].Available || !body.TemporaryRoots[1].Available {
		t.Fatalf("temporary results = %#v, want first unavailable and healthy sibling measured", body.TemporaryRoots)
	}
}

func TestGetStorageDiskMeasuresRootWhenIdentityProbeBlocks(t *testing.T) {
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer func() {
		releaseOnce.Do(func() { close(release) })
		waitForDiskProbesToDrain(t)
	}()
	handler := NewHandler(HandlerConfig{
		DiskPath: "/home",
		DiskCapacity: func(context.Context, string) (DiskCapacity, error) {
			return measuredDiskCapacity(), nil
		},
		DiskRoots: func(context.Context) ([]DiskRootCandidate, error) {
			return []DiskRootCandidate{{RequestedPath: "/tmp"}}, nil
		},
		DiskIdentity: func(_ context.Context, path string) (string, error) {
			if path == "/tmp" {
				<-release
			}
			return "", errors.New("identity unavailable")
		},
	})
	handler.diskProbeTimeout = 30 * time.Millisecond
	router := newTestRouter(handler)
	response := httptest.NewRecorder()
	started := time.Now()
	done := make(chan struct{})
	go func() {
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/system/storage/disk", nil))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		releaseOnce.Do(func() { close(release) })
		<-done
		t.Fatal("disk response waited for filesystem identity")
	}
	releaseOnce.Do(func() { close(release) })
	if elapsed := time.Since(started); elapsed > 150*time.Millisecond {
		t.Fatalf("response took %s, want bounded identity probe", elapsed)
	}
	var body DiskCapacityResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.TemporaryRoots) != 1 || !body.TemporaryRoots[0].Available {
		t.Fatalf("temporary roots = %#v, want capacity despite unknown identity", body.TemporaryRoots)
	}
	if body.TemporaryRoots[0].SharedWithHome != nil {
		t.Fatalf("shared_with_home = %v, want unknown", body.TemporaryRoots[0].SharedWithHome)
	}
}

func TestDiskProbesBoundProcessWideBlockedOperations(t *testing.T) {
	const probeLimit = 8
	const requestCount = probeLimit + 1
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer func() {
		releaseOnce.Do(func() { close(release) })
		waitForDiskProbesToDrain(t)
	}()
	var started atomic.Int32
	entered := make(chan struct{}, requestCount)
	handler := NewHandler(HandlerConfig{
		DiskPath: "/home",
		DiskCapacity: func(context.Context, string) (DiskCapacity, error) {
			started.Add(1)
			entered <- struct{}{}
			<-release
			return measuredDiskCapacity(), nil
		},
	})
	handler.diskProbeTimeout = 100 * time.Millisecond
	router := newTestRouter(handler)
	done := make(chan *httptest.ResponseRecorder, requestCount)
	for range requestCount {
		go func() {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/system/storage/disk", nil))
			done <- response
		}()
	}

	var overLimit bool
	for range probeLimit {
		select {
		case <-entered:
		case <-time.After(300 * time.Millisecond):
			releaseOnce.Do(func() { close(release) })
			for range requestCount {
				<-done
			}
			t.Fatal("fewer than the allowed blocked probes started")
		}
	}
	select {
	case <-entered:
		overLimit = true
	case <-time.After(150 * time.Millisecond):
	}
	releaseOnce.Do(func() { close(release) })
	responseCount := 0
	for responseCount < requestCount {
		select {
		case <-done:
			responseCount++
		case <-time.After(500 * time.Millisecond):
			releaseOnce.Do(func() { close(release) })
			for responseCount < requestCount {
				<-done
				responseCount++
			}
			t.Fatal("request deadline did not release blocked probe callers")
		}
	}
	if overLimit || int(started.Load()) != probeLimit {
		t.Fatalf("started probes = %d, want process-wide limit %d", started.Load(), probeLimit)
	}
}

func waitForDiskProbesToDrain(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for len(diskProbeSlots) > 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(diskProbeSlots) > 0 {
		t.Errorf("%d disk probes did not exit after their blockers were released", len(diskProbeSlots))
	}
}

func measuredDiskCapacity() DiskCapacity {
	return DiskCapacity{TotalBytes: 100, UsedBytes: 70, AvailableBytes: 30, UsedPercent: 70}
}
