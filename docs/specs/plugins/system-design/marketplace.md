---
status: draft
system: plugins
requirements:
  - REQ-PLUGINS-MARKETPLACE-001
created: 2026-07-18
owners:
  - jcfs
---
# Plugin Marketplace System Design

## Purpose and boundaries

This design preserves the technical source detail for `REQ-PLUGINS-MARKETPLACE-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLUGINS-MARKETPLACE-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

> **Implementation status (2026-07-18):** backend catalog + source store + HTTP
> API and the in-app Browse UI are implemented and unit/integration-tested; the
> git-hosted registry scaffolding (`plugin-registry/` + the index-build /
> star-refresh GitHub Actions) is committed. Remaining: a full browser E2E
> (needs a reachable fixture `index.json` + `KANDEV_PLUGIN_MARKETPLACE_URL`),
> and the four "Open questions" below (Pages domain, private-source auth,
> cross-source id collisions, provenance enforcement).

## Why

Today a user can only install a plugin if they already know its release tarball URL
or have a `.tar.gz` to upload (see [plugins spec](../requirements/plugins.md) → "Install pipeline").
There is no way to *discover* plugins from inside kandev, no curated list of what
exists, and no signal for which plugins are worth trusting. Plugin authors have
nowhere to publish, and teams have no sanctioned way to share an internal set of
plugins. This feature adds a discoverable, curated catalog — kandev's marketplace —
while keeping install-by-URL and sideloading as escape hatches.

## What

- Users SHALL be able to browse a catalog of available plugins from inside kandev
  (Settings > Plugins > **Browse**) without knowing any URL in advance.
- The catalog SHALL be **searchable** (by name/description) and **filterable by
  category**, and SHALL be **sorted by GitHub stars descending by default**. Stars are
  a **sort hint, not a quality score** — no open-source first-party store actually
  ranks by stars (Obsidian ranks by downloads), so the catalog SHOULD also expose
  **"recently updated"** ordering (from each repo's last release / `pushed_at`) so new
  or actively-maintained plugins are not buried under older high-star incumbents.
- Each catalog entry shows: display name, description, author, categories, the source
  repository link, the latest published version, and its star count.
- Release authors and registry reviewers SHALL apply the attribution convention:
  first-party `kdlbs/kandev-plugin-*` releases declare `author: "kandev"`,
  community releases declare the contributor's stable identity, and a plugin is
  not labeled `kandev` only because a maintainer curated it into the official
  source. This is a release and curation check, not an index-builder ownership
  validation rule. The builder preserves a declared manifest author and falls
  back to the repository owner for legacy releases without one.
- The catalog source repository link SHALL identify the repository named by the
  registry entry. The index builder derives this value from the registry pointer.
  A release manifest's `repo_url`, when present, SHOULD identify the same
  repository so installed and marketplace views do not disagree; release and
  registry review is responsible for flagging mismatches.
- Installing from the catalog SHALL be **one click**: it resolves to the plugin's
  latest release tarball URL and runs the existing verified install pipeline
  (`POST /api/plugins/install`). No new install mechanism is introduced.
- A catalog entry for a plugin that is already installed SHALL show an **Installed**
  state; when the catalog's latest version is newer than the installed version, it
  SHALL show a highlighted **Update** button (which reinstalls the newer tarball).
- The **Installed** tab SHALL show each installed plugin's latest known marketplace
  version alongside its installed version — not only when an update is available — so
  an operator can see version drift without switching to Browse. This check runs once
  on page load and when the operator selects **Check for updates**. The explicit check
  clears the server's five-minute catalog cache before it retrieves current versions.
  **Sync** remains a separate filesystem action. A failed marketplace check degrades to
  an inline "couldn't check" state and never blocks installed-plugin management.
- Each installed row SHALL show a visible **Settings** link that opens the plugin's
  settings page. This link makes plugin configuration discoverable without requiring
  the operator to know that the row itself is also navigable.
- The catalog SHALL be assembled from **one or more marketplace sources**. kandev
  ships with the **official kandev source** enabled by default; operators MAY add
  **additional sources** (a team or corporate registry) and the catalog merges them.
- Ranking SHALL use **GitHub stars only**. kandev collects **no download or usage
  telemetry** and exposes no "most installed" metric.
- The public registry is **curated**: a plugin appears in the official source only
  after a maintainer-approved pull request adds it to the registry list. Anyone can
  still install a non-listed plugin by URL/upload (sideloading is unaffected).

### Ecosystem shape (context, not a runtime contract)

The marketplace is backed by a git-hosted, PR-curated registry. The closest prior
art is **Obsidian's community plugins** (a central pointer list —
`community-plugins.json` — plus PR-to-add, with plugin code living in each author's
own GitHub releases), extended with **Homebrew's tap model** for third-party/corporate
sources and **`formulae.brew.sh`'s build pipeline** (a scheduled Action pulls the
source list, enriches it, emits a static JSON API, and serves it from GitHub Pages):

- **One Git repository per plugin** (`kdlbs/kandev-plugin-<name>`), each publishing
  the plugin package as a GitHub **Release** asset in the existing
  `<id>-<version>.tar.gz` format. A `kdlbs/kandev-plugin-template` starter repo is the
  recommended way to bootstrap one.
- **The registry list** lives in the main kandev repo under `plugin-registry/`
  (`plugins.yaml` + `schema.json`). It records *which repos* are in the official
  catalog — not the plugin metadata itself, which is read from each plugin's own
  release.
- **A GitHub Action builds `index.json`** from `plugins.yaml` (resolving each repo's
  latest release, its manifest metadata, tarball URL + checksum, and current star
  count) and publishes it to **GitHub Pages**. A scheduled run refreshes star counts.
- kandev fetches `index.json` — the official one plus any operator-added source URLs
  pointing at the same-shaped document.

The per-repo publishing convention and the two Actions are an operational contract,
not a user-facing API; their normative shape is the `schema.json` and the
`index.json` document format defined under "Data model".

## Data model

### `plugins.yaml` (registry list — the curated, human-edited source of truth)

Lives at `plugin-registry/plugins.yaml` in the main kandev repo. A list of entries,
each pointing at one plugin repository. Deliberately minimal — descriptive metadata
is read from the plugin's own release manifest at build time so it can never drift
from what actually ships.

```yaml
plugins:
  - id: agent-stats            # MUST equal the plugin manifest `id`
    repo: kdlbs/kandev-plugin-agent-stats   # owner/name; the release source
    # optional overrides / curation-only fields:
    featured: false            # maintainer may pin an entry above star order
```

Constraints:
- `id` unique across the file; MUST match the manifest `id` of the repo's latest
  release (CI validates this).
- `repo` MUST be a resolvable `owner/name` with at least one release whose asset set
  passes the existing package integrity gate (`checksums.txt`).
- The file MUST validate against `plugin-registry/schema.json` (enforced in CI on
  every PR).

### `index.json` (generated catalog document — what kandev fetches)

Built from `plugins.yaml` by the index-build Action and served from GitHub Pages.
This is the fetch contract between a marketplace source and kandev. Additional
(corporate/team) sources MUST serve a document of this shape to be consumable.

```jsonc
{
  "schema_version": 1,
  "generated_at": "2026-07-18T10:00:00Z",
  "source": { "name": "Kandev Official", "url": "https://<pages-host>/plugins/index.json" },
  "plugins": [
    {
      "id": "agent-stats",
      "name": "Agent Stats",              // from manifest display_name
      "description": "Per-session token & LOC dashboard",
      "author": "kandev",                 // from the release manifest
      "categories": ["analytics"],
      "icon_url": "https://raw.githubusercontent.com/kdlbs/kandev-plugin-agent-stats/v1.4.0/icon.svg",
      "repo_url": "https://github.com/kdlbs/kandev-plugin-agent-stats",
      "version": "1.4.0",                 // latest release
      "min_kandev_version": "0.9.0",      // from manifest, nullable
      "package_url": "https://github.com/.../agent-stats-1.4.0.tar.gz",
      "package_sha256": "…",              // expected tarball digest (provenance)
      "stars": 128,                       // GitHub stargazers at last refresh
      "updated_at": "2026-07-17T12:00:00Z"
    }
  ]
}
```

Constraints:
- `id` unique within a document. When the same `id` appears in more than one
  configured source, the **first configured source wins** and later duplicates are
  hidden (the official source is always first).
- `package_url` MUST point at a release asset in the existing package format.
- `icon_url`, when present, is an absolute URL to the plugin's icon (rendered on
  the catalog card; the UI falls back to a letter tile when empty). It is
  resolved by the index-build from the plugin manifest's optional `icon` field
  (a package-relative path such as `icon.svg`) → a `raw.githubusercontent.com`
  URL pinned to the release tag.
- `package_sha256`, when present, is the digest the client MAY use to confirm the
  downloaded tarball matches what was curated, in addition to the package's own
  internal `checksums.txt` gate.

### Marketplace sources (client-side configuration — SQLite)

The set of source documents kandev fetches. Stored as an application setting.

```
plugin_marketplace_source
  id          string  PK
  name        string  human label (e.g. "Acme Internal")
  url         string  absolute URL of an index.json document
  enabled     bool    default true
  builtin     bool    true for the official source (present by default, not deletable)
  created_at  timestamp
```

The official source is seeded as a `builtin` row (its URL is a build/config
constant). Operators can add, enable/disable, and delete non-builtin sources.

## API surface

The catalog is fetched and merged **server-side** so that (a) cross-origin fetching
and per-source caching are handled in one place, (b) private corporate sources can
carry auth without exposing tokens to the browser, and (c) the "already installed /
update available" join against local records happens once. The browser talks only to
kandev; kandev talks to the source URLs.

```
GET    /api/plugins/marketplace                 # Merged, deduped catalog across all enabled sources
                                                #   query: ?q=<search>&category=<cat>&sort=stars|name|recent
                                                #     stars  (default): star count desc, unknown (null) last
                                                #     name:   display name asc (case-insensitive)
                                                #     recent: updated_at desc (last release / repo push)
                                                #   each entry annotated with install_state:
                                                #     available | installed | update_available
GET    /api/plugins/marketplace/sources         # List configured sources
POST   /api/plugins/marketplace/sources         # Add a source {name, url}
PATCH  /api/plugins/marketplace/sources/{id}    # Enable/disable/rename a source
DELETE /api/plugins/marketplace/sources/{id}    # Remove a non-builtin source
POST   /api/plugins/marketplace/refresh         # Force a re-fetch of all sources (bypass cache)
```

Installing from the catalog reuses the existing contract unchanged:
`POST /api/plugins/install` with `{"url": "<package_url>"}` (see
[plugins spec](../requirements/plugins.md) → "Plugin management API"). The marketplace endpoints are
read/config only; they never spawn or mutate a plugin process.

`GET /api/plugins/marketplace` responses are cached per source with a short TTL;
`refresh` and adding/removing a source invalidate the cache. A source that fails to
fetch or parse is reported as a degraded source in the response and its entries are
omitted — it never aborts the merge of the healthy sources.

## State machine

Catalog entries have a derived **install_state** computed by joining a catalog entry
against local plugin records (by `id`):

| install_state      | condition |
|--------------------|-----------|
| `available`        | no installed record with this `id` |
| `installed`        | installed record exists, version ≥ catalog `version` |
| `update_available` | installed record exists, installed version < catalog `version` |

Installing an `available` entry, or updating an `update_available` one, both go
through `POST /api/plugins/install`; the plugin's own lifecycle
(`registered → active → error`, enable/disable/uninstall) is unchanged from the
[plugins spec](../requirements/plugins.md) → "State machine". The marketplace adds no new plugin states.

A **source** is `enabled` or `disabled`; only `enabled` sources contribute entries. A
transiently unreachable enabled source is surfaced as `degraded` in the catalog
response but stays `enabled`.

## Permissions

- Authenticated members may browse the catalog and inspect installed-plugin and
  manifest metadata. Their UI is read-only: it omits install, update, enable,
  disable, uninstall, auto-update, source-management, and plugin-configuration
  controls.
- Installing or updating a plugin and managing marketplace sources
  (add/remove/enable) require administrator authority. Adding a source is an
  explicit act of trust in that source's maintainer, equivalent to `brew tap`-ing
  a third-party tap.
- Appearing in the **official** source requires a maintainer to approve the
  registry-list PR. Third-party/corporate sources are governed by whoever controls
  that source repo, not by kandev.

## Failure modes

- **A source URL is unreachable or returns malformed JSON.** That source is marked
  `degraded` in the `GET /api/plugins/marketplace` response, its entries are omitted,
  and the healthy sources still return. No error is surfaced for the catalog as a
  whole unless *every* source failed.
- **A catalog `package_url` 404s or the download fails at install time.** The
  existing install pipeline surfaces the install error unchanged; the catalog entry
  is unaffected.
- **`package_sha256` mismatch** between the catalog and the downloaded tarball:
  once digest enforcement is enabled (a planned option — see "Open questions"),
  install fails closed with a provenance error even though the tarball's own
  `checksums.txt` may be internally consistent. In v1 the digest is advisory and
  install proceeds through the existing pipeline unchanged.
- **Duplicate `id` across sources:** the first configured source wins; later
  duplicates are silently hidden (documented, not an error).
- **GitHub star-refresh Action hits API rate limits:** the previous star counts in
  `index.json` are retained (stale, not zeroed); the build must never publish `0`
  stars for a repo it failed to query. (The Action should batch star lookups via the
  GraphQL API under an authenticated PAT/App token — authenticated REST is 5,000
  req/hr, unauthenticated only 60/hr, and the default `GITHUB_TOKEN` cannot reliably
  read arbitrary external repos — so it stays well inside limits even as the list
  grows.)
- **A listed repo deletes its latest release / the tarball asset:** the index-build
  Action drops that entry (or keeps the last good version) and logs it; a plugin with
  no installable release never appears with a dead `package_url`.

## Persistence guarantees

- Configured marketplace sources survive restart (SQLite).
- The fetched catalog is a **cache**, not durable state: it does not survive restart
  and is re-fetched on demand. Star counts and versions are always as fresh as the
  last successful source fetch, never authoritative locally.
- Installed plugins are unaffected by marketplace state — uninstalling every source,
  or the official source going offline, never disables or removes an installed
  plugin.

## Scenarios

- **GIVEN** the official source is reachable, **WHEN** the user opens Settings >
  Plugins > Browse, **THEN** the plugins are listed sorted by star count descending,
  each showing name, description, author, categories, version, and stars.
- **GIVEN** the catalog is showing, **WHEN** the user types a query, **THEN** only
  entries whose name or description matches remain; **AND WHEN** the user picks a
  category filter, **THEN** only entries in that category remain.
- **GIVEN** a catalog entry with `install_state: available`, **WHEN** the user clicks
  Install, **THEN** kandev calls `POST /api/plugins/install` with the entry's
  `package_url` and the plugin installs via the normal verified pipeline.
- **GIVEN** a plugin is already installed at the catalog's latest version, **WHEN**
  the catalog renders, **THEN** its entry shows **Installed** and no Install button.
- **GIVEN** a plugin is installed at a version older than the catalog's, **WHEN** the
  catalog renders, **THEN** its Installed row shows a highlighted **Update** button,
  and clicking it installs the newer tarball.
- **GIVEN** the Installed tab, **WHEN** it loads, **THEN** every installed plugin with a
  catalog entry shows its latest known version (e.g. "Latest v2.1.0"), and a plugin with
  no catalog entry anywhere shows a "not in the marketplace" hint instead — never before
  the first successful check completes.
- **GIVEN** a check that reached only some of the enabled sources, **WHEN** it completes,
  **THEN** plugins missing from that response show **no** "not in the marketplace" hint —
  a source that failed contributes no entries, which is indistinguishable from delisting —
  while the healthy sources' known versions and **Update** affordances render as usual, and
  the degraded source's reason is reported inline.
- **GIVEN** no marketplace source is enabled, **WHEN** a check completes, **THEN** kandev
  reports that versions can't be checked and where to enable a source, and no plugin claims
  to be missing from the marketplace — nothing was queried, so nothing was learned.
- **GIVEN** the Installed tab, **WHEN** the operator selects **Check for updates**,
  **THEN** kandev busts the marketplace cache and re-fetches the catalog, and any
  plugin's latest-version text and highlighted **Update** button state update in place.
- **GIVEN** the Installed tab, **WHEN** the operator selects **Sync**, **THEN** kandev
  reconciles the local plugins directory without starting a marketplace refresh.
- **GIVEN** an installed plugin row, **WHEN** the operator selects **Settings**, **THEN**
  kandev opens that plugin's settings page.
- **GIVEN** the marketplace is unreachable or every enabled source reports unhealthy,
  **WHEN** a check on load or through **Check for updates** fails, **THEN** an inline error explains the
  check couldn't complete, while the installed-plugin list, enable/disable/uninstall,
  and the filesystem-sync summary are all unaffected.
- **GIVEN** an installed plugin with an available update, **WHEN** the operator clicks
  its **Update** button, **THEN** the button shows a busy/spinner state and the plugin's
  Enable/Disable/Uninstall controls are disabled until the install settles; on success
  the version and Update affordance refresh, and on failure an inline error shows the
  backend's message while the button stays clickable for a retry.
- **GIVEN** an operator adds a source `{name: "Acme Internal", url: …}`, **WHEN** the
  catalog is next fetched, **THEN** Acme's plugins appear alongside the official ones,
  and an Acme entry with an id that also exists in the official source is hidden in
  favor of the official one.
- **GIVEN** a configured source is unreachable, **WHEN** the catalog is fetched,
  **THEN** the response marks that source `degraded`, omits its entries, and still
  returns entries from the reachable sources.
- **GIVEN** digest enforcement is enabled (planned) and a catalog entry whose
  `package_sha256` does not match the downloaded tarball, **WHEN** the user installs
  it, **THEN** the install fails closed with a provenance error and no plugin process
  is spawned. (In v1 the digest is advisory; install is unchanged.)
- **GIVEN** the marketplace is empty or entirely offline, **WHEN** the user opens the
  install dialog, **THEN** install-by-URL and upload still work exactly as before.
- **GIVEN** a PR that adds an entry to `plugins.yaml` whose `id` does not match the
  target repo's latest-release manifest `id`, **WHEN** registry CI runs, **THEN** the
  schema/consistency check fails and the PR is blocked.
- **GIVEN** the latest release of the first-party Bitbucket plugin declares
  `author: "kandev"` and its `repo_url` points to `kdlbs/kandev-plugin-bitbucket`,
  **WHEN** the official index is built, **THEN** the catalog shows **by kandev** and
  links to the `kdlbs` Bitbucket repository.
- **GIVEN** the latest release of the community YouTrack plugin is owned by
  `ahmedbally` and declares `author: "ahmedbally"` with its matching repository URL,
  **WHEN** the official index is built, **THEN** the catalog shows **by ahmedbally**
  and links to `ahmedbally/kandev-plugin-youtrack`, not a `kdlbs` repository.

## Auto-update (opt-in)

Beyond the manual **Update** affordance, kandev can update installed
plugins in the background — **off by default, opt-in**. This does not add a new
install mechanism: an auto-update is exactly the existing verified reinstall of a
newer catalog `package_url` (`Service.InstallFromURL`), driven automatically
instead of by a click.

- **Two-tier toggle (Settings > Plugins).** An instance-wide **"Automatically
  update plugins"** switch sets the default for every installed plugin. Each
  plugin row also has its own switch that can **override** the default either way.
  The effective setting is: per-plugin override when set, otherwise the
  instance-wide default. A row shows an **override** badge and a **Reset** control
  (clears the override → inherit the default again) whenever an explicit
  per-plugin value is set. Both tiers persist across restart: the default in the
  SQLite `plugin_settings` row, the per-plugin override in the plugin's stored
  record (`auto_update`, carried forward verbatim on upgrade so an auto-update
  never resets the toggle that triggered it).
- **Active plugins only.** A background poller (`AutoUpdatePoller`, ~90-minute
  cadence, plus one sweep shortly after boot) upgrades a plugin only when it is
  currently **active** *and* opted in *and* the catalog reports
  `update_available`. **Disabled/errored plugins are never auto-updated** — a
  disabled plugin stays on its installed version until re-enabled. Eligibility is
  re-checked immediately before each install, so an operator who disables or
  opts a plugin out mid-sweep is never overridden by a stale candidate.
- **Failure handling reuses the install pipeline unchanged.** An upgrade runs the
  same verified path as a manual update: a package that fails its checksum/extract
  gate is rejected with the installed version left untouched, and a package that
  extracts but fails to spawn leaves the plugin in `error` (exactly as a manual
  update would) rather than being silently reverted to the prior version — kandev
  does not re-run a superseded release. A single plugin's failure is logged and
  never aborts the rest of the sweep.
- **No new surface for the plugin itself.** Auto-update is an operator preference
  layered over the existing install/lifecycle machinery; the plugin's own state
  machine (`registered → active → disabled → error`) is unchanged.

API: `GET/PUT /api/plugins/settings` (`{auto_update_default}`) for the
instance-wide default and `PUT /api/plugins/:id/auto-update` (`{auto_update:
true|false|null}`) for the per-plugin override. Both require the same operator
authority as the rest of plugin management.

## Out of scope

- **Download / usage / install telemetry.** No counters, no "most installed", no
  phone-home. Ranking is GitHub stars only. (Explicit product decision; revisit only
  with opt-in telemetry — see the telemetry direction ADR/plan.)
- **Update channels / pinning.** Auto-update always tracks a source's latest
  published version. There are no stable/beta channels, per-plugin version pins,
  or scheduled maintenance windows — only the opt-in on/off toggles described in
  "Auto-update (opt-in)" above.
- **Ratings, reviews, comments, or a paid/commercial plugin tier.**
- **A hosted web marketplace browse *site*.** GitHub Pages serves the `index.json`
  document; a rich public browsing website is a later phase (the in-app catalog is the
  v1 surface).
- **Signature *requirement*.** Package signing/provenance stays optional exactly as in
  the [plugins spec](../requirements/plugins.md); the marketplace adds an *advisory* `package_sha256` but
  does not make signing mandatory.
- **Runtime hardening / sandboxing of plugin JS.** Unchanged from the plugins spec;
  the marketplace does not alter the trust boundary of an installed plugin.

## Open questions

- **Pages host / canonical official URL.** `kdlbs.github.io/kandev/plugins/index.json`
  vs. a branded `plugins.kandev.*` domain — pick before wiring the builtin source
  constant.
- **Private corporate sources & auth.** Do we support authenticated source URLs
  (token/header) in v1, or only public `index.json` documents? Affects the source
  data model (an optional credential ref) and the backend fetch.
- **Star refresh cadence & storage.** Confirm the schedule (daily?) and that star
  counts live only in the generated `index.json` (not committed back to
  `plugins.yaml`, to avoid noisy commits).
- **`package_sha256` provenance.** Confirm the index build pins the digest and the
  client enforces it (recommended above) vs. relying solely on the package's internal
  `checksums.txt`.
- **Cross-source id collisions.** v1 hides later-source duplicates (official wins). Do
  we instead want Homebrew-style **fully-qualified `source/id`** names so a corporate
  source can intentionally ship its own build of an official plugin without it being
  silently hidden?
- **Anti-typosquatting / verified authorship.** Worth adopting Open VSX's
  verified-namespace idea (a "verified" badge only when repo ownership is proven) and
  cheap publish-time scans in the registry-validation Action (secret-leak / blocklist /
  typosquat checks on the PR)? Deferred, but flagged as the natural next trust layer.
