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
| `build-index.mjs` | Node builder that resolves releases, calls the repository package verifier, and emits `index.json`. |
| `check-releases.mjs` | Central allowlist-only detector for newer curated releases. |
| `*.test.mjs` | `node --test` coverage for detection, package/index construction, fallback, and workflow contracts. |

The generated `index.json` is published to GitHub Pages by the
[`plugin-registry-index`](../.github/workflows/plugin-registry-index.yml)
workflow and served as the fetch contract Kandev consumes.

## One repository per plugin

Each plugin lives in its **own** public GitHub repository, named
`kdlbs/kandev-plugin-<name>` for first-party plugins (third parties may use any
`owner/name`). Every plugin repo publishes its package as a GitHub **Release**
asset in the standard Kandev package format:

- `<id>-<version>.tar.gz` — the required plugin package. The archive contains
  its own generated `checksums.txt`, which the install pipeline verifies.
- `checksums.txt` — an optional release asset containing the tarball digest.
  When present, the registry builder requires its digest for the exact package
  to match. Every newly accepted package also receives a computed
  `package_sha256` in the index.

The `kdlbs/kandev-plugin-template` starter repo is the recommended way to
bootstrap a new plugin with the right layout and a release workflow.

The catalog ranks plugins by **GitHub stars** (with a "recently updated"
alternative from each repo's last release). Kandev collects **no download or
usage telemetry** — there is no "most installed" metric, by design.

## Submitting a plugin to the official catalog

1. **Publish your plugin.** Push it to a public GitHub repository and cut a
   GitHub **Release** whose assets include `<id>-<version>.tar.gz`. A separate
   release-level `checksums.txt` is optional; the package's internal generated
   checksum manifest is mandatory. The release must pass the standard package
   integrity gate.
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
   tests, no Pages deploy) and resolves your entry against the GitHub API. The
   latest release must contain the exact `<id>-<version>.tar.gz` package. The
   builder verifies the archive's internal checksums and managed manifest, and
   requires the verified manifest ID/version to match the curated entry and
   release tag. `plugins.yaml` must also follow the `schema.json` pointer-list
   contract.
5. **A maintainer reviews and merges** — maintainer approval is what gates the
   official catalog. Once listed, a central read-only poll checks curated
   repositories every five minutes and targets publication within 10 minutes
   under normal GitHub Actions scheduling. GitHub schedules can be delayed or
   dropped, so this is an operational SLO rather than a hard wall-clock
   guarantee. The daily 06:00 UTC rebuild remains the fallback and refreshes
   star counts.

## Publication security and failure behavior

The checked-out `plugins.yaml` is the only repository allowlist. The poll
accepts no repository or release payload from plugin repositories, and those
repositories receive no Kandev credential. Its token is read-only; Pages write
and OIDC permissions exist only on the central deployment-capable workflow.

All push, manual, daily, and release-poll builds share the static
`plugin-registry-pages` concurrency group. An active deployment finishes and
pending requests coalesce, preventing Pages races while every retained build
resolves current releases when it starts.

If one latest release is missing or invalid, the builder retains the prior
record for that same still-curated repository and reports an Actions warning
and step summary; other valid releases may advance. Delisted repositories are
never restored from prior output. A provider-wide failure or an invalid release
without a trusted prior fails before artifact upload, so the published Pages
site stays at its last known-good catalog. The failed poll/build run and its
annotations are the operator-visible evidence.

## Teams and corporates: host your own source instead

You do **not** have to PR into this registry to share plugins internally. Kandev
supports **additional marketplace sources**: any URL that serves an `index.json`
document of the same shape can be added under **Settings > Plugins** as an extra
source, and its plugins are merged into the catalog alongside the official ones.

This is the recommended path for a team or corporate registry — host your own
`index.json` (for example by adapting this repository's registry workflows,
pointer list, builder, and Go package verifier, then pointing Kandev at the
resulting Pages URL). Adding a source is an explicit act of trust in that
source's maintainer,
much like `brew tap`-ing a third-party tap. When the same `id` appears in more
than one source, the official source wins and later duplicates are hidden.

## Sideloading is always available

Appearing in this registry is only about **discovery**. Anyone can still install
an unlisted plugin by URL or by uploading a `.tar.gz` directly — the marketplace
never removes those escape hatches.
