# Kandev plugin registry

This directory is the **official Kandev marketplace source** — the curated,
PR-edited list of plugins that show up when a user opens **Settings > Plugins >
Browse** inside Kandev.

It is deliberately minimal. It records *which repositories* are in the official
catalog; it does **not** store plugin metadata (display name, description,
author, version, tarball URL, checksum, stars). All of that is read from each
plugin's own GitHub release when the catalog index is built, so the listing can
never drift from what actually ships.

## Files

| File | Purpose |
| --- | --- |
| `plugins.yaml` | The curated pointer list — one entry per plugin repo. Human-edited via PR. |
| `schema.json` | JSON Schema (draft 2020-12) that `plugins.yaml` MUST validate against. Enforced in CI. |
| `build-index.mjs` | Build script (zero-dependency Node) that resolves each entry to a full catalog record and emits `index.json`. |
| `build-index.test.mjs` | `node --test` coverage for the build script's parser and enrichment. |

The generated `index.json` is published to GitHub Pages by the
[`plugin-registry-index`](../.github/workflows/plugin-registry-index.yml)
workflow and served as the fetch contract Kandev consumes.

## One repository per plugin

Each plugin lives in its **own** public GitHub repository, named
`kdlbs/kandev-plugin-<name>` for first-party plugins (third parties may use any
`owner/name`). Every plugin repo publishes its package as a GitHub **Release**
asset in the standard Kandev package format:

- `<id>-<version>.tar.gz` — the plugin package, and
- `checksums.txt` — the integrity manifest the install pipeline verifies.

The `kdlbs/kandev-plugin-template` starter repo is the recommended way to
bootstrap a new plugin with the right layout and a release workflow.

The catalog ranks plugins by **GitHub stars** (with a "recently updated"
alternative from each repo's last release). Kandev collects **no download or
usage telemetry** — there is no "most installed" metric, by design.

## Submitting a plugin to the official catalog

1. **Publish your plugin.** Push it to a public GitHub repository and cut a
   GitHub **Release** whose assets include `<id>-<version>.tar.gz` and
   `checksums.txt`. The release must pass the standard package integrity gate.
2. **Fork this repo** (`kdlbs/kandev`).
3. **Add one entry** to `plugin-registry/plugins.yaml` pointing at your public
   repo:

   ```yaml
   - id: my-plugin            # MUST equal the `id` in your plugin manifest
     repo: your-org/your-plugin-repo
     categories: [productivity]
   ```

   The `id` **MUST match the `id` in your plugin manifest** and be unique across
   the file. `repo` is `owner/name`. `featured` is a maintainer-only pin and
   should be left out of submissions.
4. **Open a pull request.** The index-build workflow runs on your PR (build +
   tests, no Pages deploy) and resolves your entry against the GitHub API — your
   repo must have a latest release whose assets include a correctly named
   `<id>-<version>.tar.gz` package, or the entry can't be built. `plugins.yaml`
   must follow the `schema.json` pointer-list contract, and your `id` must match
   your manifest `id` (the index looks for `<id>-<version>.tar.gz`, so a
   mismatch means your package can't be resolved).
5. **A maintainer reviews and merges** — maintainer approval is what gates the
   official catalog. Once merged, the index-build workflow picks up your entry
   and your plugin appears in the in-app catalog on the next build.

## Teams and corporates: host your own source instead

You do **not** have to PR into this registry to share plugins internally. Kandev
supports **additional marketplace sources**: any URL that serves an `index.json`
document of the same shape can be added under **Settings > Plugins** as an extra
source, and its plugins are merged into the catalog alongside the official ones.

This is the recommended path for a team or corporate registry — host your own
`index.json` (for example by copying this directory's `plugins.yaml` +
`build-index.mjs` + workflow into your own repo and pointing Kandev at your Pages
URL). Adding a source is an explicit act of trust in that source's maintainer,
much like `brew tap`-ing a third-party tap. When the same `id` appears in more
than one source, the official source wins and later duplicates are hidden.

## Sideloading is always available

Appearing in this registry is only about **discovery**. Anyone can still install
an unlisted plugin by URL or by uploading a `.tar.gz` directly — the marketplace
never removes those escape hatches.
