package modelsdev

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/shared"
)

// TestClient_CacheIfVersionCurrent_SkipsStaleWrite is the deterministic
// regression test for the P1 cold-buffer-parse race: a caller that
// snapshotted an old catalogue generation must not write into the index once
// a concurrent Refresh has moved the catalogue on, or a later, unrelated
// lookup for the same key would read old rates paired with the new
// catalogue version. Runs entirely single-threaded (no goroutines, no
// timing-dependent race) by driving cacheIfVersionCurrent directly with a
// snapshot generation that is deliberately stale.
func TestClient_CacheIfVersionCurrent_SkipsStaleWrite(t *testing.T) {
	c := &Client{index: make(map[string]shared.ModelPricing)}
	c.loadedAt = time.Now().UTC()
	c.catalogGen = 1
	staleGen := c.catalogGen

	// Simulate the concurrent Refresh that moved the catalogue on after the
	// caller's snapshot was taken.
	c.loadedAt = c.loadedAt.Add(2 * time.Second)
	c.catalogGen++
	currentGen := c.catalogGen

	stalePricing := shared.ModelPricing{InputPerMillion: 999}
	c.cacheIfVersionCurrent("stale-key", stalePricing, staleGen)
	if _, ok := c.index["stale-key"]; ok {
		t.Error("cacheIfVersionCurrent must not write when snapshotGen is stale (catalogue moved on)")
	}

	freshPricing := shared.ModelPricing{InputPerMillion: 111}
	c.cacheIfVersionCurrent("fresh-key", freshPricing, currentGen)
	if got, ok := c.index["fresh-key"]; !ok || got != freshPricing {
		t.Errorf("cacheIfVersionCurrent must write when snapshotGen matches the current catalogue, got %+v ok=%v", got, ok)
	}
}

// TestClient_CacheIfVersionCurrent_SameSecondInstallsAreDistinguishable is
// the regression test for the CodeRabbit finding on PR #2606: CatalogVersion
// is an RFC3339 string with one-second resolution, so two catalogue installs
// landing in the same wall-clock second would report the identical version
// string and defeat a string-based staleness guard. catalogGen has no such
// resolution limit — this asserts a stale snapshot is still rejected even
// when loadedAt (and therefore CatalogVersion()) does not advance at all.
func TestClient_CacheIfVersionCurrent_SameSecondInstallsAreDistinguishable(t *testing.T) {
	c := &Client{index: make(map[string]shared.ModelPricing)}
	now := time.Now().UTC()
	c.loadedAt = now
	c.catalogGen = 1
	staleGen := c.catalogGen
	staleVersion := c.CatalogVersion()

	// Second install lands in the exact same wall-clock second: loadedAt
	// (and thus CatalogVersion()'s RFC3339 string) is unchanged, but this is
	// a genuinely distinct catalogue install.
	c.loadedAt = now
	c.catalogGen++
	if c.CatalogVersion() != staleVersion {
		t.Fatal("test setup bug: CatalogVersion must collide within the same second")
	}

	stalePricing := shared.ModelPricing{InputPerMillion: 999}
	c.cacheIfVersionCurrent("stale-key", stalePricing, staleGen)
	if _, ok := c.index["stale-key"]; ok {
		t.Error("cacheIfVersionCurrent must reject a stale generation even when CatalogVersion()'s string is unchanged")
	}
}
