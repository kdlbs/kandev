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
	"golang.org/x/sync/singleflight"

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

	once         sync.Once
	refreshGroup singleflight.Group

	mu       sync.RWMutex
	index    map[string]shared.ModelPricing
	info     map[string]ModelInfo
	loadedAt time.Time
	// catalogGen increments on every catalogue install (warmFromDisk or
	// refreshPhysical). CatalogVersion()'s RFC3339 string only has
	// one-second resolution, so two installs landing in the same
	// wall-clock second would report the same version — cacheIfVersionCurrent
	// uses this counter instead, so it can't mistake a fresher install for
	// the one a caller snapshotted.
	catalogGen uint64
	cacheBuf   []byte // raw on-disk JSON (parsed lazily on miss)
}

// ModelInfo holds non-pricing metadata from models.dev for a model.
type ModelInfo struct {
	ContextWindow int64
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
		info:       make(map[string]ModelInfo),
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

// LookupModelInfo returns model metadata from models.dev using the
// same normalization, lazy disk warm, and stale-while-revalidate
// behavior as LookupForModel.
func (c *Client) LookupModelInfo(ctx context.Context, modelID string) (ModelInfo, bool) {
	key, strategy := Normalize(modelID)
	if strategy != StrategyLookup {
		return ModelInfo{}, false
	}
	c.once.Do(func() { c.warmFromDisk(ctx) })

	c.mu.RLock()
	info, ok := c.info[key]
	c.mu.RUnlock()
	if ok {
		c.maybeRefresh(ctx)
		return info, true
	}

	if info, ok = c.parseModelInfoFromBuffer(key); ok {
		c.maybeRefresh(ctx)
		return info, true
	}

	c.maybeRefresh(ctx)
	return ModelInfo{}, false
}

// CatalogVersion implements shared.PricingCatalogVersioner. Returns the
// on-disk cache's load time in RFC3339 (UTC), or "" when nothing has been
// loaded yet (cold cache, no Lookup call has run). models.dev's dataset
// carries no version field of its own (see datasetEntry below) — the
// cache load/fetch time is the honest "as-of" identifier available.
func (c *Client) CatalogVersion() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.catalogVersionLocked()
}

// catalogVersionLocked returns CatalogVersion's value assuming c.mu is
// already held (read or write) by the caller. Factored out so
// LookupForModelWithVersion can read pricing and version together under one
// lock acquisition instead of two independent ones.
func (c *Client) catalogVersionLocked() string {
	if c.loadedAt.IsZero() {
		return ""
	}
	return c.loadedAt.UTC().Format(time.RFC3339)
}

// LookupForModelWithVersion implements shared.PricingLookupWithVersion.
// Identical lookup behavior to LookupForModel, but reads pricing and
// CatalogVersion from the same snapshot in every branch — including the
// cold-cache-buffer parse path, where the buffer and its version are
// captured together before parsing — so a background refresh landing
// mid-call can never pair one catalogue's rates with a different
// catalogue's version identifier (docs/specs/office/requirements/costs.md).
func (c *Client) LookupForModelWithVersion(ctx context.Context, modelID string) (shared.ModelPricing, string, bool) {
	key, strategy := Normalize(modelID)
	if strategy != StrategyLookup {
		return shared.ModelPricing{}, "", false
	}
	c.once.Do(func() { c.warmFromDisk(ctx) })

	c.mu.RLock()
	pricing, ok := c.index[key]
	version := c.catalogVersionLocked()
	c.mu.RUnlock()
	if ok {
		c.maybeRefresh(ctx)
		return pricing, version, true
	}

	buf, bufVersion, bufGen := c.snapshotBufferAndVersion()
	if len(buf) > 0 {
		if pricing, ok = lookupInDataset(buf, key); ok {
			c.cacheIfVersionCurrent(key, pricing, bufGen)
			c.maybeRefresh(ctx)
			return pricing, bufVersion, true
		}
	}

	c.maybeRefresh(ctx)
	return shared.ModelPricing{}, "", false
}

// cacheIfVersionCurrent stores pricing into the index under key, but only if
// the catalogue is still the one snapshotGen was captured from. Without this
// guard, a Refresh landing between the caller's snapshotBufferAndVersion call
// and this write would let a stale rate get written into the NEW index —
// refreshPhysical rebuilds c.index from the new buffer and replaces the map
// wholesale under c.mu — so a later, unrelated lookup for the same key would
// then read old rates paired with the new catalogue version, the exact
// provenance lie LookupForModelWithVersion exists to prevent
// (docs/specs/office/requirements/costs.md). Compares catalogGen rather than the RFC3339
// version string: the string only has one-second resolution, so two installs
// landing in the same wall-clock second would otherwise compare equal and
// defeat this guard. The returned (pricing, bufVersion) pair for THIS call is
// unaffected either way, since both were derived from the same buf snapshot.
func (c *Client) cacheIfVersionCurrent(key string, pricing shared.ModelPricing, snapshotGen uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.catalogGen != snapshotGen {
		return
	}
	c.index[key] = pricing
}

// snapshotBufferAndVersion returns the cache buffer, its catalogue version
// string, and its generation counter together under one lock, so a caller
// that parses buf afterward can report the version that actually produced
// it (and guard a delayed write with the generation) rather than whatever
// is current by the time the parse finishes.
func (c *Client) snapshotBufferAndVersion() ([]byte, string, uint64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cacheBuf, c.catalogVersionLocked(), c.catalogGen
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
		c.startBackgroundRefresh(context.WithoutCancel(ctx))
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
	c.catalogGen++
	c.mu.Unlock()
	if time.Since(stat.ModTime()) >= c.ttl {
		c.startBackgroundRefresh(context.WithoutCancel(ctx))
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

// parseModelInfoFromBuffer pulls one model entry out of the on-disk
// JSON and caches resulting metadata in the in-memory index. Returns
// (zero, false) when the key or metadata is absent.
func (c *Client) parseModelInfoFromBuffer(key string) (ModelInfo, bool) {
	c.mu.RLock()
	buf := c.cacheBuf
	c.mu.RUnlock()
	if len(buf) == 0 {
		return ModelInfo{}, false
	}
	info, ok := lookupModelInfoInDataset(buf, key)
	if !ok {
		return ModelInfo{}, false
	}
	c.mu.Lock()
	c.info[key] = info
	c.mu.Unlock()
	return info, true
}

// maybeRefresh fires a background refresh when the loaded buffer is stale.
// Refresh's per-client singleflight guard coalesces all callers, including
// concurrent pricing and metadata lookups.
func (c *Client) maybeRefresh(ctx context.Context) {
	c.mu.RLock()
	stale := c.loadedAt.IsZero() || time.Since(c.loadedAt) >= c.ttl
	c.mu.RUnlock()
	if stale {
		c.startBackgroundRefresh(context.WithoutCancel(ctx))
	}
}

// startBackgroundRefresh registers the refresh synchronously so every stale
// lookup joins the same in-flight singleflight call before returning. The
// result is still observed asynchronously, so stale lookups remain nonblocking.
// Callers choose whether the refresh should inherit or detach from their context.
func (c *Client) startBackgroundRefresh(ctx context.Context) {
	result := c.refreshResult(ctx)
	go func() {
		if err := (<-result).Err; err != nil {
			c.logger.Warn("models.dev refresh failed", zap.Error(err))
		}
	}()
}

// Refresh pulls the latest dataset from models.dev and atomically
// swaps the cache file. Network or write errors leave the existing
// file (and in-memory index) untouched.
func (c *Client) Refresh(ctx context.Context) error {
	result := c.refreshResult(ctx)
	return (<-result).Err
}

func (c *Client) refreshResult(ctx context.Context) <-chan singleflight.Result {
	return c.refreshGroup.DoChan("modelsdev-refresh", func() (interface{}, error) {
		return c.refreshPhysicalSafely(ctx)
	})
}

func (c *Client) refreshPhysicalSafely(ctx context.Context) (value interface{}, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			c.logger.Warn("models.dev refresh panicked", zap.Any("recover", recovered))
			err = fmt.Errorf("models.dev refresh panicked")
		}
	}()
	return nil, c.refreshPhysical(ctx)
}

func (c *Client) refreshPhysical(ctx context.Context) error {
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
	newIndex := make(map[string]shared.ModelPricing, len(c.index))
	for k := range c.index {
		if pricing, ok := lookupInDataset(buf, k); ok {
			newIndex[k] = pricing
		}
	}
	newInfo := make(map[string]ModelInfo, len(c.info))
	for k := range c.info {
		if info, ok := lookupModelInfoInDataset(buf, k); ok {
			newInfo[k] = info
		}
	}
	c.cacheBuf = buf
	c.index = newIndex
	c.info = newInfo
	c.loadedAt = time.Now().UTC()
	c.catalogGen++
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
	tmp, err := os.CreateTemp(filepath.Dir(c.cachePath), "."+filepath.Base(c.cachePath)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create cache tmp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()
	if written, err := tmp.Write(buf); err != nil {
		return fmt.Errorf("write cache tmp: %w", err)
	} else if written != len(buf) {
		return fmt.Errorf("write cache tmp: %w", io.ErrShortWrite)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync cache tmp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close cache tmp: %w", err)
	}
	if err := os.Rename(tmpName, c.cachePath); err != nil {
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
	Limit struct {
		Context int64 `json:"context"`
	} `json:"limit"`
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

// lookupModelInfoInDataset searches the provider-keyed JSON for a
// model id and returns metadata when models.dev exposes it.
func lookupModelInfoInDataset(buf []byte, key string) (ModelInfo, bool) {
	dataset := make(map[string]datasetProvider)
	if err := json.Unmarshal(buf, &dataset); err != nil {
		return ModelInfo{}, false
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
			if entry.Limit.Context <= 0 {
				continue
			}
			return ModelInfo{ContextWindow: entry.Limit.Context}, true
		}
	}
	return ModelInfo{}, false
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
