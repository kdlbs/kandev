---
status: active
system: release
created: 2026-07-31
owners:
  - kandev
---
# npm nightly channel Requirements

## Overview

Users who want fixes from `main` currently have to build Kandev themselves or wait for the next stable release. Maintainers also lack one supported prerelease path that exercises the same npm launcher and native runtime packages users install in production.

## Requirements

### REQ-RELEASE-NPM-NIGHTLY-CHANNEL-001: npm nightly channel

**Intent:** Users who want fixes from `main` currently have to build Kandev themselves or wait for the next stable release. Maintainers also lack one supported prerelease path that exercises the same npm launcher and native runtime packages users install in production.

#### Acceptance criteria

- **AC-RELEASE-NPM-NIGHTLY-CHANNEL-001.1:** Kandev publishes an npm nightly from `main` at 12:00 UTC when `main` contains commits after the latest stable release and that exact commit has not already been published.
- **AC-RELEASE-NPM-NIGHTLY-CHANNEL-001.2:** Maintainers can run the same Nightly path manually from `main`. A manual Nightly dry run performs metadata and registry preflight only, with no platform builds or npm writes.
- **AC-RELEASE-NPM-NIGHTLY-CHANNEL-001.3:** A nightly for stable `X.Y.Z` and commit `abcdef123456...` has version `X.Y.(Z+1)-nightly.shaabcdef123456`.
- **AC-RELEASE-NPM-NIGHTLY-CHANNEL-001.4:** The 12-hex abbreviation is an accepted compactness trade-off. Before an existing version can skip publication, Git must resolve that abbreviation to the exact scheduled commit; an ambiguous prefix or identity mismatch fails closed for maintainer resolution.
- **AC-RELEASE-NPM-NIGHTLY-CHANNEL-001.5:** Every nightly publishes `kandev` and all five `@kdlbs/runtime-*` packages at the same immutable version under the npm `nightly` dist-tag. The stable `latest` dist-tag does not move.
- **AC-RELEASE-NPM-NIGHTLY-CHANNEL-001.6:** Users persistently install the channel with `npm install -g kandev@nightly`. The command `npx -y kandev@nightly` runs a transient Nightly copy without changing a global installation.
- **AC-RELEASE-NPM-NIGHTLY-CHANNEL-001.7:** Settings > System > Updates exposes Stable and Nightly for a verified, Kandev-managed npm or npx user service. Stable is the default and the setting is install-wide.
- **AC-RELEASE-NPM-NIGHTLY-CHANNEL-001.8:** An npx run is managed only after it installs the service (for example, `npx -y kandev@nightly service install`); a transient `npx -y kandev@nightly` run alone neither creates a managed service nor persists an update-channel selection.

## Migrated source detail

## Why

Users who want fixes from `main` currently have to build Kandev themselves or wait for the next
stable release. Maintainers also lack one supported prerelease path that exercises the same npm
launcher and native runtime packages users install in production.

## What

- Kandev publishes an npm nightly from `main` at 12:00 UTC when `main` contains commits after the
  latest stable release and that exact commit has not already been published.
- Maintainers can run the same Nightly path manually from `main`. A manual Nightly dry run performs
  metadata and registry preflight only, with no platform builds or npm writes.
- A nightly for stable `X.Y.Z` and commit `abcdef123456...` has version
  `X.Y.(Z+1)-nightly.shaabcdef123456`.
- The 12-hex abbreviation is an accepted compactness trade-off. Before an existing version can
  skip publication, Git must resolve that abbreviation to the exact scheduled commit; an ambiguous
  prefix or identity mismatch fails closed for maintainer resolution.
- Every nightly publishes `kandev` and all five `@kdlbs/runtime-*` packages at the same immutable
  version under the npm `nightly` dist-tag. The stable `latest` dist-tag does not move.
- Users persistently install the channel with `npm install -g kandev@nightly`. The command
  `npx -y kandev@nightly` runs a transient Nightly copy without changing a global installation.
- Settings > System > Updates exposes Stable and Nightly for a verified, Kandev-managed npm or
  npx user service. Stable is the default and the setting is install-wide.
- An npx run is managed only after it installs the service (for example,
  `npx -y kandev@nightly service install`); a transient `npx -y kandev@nightly` run alone neither
  creates a managed service nor persists an update-channel selection.
- Desktop, Homebrew, Scoop, local-checkout, unknown, unmanaged, and system-service installations remain
  on the Stable channel.
- Stable update discovery continues to use GitHub Releases. Nightly discovery follows npm's
  `kandev@nightly` dist-tag.
- Applying an npm/npx update submits the exact version shown in the Updates page. The backend
  accepts it only while it still matches the selected channel's cached target, then installs that
  immutable version; a changed cache returns `409 Conflict` and requires a refresh.
- Nightly publication creates no Git tag, GitHub Release, changelog commit, desktop update,
  container tag, Homebrew update, or Scoop update. Scoop remains a Stable-only channel.

Decision: [ADR-2026-07-31-npm-nightly-release-channel](../../../decisions/2026-07-31-npm-nightly-release-channel.md).

Implementation plan: [npm nightly channel](../../../plans/npm-nightly-channel/plan.md).

## Data model

The install-wide `settings` table contains key `updates_channel`. Its value is the UTF-8 string
`stable` or `nightly`; a missing or invalid value reads as `stable`.

The existing stable target cache remains in these `kandev_meta` keys:

- `latest_version`
- `latest_version_url`
- `latest_version_checked_at`

Nightly uses an isolated cache:

- `latest_version_nightly`
- `latest_version_nightly_url`
- `latest_version_nightly_checked_at`

Changing channel never reinterprets cached data from the other source.

## API surface

`GET /api/v1/system/updates` and `POST /api/v1/system/updates/check` retain their current fields
and add:

```json
{
  "channel": "stable",
  "channel_editable": true,
  "channel_unsupported_reason": ""
}
```

`PATCH /api/v1/system/updates/channel` accepts:

```json
{ "channel": "nightly" }
```

It returns the complete updates response for the selected channel. Invalid channel names return
`400`. Selecting Nightly for an unsupported installation returns `409`. Persistence or resolver
failures return `500` or `502` using the existing System handler error conventions.

The npm registry contract is the public metadata document for package `kandev`. The resolver reads
`dist-tags.nightly`, requires a valid SemVer, and requires the same version to exist in `versions`.

## Permissions

Anyone allowed to view System settings may read update status. The existing admin guard applies to
manual checks, channel changes, and update application. Channel choice is shared by all users of
the installation.

## Failure modes

- A GitHub or npm discovery failure preserves that channel's previous cache and surfaces the stale
  checked time plus the request error.
- Apply never re-resolves a moving channel tag. It locks the selected-channel cache, rejects a
  submitted `target_version` that no longer matches, and writes the matching exact version into the
  update intent.
- A malformed or missing npm `nightly` tag fails closed; it is never offered or installed.
- A failed channel save keeps the draft dirty and surfaces that save failure instead of retaining a
  stale manual-check error or retry countdown.
- A scheduled run with no commits after the stable tag exits successfully without building.
- A manual Nightly dispatch from a ref other than `main` fails before resolving metadata.
- Manual Nightly rejects the Stable-only desktop-validation and backfill modes. The workflow's
  required bump selector is ignored for Nightly, while `dry_run` remains supported.
- A scheduled retry for an already-published commit exits successfully only when the main package
  and `nightly` tag agree.
- Before building, the workflow resolves the commit prefix in the current `nightly` tag. A rerun
  behind a newer published commit is superseded and exits without building. The same commit skips
  only when every package exists and its `nightly` tag matches; an incomplete current target
  proceeds to repair. Divergent or unresolvable history fails closed.
- A 12-hex collision makes Git abbreviation resolution ambiguous, so the run fails closed instead
  of treating the colliding commit as already published.
- Stable and Nightly workflow runs share one non-cancelling release-wide concurrency group. This
  covers Stable tag creation through npm publication, so Nightly cannot derive a version while a
  newly tagged Stable release is still pending on npm.
- Before building and again before publishing, Nightly requires the highest stable Git tag to match
  `kandev@latest`. A pending Stable tag or a scheduled commit superseded by the current Stable tag
  exits successfully without publishing.
- Before publishing, a Nightly run rechecks `kandev@latest`. If the baseline moved while Nightly
  was building, the stale run exits without publishing.
- The same locked preflight requires `kandev@nightly` to equal the value observed before building;
  an overlapping run that already moved the tag supersedes the stale run.
- Runtime packages publish before `kandev`. If any runtime fails, the main launcher is not
  published, so no visible launcher references missing exact dependencies.
- Trusted-publisher OIDC is used only by `npm publish --tag nightly`. An existing version whose
  `nightly` tag does not match fails with recovery guidance because OIDC cannot run
  `npm dist-tag add`.
- GitHub's scheduled start may be later than 12:00 UTC; delayed execution does not change the
  deterministic version.

## Persistence guarantees

- Channel choice and both target caches survive backend restarts.
- One source failure never overwrites the other source's cache.
- Each full commit deterministically maps to one npm version, so a retry never creates a second
  version for the same source state. The accepted 12-hex collision case halts automatic
  publication rather than mapping a second commit to the existing package.
- npm nightly versions are immutable and retained; this feature performs no automated deletion.

## Scenarios

- **GIVEN** stable `0.82.0` and `main` commit `abc123def456...`, **WHEN** the nightly schedule runs,
  **THEN** all six packages publish as `0.82.1-nightly.shaabc123def456` under `nightly` and
  `kandev@latest` is unchanged.
- **GIVEN** an eligible `main` commit, **WHEN** a maintainer manually dispatches the Nightly
  channel, **THEN** the same metadata, shared builds, safeguards, and npm publication run as the
  schedule.
- **GIVEN** a manual Nightly with `dry_run=true`, **WHEN** its preflight resolves an eligible
  target, **THEN** the run reports that exact version and commit without building bundles or
  changing npm.
- **GIVEN** a manual Nightly dispatched from a non-`main` ref or combined with desktop validation
  or backfill, **WHEN** validation runs, **THEN** it fails with an actionable input error.
- **GIVEN** `main` points at the latest stable tag's commit, **WHEN** the nightly schedule runs,
  **THEN** it exits successfully without a platform build or npm publication.
- **GIVEN** the highest Stable Git tag is newer than npm `latest`, **WHEN** Nightly prepares or
  publishes, **THEN** it exits successfully rather than deriving from the stale npm baseline.
- **GIVEN** a Nightly schedule was queued before a Stable release superseded its commit, **WHEN**
  that Nightly starts after Stable completes, **THEN** it exits successfully without building.
- **GIVEN** the current `main` nightly already exists and `kandev@nightly` points to it, **WHEN**
  the schedule runs again, **THEN** it exits successfully without rebuilding.
- **GIVEN** a previous run published only some runtime packages, **WHEN** the same commit retries,
  **THEN** matching packages are accepted, missing packages publish, and `kandev` publishes last.
- **GIVEN** an older run left only some runtime tags advanced, **WHEN** a later `main` commit runs,
  **THEN** ancestor-tagged partial packages are repaired by the later publish without manual tag
  edits.
- **GIVEN** no channel setting, **WHEN** a user opens Updates, **THEN** Stable is selected and the
  target comes from GitHub Releases.
- **GIVEN** a verified npm managed user service, **WHEN** an admin selects and saves Nightly,
  **THEN** the setting survives reload and the target resolves from `kandev@nightly`.
- **GIVEN** a Nightly save is pending, **WHEN** the admin changes the draft back to Stable before
  the response arrives, **THEN** the returned Nightly state becomes the saved baseline while the
  newer Stable draft remains selected, dirty, and available for a follow-up save.
- **GIVEN** a Homebrew, Desktop, unmanaged, system-service, local, or unknown installation,
  **WHEN** Updates renders, **THEN** Nightly is unavailable with a visible reason and Stable stays
  effective.
- **GIVEN** an installed nightly whose SHA sorts lexically after the new target SHA, **WHEN** npm's
  `nightly` tag changes, **THEN** the new unequal target is offered; SHA text is not treated as a
  chronological counter.
- **GIVEN** a user running a nightly selects Stable, **WHEN** a valid stable target differs,
  **THEN** the UI offers an explicit return to that exact stable version without announcing it as
  a normal upgrade notification.
- **GIVEN** the Updates page shows target A and discovery changes the selected-channel cache to B,
  **WHEN** the user submits Apply for A, **THEN** the backend returns `409 Conflict` and installs
  neither target.
- **GIVEN** a Pixel 5 viewport, **WHEN** the user selects Nightly and saves, **THEN** the same
  persisted outcome is reachable through 44px rows with no horizontal document overflow.

## Out of scope

- Homebrew `HEAD`, a nightly formula, or a second tap
- Desktop nightly updater feeds
- GHCR nightly tags
- Nightly GitHub Releases or Git tags
- Automatic update application
- Per-user channels
- Additional beta/canary channels
- Timestamped versions
