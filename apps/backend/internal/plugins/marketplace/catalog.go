package marketplace

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/plugins/manifest"
)

// maxIndexBytes caps the index.json body read from any source, bounding
// worst-case memory from a hostile or misconfigured source.
const maxIndexBytes = 5 << 20 // 5 MiB

// defaultCacheTTL is how long a fetched index document is reused before the
// source is re-fetched. Refresh() (and adding/removing a source) invalidates
// the cache immediately.
const defaultCacheTTL = 5 * time.Minute

// Service fetches, caches, and merges catalog documents across the configured
// marketplace sources.
type Service struct {
	store  *SourceStore
	client *http.Client
	log    *logger.Logger

	ttl time.Duration
	now func() time.Time

	mu    sync.Mutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	doc *IndexDocument
	at  time.Time
}

// NewService builds a marketplace Service over the given source store.
func NewService(store *SourceStore, log *logger.Logger) *Service {
	return &Service{
		store:  store,
		client: &http.Client{Timeout: 20 * time.Second},
		log:    log,
		ttl:    defaultCacheTTL,
		now:    time.Now,
		cache:  map[string]cacheEntry{},
	}
}

// Sources returns every configured source (built-in first).
func (s *Service) Sources() ([]SourceRecord, error) { return s.store.List() }

// AddSource registers a new operator source and clears the cache.
func (s *Service) AddSource(name, url string) (*SourceRecord, error) {
	rec, err := s.store.Add(name, url)
	if err == nil {
		s.Refresh()
	}
	return rec, err
}

// UpdateSource renames or enables/disables a source and clears the cache.
func (s *Service) UpdateSource(id string, name *string, enabled *bool) (*SourceRecord, error) {
	rec, err := s.store.Update(id, name, enabled)
	if err == nil {
		s.Refresh()
	}
	return rec, err
}

// DeleteSource removes a non-builtin source and clears the cache.
func (s *Service) DeleteSource(id string) error {
	err := s.store.Delete(id)
	if err == nil {
		s.Refresh()
	}
	return err
}

// Refresh drops every cached index document so the next Catalog call re-fetches.
func (s *Service) Refresh() {
	s.mu.Lock()
	s.cache = map[string]cacheEntry{}
	s.mu.Unlock()
}

// Catalog fetches and merges every enabled source into a single deduped
// catalog, annotating each entry with its install state against `installed`.
// A source that fails to fetch/parse is reported degraded (Healthy=false) and
// contributes no entries; the healthy sources still return. When the same id
// appears in more than one source, the first configured source (built-in
// first) wins and later duplicates are dropped.
func (s *Service) Catalog(ctx context.Context, installed []InstalledPlugin) (*CatalogResult, error) {
	sources, err := s.store.List()
	if err != nil {
		return nil, err
	}
	installedByID := indexInstalled(installed)
	result := &CatalogResult{Plugins: []CatalogEntry{}, Sources: []SourceStatus{}}
	seen := map[string]bool{}

	for _, src := range sources {
		status := statusFor(src)
		if !src.Enabled {
			result.Sources = append(result.Sources, status)
			continue
		}
		doc, ferr := s.fetch(ctx, src.URL)
		if ferr != nil {
			status.Healthy = false
			status.Error = ferr.Error()
			result.Sources = append(result.Sources, status)
			continue
		}
		result.Plugins = append(result.Plugins, mergeEntries(doc, src, installedByID, seen)...)
		result.Sources = append(result.Sources, status)
	}
	return result, nil
}

// mergeEntries appends this source's not-yet-seen entries as annotated catalog
// entries, marking their ids seen so later sources can't shadow them.
func mergeEntries(doc *IndexDocument, src SourceRecord, installed map[string]string, seen map[string]bool) []CatalogEntry {
	out := make([]CatalogEntry, 0, len(doc.Plugins))
	for _, e := range doc.Plugins {
		if e.ID == "" || seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		out = append(out, annotate(e, src, installed))
	}
	return out
}

func statusFor(src SourceRecord) SourceStatus {
	return SourceStatus{
		ID:      src.ID,
		Name:    src.Name,
		URL:     src.URL,
		Enabled: src.Enabled,
		Builtin: src.Builtin,
		Healthy: true,
	}
}

func indexInstalled(installed []InstalledPlugin) map[string]string {
	m := make(map[string]string, len(installed))
	for _, p := range installed {
		m[p.ID] = p.Version
	}
	return m
}

// annotate derives a catalog entry's install state from what is installed.
func annotate(e IndexEntry, src SourceRecord, installed map[string]string) CatalogEntry {
	ce := CatalogEntry{IndexEntry: e, SourceID: src.ID, SourceName: src.Name, InstallState: StateAvailable}
	if v, ok := installed[e.ID]; ok {
		ce.InstalledVersion = v
		if manifest.CompareVersions(v, e.Version) < 0 {
			ce.InstallState = StateUpdateAvailable
		} else {
			ce.InstallState = StateInstalled
		}
	}
	return ce
}

// fetch returns a source's index document from cache when fresh, otherwise
// downloads and caches it.
func (s *Service) fetch(ctx context.Context, url string) (*IndexDocument, error) {
	s.mu.Lock()
	if ce, ok := s.cache[url]; ok && s.now().Sub(ce.at) < s.ttl {
		doc := ce.doc
		s.mu.Unlock()
		return doc, nil
	}
	s.mu.Unlock()

	doc, err := s.download(ctx, url)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.cache[url] = cacheEntry{doc: doc, at: s.now()}
	s.mu.Unlock()
	return doc, nil
}

func (s *Service) download(ctx context.Context, url string) (*IndexDocument, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("source returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxIndexBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxIndexBytes {
		return nil, fmt.Errorf("index exceeds %d bytes", maxIndexBytes)
	}
	var doc IndexDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse index: %w", err)
	}
	return &doc, nil
}

// ApplyQuery filters and sorts a merged catalog per the request. Filtering:
// Text matches id/name/description (case-insensitive substring); Category
// matches any of an entry's categories. Sort: "name" (asc), "recent"
// (updated_at desc), or "stars" (desc, the default).
func ApplyQuery(entries []CatalogEntry, q Query) []CatalogEntry {
	out := make([]CatalogEntry, 0, len(entries))
	text := strings.ToLower(strings.TrimSpace(q.Text))
	category := strings.ToLower(strings.TrimSpace(q.Category))
	for _, e := range entries {
		if matchesText(e, text) && matchesCategory(e, category) {
			out = append(out, e)
		}
	}
	sortEntries(out, q.Sort)
	return out
}

func matchesText(e CatalogEntry, text string) bool {
	if text == "" {
		return true
	}
	hay := strings.ToLower(e.ID + " " + e.Name + " " + e.Description)
	return strings.Contains(hay, text)
}

func matchesCategory(e CatalogEntry, category string) bool {
	if category == "" {
		return true
	}
	for _, c := range e.Categories {
		if strings.ToLower(c) == category {
			return true
		}
	}
	return false
}

func sortEntries(entries []CatalogEntry, mode string) {
	switch mode {
	case "name":
		sort.SliceStable(entries, func(i, j int) bool {
			return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
		})
	case "recent":
		sort.SliceStable(entries, func(i, j int) bool {
			return entries[i].UpdatedAt > entries[j].UpdatedAt
		})
	default: // "stars"
		sort.SliceStable(entries, func(i, j int) bool {
			return entries[i].Stars > entries[j].Stars
		})
	}
}
