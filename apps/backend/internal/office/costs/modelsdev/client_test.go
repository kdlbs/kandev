package modelsdev_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/costs/modelsdev"
)

// sampleDataset mimics the models.dev /api.json shape: provider keys
// at the top level, each carrying a `models` map.
const sampleDataset = `{
  "anthropic": {
    "models": {
      "claude-opus-4-7":  {"cost": {"input": 15.0,  "output": 75.0, "cache_read": 1.5, "cache_write": 18.75}},
      "claude-sonnet-4-5": {"cost": {"input": 3.0,   "output": 15.0, "cache_read": 0.3, "cache_write": 3.75}}
    }
  },
	  "openai": {
	    "models": {
	      "gpt-5-mini":     {"cost": {"input": 0.4,  "output": 1.6, "cache_read": 0.1, "cache_write": 0.5}},
	      "gpt-5.3-codex-spark": {"cost": {"input": 0.4, "output": 1.6}, "limit": {"context": 128000}},
	      "gpt-5.4-zero": {"cost": {"input": 0.4, "output": 1.6}, "limit": {"context": 0}},
	      "gpt.5-4.zero": {"cost": {"input": 0.4, "output": 1.6}, "limit": {"context": 64000}},
	      "gpt-5.4-mini":   {"cost": {"input": 0.5,  "output": 2.0, "cache_read": 0.1, "cache_write": 0.6}, "limit": {"context": 256000}}
	    }
	  },
  "google": {
    "models": {
      "gemini-2.5-pro": {"cost": {"input": 1.25, "output": 10.0, "cache_read": 0.31, "cache_write": 1.56}}
    }
  }
}`

func newStubServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newTestClient(t *testing.T, cachePath string) (*modelsdev.Client, *httptest.Server) {
	t.Helper()
	srv := newStubServer(t, sampleDataset)
	log := logger.Default()
	c := modelsdev.New(modelsdev.Config{
		CachePath:  cachePath,
		URL:        srv.URL,
		TTL:        time.Hour,
		HTTPClient: srv.Client(),
	}, log)
	return c, srv
}

// Refresh writes a parseable cache file from a stubbed HTTP server.
func TestClient_RefreshWritesCache(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "models-dev.json")
	c, _ := newTestClient(t, cachePath)

	if err := c.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("cache file not created: %v", err)
	}
}

// Lookup returns expected pricing for a known model, returns
// (zero, false) for unknown models, and returns (zero, false) for
// logical-alias model ids (claude-acp's sonnet / haiku).
func TestClient_Lookup(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "models-dev.json")
	c, _ := newTestClient(t, cachePath)

	if err := c.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	pricing, ok := c.LookupForModel(context.Background(), "claude-opus-4-7")
	if !ok {
		t.Fatal("expected hit on claude-opus-4-7")
	}
	// 15 USD/M input -> 150000 subcents/M.
	if pricing.InputPerMillion != 150000 {
		t.Errorf("InputPerMillion = %d, want 150000", pricing.InputPerMillion)
	}
	if pricing.OutputPerMillion != 750000 {
		t.Errorf("OutputPerMillion = %d, want 750000", pricing.OutputPerMillion)
	}
	if pricing.CachedReadPerMillion != 15000 {
		t.Errorf("CachedReadPerMillion = %d, want 15000", pricing.CachedReadPerMillion)
	}
	if pricing.CachedWritePerMillion != 187500 {
		t.Errorf("CachedWritePerMillion = %d, want 187500", pricing.CachedWritePerMillion)
	}

	// Logical alias short-circuits to miss.
	if _, ok := c.LookupForModel(context.Background(), "sonnet"); ok {
		t.Error("expected miss on logical alias sonnet")
	}
	// Unknown model.
	if _, ok := c.LookupForModel(context.Background(), "claude-unknown-99"); ok {
		t.Error("expected miss on unknown model")
	}
}

// codex-acp model ids carry a /<effort> suffix and use dotted
// versions. Normalize strips the effort; the dataset uses dotted form
// too so the verbatim lookup hits.
func TestClient_NormalizesCodexAndOpencodeForms(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "models-dev.json")
	c, _ := newTestClient(t, cachePath)
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	// codex-acp: gpt-5.4-mini/medium -> gpt-5.4-mini.
	if _, ok := c.LookupForModel(context.Background(), "gpt-5.4-mini/medium"); !ok {
		t.Error("expected hit on codex-acp shaped id")
	}
	// opencode-acp: github-copilot/gpt-5-mini -> gpt-5-mini.
	if _, ok := c.LookupForModel(context.Background(), "github-copilot/gpt-5-mini"); !ok {
		t.Error("expected hit on opencode-acp shaped id")
	}
}

func TestClient_LookupModelInfo(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "models-dev.json")
	c, _ := newTestClient(t, cachePath)
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	info, ok := c.LookupModelInfo(context.Background(), "gpt-5.3-codex-spark")
	if !ok {
		t.Fatal("expected hit on gpt-5.3-codex-spark")
	}
	if info.ContextWindow != 128000 {
		t.Errorf("ContextWindow = %d, want 128000", info.ContextWindow)
	}
}

func TestClient_LookupModelInfoNormalizesModelID(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "models-dev.json")
	c, _ := newTestClient(t, cachePath)
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	info, ok := c.LookupModelInfo(context.Background(), "github-copilot/gpt-5.4-mini/medium")
	if !ok {
		t.Fatal("expected hit on normalized gpt-5.4-mini")
	}
	if info.ContextWindow != 256000 {
		t.Errorf("ContextWindow = %d, want 256000", info.ContextWindow)
	}
}

func TestClient_LookupModelInfoTriesSwappedCandidateAfterZeroLimit(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "models-dev.json")
	c, _ := newTestClient(t, cachePath)
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	info, ok := c.LookupModelInfo(context.Background(), "gpt-5.4-zero")
	if !ok {
		t.Fatal("expected fallback hit on swapped model id")
	}
	if info.ContextWindow != 64000 {
		t.Errorf("ContextWindow = %d, want 64000", info.ContextWindow)
	}
}

func TestClient_LookupModelInfoMissesGracefully(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "models-dev.json")
	c, _ := newTestClient(t, cachePath)
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if _, ok := c.LookupModelInfo(context.Background(), "claude-opus-4-7"); ok {
		t.Error("expected miss when model has no context limit")
	}
	if _, ok := c.LookupModelInfo(context.Background(), "gpt-unknown"); ok {
		t.Error("expected miss on unknown model")
	}
	if _, ok := c.LookupModelInfo(context.Background(), "sonnet"); ok {
		t.Error("expected miss on logical alias sonnet")
	}
}

// First boot with no cache file returns miss without crashing.
func TestClient_FirstBootMissesGracefully(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "models-dev.json")

	// A cold-boot lookup schedules a background refresh against the
	// configured URL. Block that refresh at the server so it can't
	// populate the cache before the lookup reads it — otherwise the
	// "miss" we're asserting races a fast background fetch and flakes
	// into a hit. The request exits when the lookup context is canceled,
	// before TempDir cleanup can race a cache write.
	ctx, cancel := context.WithCancel(context.Background())
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(func() {
		cancel()
		srv.Close()
	})
	c := modelsdev.New(modelsdev.Config{
		CachePath:  cachePath,
		URL:        srv.URL,
		TTL:        time.Hour,
		HTTPClient: srv.Client(),
	}, logger.Default())

	// No Refresh — simulating cold boot before any HTTP fetch.
	if _, ok := c.LookupForModel(ctx, "claude-opus-4-7"); ok {
		t.Error("expected miss on cold-boot lookup")
	}
}

type requestGate struct {
	server   *httptest.Server
	started  chan struct{}
	release  chan struct{}
	requests atomic.Int32
}

func newRequestGate(t *testing.T, body string) *requestGate {
	t.Helper()
	gate := &requestGate{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	gate.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := gate.requests.Add(1)
		if count == 1 {
			close(gate.started)
		}
		select {
		case <-gate.release:
		case <-r.Context().Done():
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(gate.server.Close)
	return gate
}

func (g *requestGate) waitForFirstRequest(t *testing.T) {
	t.Helper()
	select {
	case <-g.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the first models.dev request")
	}
}

func TestClient_ConcurrentRefreshCallsShareOneFetch(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "models-dev.json")
	gate := newRequestGate(t, sampleDataset)
	c := modelsdev.New(modelsdev.Config{
		CachePath:  cachePath,
		URL:        gate.server.URL,
		TTL:        time.Hour,
		HTTPClient: gate.server.Client(),
	}, logger.Default())

	start := make(chan struct{})
	var wg sync.WaitGroup
	for index := 0; index < 8; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if err := c.Refresh(context.Background()); err != nil {
				t.Errorf("Refresh: %v", err)
			}
		}()
	}
	close(start)
	gate.waitForFirstRequest(t)
	close(gate.release)
	wg.Wait()

	if got := gate.requests.Load(); got != 1 {
		t.Fatalf("models.dev request count = %d, want 1", got)
	}
}

func TestClient_ConcurrentLookupsShareOneBackgroundFetch(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "models-dev.json")
	if err := os.WriteFile(cachePath, []byte(sampleDataset), 0o644); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(cachePath, old, old); err != nil {
		t.Fatalf("age cache: %v", err)
	}
	gate := newRequestGate(t, sampleDataset)
	c := modelsdev.New(modelsdev.Config{
		CachePath:  cachePath,
		URL:        gate.server.URL,
		TTL:        time.Millisecond,
		HTTPClient: gate.server.Client(),
	}, logger.Default())

	start := make(chan struct{})
	var wg sync.WaitGroup
	for index := 0; index < 12; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			if index%2 == 0 {
				_, _ = c.LookupForModel(context.Background(), "gpt-5.3-codex-spark")
				return
			}
			_, _ = c.LookupModelInfo(context.Background(), "gpt-5.3-codex-spark")
		}(index)
	}
	close(start)
	wg.Wait()
	gate.waitForFirstRequest(t)
	if got := gate.requests.Load(); got != 1 {
		t.Fatalf("background models.dev request count before release = %d, want 1", got)
	}
	refreshDone := make(chan error, 1)
	go func() { refreshDone <- c.Refresh(context.Background()) }()
	close(gate.release)
	select {
	case err := <-refreshDone:
		if err != nil {
			t.Fatalf("joined background Refresh: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("joined background Refresh did not complete")
	}
	if got := gate.requests.Load(); got != 1 {
		t.Fatalf("background models.dev request count = %d, want 1", got)
	}
}

func TestClient_CanceledStaleLookupDoesNotCancelJoinedRefresh(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "models-dev.json")
	if err := os.WriteFile(cachePath, []byte(sampleDataset), 0o644); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(cachePath, old, old); err != nil {
		t.Fatalf("age cache: %v", err)
	}
	gate := newRequestGate(t, sampleDataset)
	c := modelsdev.New(modelsdev.Config{
		CachePath:  cachePath,
		URL:        gate.server.URL,
		TTL:        time.Millisecond,
		HTTPClient: gate.server.Client(),
	}, logger.Default())

	lookupCtx, cancelLookup := context.WithCancel(context.Background())
	_, _ = c.LookupForModel(lookupCtx, "gpt-5.3-codex-spark")
	gate.waitForFirstRequest(t)
	cancelLookup()

	refreshDone := make(chan error, 1)
	go func() { refreshDone <- c.Refresh(context.Background()) }()
	select {
	case err := <-refreshDone:
		t.Fatalf("joined refresh returned before the shared request completed: %v", err)
	case <-time.After(25 * time.Millisecond):
	}

	close(gate.release)
	select {
	case err := <-refreshDone:
		if err != nil {
			t.Fatalf("joined Refresh: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("joined Refresh did not complete")
	}
	if got := gate.requests.Load(); got != 1 {
		t.Fatalf("models.dev request count after canceled lookup = %d, want 1", got)
	}
}

func TestClient_FailedRefreshPreservesCacheAndAllowsRetry(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "models-dev.json")
	var fail atomic.Bool
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		if fail.Load() {
			http.Error(w, "upstream unavailable", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(sampleDataset))
	}))
	t.Cleanup(server.Close)
	c := modelsdev.New(modelsdev.Config{
		CachePath:  cachePath,
		URL:        server.URL,
		TTL:        time.Hour,
		HTTPClient: server.Client(),
	}, logger.Default())

	if err := c.Refresh(context.Background()); err != nil {
		t.Fatalf("initial Refresh: %v", err)
	}
	before, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read initial cache: %v", err)
	}
	if _, ok := c.LookupForModel(context.Background(), "claude-opus-4-7"); !ok {
		t.Fatal("initial cache lookup failed")
	}

	fail.Store(true)
	if err := c.Refresh(context.Background()); err == nil {
		t.Fatal("failed Refresh unexpectedly succeeded")
	}
	after, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read cache after failed refresh: %v", err)
	}
	if string(after) != string(before) {
		t.Fatal("failed refresh replaced the valid cache")
	}
	if _, ok := c.LookupForModel(context.Background(), "claude-opus-4-7"); !ok {
		t.Fatal("failed refresh discarded valid in-memory data")
	}

	fail.Store(false)
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatalf("retry Refresh: %v", err)
	}
	if got := requests.Load(); got != 3 {
		t.Fatalf("request count after failed retry = %d, want 3", got)
	}
	assertNoCacheTemps(t, cachePath)
}

func TestClient_CanceledRefreshAllowsRetryAndCleansTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "models-dev.json")
	firstStarted := make(chan struct{})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			close(firstStarted)
			<-r.Context().Done()
			return
		}
		_, _ = w.Write([]byte(sampleDataset))
	}))
	t.Cleanup(server.Close)
	c := modelsdev.New(modelsdev.Config{
		CachePath:  cachePath,
		URL:        server.URL,
		TTL:        time.Hour,
		HTTPClient: server.Client(),
	}, logger.Default())

	ctx, cancel := context.WithCancel(context.Background())
	refreshDone := make(chan error, 1)
	go func() { refreshDone <- c.Refresh(ctx) }()
	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for cancelable refresh")
	}
	cancel()
	select {
	case err := <-refreshDone:
		if err == nil {
			t.Fatal("canceled Refresh unexpectedly succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("canceled Refresh did not return")
	}

	if err := c.Refresh(context.Background()); err != nil {
		t.Fatalf("retry Refresh: %v", err)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("request count after canceled retry = %d, want 2", got)
	}
	assertNoCacheTemps(t, cachePath)
}

func assertNoCacheTemps(t *testing.T, cachePath string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(cachePath), "."+filepath.Base(cachePath)+".tmp-*"))
	if err != nil {
		t.Fatalf("glob cache temps: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("cache temporary files remain: %v", matches)
	}
}
