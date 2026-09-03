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
	"github.com/kandev/kandev/internal/office/shared"
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

// altDataset prices claude-opus-4-7 differently from sampleDataset (20/100
// vs 15/75 USD per million input/output). Used only by the
// LookupForModelWithVersion concurrency test to make a mismatched
// (pricing, version) pairing observable.
const altDataset = `{
  "anthropic": {
    "models": {
      "claude-opus-4-7": {"cost": {"input": 20.0, "output": 100.0, "cache_read": 2.0, "cache_write": 25.0}}
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

// CatalogVersion implements shared.PricingCatalogVersioner: "" before
// anything has loaded, and a non-empty RFC3339 timestamp once the cache
// has warmed from disk or refreshed from the network — models.dev's
// dataset carries no version field of its own, so the writer needs this
// "as-of" signal to attribute a models_dev_list-priced row.
func TestClient_CatalogVersion(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "models-dev.json")
	c, _ := newTestClient(t, cachePath)

	if v := c.CatalogVersion(); v != "" {
		t.Fatalf("CatalogVersion before any load = %q, want empty", v)
	}

	if err := c.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	v := c.CatalogVersion()
	if v == "" {
		t.Fatal("CatalogVersion after refresh = empty, want a timestamp")
	}
	if _, err := time.Parse(time.RFC3339, v); err != nil {
		t.Errorf("CatalogVersion = %q, not RFC3339: %v", v, err)
	}
}

// LookupForModelWithVersion implements shared.PricingLookupWithVersion:
// pricing matches LookupForModel and version matches CatalogVersion for the
// same warm cache state, and both are zero-valued together on a miss.
func TestClient_LookupForModelWithVersion(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "models-dev.json")
	c, _ := newTestClient(t, cachePath)

	if err := c.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	wantPricing, ok := c.LookupForModel(context.Background(), "claude-opus-4-7")
	if !ok {
		t.Fatal("expected hit on claude-opus-4-7")
	}
	wantVersion := c.CatalogVersion()

	pricing, version, ok := c.LookupForModelWithVersion(context.Background(), "claude-opus-4-7")
	if !ok {
		t.Fatal("expected hit on claude-opus-4-7")
	}
	if pricing != wantPricing {
		t.Errorf("LookupForModelWithVersion pricing = %+v, want %+v", pricing, wantPricing)
	}
	if version != wantVersion {
		t.Errorf("LookupForModelWithVersion version = %q, want %q", version, wantVersion)
	}

	if pricing, version, ok := c.LookupForModelWithVersion(context.Background(), "claude-unknown-99"); ok || pricing != (shared.ModelPricing{}) || version != "" {
		t.Errorf("expected miss on unknown model, got pricing=%+v version=%q ok=%v", pricing, version, ok)
	}
}

// TestClient_LookupForModelWithVersion_ConsistentUnderConcurrentRefresh is
// the regression test for the P1 "cost provenance lies" race: reading
// pricing and CatalogVersion via two independent lock acquisitions (as
// resolveCostForUsage did before this fix) lets a concurrent Refresh land
// in between and pair one catalogue's rates with a different catalogue's
// version identifier on the stored row. LookupForModelWithVersion must
// return the two from one atomic snapshot, so every observed pairing is one
// of the two catalogues actually served — never a hybrid.
//
// CatalogVersion has one-second resolution (it's RFC3339, by design — see
// its doc comment), so the base and alt refreshes are separated by a real
// sleep to guarantee distinguishable version strings; a tight refresh loop
// within the same wall-clock second would produce identical version labels
// for genuinely different catalogues and make this test unable to tell them
// apart. Reader goroutines hammer LookupForModelWithVersion continuously
// through the single base-to-alt transition, which is where the race
// window this test targets actually lives.
func TestClient_LookupForModelWithVersion_ConsistentUnderConcurrentRefresh(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "models-dev.json")

	var useAltDataset atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if useAltDataset.Load() {
			_, _ = w.Write([]byte(altDataset))
			return
		}
		_, _ = w.Write([]byte(sampleDataset))
	}))
	t.Cleanup(srv.Close)

	log := logger.Default()
	c := modelsdev.New(modelsdev.Config{
		CachePath:  cachePath,
		URL:        srv.URL,
		TTL:        time.Hour,
		HTTPClient: srv.Client(),
	}, log)

	ctx := context.Background()
	if err := c.Refresh(ctx); err != nil {
		t.Fatalf("initial refresh: %v", err)
	}
	basePricing, baseVersion, ok := c.LookupForModelWithVersion(ctx, "claude-opus-4-7")
	if !ok {
		t.Fatal("expected hit on claude-opus-4-7 after initial refresh")
	}

	// Guarantee the alt refresh lands in a different RFC3339 second than the
	// base refresh above, so the two version strings are distinguishable.
	time.Sleep(1100 * time.Millisecond)

	stop := make(chan struct{})
	var mismatches atomic.Int64

	// altPricing/altVersion are set once by the main goroutine after the alt
	// refresh below, and read concurrently by the reader goroutines started
	// just above it; guarded by altMu (not plain vars) so that sharing is
	// race-detector-clean, independent of the LookupForModelWithVersion
	// atomicity this test is actually exercising.
	var altMu sync.Mutex
	var altPricing shared.ModelPricing
	var altVersion string
	getAlt := func() (shared.ModelPricing, string) {
		altMu.Lock()
		defer altMu.Unlock()
		return altPricing, altVersion
	}

	const readers = 8
	var readersWG sync.WaitGroup
	readersWG.Add(readers)
	for i := 0; i < readers; i++ {
		go func() {
			defer readersWG.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				pricing, version, ok := c.LookupForModelWithVersion(ctx, "claude-opus-4-7")
				if !ok {
					continue
				}
				wantAltPricing, wantAltVersion := getAlt()
				switch version {
				case baseVersion:
					if pricing != basePricing {
						mismatches.Add(1)
					}
				case wantAltVersion:
					if wantAltVersion != "" && pricing != wantAltPricing {
						mismatches.Add(1)
					}
				default:
					// Neither known version yet (e.g. read raced ahead of the
					// alt refresh completing) — not itself a mismatch, skip.
				}
			}
		}()
	}

	// Perform the single base->alt transition while readers are hammering
	// the lookup concurrently — this Refresh call is exactly the race
	// window the pre-fix two-lock pattern could straddle.
	useAltDataset.Store(true)
	if err := c.Refresh(ctx); err != nil {
		t.Fatalf("alt refresh: %v", err)
	}
	newAltPricing, newAltVersion, ok := c.LookupForModelWithVersion(ctx, "claude-opus-4-7")
	if !ok {
		t.Fatal("expected hit on claude-opus-4-7 after alt refresh")
	}
	if newAltPricing == basePricing {
		t.Fatal("test setup bug: altDataset must price claude-opus-4-7 differently from sampleDataset")
	}
	if newAltVersion == baseVersion {
		t.Fatal("test setup bug: the alt refresh must produce a different catalogue version")
	}
	altMu.Lock()
	altPricing, altVersion = newAltPricing, newAltVersion
	altMu.Unlock()

	// Let readers keep hammering briefly against the now-settled alt state
	// before stopping, so the alt-version branch above gets real coverage.
	time.Sleep(50 * time.Millisecond)
	close(stop)
	readersWG.Wait()

	if mismatches.Load() > 0 {
		t.Fatalf("observed %d mismatched (pricing, version) pairings — LookupForModelWithVersion is not atomic", mismatches.Load())
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

	// A cold-boot lookup schedules a detached background refresh. Hold that
	// request at the gate so it cannot populate the cache before the lookup
	// reads it, then release and join the refresh before TempDir cleanup.
	gate := newRequestGate(t, sampleDataset)
	c := modelsdev.New(modelsdev.Config{
		CachePath:  cachePath,
		URL:        gate.server.URL,
		TTL:        time.Hour,
		HTTPClient: gate.server.Client(),
	}, logger.Default())

	// No Refresh — simulating cold boot before any HTTP fetch.
	if _, ok := c.LookupForModel(context.Background(), "claude-opus-4-7"); ok {
		t.Error("expected miss on cold-boot lookup")
	}
	gate.waitForFirstRequest(t)
	gate.releaseAll()
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatalf("join background refresh: %v", err)
	}
}

type requestGate struct {
	server      *httptest.Server
	started     chan struct{}
	release     chan struct{}
	releaseOnce sync.Once
	requests    atomic.Int32
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
	// Cleanup runs LIFO: registering releaseAll after server.Close means it
	// runs first, so a t.Fatal (which skips the explicit release below via
	// runtime.Goexit) still unblocks any parked handler before Close waits
	// on it.
	t.Cleanup(gate.server.Close)
	t.Cleanup(gate.releaseAll)
	return gate
}

func (g *requestGate) releaseAll() {
	g.releaseOnce.Do(func() { close(g.release) })
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
	// If waitForFirstRequest below times out, its t.Fatal runs
	// runtime.Goexit and skips the explicit releaseAll+wg.Wait() that
	// follows, leaving these 8 goroutines parked in the gate. This
	// Cleanup performs release-then-join as one atomic unit so it is
	// safe regardless of its LIFO position relative to newRequestGate's
	// own release Cleanup: without it, the parked goroutines complete
	// only once the deferred release fires, after the test has already
	// been marked done, and their t.Errorf call above then panics with
	// "Log in goroutine after Test has completed" instead of failing
	// cleanly.
	t.Cleanup(func() {
		gate.releaseAll()
		wg.Wait()
	})
	close(start)
	gate.waitForFirstRequest(t)
	gate.releaseAll()
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
	gate.releaseAll()
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

	gate.releaseAll()
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

// releaseAll must be safe to call more than once (the happy-path release
// races the t.Cleanup-registered release on the same gate) and must unblock
// a handler already parked in the select inside newRequestGate.
func TestRequestGate_ReleaseAllIsIdempotentAndUnblocksParkedHandlers(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "models-dev.json")
	gate := newRequestGate(t, sampleDataset)
	c := modelsdev.New(modelsdev.Config{
		CachePath:  cachePath,
		URL:        gate.server.URL,
		TTL:        time.Hour,
		HTTPClient: gate.server.Client(),
	}, logger.Default())

	var refreshErr error
	refreshDone := make(chan struct{})
	go func() {
		refreshErr = c.Refresh(context.Background())
		close(refreshDone)
	}()
	// Same self-contained release-then-join shape as
	// TestClient_ConcurrentRefreshCallsShareOneFetch: if waitForFirstRequest
	// below times out, this Cleanup still drains the goroutine before
	// t.TempDir() removes the directory it may be writing into. refreshDone
	// is closed (not a single-value send) so both this Cleanup and the
	// select below can receive from it without racing over who drains it.
	t.Cleanup(func() {
		gate.releaseAll()
		<-refreshDone
	})
	gate.waitForFirstRequest(t)

	gate.releaseAll()
	gate.releaseAll()

	select {
	case <-refreshDone:
		if refreshErr != nil {
			t.Fatalf("Refresh: %v", refreshErr)
		}
	case <-time.After(time.Second):
		t.Fatal("Refresh did not complete after releaseAll")
	}
	// t.Cleanup(gate.releaseAll) fires a third call on the same gate below.
}

// A parked requestGate handler must not block test teardown when the calling
// t.Run returns without releasing it — the same shape as waitForFirstRequest
// timing out and running t.Fatal (runtime.Goexit skips the explicit release
// that follows it), minus the induced failure so the assertion below is
// meaningful. Pre-fix (raw close(gate.release) only, no t.Cleanup release),
// the inner t.Run and the outer test both hang until the package's -timeout
// fires; post-fix it returns in milliseconds because the cleanup-registered
// gate.releaseAll unblocks the handler so httptest.Server.Close can proceed.
//
// dir and refreshDone are hoisted to the outer t so the Refresh goroutine is
// explicitly joined before this function returns: httptest.Server.Close only
// waits for the handler goroutine, not for the client goroutine to finish
// writeCacheAtomic, so leaving it unjoined races the inner t.TempDir cleanup
// against files the goroutine is still writing into that same directory.
func TestRequestGate_ParkedHandlerDoesNotBlockTestTeardown(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "models-dev.json")
	refreshDone := make(chan struct{})

	start := time.Now()
	t.Run("park-without-release", func(t *testing.T) {
		gate := newRequestGate(t, sampleDataset)
		c := modelsdev.New(modelsdev.Config{
			CachePath:  cachePath,
			URL:        gate.server.URL,
			TTL:        time.Hour,
			HTTPClient: gate.server.Client(),
		}, logger.Default())

		go func() {
			defer close(refreshDone)
			_ = c.Refresh(context.Background())
		}()
		gate.waitForFirstRequest(t)
		// Intentionally return without releasing the gate — the subtest's
		// t.Cleanup chain must release it during teardown.
	})
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("parked handler blocked teardown for %v, want well under 10s", elapsed)
	}

	select {
	case <-refreshDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Refresh goroutine did not finish after the subtest released the gate")
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
