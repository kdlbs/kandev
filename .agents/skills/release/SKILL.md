---
name: release
description: Kandev release and version-channel conventions — unified Stable SemVer plus deterministic npm-only Nightlies. Use when cutting a release, changing channels, debugging artifacts, or answering version-channel questions.
---

# Release & Versioning

Kandev Stable releases use a **single SemVer** `X.Y.Z` shared across all distribution channels.
npm also has an explicit prerelease-only `nightly` channel; it is not part of the unified Stable
artifact set.

## Version targets

- `apps/cli/package.json` version → `X.Y.Z`
- npm main package: `kandev@X.Y.Z`
- npm runtime packages: `@kdlbs/runtime-{platform}@X.Y.Z` (5 platforms; declared as `optionalDependencies` in main package)
- Git tag: `vX.Y.Z` (three-part; legacy `vM.m` tags normalize to `M.m.0`)
- Homebrew formula: `kdlbs/homebrew-kandev` `Formula/kandev.rb` `version "X.Y.Z"`
- GitHub release: `vX.Y.Z` with platform tarballs `kandev-{platform}.tar.gz` + `.sha256`

**npm and Homebrew are sibling channels**, not chained. Both consume the same GitHub release artifacts; neither depends on the other.

For npm Nightly, Stable `X.Y.Z` plus a full `main` SHA produces
`X.Y.(Z+1)-nightly.sha<first-12-lowercase-hex>`. `kandev` and all five runtime packages publish at
that exact version under npm's `nightly` dist-tag. Nightly never moves `latest` and creates no Git
tag, GitHub Release, Homebrew formula, Desktop feed/build, or container tag.

## Release flow

Stable runs entirely in CI via `.github/workflows/release.yml`, triggered by a maintainer from the GitHub Actions UI:

1. Maintainer clicks "Run workflow" → keeps `channel=stable` → picks `bump` (patch/minor/major) → optional `dry_run` or `desktop_validation_only`.
2. `prepare` job bumps version + regenerates CHANGELOG, opens release PR, squash-merges, tags `vX.Y.Z`.
3. `build-web` + `build-cli` + `build-bundles` (5 platforms) build the release artifacts.
4. `publish-release` creates the GitHub release with platform tarballs + sha256 + auto-generated notes.
5. `publish-npm` publishes 5 `@kdlbs/runtime-*` packages + main `kandev` package to npmjs.
6. `update-homebrew-tap` pushes updated `Formula/kandev.rb` to `kdlbs/homebrew-kandev` via SSH deploy key.

Stable has no local release driver; the entire Stable flow runs in GHA. The Nightly metadata and
publication revalidation state machine lives in `scripts/release/nightly-release.sh`, which GHA
invokes for scheduled and manual Nightly runs.

The same workflow schedules npm Nightly with cron `0 12 * * *`. It skips before building when
`main` has no commit after the latest Stable tag, the exact commit is already published, or a same
or newer `main` Nightly supersedes the scheduled commit. Eligible runs build only the shared web
bundle and five native runtime archives, then publish runtimes first and `kandev` last with OIDC
provenance. Stable and Nightly workflow runs share one non-cancelling release-wide concurrency
slot. Before publishing, Nightly rechecks the stable Git/npm baseline and the previously observed
`nightly` tag; a pending Stable tag or moved value safely suppresses stale publication.

Maintainers may run that same Nightly path from the Actions UI with the `main` ref and
`channel=nightly`. `dry_run=true` retains the real metadata and registry preflight but skips shared
builds and all npm writes. The shared form's required `bump` value is ignored for Nightly;
`desktop_validation_only` and `backfill_tag` are Stable-only and rejected when combined with it.

Validate Nightly automation changes with:

```bash
node --test scripts/release/nightly-version.test.mjs scripts/release/nightly-release.test.mjs
python3 .github/scripts/release-workflow-contract_test.py
bash -n scripts/release/nightly-release.sh scripts/release/publish-npm.sh
```

## Release-tag signing configuration

The release workflow reads signing configuration from the GitHub `release`
environment. `RELEASE_GPG_PRIVATE_KEY` and the optional
`RELEASE_GPG_PASSPHRASE` are environment secrets. The full 40-character
`RELEASE_GPG_FINGERPRINT` is an environment variable, not a secret: the
workflow reads `vars.RELEASE_GPG_FINGERPRINT`, so storing it as a secret makes
normal-release preflight treat it as missing.

`.github/release-signing-key.asc` must contain exactly one public primary key
whose fingerprint matches that variable; never commit private key material.
`backfill_tag` repairs publication for an already-signed existing tag only and
does not bypass the normal-release signing checks.

Desktop signing is automatic. Complete macOS/Windows signing and notarization secrets produce signed artifacts; missing or incomplete signing inputs produce unsigned desktop artifacts and the GitHub release notes get an unsigned-artifact warning. `desktop_validation_only=true` builds artifacts from the current workflow ref for maintainer inspection and skips the release PR, tag, GitHub release, npm publish, Homebrew update, and public container tags.

## Runtime resolution

In `apps/cli/src/runtime.ts`, the CLI locates its bundled runtime via:

1. `KANDEV_BUNDLE_DIR` env var (set by Homebrew wrapper, used by tests).
2. Installed `@kdlbs/runtime-{platform}` npm package via `require.resolve()`.
3. `--runtime-version <tag>` cache fallback (debug only — downloads from GitHub).

## Runtime helper binary checklist

When adding, renaming, or removing bundled helper binaries such as `agentctl-<goos>-<goarch>`, update every packaging surface in the same PR:

- backend build targets and scripts
- Docker/runtime image copy steps
- `.github/workflows/release.yml` bundle, macOS signing, and notarization loops
- `scripts/release/prepare-desktop-runtime.sh`
- `scripts/release/verify-desktop-runtime.sh`
- `scripts/release-desktop.test.sh`
- `apps/desktop/AGENTS.md` runtime resource list

Verify with the helper build plus release-runtime tests, for example:

```bash
make -C apps/backend build-agentctl-remote
bash scripts/release-desktop.test.sh
```
