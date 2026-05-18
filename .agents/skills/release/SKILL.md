---
name: release
description: Kandev release & versioning conventions — single SemVer across npm, Homebrew, GitHub release. Use when cutting a release, debugging release artifacts, or answering questions about version channels.
---

# Release & Versioning

Kandev uses a **single SemVer** `X.Y.Z` shared across all distribution channels.

## Version targets

- `apps/cli/package.json` version → `X.Y.Z`
- npm main package: `kandev@X.Y.Z`
- npm runtime packages: `@kdlbs/runtime-{platform}@X.Y.Z` (5 platforms; declared as `optionalDependencies` in main package)
- Git tag: `vX.Y.Z` (three-part; legacy `vM.m` tags normalize to `M.m.0`)
- Homebrew formula: `kdlbs/homebrew-kandev` `Formula/kandev.rb` `version "X.Y.Z"`
- GitHub release: `vX.Y.Z` with platform tarballs `kandev-{platform}.tar.gz` + `.sha256`

**npm and Homebrew are sibling channels**, not chained. Both consume the same GitHub release artifacts; neither depends on the other.

## Release flow

Entirely in CI via `.github/workflows/release.yml`, triggered by a maintainer from the GitHub Actions UI:

1. Maintainer clicks "Run workflow" → picks `bump` (patch/minor/major) → optional `dry_run`.
2. `prepare` job bumps version + regenerates CHANGELOG, opens release PR, squash-merges, tags `vX.Y.Z`.
3. `build-web` + `build-cli` + `build-bundles` (5 platforms) build the release artifacts.
4. `publish-release` creates the GitHub release with platform tarballs + sha256 + auto-generated notes.
5. `publish-npm` publishes 5 `@kdlbs/runtime-*` packages + main `kandev` package to npmjs.
6. `update-homebrew-tap` pushes updated `Formula/kandev.rb` to `kdlbs/homebrew-kandev` via SSH deploy key.

There is no local release script — the entire flow runs in GHA.

## Runtime resolution

In `apps/cli/src/runtime.ts`, the CLI locates its bundled runtime via:

1. `KANDEV_BUNDLE_DIR` env var (set by Homebrew wrapper, used by tests).
2. Installed `@kdlbs/runtime-{platform}` npm package via `require.resolve()`.
3. `--runtime-version <tag>` cache fallback (debug only — downloads from GitHub).
