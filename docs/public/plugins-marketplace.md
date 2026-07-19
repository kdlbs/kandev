---
title: "Plugin Marketplace"
description: "Discover and install kandev plugins from the in-app catalog, keep them updated, add a team or corporate source, and publish your own plugin to the official catalog."
status: experimental
---

# Plugin Marketplace

The marketplace is a discoverable, curated catalog of kandev plugins you can
browse and install from inside the app — no tarball URL required. It is
assembled from one or more **sources**: kandev ships with the official source
enabled by default, and you can add team or corporate sources alongside it.

This page covers using the marketplace (browse, install, update, add sources)
and publishing a plugin into it. For what plugins are and how the install
pipeline, enable/disable, and security posture work, see
[Plugins](plugins.md). For building one, see [Authoring a
plugin](plugins-authoring.md).

Like the rest of plugins, the marketplace is gated behind the `plugins` feature
flag (**Settings > System > Feature Toggles**) — off by default in production,
on by default in dev/e2e builds. Enabling it requires a backend restart and
surfaces **Settings > Plugins** in the sidebar.

> **Sideloading still works.** The marketplace only adds *discovery*. Installing
> a plugin by URL or by uploading a `.tar.gz` is unchanged and always available,
> even with every source disabled or offline — see
> [Plugins → Installing a plugin](plugins.md#installing-a-plugin).

## Using the marketplace

**Settings > Plugins** has two tabs:

- **Installed** — the plugins on this instance, with Enable / Disable /
  Uninstall and an **Update** button when a newer version is available.
- **Browse** — the merged catalog across all enabled sources.

### Browse and install

Open **Settings > Plugins > Browse**. Each plugin shows as a card with its
name, description, author, categories, source repository link, latest version,
and GitHub star count. To narrow the list:

- **Search** — type in the search box to match plugin name or description.
- **Category** — filter to a single category with the category dropdown.
- **Sort** — **Most stars** (default), **Recently updated** (by latest release
  / repo activity), or **Name**.

Stars are a **sort hint, not a quality score**. kandev collects no download or
usage telemetry, so there is no "most installed" metric — ranking is GitHub
stars only, and "Recently updated" surfaces new or actively maintained plugins
that high-star incumbents would otherwise bury.

Click **Install** on a card to install it. This resolves the entry to its
latest release tarball and runs the same verified install pipeline as
install-by-URL (`POST /api/plugins/install`) — see [Plugins → Installing a
plugin](plugins.md#installing-a-plugin) for the exact steps and integrity
checks. A card for a plugin you already have at the latest version shows
**Installed** instead of an install button.

Plugin-provided icons render on the cards. A plugin that ships an icon (via the
manifest's `icon` field) shows it; otherwise the card falls back to a neutral
letter tile.

### Keep plugins updated

When the marketplace advertises a newer version than the one you have
installed, the **Installed** tab shows an **Update to v`<version>`** button on
that plugin's row. Clicking it reinstalls the newer tarball through the normal
pipeline; Enable, Disable, and Uninstall are unchanged.

Updates are never automatic — kandev surfaces the newer version and waits for
an explicit click. There are no update channels and no background auto-update.

### Add a team or corporate source

The **Sources** button on the Browse tab opens the **Marketplace sources**
dialog. The official kandev source is present by default, badged **Official**,
and cannot be removed. To add another:

1. Click **Sources**.
2. Enter a **name** (e.g. `Acme Internal`) and the **URL** of an `index.json`
   document (see [Host your own source](#host-your-own-source) below).
3. Click **Add**.

Its plugins are then merged into the Browse tab alongside the official ones.
You can enable/disable a source without deleting it, and remove any non-official
source. Adding a source is an explicit act of trust in its maintainer, much
like `brew tap`-ing a third-party tap.

Notes:

- When the same plugin `id` appears in more than one source, the **first
  configured source wins** (the official source is always first); later
  duplicates are hidden.
- A source that is unreachable or serves malformed JSON is reported as
  **degraded** — its entries are omitted, but the healthy sources still load.
- Configured sources persist across restarts; the fetched catalog itself is a
  short-lived cache and is re-fetched on demand.

## Publishing a plugin

Getting a plugin into the catalog follows a **one-repo-per-plugin** model,
mirroring Obsidian's community-plugin registry: each plugin lives in its own
public GitHub repository and publishes its package as a GitHub **Release**
asset; the official catalog is a curated pointer list that names *which repos*
are included. The descriptive metadata (name, description, version, tarball
URL, stars) is always read from your latest release, so a listing can never
drift from what actually ships.

### 1. Publish the plugin package as a release

Package your plugin as usual (see [Authoring a plugin →
Packaging](plugins-authoring.md#packaging)) and cut a GitHub **Release** whose
assets include both:

- `<id>-<version>.tar.gz` — the plugin package, and
- `checksums.txt` — the integrity manifest the install pipeline verifies.

The release must pass the standard package integrity gate. The
`kdlbs/kandev-plugin-template` starter repo is the recommended way to bootstrap
a repo with the right layout and a release workflow.

### 2. Add an icon (optional)

Add an `icon:` field to your `manifest.yaml` — a **package-relative path** to an
image your package ships (for example `icon.svg`). The catalog resolves it to
your icon and renders it on the plugin card; without it, the card shows a letter
tile. See the [Plugin manifest reference](plugins-manifest.md#field-reference).

### 3. Submit to the official catalog

The official source is curated: a plugin appears only after a maintainer merges
a PR that lists it.

1. Fork `kdlbs/kandev` and add one entry to
   [`plugin-registry/plugins.yaml`](https://github.com/kdlbs/kandev/blob/main/plugin-registry/plugins.yaml)
   pointing at your public repo:

   ```yaml
   plugins:
     - id: my-plugin              # MUST equal the `id` in your plugin manifest
       repo: your-org/your-plugin-repo   # owner/name
       categories: [productivity] # optional
   ```

   `id` must match your manifest `id` and be unique in the file — the index
   build looks for a `<id>-<version>.tar.gz` release asset, so a mismatch means
   your package can't be resolved. `categories` here are free-form curation tags
   for catalog filtering (not the manifest's category enum). `featured` is a
   maintainer-only pin — leave it out of submissions. The pointer-list shape is
   defined by
   [`plugin-registry/schema.json`](https://github.com/kdlbs/kandev/blob/main/plugin-registry/schema.json).
2. Open a pull request. The registry index-build workflow runs on your PR
   (build + tests, no Pages deploy), resolving your entry against the GitHub API
   — your repo must have a latest release whose assets include the correctly
   named `<id>-<version>.tar.gz` package, or the entry can't be built.
3. A maintainer reviews and merges — maintainer approval is what gates the
   official catalog. The index-build workflow then picks up your entry and your
   plugin appears in the in-app catalog on the next build.

Ranking in the catalog is **GitHub stars only** — there is no download or usage
telemetry to game. Full submission details are in
[`plugin-registry/README.md`](https://github.com/kdlbs/kandev/blob/main/plugin-registry/README.md).

## Host your own source

You do not have to PR into the official registry to share plugins internally.
Any URL that serves an `index.json` document of the catalog shape can be added
as a marketplace source (see [Add a team or corporate
source](#add-a-team-or-corporate-source)), and its plugins merge into the Browse
tab alongside the official ones. This is the recommended path for a team or
corporate registry — no PR to the main repo.

The `index.json` document is the fetch contract between a source and kandev. The
simplest way to produce one is to copy the official registry's
`plugin-registry/` directory into your own repo — its `plugins.yaml` +
[`build-index.mjs`](https://github.com/kdlbs/kandev/blob/main/plugin-registry/build-index.mjs)
build script (zero-dependency Node) + GitHub Action resolve each listed repo's
latest release into a full catalog record and publish the generated
`index.json` to GitHub Pages. Point kandev at that Pages URL. The document
shape, the build pipeline, and the source data model are specified in the
[plugin marketplace spec](https://github.com/kdlbs/kandev/blob/main/docs/specs/plugins/marketplace.md).

Related: [Plugins](plugins.md), [Authoring a
plugin](plugins-authoring.md), [Plugin manifest
reference](plugins-manifest.md).
