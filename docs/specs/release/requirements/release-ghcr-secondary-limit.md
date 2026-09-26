---
status: active
system: release
created: 2026-08-12
owners:
  - Kandev maintainers
---
# Resilient GHCR release publishing Requirements

## Overview

Stable release jobs can hit a transient GitHub Container Registry secondary rate limit after a valid image build has completed. The release should recover from that service response without asking a maintainer to start a second version bump or blindly repeat unrelated release work.

## Requirements

### REQ-RELEASE-RELEASE-GHCR-SECONDARY-LIMIT-001: Resilient GHCR release publishing

**Intent:** Stable release jobs can hit a transient GitHub Container Registry secondary rate limit after a valid image build has completed. The release should recover from that service response without asking a maintainer to start a second version bump or blindly repeat unrelated release work.

#### Acceptance criteria

- **AC-RELEASE-RELEASE-GHCR-SECONDARY-LIMIT-001.1:** Per-architecture GHCR staging publishes retry recognized secondary-rate-limit failures with bounded exponential backoff: four total attempts with waits of 60, 120, and 240 seconds.
- **AC-RELEASE-RELEASE-GHCR-SECONDARY-LIMIT-001.2:** GHCR manifest creation and promotion retry the same recognized transient failures.
- **AC-RELEASE-RELEASE-GHCR-SECONDARY-LIMIT-001.3:** Non-rate-limit failures, including build errors and authentication or permission failures, fail immediately and preserve their original exit status.
- **AC-RELEASE-RELEASE-GHCR-SECONDARY-LIMIT-001.4:** Base-image architecture publishes run sequentially, and universal-image architecture publishes run sequentially. A manifest job runs only after both architecture jobs succeed.
- **AC-RELEASE-RELEASE-GHCR-SECONDARY-LIMIT-001.5:** A successful retry exposes the same image digest contract to downstream manifest jobs as a first-attempt success.
- **AC-RELEASE-RELEASE-GHCR-SECONDARY-LIMIT-001.6:** Every retry logs the attempt number and wait duration without printing credentials or tokens.
- **AC-RELEASE-RELEASE-GHCR-SECONDARY-LIMIT-001.7:** When all retry attempts fail, the workflow fails closed. It does not promote the affected staging image or run downstream release publication jobs.
- **AC-RELEASE-RELEASE-GHCR-SECONDARY-LIMIT-001.8:** Existing recovery semantics remain unchanged: maintainers rerun failed jobs after the throttle clears, or use `backfill_tag` for a partial release whose signed tag already exists.

## Migrated source detail

## Why

Stable release jobs can hit a transient GitHub Container Registry secondary rate limit after a valid image build has completed. The release should recover from that service response without asking a maintainer to start a second version bump or blindly repeat unrelated release work.

## What

- Per-architecture GHCR staging publishes retry recognized secondary-rate-limit failures with bounded exponential backoff: four total attempts with waits of 60, 120, and 240 seconds.
- GHCR manifest creation and promotion retry the same recognized transient failures.
- Non-rate-limit failures, including build errors and authentication or permission failures, fail immediately and preserve their original exit status.
- Base-image architecture publishes run sequentially, and universal-image architecture publishes run sequentially. A manifest job runs only after both architecture jobs succeed.
- A successful retry exposes the same image digest contract to downstream manifest jobs as a first-attempt success.
- Every retry logs the attempt number and wait duration without printing credentials or tokens.
- When all retry attempts fail, the workflow fails closed. It does not promote the affected staging image or run downstream release publication jobs.
- Existing recovery semantics remain unchanged: maintainers rerun failed jobs after the throttle clears, or use `backfill_tag` for a partial release whose signed tag already exists.

## Failure modes

- A GHCR response containing a secondary-rate-limit or equivalent throttling signal causes the bounded retry sequence.
- A persistent throttling response fails after the final attempt and blocks manifest promotion and release publication.
- A build, authentication, permission, or other non-throttling error fails without waiting through the retry sequence.
- A retry after a partially accepted staging push is safe because staging tags are unique to the workflow run and architecture.

## Scenarios

- **GIVEN** a per-architecture image command fails with a secondary-rate-limit response on its first attempt and succeeds on its second attempt, **WHEN** the release job runs, **THEN** the job waits according to the backoff policy, succeeds, and publishes the resulting digest for downstream use.
- **GIVEN** a per-architecture image command fails with a non-throttling build or permission error, **WHEN** the release job runs, **THEN** the command fails without a retry delay and the dependent manifest job does not run.
- **GIVEN** a per-architecture image command returns a secondary-rate-limit response on every attempt, **WHEN** all four attempts complete, **THEN** the job fails, no user-facing manifest is promoted, and GitHub Release, npm, and Homebrew publication do not run.
- **GIVEN** a stable release reaches the base-image stage, **WHEN** the architecture jobs publish their staging images, **THEN** only one architecture publish is active at a time and the manifest job waits for both successful digests.
- **GIVEN** a stable release reaches the universal-image stage, **WHEN** the architecture jobs publish their staging images, **THEN** only one universal architecture publish is active at a time and the promotion job waits for both successful digests.

## Out of scope

- Moving Kandev images to another registry.
- Changing GHCR credentials, package visibility, or repository permissions.
- Increasing GitHub primary API quotas or changing Docker Hub behavior.
- Changing release versioning, tag immutability, or the `backfill_tag` contract.
