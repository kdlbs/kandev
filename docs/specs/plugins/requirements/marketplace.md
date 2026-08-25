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
- **AC-PLUGINS-MARKETPLACE-001.4:** Release authors and registry reviewers SHALL apply the attribution convention: first-party `kdlbs/kandev-plugin-*` releases declare `author: "kandev"`, community releases declare the contributor's stable identity, and a plugin is not labeled `kandev` only because a maintainer curated it into the official source. This is a release and curation check, not an index-builder ownership validation rule. The builder preserves a declared manifest author and falls back to the repository owner for legacy releases without one.
- **AC-PLUGINS-MARKETPLACE-001.5:** The catalog source repository link SHALL identify the repository named by the registry entry. The index builder derives this value from the registry pointer. A release manifest's `repo_url`, when present, SHOULD identify the same repository so installed and marketplace views do not disagree; release and registry review is responsible for flagging mismatches.
- **AC-PLUGINS-MARKETPLACE-001.6:** Installing from the catalog SHALL be **one click**: it resolves to the plugin's latest release tarball URL and runs the existing verified install pipeline (`POST /api/plugins/install`). No new install mechanism is introduced.
- **AC-PLUGINS-MARKETPLACE-001.7:** A catalog entry for a plugin that is already installed SHALL show an **Installed** state; when the catalog's latest version is newer than the installed version, it SHALL show an **Update available** affordance (which reinstalls the newer tarball).
- **AC-PLUGINS-MARKETPLACE-001.8:** The catalog SHALL be assembled from **one or more marketplace sources**. kandev ships with the **official kandev source** enabled by default; operators MAY add **additional sources** (a team or corporate registry) and the catalog merges them.

## System design

The migrated technical source is split into [part 1](../system-design/marketplace.md).
