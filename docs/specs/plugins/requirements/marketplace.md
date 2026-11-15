---
status: active
system: plugins
created: 2026-07-18
updated: 2026-10-30
owners:
  - jcfs
---
# Plugin Marketplace Requirements

## Overview

Today a user can only install a plugin if they already know its release tarball URL
or have a `.tar.gz` to upload (see the [plugin installation requirement](plugins.md)
for the existing install pipeline).
There is no way to *discover* plugins from inside kandev, no curated list of what
exists, and no signal for which plugins are worth trusting. Plugin authors have
nowhere to publish, and teams have no sanctioned way to share an internal set of
plugins. This feature adds a discoverable, curated catalog — kandev's marketplace —
while keeping install-by-URL and sideloading as escape hatches.

## Requirements

### REQ-PLUGINS-MARKETPLACE-001: Plugin Marketplace

**Intent:** Today a user can only install a plugin if they already know its release tarball URL or
have a `.tar.gz` to upload (see the [plugin installation requirement](plugins.md) for the existing
install pipeline). There is no way to
*discover* plugins from inside kandev, no curated list of what exists, and no signal for which
plugins are worth trusting. Plugin authors have nowhere to publish, and teams have no sanctioned way
to share an internal set of plugins. This feature adds a discoverable, curated catalog — kandev's
marketplace — while keeping install-by-URL and sideloading as escape hatches.

#### Acceptance criteria

- **AC-PLUGINS-MARKETPLACE-001.1:** Users SHALL be able to browse a catalog of available plugins from inside kandev (Settings > Plugins > **Browse**) without knowing any URL in advance.
- **AC-PLUGINS-MARKETPLACE-001.2:** The catalog SHALL be **searchable** (by name/description) and **filterable by category**, and SHALL be **sorted by GitHub stars descending by default**. Stars are a **sort hint, not a quality score** — no open-source first-party store actually ranks by stars (Obsidian ranks by downloads), so the catalog SHOULD also expose **"recently updated"** ordering (from each repo's last release / `pushed_at`) so new or actively-maintained plugins are not buried under older high-star incumbents.
- **AC-PLUGINS-MARKETPLACE-001.3:** Each catalog entry shows: display name, description, author, categories, the source repository link, the latest published version, and its star count.
- **AC-PLUGINS-MARKETPLACE-001.4:** Native-plugin listings in the official catalog shall attribute the plugin to the owner of its curated repository. A manifest author or metadata lookup failure shall not change this attribution. A community repository owned by `Fclem` shall show `by Fclem`. A first-party repository owned by `kdlbs` shall show `by kdlbs`.
- **AC-PLUGINS-MARKETPLACE-001.5:** The catalog source repository link SHALL identify the repository named by the registry entry. The index builder derives this value from the registry pointer. A release manifest's `repo_url`, when present, SHOULD identify the same repository so installed and marketplace views do not disagree; release and registry review is responsible for flagging mismatches.
- **AC-PLUGINS-MARKETPLACE-001.6:** Installing from the catalog SHALL be **one click**: it resolves to the plugin's latest release tarball URL and runs the existing verified install pipeline (`POST /api/plugins/install`). No new install mechanism is introduced.
- **AC-PLUGINS-MARKETPLACE-001.7:** A catalog entry for a plugin that is already installed SHALL show an **Installed** state; when the catalog's latest version is newer than the installed version, it SHALL show an **Update available** affordance (which reinstalls the newer tarball).
- **AC-PLUGINS-MARKETPLACE-001.8:** The catalog SHALL be assembled from **one or more marketplace sources**. kandev ships with the **official kandev source** enabled by default; operators MAY add **additional sources** (a team or corporate registry) and the catalog merges them.
- **AC-PLUGINS-MARKETPLACE-001.9:** When the official catalog retains an earlier native-plugin listing after a release failure, attribution shall follow its current curated repository owner. The retained package version, URL, and digest shall remain unchanged. An all-failed rebuild shall preserve the published catalog.
- **AC-PLUGINS-MARKETPLACE-001.10:** On desktop and phone, installed-plugin details shall identify the manifest author as `Declared author`. This value shall not imply verified publisher identity. Missing author data shall keep the existing unavailable marker.

These attribution rules cover native plugins. Canvas attribution, publisher
verification, and package provenance remain separate contracts. Custom source
metadata remains subject to the operator's trust in that source.

### REQ-PLUGINS-MARKETPLACE-002: Prompt curated-release publication

**Intent:** A valid release from an already-curated plugin repository should become discoverable
without waiting for a Kandev source commit or a manual registry rebuild, while preserving central
curation and package-integrity authority.

#### Acceptance criteria

- **AC-PLUGINS-MARKETPLACE-002.1:** The official registry SHALL poll only repositories listed in the
  checked-out `plugin-registry/plugins.yaml` every three hours and target publication within four
  hours under normal provider scheduling. The provider's best-effort scheduling caveat SHALL be
  documented and the 06:00 UTC daily rebuild SHALL remain enabled.
- **AC-PLUGINS-MARKETPLACE-002.2:** A release SHALL enter the official index only when its exact
  `<id>-<version>.tar.gz` asset passes the package checksum and manifest safety gate and the verified
  manifest ID/version matches the curated ID/release version.
- **AC-PLUGINS-MARKETPLACE-002.3:** A bad or missing release SHALL preserve the last known-good record
  for that still-curated repository and emit visible workflow evidence. A provider-wide failure SHALL
  preserve the published Pages site rather than replacing it.
- **AC-PLUGINS-MARKETPLACE-002.4:** All official registry deployments SHALL share one serialized,
  coalescing concurrency group. A release signal SHALL carry no repository selector or release
  payload from a plugin repository.

Decision: `ADR-2026-08-30-central-curated-plugin-release-polling`.

### REQ-PLUGINS-MARKETPLACE-003: Registry preview images

**Intent:** Registry maintainers can add a visual preview gallery to plugin
listings without changing the plugin package.

#### Acceptance criteria

- **AC-PLUGINS-MARKETPLACE-003.1:** Official and custom marketplace entries
  shall accept an ordered `previews` list of HTTPS image URLs and alternative
  text. Plugin entries can omit the list; existing entries shall remain valid.
- **AC-PLUGINS-MARKETPLACE-003.2:** A plugin listing with screenshots shall show
  its first image as the preview cover and expose all images in a details
  gallery. Entries without screenshots shall keep their current presentation.
- **AC-PLUGINS-MARKETPLACE-003.3:** Gallery navigation shall support keyboard
  and touch, display descriptions and image position, and keep install actions
  available when an image fails. Package code shall not run in the preview.
- **AC-PLUGINS-MARKETPLACE-003.4:** Preview metadata shall belong to the registry,
  not the plugin manifest or release bundle. Maintainers shall be able to change
  URLs, descriptions, or image order without releasing a new plugin version.

Canvas listing image requirements are owned by
[canvas marketplace and sharing](../../canvases/requirements/marketplace-sharing.md).

### REQ-PLUGINS-MARKETPLACE-004: Immediate auto-update preferences

The plugin system owns the operator's opt-in default and each plugin's explicit
override, including their immediate desktop and phone settings behavior.

#### Acceptance criteria

- **AC-PLUGINS-MARKETPLACE-004.1:** The global default shall start off for a new
  instance. Rows without an override shall reflect the displayed default;
  explicit true or false overrides shall remain independent of global changes.
- **AC-PLUGINS-MARKETPLACE-004.2:** Within one mounted settings page, rapid
  changes shall remain immediately usable and display the latest choice while
  pending. Successful writes shall persist in choice order; earlier receipts
  shall not replace a later pending choice on desktop or phone.
- **AC-PLUGINS-MARKETPLACE-004.3:** Each failed write shall report an error. After
  pending choices settle, the controls shall reflect the last successful read
  or write, never an unaccepted optimistic snapshot. A failed initial read shall
  retain the existing off fallback and enable later writes; that fallback is
  not confirmation of persisted state.

Cross-page, cross-tab, remount ordering and ambiguous transport outcomes are
excluded. Per-plugin writes, background eligibility and installation are unchanged.

## System design

The technical sources are the [marketplace design](../system-design/marketplace.md)
and [native-plugin attribution](../system-design/native-plugin-attribution.md).
