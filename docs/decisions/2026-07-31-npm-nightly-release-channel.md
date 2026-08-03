# ADR-2026-07-31-npm-nightly-release-channel: Publish deterministic npm-only nightlies

**Status:** accepted (amended 2026-08-03)
**Date:** 2026-07-31
**Area:** workflow, backend, frontend, cli

## Context

Kandev's stable release workflow intentionally gives npm, Homebrew, GitHub Releases, Desktop, and
containers one shared `X.Y.Z`. Users also need prerelease access to current `main`, but Homebrew
and Desktop require separate mutable-feed and signing designs. npm already has six coordinated
packages, trusted publishing bound to `.github/workflows/release.yml`, and native dist-tags.

A commit-derived prerelease must be deterministic for retries and valid even when the abbreviated
SHA is digits-only or begins with zero. Update discovery must not sort SHA text as time.

## Decision

Kandev has an npm-only `nightly` channel. A stable baseline `X.Y.Z` and current `main` SHA produce
`X.Y.(Z+1)-nightly.sha{first-12-lowercase-hex}`. Time and git-describe's `g` marker are excluded:
the commit is the identity, and the `sha` prefix makes the SemVer identifier unambiguously
nonnumeric.

The 12-hex abbreviation is an accepted compactness trade-off. Before an already-published version
can skip a run, the workflow resolves the abbreviation against full `main` history and requires the
resolved commit to equal the scheduled SHA. Git rejects an ambiguous abbreviation, so a prefix
collision fails closed for maintainer resolution rather than silently treating a different commit
as published.

The existing release workflow owns both stable and nightly npm publication because npm allows one
trusted publisher per package and validates the workflow filename. Its manual dispatch defaults to
Stable but also permits a Nightly run from `main`; the scheduled and manual Nightly paths use the
same metadata, build, and publish jobs. Manual Nightly dry runs execute the real metadata and
registry preflight, then stop before builds or npm writes. The workflow's required Stable bump
selector is ignored for Nightly, while Stable-only desktop validation and backfill inputs are
rejected instead of silently changing the requested operation.

Nightlies publish all five runtime packages before the launcher with
`npm publish --tag nightly`; stable `latest` tags remain untouched. The publisher and Nightly
preflight load one shared package inventory. Nightly jobs do not enter the Git tag, GitHub Release,
Desktop, container, or Homebrew graph. Stable and Nightly workflow runs share one non-cancelling
concurrency group from start through publication.
This release-wide lock is required because Stable pushes its Git tag before its npm publish job is
ready; locking only the two npm jobs leaves a window where Nightly can derive from the previous npm
baseline. Before building and again before publishing, Nightly requires the highest stable Git tag
to match npm `latest`. A scheduled commit superseded by that Stable tag exits successfully.

A Nightly target must be newer in `main` ancestry than the published Nightly unless the current
target is incomplete and needs repair. An older partial runtime publication is recoverable only
when its embedded commit is an ancestor of current `main`. Divergent or newer partial tags fail
closed. Before publishing, both the Stable baseline and previously observed Nightly tag must still
match.

The backend owns an install-wide Stable/Nightly preference. Stable remains the default and resolves
GitHub Releases. Nightly resolves npm's `kandev@nightly` target and is selectable only for verified
managed npm/npx user services. Update intents always contain an exact version. Nightly-to-nightly
availability follows dist-tag inequality, not SemVer ordering of SHA text. Apply requests bind to
the exact cached version shown to the user; if discovery has since changed that cache, the backend
rejects the stale target instead of re-resolving or installing a different artifact.

## Consequences

Users get one documented prerelease path without weakening stable channels or Desktop signing.
One full commit deterministically maps to one immutable version, making scheduled retries safe and
observable. A collision in the accepted 12-hex namespace blocks automatic publication and needs
maintainer resolution. Publishing six packages can still fail partially, so
runtime-first/main-last order and tag-consistency checks are required.

npm accumulates immutable nightly versions. Homebrew and Desktop users do not receive channel
parity in this iteration. The release workflow must explicitly gate every stable-only job when
handling Nightly events. The shared dispatch form still displays Stable-only inputs for a manual
Nightly, so validation and input descriptions must keep their meaning explicit. A manual Stable
release can wait for an in-flight Nightly workflow; Nightlies also wait behind a Stable run,
including any release-environment approval. Queued Nightlies whose selected commit was superseded
by the completed Stable release skip.

## Alternatives Considered

- **Timestamp plus SHA:** gives chronological SemVer ordering but creates different immutable
  versions for the same commit and complicates retries.
- **Raw abbreviated SHA:** can become a numeric SemVer identifier with an illegal leading zero;
  `sha` is a small explicit validity guard.
- **Full 40-hex SHA:** eliminates abbreviation collisions but makes every user-visible package
  version substantially longer; the shorter identity plus fail-closed ambiguity check is preferred.
- **Separate scheduled or manual Nightly workflow:** cleaner YAML isolation, but it conflicts with
  npm's single trusted publisher configuration for the existing six packages and would duplicate
  the Nightly safety state machine.
- **Serialize only npm publish jobs:** permits more build overlap, but Stable creates its tag before
  its npm job enters that lock, allowing Nightly to derive from a stale npm baseline.
- **GitHub prereleases:** would duplicate stable release artifacts and feeds when npm is the only
  requested consumer.
- **Homebrew `HEAD` or a nightly formula:** viable future designs, but they need source builds or a
  separate mutable formula/tap and do not match the current immutable-asset formula.
