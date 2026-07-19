// Package marketplace implements the plugin marketplace: a discoverable,
// curated catalog of installable plugins assembled from one or more
// git-hosted "sources" (each an index.json document, the official kandev
// source plus any operator-added team/corporate registries).
//
// The marketplace is a discovery layer only — it never installs, spawns, or
// mutates a plugin. Installing a catalog entry reuses the existing
// POST /api/plugins/install pipeline with the entry's package_url. See
// docs/specs/plugins/marketplace.md.
package marketplace

import "time"

// OfficialSourceName / OfficialSourceURL identify the built-in kandev source
// seeded on first boot. The URL is overridable at boot via
// KANDEV_PLUGIN_MARKETPLACE_URL (used by e2e to point at a local fixture).
const (
	OfficialSourceName = "Kandev Official"
	OfficialSourceURL  = "https://kdlbs.github.io/kandev/plugins/index.json"
)

// InstallState is the derived state of a catalog entry relative to what is
// installed locally (docs/specs/plugins/marketplace.md → "State machine").
type InstallState string

const (
	// StateAvailable: no installed plugin shares this entry's id.
	StateAvailable InstallState = "available"
	// StateInstalled: installed at a version >= the catalog version.
	StateInstalled InstallState = "installed"
	// StateUpdateAvailable: installed at a version < the catalog version.
	StateUpdateAvailable InstallState = "update_available"
)

// IndexSource is the self-describing header of an index.json document.
type IndexSource struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// IndexEntry is one plugin as published in a source's index.json. This is the
// hard fetch contract shared with the registry's index-build Action
// (plugin-registry/build-index.mjs); additional/corporate sources must serve
// the same shape. Missing/`null` optional fields decode to their zero value.
type IndexEntry struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Author      string   `json:"author"`
	Categories  []string `json:"categories"`
	// IconURL is an absolute URL to the plugin's icon, resolved by the
	// registry index-build from the plugin manifest's `icon` path. Empty
	// when the plugin ships no icon; the UI falls back to a letter tile.
	IconURL          string `json:"icon_url"`
	RepoURL          string `json:"repo_url"`
	Version          string `json:"version"`
	MinKandevVersion string `json:"min_kandev_version"`
	PackageURL       string `json:"package_url"`
	PackageSHA256    string `json:"package_sha256"`
	Stars            int    `json:"stars"`
	UpdatedAt        string `json:"updated_at"`
}

// IndexDocument is a full index.json fetched from a source URL.
type IndexDocument struct {
	SchemaVersion int          `json:"schema_version"`
	GeneratedAt   string       `json:"generated_at"`
	Source        IndexSource  `json:"source"`
	Plugins       []IndexEntry `json:"plugins"`
}

// CatalogEntry is an IndexEntry annotated with the source it came from and
// its install state relative to local installs.
type CatalogEntry struct {
	IndexEntry
	InstallState     InstallState `json:"install_state"`
	InstalledVersion string       `json:"installed_version,omitempty"`
	SourceID         string       `json:"source_id"`
	SourceName       string       `json:"source_name"`
}

// SourceStatus is a configured source plus its live fetch health, returned in
// every catalog response so the UI can flag a degraded (unreachable/malformed)
// source without failing the whole catalog.
type SourceStatus struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	URL     string `json:"url"`
	Enabled bool   `json:"enabled"`
	Builtin bool   `json:"builtin"`
	Healthy bool   `json:"healthy"`
	Error   string `json:"error,omitempty"`
}

// CatalogResult is the merged, deduped catalog across all enabled sources.
type CatalogResult struct {
	Plugins []CatalogEntry `json:"plugins"`
	Sources []SourceStatus `json:"sources"`
}

// InstalledPlugin is the minimal installed-plugin fact the catalog needs to
// compute install state (id + version), decoupling this package from the
// plugins store.Record type.
type InstalledPlugin struct {
	ID      string
	Version string
}

// SourceRecord is a configured marketplace source (SQLite row).
type SourceRecord struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	Enabled   bool      `json:"enabled"`
	Builtin   bool      `json:"builtin"`
	CreatedAt time.Time `json:"created_at"`
}

// Query is the filter/sort applied to a merged catalog by the HTTP handler.
type Query struct {
	Text     string
	Category string
	Sort     string // "stars" (default) | "name" | "recent"
}
