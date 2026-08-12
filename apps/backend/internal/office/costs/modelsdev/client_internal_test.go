package modelsdev

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/shared"
)

// TestClient_CacheIfVersionCurrent_SkipsStaleWrite is the deterministic
// regression test for the P1 cold-buffer-parse race: a caller that
// snapshotted an old catalogue version must not write into the index once a
// concurrent Refresh has moved the catalogue on, or a later, unrelated
// lookup for the same key would read old rates paired with the new
// catalogue version. Runs entirely single-threaded (no goroutines, no
// timing-dependent race) by driving cacheIfVersionCurrent directly with a
// snapshot version that is deliberately stale.
func TestClient_CacheIfVersionCurrent_SkipsStaleWrite(t *testing.T) {
	c := &Client{index: make(map[string]shared.ModelPricing)}
	c.loadedAt = time.Now().UTC()
	staleVersion := c.CatalogVersion()

	// Advance the catalogue past a full RFC3339 second so the two versions
	// are guaranteed distinguishable, then simulate the concurrent Refresh
	// that moved the catalogue on after the caller's snapshot was taken.
	c.loadedAt = c.loadedAt.Add(2 * time.Second)
	currentVersion := c.CatalogVersion()
	if currentVersion == staleVersion {
		t.Fatal("test setup bug: currentVersion must differ from staleVersion")
	}

	stalePricing := shared.ModelPricing{InputPerMillion: 999}
	c.cacheIfVersionCurrent("stale-key", stalePricing, staleVersion)
	if _, ok := c.index["stale-key"]; ok {
		t.Error("cacheIfVersionCurrent must not write when snapshotVersion is stale (catalogue moved on)")
	}

	freshPricing := shared.ModelPricing{InputPerMillion: 111}
	c.cacheIfVersionCurrent("fresh-key", freshPricing, currentVersion)
	if got, ok := c.index["fresh-key"]; !ok || got != freshPricing {
		t.Errorf("cacheIfVersionCurrent must write when snapshotVersion matches the current catalogue, got %+v ok=%v", got, ok)
	}
}
