package modelsdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/shared"

	"go.uber.org/zap"
)

// DefaultDatasetURL is the public models.dev dataset endpoint. The
// schema is checked at lookup time so a future format change degrades
// to "miss + estimated" rather than crashing the subscriber.
const DefaultDatasetURL = "https://models.dev/api.json"

// DefaultTTL controls how long an on-disk cache file is treated as
// fresh. Stale-while-revalidate: the lookup still serves the existing
// file and a background goroutine refreshes.
const DefaultTTL = 24 * time.Hour

// Client serves per-model pricing lookups backed by a daily-refreshed
// disk cache. See the package doc for the per-CLI shape contract.
//
// Lifecycle is zero-cost when unused — the cache file is only read on
// first Lookup, and only the queried model's entry is parsed into the
// in-memory index. Workspaces running only claude-acp never trip a
// fetch because Layer A handles every event before Lookup is reached.
type Client struct {
	cachePath  string
	url        string
	ttl        time.Duration
	httpClient *http.Client
	logger     *logger.Logger

	once sync.Once

	mu       sync.RWMutex
	index    map[string]shared.ModelPricing
	loadedAt time.Time
	cacheBuf []byte // raw on-disk JSON (parsed lazily on miss)
}

// Config bundles construction parameters.
type Config struct {
	CachePath  string
	URL        string
	TTL        time.Duration
	HTTPClient *http.Client
}

// New constructs a Client. No disk or network I/O happens here — both
// are deferred until the first Lookup that misses the in-memory index.
// CachePath should be `<workspace-data-dir>/cache/models-dev.json`;
// callers are responsible for resolving the workspace data dir.
func New(cfg Config, log *logger.Logger) *Client {
	if cfg.URL == "" {
		cfg.URL = DefaultDatasetURL
	}
	if cfg.TTL <= 0 {
		cfg.TTL = DefaultTTL
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		cachePath:  cfg.CachePath,
		url:        cfg.URL,
		ttl:        cfg.TTL,
		httpClient: cfg.HTTPClient,
		logger:     log.WithFields(zap.String("component", "modelsdev")),
		index:      make(map[string]shared.ModelPricing),
	}
}

// LookupForModel implements shared.PricingLookup. The model id is
// normalized first; logical aliases and auto-routers short-circuit to
// (zero, false) so the subscriber records the row as estimated. A
// missing or stale cache file kicks off a background refresh and
// returns whatever the current cache has (which may be (zero, false)
// on a cold first boot).
func (c *Client) LookupForModel(ctx context.Context, modelID string) (shared.ModelPricing, bool) {
	key, strategy := Normalize(modelID)
	if strategy != StrategyLookup {
		return shared.ModelPricing{}, false
	}
	c.once.Do(func() { c.warmFromDisk(ctx) })

	c.mu.RLock()
	pricing, ok := c.index[key]
	c.mu.RUnlock()
	if ok {
		c.maybeRefresh(ctx)
		return pricing, true
	}

	if pricing, ok = c.parseFromBuffer(key); ok {
		c.maybeRefresh(ctx)
		return pricing, true
	}

	c.maybeRefresh(ctx)
	return shared.ModelPricing{}, false
}

// warmFromDisk reads the cache file into cacheBuf so subsequent
// lookups can parse individual model entries lazily. Missing or
// unreadable cache is non-fatal — the next refresh tick warms it.
func (c *Client) warmFromDisk(ctx context.Context) {
	if c.cachePath == "" {
		return
	}
	stat, err := os.Stat(c.cachePath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			c.logger.Warn("models.dev cache stat failed",
				zap.String("path", c.cachePath), zap.Error(err))
		}
		// File missing on first boot — schedule a refresh; lookup
		// returns miss this turn.
		go c.refreshSafe(ctx)
		return
	}
	buf, err := os.ReadFile(c.cachePath)
	if err != nil {
		c.logger.Warn("models.dev cache read failed",
			zap.String("path", c.cachePath), zap.Error(err))
		return
	}
	c.mu.Lock()
	c.cacheBuf = buf
	c.loadedAt = stat.ModTime()
	c.mu.Unlock()
	if time.Since(stat.ModTime()) >= c.ttl {
		go c.refreshSafe(ctx)
	}
}

// parseFromBuffer pulls one model entry out of the on-disk JSON and
// caches the resulting pricing in the in-memory index. Returns
// (zero, false) when the key isn't present in the dataset.
func (c *Client) parseFromBuffer(key string) (shared.ModelPricing, bool) {
	c.mu.RLock()
	buf := c.cacheBuf
	c.mu.RUnlock()
	if len(buf) == 0 {
		return shared.ModelPricing{}, false
	}
	pricing, ok := lookupInDataset(buf, key)
	if !ok {
		return shared.ModelPricing{}, false
	}
	c.mu.Lock()
	c.index[key] = pricing
	c.mu.Unlock()
	return pricing, true
}

// maybeRefresh fires a background refresh when the loaded buffer is
// stale. No-op when refresh is in progress (sync.Once-style guard
// implicitly enforced by atomicity of the staleness check + ttl).
func (c *Client) maybeRefresh(ctx context.Context) {
	c.mu.RLock()
	stale := c.loadedAt.IsZero() || time.Since(c.loadedAt) >= c.ttl
	c.mu.RUnlock()
	if stale {
		go c.refreshSafe(ctx)
	}
}

// refreshSafe wraps Refresh in a panic guard + warning log; safe to
// run from a goroutine.
func (c *Client) refreshSafe(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			c.logger.Warn("models.dev refresh panicked", zap.Any("recover", r))
		}
	}()
	if err := c.Refresh(ctx); err != nil {
		c.logger.Warn("models.dev refresh failed", zap.Error(err))
	}
}

// Refresh pulls the latest dataset from models.dev and atomically
// swaps the cache file. Network or write errors leave the existing
// file (and in-memory index) untouched.
func (c *Client) Refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", c.url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch %s: status %d", c.url, resp.StatusCode)
	}
	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if err := c.writeCacheAtomic(buf); err != nil {
		return err
	}

	c.mu.Lock()
	c.cacheBuf = buf
	c.loadedAt = time.Now()
	// Re-populate every entry already in the in-memory index so a
	// stale price doesn't survive the refresh.
	for k := range c.index {
		if pricing, ok := lookupInDataset(buf, k); ok {
			c.index[k] = pricing
		} else {
			delete(c.index, k)
		}
	}
	c.mu.Unlock()
	return nil
}

func (c *Client) writeCacheAtomic(buf []byte) error {
	if c.cachePath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(c.cachePath), 0o755); err != nil {
		return fmt.Errorf("mkdir cache dir: %w", err)
	}
	tmp := c.cachePath + ".tmp"
	if err := os.WriteFile(tmp, buf, 0o644); err != nil {
		return fmt.Errorf("write cache tmp: %w", err)
	}
	if err := os.Rename(tmp, c.cachePath); err != nil {
		return fmt.Errorf("rename cache: %w", err)
	}
	return nil
}

// datasetEntry is the on-the-wire shape from models.dev. The dataset
// is provider-keyed at the top level; each provider holds a `models`
// map. Pricing fields are dollars-per-million-tokens (floats).
//
// Field names follow models.dev convention. The exact schema is
// verified once on first lookup; new fields are ignored.
type datasetEntry struct {
	Cost struct {
		Input      float64 `json:"input"`
		Output     float64 `json:"output"`
		CacheRead  float64 `json:"cache_read"`
		CacheWrite float64 `json:"cache_write"`
	} `json:"cost"`
}

type datasetProvider struct {
	Models map[string]datasetEntry `json:"models"`
}

// lookupInDataset searches the (provider-keyed) JSON for a model id.
// Returns (zero, false) when the key isn't present or the schema
// drifted such that pricing fields can't be parsed. Tries the key
// verbatim first, then with hyphen <-> dot swaps to cover models.dev's
// canonical-id quirks (e.g. "gpt-5.4-mini" <-> "gpt-5-4-mini").
func lookupInDataset(buf []byte, key string) (shared.ModelPricing, bool) {
	dataset := make(map[string]datasetProvider)
	if err := json.Unmarshal(buf, &dataset); err != nil {
		return shared.ModelPricing{}, false
	}

	candidates := []string{key, swapHyphenDot(key)}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		for _, provider := range dataset {
			entry, ok := provider.Models[candidate]
			if !ok {
				continue
			}
			return shared.ModelPricing{
				InputPerMillion:       toSubcentsPerMillion(entry.Cost.Input),
				CachedReadPerMillion:  toSubcentsPerMillion(entry.Cost.CacheRead),
				CachedWritePerMillion: toSubcentsPerMillion(entry.Cost.CacheWrite),
				OutputPerMillion:      toSubcentsPerMillion(entry.Cost.Output),
			}, true
		}
	}
	return shared.ModelPricing{}, false
}

// swapHyphenDot converts hyphens to dots and vice-versa so a key like
// "gpt-5.4-mini" also tries "gpt-5-4-mini" against the dataset.
func swapHyphenDot(s string) string {
	if !strings.ContainsAny(s, "-.") {
		return ""
	}
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '-':
			out[i] = '.'
		case '.':
			out[i] = '-'
		default:
			out[i] = s[i]
		}
	}
	swapped := string(out)
	if swapped == s {
		return ""
	}
	return swapped
}

// toSubcentsPerMillion converts a dollars-per-million-tokens float
// into the integer subcents-per-million unit used by
// office_cost_events. 1 USD = 10000 subcents.
func toSubcentsPerMillion(dollars float64) int64 {
	if dollars <= 0 {
		return 0
	}
	return int64(dollars * 10000)
}
