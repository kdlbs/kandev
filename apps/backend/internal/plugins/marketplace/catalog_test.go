package marketplace

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// indexJSON is a minimal valid index.json body with the given plugins block.
func indexJSON(pluginsBlock string) string {
	return `{"schema_version":1,"generated_at":"2026-07-18T00:00:00Z",` +
		`"source":{"name":"Test","url":""},"plugins":[` + pluginsBlock + `]}`
}

func entryJSON(id, version string, stars int) string {
	return `{"id":"` + id + `","name":"` + id + `","version":"` + version +
		`","stars":` + itoa(stars) + `,"package_url":"https://ex/` + id + `.tar.gz"}`
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	store := newTestSourceStore(t)
	return NewService(store, nil)
}

func serve(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCatalogMergesSourcesAndDedupesFirstWins(t *testing.T) {
	official := serve(t, indexJSON(entryJSON("alpha", "1.0.0", 10)+","+entryJSON("beta", "2.0.0", 5)))
	acme := serve(t, indexJSON(entryJSON("beta", "9.9.9", 999)+","+entryJSON("gamma", "1.0.0", 3)))

	s := newTestService(t)
	if err := s.store.EnsureBuiltin("Official", official.URL); err != nil {
		t.Fatal(err)
	}
	if _, err := s.store.Add("Acme", acme.URL); err != nil {
		t.Fatal(err)
	}

	result, err := s.Catalog(context.Background(), nil)
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	if len(result.Plugins) != 3 {
		t.Fatalf("want 3 merged plugins, got %d", len(result.Plugins))
	}
	beta := findEntry(result.Plugins, "beta")
	if beta == nil || beta.Version != "2.0.0" || beta.SourceName != "Official" {
		t.Fatalf("first-source-wins failed for beta: %+v", beta)
	}
	if len(result.Sources) != 2 {
		t.Fatalf("want 2 source statuses, got %d", len(result.Sources))
	}
}

func TestCatalogInstallStateAnnotation(t *testing.T) {
	official := serve(t, indexJSON(
		entryJSON("installed", "1.0.0", 1)+","+
			entryJSON("outdated", "2.0.0", 1)+","+
			entryJSON("fresh", "1.0.0", 1)))
	s := newTestService(t)
	_ = s.store.EnsureBuiltin("Official", official.URL)

	installed := []InstalledPlugin{
		{ID: "installed", Version: "1.0.0"},
		{ID: "outdated", Version: "1.0.0"},
	}
	result, err := s.Catalog(context.Background(), installed)
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	assertState(t, result.Plugins, "installed", StateInstalled)
	assertState(t, result.Plugins, "outdated", StateUpdateAvailable)
	assertState(t, result.Plugins, "fresh", StateAvailable)
}

func TestCatalogDegradedSourceDoesNotAbortMerge(t *testing.T) {
	good := serve(t, indexJSON(entryJSON("alpha", "1.0.0", 1)))
	s := newTestService(t)
	_ = s.store.EnsureBuiltin("Official", good.URL)
	// A source pointing nowhere reachable.
	if _, err := s.store.Add("Broken", "http://127.0.0.1:1/index.json"); err != nil {
		t.Fatal(err)
	}
	result, err := s.Catalog(context.Background(), nil)
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	if len(result.Plugins) != 1 {
		t.Fatalf("healthy source should still return its entries, got %d", len(result.Plugins))
	}
	broken := findSource(result.Sources, "Broken")
	if broken == nil || broken.Healthy || broken.Error == "" {
		t.Fatalf("broken source should be degraded with an error, got %+v", broken)
	}
}

func TestCatalogSkipsDisabledSource(t *testing.T) {
	official := serve(t, indexJSON(entryJSON("alpha", "1.0.0", 1)))
	s := newTestService(t)
	_ = s.store.EnsureBuiltin("Official", official.URL)
	rec, _ := s.store.Add("Acme", serve(t, indexJSON(entryJSON("beta", "1.0.0", 1))).URL)
	disabled := false
	if _, err := s.UpdateSource(rec.ID, nil, &disabled); err != nil {
		t.Fatal(err)
	}
	result, err := s.Catalog(context.Background(), nil)
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	if findEntry(result.Plugins, "beta") != nil {
		t.Fatalf("disabled source must not contribute entries")
	}
}

func TestCatalogCachesWithinTTLAndRefreshInvalidates(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write([]byte(indexJSON(entryJSON("alpha", "1.0.0", 1))))
	}))
	t.Cleanup(srv.Close)

	s := newTestService(t)
	base := time.Unix(0, 0)
	s.now = func() time.Time { return base }
	_ = s.store.EnsureBuiltin("Official", srv.URL)

	for i := 0; i < 3; i++ {
		if _, err := s.Catalog(context.Background(), nil); err != nil {
			t.Fatalf("catalog: %v", err)
		}
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("within TTL want 1 fetch, got %d", got)
	}
	s.Refresh()
	if _, err := s.Catalog(context.Background(), nil); err != nil {
		t.Fatalf("catalog: %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Fatalf("after refresh want 2 fetches, got %d", got)
	}
}

func TestDownloadRejectsNon200AndOversized(t *testing.T) {
	s := newTestService(t)
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(bad.Close)
	if _, err := s.download(context.Background(), bad.URL); err == nil {
		t.Fatalf("expected non-200 to error")
	}
}

func TestCatalogPreservesIconURL(t *testing.T) {
	body := indexJSON(`{"id":"withicon","name":"With Icon","version":"1.0.0","stars":1,` +
		`"icon_url":"https://cdn.example/icon.svg","package_url":"https://ex/withicon.tar.gz"},` +
		entryJSON("noicon", "1.0.0", 1))
	srv := serve(t, body)
	s := newTestService(t)
	_ = s.store.EnsureBuiltin("Official", srv.URL)

	result, err := s.Catalog(context.Background(), nil)
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	if got := findEntry(result.Plugins, "withicon"); got == nil || got.IconURL != "https://cdn.example/icon.svg" {
		t.Fatalf("icon_url not preserved: %+v", got)
	}
	if got := findEntry(result.Plugins, "noicon"); got == nil || got.IconURL != "" {
		t.Fatalf("missing icon should decode to empty, got %+v", got)
	}
}

func TestApplyQueryFilterAndSort(t *testing.T) {
	entries := []CatalogEntry{
		{IndexEntry: IndexEntry{ID: "zed", Name: "Zed Stats", Description: "analytics", Categories: []string{"analytics"}, Stars: 5, UpdatedAt: "2026-01-01"}},
		{IndexEntry: IndexEntry{ID: "abc", Name: "Abc Chat", Description: "messaging", Categories: []string{"chat"}, Stars: 50, UpdatedAt: "2026-06-01"}},
	}
	// Default sort = stars desc.
	byStars := ApplyQuery(entries, Query{})
	if byStars[0].ID != "abc" {
		t.Fatalf("stars sort: want abc first, got %s", byStars[0].ID)
	}
	// Name sort asc.
	byName := ApplyQuery(entries, Query{Sort: "name"})
	if byName[0].ID != "abc" {
		t.Fatalf("name sort: want abc first, got %s", byName[0].ID)
	}
	// Recent sort desc by updated_at.
	byRecent := ApplyQuery(entries, Query{Sort: "recent"})
	if byRecent[0].ID != "abc" {
		t.Fatalf("recent sort: want abc first, got %s", byRecent[0].ID)
	}
	// Text filter.
	text := ApplyQuery(entries, Query{Text: "stats"})
	if len(text) != 1 || text[0].ID != "zed" {
		t.Fatalf("text filter failed: %+v", text)
	}
	// Category filter.
	cat := ApplyQuery(entries, Query{Category: "chat"})
	if len(cat) != 1 || cat[0].ID != "abc" {
		t.Fatalf("category filter failed: %+v", cat)
	}
}

func findEntry(entries []CatalogEntry, id string) *CatalogEntry {
	for i := range entries {
		if entries[i].ID == id {
			return &entries[i]
		}
	}
	return nil
}

func findSource(sources []SourceStatus, name string) *SourceStatus {
	for i := range sources {
		if sources[i].Name == name {
			return &sources[i]
		}
	}
	return nil
}

func assertState(t *testing.T, entries []CatalogEntry, id string, want InstallState) {
	t.Helper()
	e := findEntry(entries, id)
	if e == nil {
		t.Fatalf("entry %q not found", id)
	}
	if e.InstallState != want {
		t.Fatalf("entry %q: want state %q, got %q", id, want, e.InstallState)
	}
}
