package marketplace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/sync/singleflight"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/provenance"
)

var (
	ErrCanvasListingNotFound = errors.New("canvas marketplace listing not found")
	ErrCanvasListingStale    = errors.New("canvas marketplace listing is stale")
	ErrPluginListingNotFound = errors.New("plugin marketplace listing not found")
	ErrPluginListingStale    = errors.New("plugin marketplace listing is stale")
	ErrCatalogUnavailable    = errors.New("plugin marketplace catalog unavailable")
)

// maxIndexBytes caps the index.json body read from any source, bounding
// worst-case memory from a hostile or misconfigured source.
const maxIndexBytes = 5 << 20 // 5 MiB

// defaultCacheTTL is how long a fetched index document is reused before the
// source is re-fetched. Refresh() (and adding/removing a source) invalidates
// the cache immediately.
const defaultCacheTTL = 5 * time.Minute

const (
	marketplaceKindPlugin = "plugin"
	marketplaceKindCanvas = "canvas"
)

// Service fetches, caches, and merges catalog documents across the configured
// marketplace sources.
type Service struct {
	store  *SourceStore
	client *http.Client
	log    *logger.Logger

	ttl time.Duration
	now func() time.Time
	// canonicalOfficialURL is fixed to OfficialSourceURL in production. Tests
	// replace it with a local fixture to exercise the canonical transport
	// boundary without reaching the public registry.
	canonicalOfficialURL string

	mu    sync.Mutex
	cache map[string]cacheEntry
	// sf collapses concurrent downloads of the same source URL into one HTTP
	// request (Browse opened in two tabs while the cache is cold no longer
	// races two identical fetches).
	sf singleflight.Group
}

type cacheEntry struct {
	doc *IndexDocument
	at  time.Time
}

// NewService builds a marketplace Service over the given source store.
func NewService(store *SourceStore, log *logger.Logger) *Service {
	return &Service{
		store:                store,
		client:               &http.Client{Timeout: 20 * time.Second, CheckRedirect: rejectRedirect},
		log:                  log,
		ttl:                  defaultCacheTTL,
		now:                  time.Now,
		cache:                map[string]cacheEntry{},
		canonicalOfficialURL: OfficialSourceURL,
	}
}

func rejectRedirect(_ *http.Request, _ []*http.Request) error {
	return http.ErrUseLastResponse
}

// SetHTTPClient replaces the catalog transport. Production uses the default
// client with normal TLS and redirect behavior; tests inject a transport that
// maps the canonical URL to an isolated fixture.
func (s *Service) SetHTTPClient(client *http.Client) {
	if client == nil {
		return
	}
	s.mu.Lock()
	s.client = client
	s.mu.Unlock()
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
	fetched := s.fetchAll(ctx, sources)

	// Merge sequentially in source order so first-source-wins dedup is stable,
	// even though the fetches above ran concurrently.
	installedByID := indexInstalled(installed)
	result := &CatalogResult{Plugins: []CatalogEntry{}, Canvases: []CatalogEntry{}, Sources: []SourceStatus{}}
	seen := map[string]bool{}
	for i, src := range sources {
		status := statusFor(src)
		if !src.Enabled {
			result.Sources = append(result.Sources, status)
			continue
		}
		if fetched[i].err != nil {
			status.Healthy = false
			status.Error = fetched[i].err.Error()
			result.Sources = append(result.Sources, status)
			continue
		}
		if fetched[i].doc.warning != "" {
			status.Healthy = false
			status.Error = fetched[i].doc.warning
		}
		for _, entry := range s.mergeEntries(fetched[i].doc, src, installedByID, seen) {
			if entry.Kind == marketplaceKindCanvas {
				result.Canvases = append(result.Canvases, entry)
				continue
			}
			result.Plugins = append(result.Plugins, entry)
		}
		result.Sources = append(result.Sources, status)
	}
	return result, nil
}

// ResolvePluginPackage resolves an exact native package from a freshly
// fetched, enabled source. The caller supplies only selection constraints;
// package URL and publisher evidence are taken from the validated document.
// A cached Browse result is never used for installation.
func (s *Service) ResolvePluginPackage(ctx context.Context, sourceID, packageID, version, digest string) (*PluginPackageResolution, error) {
	source, err := s.store.Get(strings.TrimSpace(sourceID))
	if err != nil || !source.Enabled {
		return nil, ErrPluginListingNotFound
	}
	doc, err := s.fetchFresh(ctx, source.URL)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCatalogUnavailable, err)
	}
	for _, entry := range doc.Plugins {
		if entry.Kind == marketplaceKindCanvas || entry.ID != strings.TrimSpace(packageID) {
			continue
		}
		if entry.Version != strings.TrimSpace(version) {
			return nil, ErrPluginListingStale
		}
		catalogDigest := strings.ToLower(strings.TrimSpace(entry.PackageSHA256))
		expectedDigest := strings.ToLower(strings.TrimSpace(digest))
		if (catalogDigest == "") != (expectedDigest == "") || (catalogDigest != "" && catalogDigest != expectedDigest) {
			return nil, ErrPluginListingStale
		}
		if entry.PackageURL == "" {
			return nil, ErrPluginListingNotFound
		}
		publisher := projectPublisher(entry, *source, s.canonicalOfficialURL)
		return &PluginPackageResolution{
			Entry:      entry,
			Source:     *source,
			PackageURL: entry.PackageURL,
			Provenance: buildProvenance(entry, *source, publisher, s.now().UTC()),
			Publisher:  publisher,
		}, nil
	}
	return nil, ErrPluginListingNotFound
}

// ResolveOfficialPluginPackage resolves an exact native version from the
// canonical built-in source. It does not accept an URL override, a custom
// source, or a caller-provided repository identity.
func (s *Service) ResolveOfficialPluginPackage(ctx context.Context, packageID, version string) (*PluginPackageResolution, error) {
	sources, err := s.store.List()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCatalogUnavailable, err)
	}
	for _, source := range sources {
		if !source.Builtin || source.URL != s.canonicalOfficialURL || !source.Enabled {
			continue
		}
		doc, fetchErr := s.fetchFresh(ctx, source.URL)
		if fetchErr != nil {
			return nil, fmt.Errorf("%w: %v", ErrCatalogUnavailable, fetchErr)
		}
		for _, entry := range doc.Plugins {
			if entry.Kind == marketplaceKindCanvas || entry.ID != strings.TrimSpace(packageID) {
				continue
			}
			if entry.Version != strings.TrimSpace(version) || !validSHA256(entry.PackageSHA256) || entry.PackageURL == "" {
				return nil, ErrPluginListingStale
			}
			publisher := projectPublisher(entry, source, s.canonicalOfficialURL)
			return &PluginPackageResolution{
				Entry:      entry,
				Source:     source,
				PackageURL: entry.PackageURL,
				Provenance: buildProvenance(entry, source, publisher, s.now().UTC()),
				Publisher:  publisher,
			}, nil
		}
		return nil, ErrPluginListingNotFound
	}
	return nil, ErrPluginListingNotFound
}

// ResolveCanvasPackageWithProvenance resolves an exact canvas listing from a
// freshly fetched source document and returns the host-created provenance
// tuple. The source and expected identity/digest are selection constraints;
// neither can create publisher evidence.
func (s *Service) ResolveCanvasPackageWithProvenance(ctx context.Context, sourceID, packageID, version, digest string) (string, string, *provenance.InstallationProvenance, error) {
	source, err := s.store.Get(strings.TrimSpace(sourceID))
	if err != nil {
		return "", "", nil, ErrCanvasListingNotFound
	}
	if !source.Enabled {
		return "", "", nil, ErrCanvasListingNotFound
	}
	doc, err := s.fetchFresh(ctx, source.URL)
	if err != nil {
		return "", "", nil, ErrCanvasListingNotFound
	}
	for _, entry := range doc.Plugins {
		if entry.Kind != marketplaceKindCanvas || entry.ID != packageID {
			continue
		}
		if entry.Version != version || !validSHA256(entry.PackageSHA256) || !strings.EqualFold(entry.PackageSHA256, digest) {
			return "", "", nil, ErrCanvasListingStale
		}
		if entry.PackageURL == "" {
			return "", "", nil, ErrCanvasListingNotFound
		}
		publisher := projectPublisher(entry, *source, s.canonicalOfficialURL)
		return entry.PackageURL, entry.RepoURL, buildProvenance(entry, *source, publisher, s.now().UTC()), nil
	}
	return "", "", nil, ErrCanvasListingNotFound
}

// ResolveCanvasPackage resolves an exact canvas listing from the configured
// source. It preserves the original URL/repository-only API for callers that
// do not consume publisher provenance.
func (s *Service) ResolveCanvasPackage(ctx context.Context, sourceID, packageID, version, digest string) (string, string, error) {
	packageURL, repositoryURL, _, err := s.ResolveCanvasPackageWithProvenance(ctx, sourceID, packageID, version, digest)
	return packageURL, repositoryURL, err
}

type fetchOutcome struct {
	doc *IndexDocument
	err error
}

// fetchAll fetches every enabled source concurrently, so one slow/unreachable
// source can't serialize (up to 20s each) in front of the others — the Browse
// tab's latency is bounded by the slowest single source, not their sum.
func (s *Service) fetchAll(ctx context.Context, sources []SourceRecord) []fetchOutcome {
	out := make([]fetchOutcome, len(sources))
	var wg sync.WaitGroup
	for i, src := range sources {
		if !src.Enabled {
			continue
		}
		wg.Add(1)
		go func(i int, url string) {
			defer wg.Done()
			doc, err := s.fetch(ctx, url)
			out[i] = fetchOutcome{doc: doc, err: err}
		}(i, src.URL)
	}
	wg.Wait()
	return out
}

// mergeEntries appends this source's not-yet-seen entries as annotated catalog
// entries, marking their ids seen so later sources can't shadow them.
func (s *Service) mergeEntries(doc *IndexDocument, src SourceRecord, installed map[string]string, seen map[string]bool) []CatalogEntry {
	out := make([]CatalogEntry, 0, len(doc.Plugins))
	for _, e := range doc.Plugins {
		if e.ID == "" || seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		out = append(out, annotate(e, src, installed, s.canonicalOfficialURL))
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
func annotate(e IndexEntry, src SourceRecord, installed map[string]string, canonicalOfficialURL string) CatalogEntry {
	if e.Kind == "" {
		e.Kind = marketplaceKindPlugin
	}
	ce := CatalogEntry{
		IndexEntry:        e,
		SourceID:          src.ID,
		SourceName:        src.Name,
		InstallState:      StateAvailable,
		PublisherIdentity: projectPublisher(e, src, canonicalOfficialURL),
	}
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

func projectPublisher(entry IndexEntry, source SourceRecord, canonicalOfficialURL string) *provenance.PublisherIdentity {
	if entry.Publisher == nil || !source.Builtin || source.URL != canonicalOfficialURL || entry.PackageSHA256 == "" {
		return provenance.NewUnverified()
	}
	evidence := *entry.Publisher
	if err := evidence.Validate(); err != nil || !validSHA256(entry.PackageSHA256) {
		return provenance.NewUnverified()
	}
	if evidence.Official && !strings.EqualFold(evidence.Login, "kdlbs") {
		return provenance.NewUnverified()
	}
	if evidence.PackageSHA256 != "" && !strings.EqualFold(evidence.PackageSHA256, entry.PackageSHA256) {
		return provenance.NewUnverified()
	}
	return &provenance.PublisherIdentity{
		Status:       provenance.StatusVerified,
		RepositoryID: evidence.RepositoryID,
		OwnerID:      evidence.OwnerID,
		Login:        evidence.Login,
		Repository:   evidence.Repository,
		Official:     evidence.Official,
	}
}

func buildProvenance(entry IndexEntry, source SourceRecord, publisher *provenance.PublisherIdentity, verifiedAt time.Time) *provenance.InstallationProvenance {
	p := &provenance.InstallationProvenance{
		Origin:        provenance.OriginCatalog,
		SourceID:      source.ID,
		SourceURL:     source.URL,
		PackageID:     entry.ID,
		Version:       entry.Version,
		PackageSHA256: strings.ToLower(strings.TrimSpace(entry.PackageSHA256)),
	}
	p.SanitizePublicURLs()
	if publisher == nil || publisher.Status != provenance.StatusVerified || entry.Publisher == nil {
		return p
	}
	evidence := *entry.Publisher
	p.Publisher = &evidence
	p.VerifiedAt = &verifiedAt
	p.VerificationMethod = provenance.VerificationArchiveDownload
	return p
}

// fetch returns a source's index document from cache when fresh, otherwise
// downloads and caches it.
func (s *Service) fetch(ctx context.Context, url string) (*IndexDocument, error) {
	if doc, ok := s.cached(url); ok {
		return doc, nil
	}
	// singleflight collapses concurrent misses on the same URL into one
	// download; latecomers get the leader's result.
	v, err, _ := s.sf.Do(url, func() (any, error) {
		if doc, ok := s.cached(url); ok { // another caller may have filled it
			return doc, nil
		}
		// Detach from the leader's cancellation: the singleflight result is
		// shared across all waiters, so if the first caller navigates away its
		// canceled ctx must not fail everyone else's still-live request. Values
		// are preserved; the client's 20s timeout still bounds the download.
		doc, derr := s.download(context.WithoutCancel(ctx), url)
		if derr != nil {
			return nil, derr
		}
		s.mu.Lock()
		s.cache[url] = cacheEntry{doc: doc, at: s.now()}
		s.mu.Unlock()
		return doc, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*IndexDocument), nil
}

// fetchFresh bypasses the Browse cache. Installation and existing-version
// verification must resolve the current source document before accepting a
// package identity or digest.
func (s *Service) fetchFresh(ctx context.Context, sourceURL string) (*IndexDocument, error) {
	return s.download(context.WithoutCancel(ctx), sourceURL)
}

// cached returns a still-fresh cached document for url, if any.
func (s *Service) cached(url string) (*IndexDocument, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ce, ok := s.cache[url]; ok && s.now().Sub(ce.at) < s.ttl {
		return ce.doc, true
	}
	return nil, false
}

func (s *Service) download(ctx context.Context, url string) (*IndexDocument, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := s.clientForSource(url).Do(req)
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
	if err := validateIndexDocument(&doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

// clientForSource applies the redirect policy at the trust boundary. The
// canonical source is fetched without redirects so its HTTPS origin cannot be
// silently replaced by an untrusted host. Operator-managed sources retain the
// normal HTTP client redirect behavior because their entries are unverified.
func (s *Service) clientForSource(sourceURL string) *http.Client {
	s.mu.Lock()
	client := s.client
	canonicalURL := s.canonicalOfficialURL
	s.mu.Unlock()
	clone := *client
	if sourceURL == canonicalURL {
		clone.CheckRedirect = rejectRedirect
	} else {
		clone.CheckRedirect = nil
	}
	return &clone
}

func validateIndexDocument(doc *IndexDocument) error {
	if doc.SchemaVersion != 1 {
		return fmt.Errorf("unsupported index schema version %d", doc.SchemaVersion)
	}
	if doc.Plugins == nil {
		return fmt.Errorf("index plugins list is missing")
	}
	seen := make(map[string]struct{}, len(doc.Plugins))
	valid := make([]IndexEntry, 0, len(doc.Plugins))
	var warnings []string
	for _, entry := range doc.Plugins {
		kind := strings.TrimSpace(entry.Kind)
		if kind == "" {
			kind = marketplaceKindPlugin
		}
		if entry.ID == "" {
			warnings = append(warnings, "an entry has no id")
			continue
		}
		if _, exists := seen[entry.ID]; exists {
			warnings = append(warnings, "duplicate entry "+entry.ID)
			continue
		}
		seen[entry.ID] = struct{}{}
		if kind != marketplaceKindPlugin && kind != marketplaceKindCanvas {
			warnings = append(warnings, "entry "+entry.ID+" has an unsupported kind")
			continue
		}
		if err := validatePreviews(entry.Previews, kind == marketplaceKindCanvas); err != nil {
			warnings = append(warnings, "entry "+entry.ID+": "+err.Error())
			continue
		}
		if entry.PackageSHA256 != "" && !validSHA256(entry.PackageSHA256) {
			warnings = append(warnings, "entry "+entry.ID+": package digest is invalid")
			continue
		}
		if kind == marketplaceKindCanvas && !validSHA256(entry.PackageSHA256) {
			warnings = append(warnings, "entry "+entry.ID+": canvas package digest is missing or invalid")
			continue
		}
		if entry.Publisher != nil {
			if err := entry.Publisher.Validate(); err != nil {
				warnings = append(warnings, "entry "+entry.ID+": publisher evidence is invalid")
				entry.Publisher = nil
			}
		}
		entry.Kind = kind
		valid = append(valid, entry)
	}
	doc.Plugins = valid
	if len(warnings) > 0 {
		doc.warning = strings.Join(warnings, "; ")
	}
	return nil
}

func validatePreviews(previews []Preview, required bool) error {
	if required && len(previews) == 0 {
		return fmt.Errorf("canvas listings require at least one preview")
	}
	if len(previews) > 8 {
		return fmt.Errorf("preview limit exceeded")
	}
	for _, preview := range previews {
		if utf8.RuneCountInString(strings.TrimSpace(preview.Alt)) < 1 || utf8.RuneCountInString(strings.TrimSpace(preview.Alt)) > 300 {
			return fmt.Errorf("preview alt text is invalid")
		}
		if len(preview.URL) > 2048 {
			return fmt.Errorf("preview URL is too long")
		}
		parsed, err := url.Parse(preview.URL)
		if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" || parsed.Opaque != "" {
			return fmt.Errorf("preview URL must be an HTTPS URL without credentials or fragments")
		}
	}
	return nil
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') && (char < 'A' || char > 'F') {
			return false
		}
	}
	return true
}

// ApplyQuery filters and sorts a merged catalog per the request. Filtering:
// Text matches id/name/description (case-insensitive substring); Category
// matches any of an entry's categories. Sort: "name" (asc), "recent"
// (updated_at desc), or "stars" (desc, the default).
func ApplyQuery(entries []CatalogEntry, q Query) []CatalogEntry {
	out := make([]CatalogEntry, 0, len(entries))
	text := strings.ToLower(strings.TrimSpace(q.Text))
	category := strings.ToLower(strings.TrimSpace(q.Category))
	kind := strings.ToLower(strings.TrimSpace(q.Kind))
	for _, e := range entries {
		entryKind := strings.ToLower(strings.TrimSpace(e.Kind))
		if entryKind == "" {
			entryKind = marketplaceKindPlugin
		}
		if (kind == "" || entryKind == kind) && matchesText(e, text) && matchesCategory(e, category) {
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
			return starsBefore(entries[i].Stars, entries[j].Stars)
		})
	}
}

// starsBefore orders by star count descending, with unknown (nil) stars sorted
// last so a repo whose star lookup failed never outranks a real one.
func starsBefore(a, b *int) bool {
	if a == nil {
		return false
	}
	if b == nil {
		return true
	}
	return *a > *b
}
