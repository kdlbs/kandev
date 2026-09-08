---
status: active
system: release
created: 2026-08-11
owners:
  - Kandev
---
# Scoop Stable Release Automation Requirements

## Overview

Windows users can install Kandev from the supported Scoop bucket. The bucket can remain on an older version after Kandev publishes a new Stable release. This delay prevents `scoop update kandev` from installing the current Stable version.

## Requirements

### REQ-RELEASE-SCOOP-RELEASE-AUTOMATION-001: Scoop Stable Release Automation

**Intent:** Windows users can install Kandev from the supported Scoop bucket. The bucket can remain on an older version after Kandev publishes a new Stable release. This delay prevents `scoop update kandev` from installing the current Stable version.

#### Acceptance criteria

- **AC-RELEASE-SCOOP-RELEASE-AUTOMATION-001.1:** Each Kandev Stable release updates `kdlbs/scoop-kandev` after the GitHub Release and its Windows runtime checksum are available.
- **AC-RELEASE-SCOOP-RELEASE-AUTOMATION-001.2:** The Scoop manifest uses the exact Stable version, release asset URL, and published SHA-256 value for `kandev-windows-x64.tar.gz`.
- **AC-RELEASE-SCOOP-RELEASE-AUTOMATION-001.3:** The Scoop update is a sibling publication channel to npm and Homebrew.
- **AC-RELEASE-SCOOP-RELEASE-AUTOMATION-001.4:** A Stable release is incomplete until the Scoop update succeeds and its manifest is correct.
- **AC-RELEASE-SCOOP-RELEASE-AUTOMATION-001.5:** A release backfill reconciles the Scoop manifest for the selected existing Stable tag.
- **AC-RELEASE-SCOOP-RELEASE-AUTOMATION-001.6:** A repeated update for the same version succeeds without an empty commit.
- **AC-RELEASE-SCOOP-RELEASE-AUTOMATION-001.7:** A Nightly, dry run, or Desktop validation run does not change the Scoop bucket.
- **AC-RELEASE-SCOOP-RELEASE-AUTOMATION-001.8:** The release workflow uses a separate repository-scoped credential for the Scoop bucket. It does not use the Kandev workflow token for cross-repository writes.

## Migrated source detail

## Why

Windows users can install Kandev from the supported Scoop bucket. The bucket
can remain on an older version after Kandev publishes a new Stable release.
This delay prevents `scoop update kandev` from installing the current Stable
version.

## What

- Each Kandev Stable release updates `kdlbs/scoop-kandev` after the GitHub
  Release and its Windows runtime checksum are available.
- The Scoop manifest uses the exact Stable version, release asset URL, and
  published SHA-256 value for `kandev-windows-x64.tar.gz`.
- The Scoop update is a sibling publication channel to npm and Homebrew.
- A Stable release is incomplete until the Scoop update succeeds and its
  manifest is correct.
- A release backfill reconciles the Scoop manifest for the selected existing
  Stable tag.
- A repeated update for the same version succeeds without an empty commit.
- A Nightly, dry run, or Desktop validation run does not change the Scoop
  bucket.
- The release workflow uses a separate repository-scoped credential for the
  Scoop bucket. It does not use the Kandev workflow token for cross-repository
  writes.

## Permissions

The release workflow reads Kandev release assets with its built-in GitHub
token. It writes only to `kdlbs/scoop-kandev` with the
`SCOOP_BUCKET_DEPLOY_KEY` repository secret. The matching public deploy key has
write access only to the Scoop bucket repository.

## Failure modes

- If the Windows archive or checksum is absent, the Scoop update stops before
  it changes the bucket.
- If the checksum content is invalid, the Scoop update stops before it changes
  the bucket.
- If the deploy key is absent or invalid, the Scoop publication job fails.
- If the bucket clone, commit, or push fails, the publication job fails. The
  release workflow does not report the Stable release as complete.
- If the manifest already contains the requested version, URL, and hash, the
  publication job succeeds without a commit.
- If another process changes the bucket during publication, the push fails.
  A maintainer can rerun the latest Stable release with `backfill_tag`.

## Persistence guarantees

The generated manifest is stored as a Git commit on the Scoop bucket `main`
branch. A successful rerun does not change that commit when the manifest is
already correct.

## Scenarios

- **GIVEN** a Stable GitHub Release with a Windows runtime archive and valid
  checksum, **WHEN** the Stable release workflow publishes package-manager
  channels, **THEN** the Scoop manifest contains that version, URL, and hash.
- **GIVEN** a Stable release whose Scoop manifest is already correct, **WHEN**
  the Scoop publication job runs again, **THEN** the job succeeds without a
  new bucket commit.
- **GIVEN** a valid latest Stable tag with a stale Scoop manifest, **WHEN** a
  maintainer runs `backfill_tag`, **THEN** the manifest is reconciled to that
  tag without creating or moving a release tag.
- **GIVEN** a Nightly, dry run, or Desktop validation run, **WHEN** the release
  workflow runs, **THEN** the Scoop repository does not change.
- **GIVEN** an absent or malformed Windows checksum, **WHEN** the Scoop
  publication job runs, **THEN** the job fails before it commits a manifest.
- **GIVEN** an absent or invalid Scoop deploy key, **WHEN** the Scoop
  publication job runs, **THEN** the job fails and the Stable release remains
  incomplete.

## Out of scope

- A Scoop Nightly channel.
- Changes to the existing Scoop installation layout or launcher environment.
- Scoop support for Windows ARM64.
- Automation for Winget, Chocolatey, or other package repositories.
- A general scheduled update workflow in `kdlbs/scoop-kandev`.

## Implementation plan

See [the implementation plan](../../../plans/scoop-release-automation/plan.md).
