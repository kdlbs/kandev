package modelsdev_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
      "gpt-5.4-mini":   {"cost": {"input": 0.5,  "output": 2.0, "cache_read": 0.1, "cache_write": 0.6}}
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

// First boot with no cache file returns miss without crashing.
func TestClient_FirstBootMissesGracefully(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "models-dev.json")
	c, _ := newTestClient(t, cachePath)
	// No Refresh — simulating cold boot before any HTTP fetch.
	if _, ok := c.LookupForModel(context.Background(), "claude-opus-4-7"); ok {
		t.Error("expected miss on cold-boot lookup")
	}
}
