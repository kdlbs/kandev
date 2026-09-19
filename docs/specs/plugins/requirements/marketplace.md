---
status: active
system: plugins
created: 2026-07-18
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
- **AC-PLUGINS-MARKETPLACE-001.4:** The catalog shall show declared author credit separately from publisher identity. Official-source membership alone shall not identify a community plugin as Kandev-authored. The [publisher identity requirements](publisher-identity.md) own verification and reserved attribution.
- **AC-PLUGINS-MARKETPLACE-001.5:** The catalog repository link shall identify the repository named by the registry entry. Official publication shall reject a declared repository URL that differs from that repository.
- **AC-PLUGINS-MARKETPLACE-001.6:** Installing from the catalog shall remain one click. The backend shall resolve the selected entry and use the existing package installation pipeline. Publisher and package evidence follow the [publisher identity requirements](publisher-identity.md).
- **AC-PLUGINS-MARKETPLACE-001.7:** A catalog entry for a plugin that is already installed SHALL show an **Installed** state; when the catalog's latest version is newer than the installed version, it SHALL show an **Update available** affordance (which reinstalls the newer tarball).
- **AC-PLUGINS-MARKETPLACE-001.8:** The catalog SHALL be assembled from **one or more marketplace sources**. kandev ships with the **official kandev source** enabled by default; operators MAY add **additional sources** (a team or corporate registry) and the catalog merges them.

### REQ-PLUGINS-MARKETPLACE-002: Registry preview images

**Intent:** Registry maintainers can add a visual preview gallery to plugin
listings without changing the plugin package.

#### Acceptance criteria

- **AC-PLUGINS-MARKETPLACE-002.1:** Official and custom marketplace entries
  shall accept an ordered `previews` list of HTTPS image URLs and alternative
  text. Plugin entries can omit the list; existing entries shall remain valid.
- **AC-PLUGINS-MARKETPLACE-002.2:** A plugin listing with screenshots shall show
  its first image as the preview cover and expose all images in a details
  gallery. Entries without screenshots shall keep their current presentation.
- **AC-PLUGINS-MARKETPLACE-002.3:** Gallery navigation shall support keyboard
  and touch, display descriptions and image position, and keep install actions
  available when an image fails. Package code shall not run in the preview.
- **AC-PLUGINS-MARKETPLACE-002.4:** Preview metadata shall belong to the registry,
  not the plugin manifest or release bundle. Maintainers shall be able to change
  URLs, descriptions, or image order without releasing a new plugin version.

Canvas listing image requirements are owned by
[canvas marketplace and sharing](../../canvases/requirements/marketplace-sharing.md).

## System design

The migrated technical source is split into [part 1](../system-design/marketplace.md).
