# ADR-2026-08-30: Cache host-runner E2E browser provisioning

**Status:** accepted
**Date:** 2026-08-30
**Area:** infra, workflow

## Context

The container-backed E2E jobs run on host runners because they need the Docker
daemon. Each job currently pulls the runtime image and copies its baked
Playwright browsers from `/ms-playwright` to `/tmp/ms-playwright`. Recent job
logs show about 54 seconds of browser provisioning per container shard. The
same setup is repeated across all six parallel jobs, even though the browser
source is identical.

The runtime image remains the reviewed source of the browser binaries. A cache
must reduce repeat work without allowing an unavailable or stale cache to
change test selection or block a correct E2E run.

## Decision

The host-runner container jobs use a GitHub Actions cache for the Playwright
browser directory. The workflow resolves the `runtime-latest` convenience tag
to a sha256 digest before it constructs the cache key. The key includes the
runner operating system, the Playwright browser source, the resolved image
digest, and a unique workflow run ID and attempt. A stable digest-scoped
restore prefix selects the newest compatible entry.

The job restores the cache before browser provisioning and verifies Chromium
from the restored path. A verified exact or prefix hit skips the image pull and
browser copy. A miss, stale key, cache-service error, or failed verification
uses the digest-pinned runtime-image extraction and smoke check. A successful
fallback populates a new run-specific cache key, so a failed cache entry cannot
remain the newest exact candidate forever. If the image digest cannot be
resolved, the job fails before using the mutable tag. Cache restore and save
are best-effort acceleration steps and are reported separately from test
correctness.

The cache contains browser binaries only. It does not contain source,
credentials, Docker state, or test artifacts.

## Consequences

- Warm container jobs avoid the repeated browser copy and usually start their
  tests sooner.
- The first run after a browser-source change still pays the current setup
  cost while creating a new cache entry.
- Cache service incidents do not block E2E when the pinned-image fallback is
  healthy.
- Updating the runtime image digest invalidates the cache, including changes
  that do not affect browsers. This is conservative and binds the cache to the
  exact fallback image.
- The workflow must report cache state and provisioning duration so a cache
  hit rate and actual wall-time reduction can be evaluated after merge.

## Alternatives considered

### Keep extracting browsers on every host runner

Rejected. It has no cache failure mode, but repeats a measured setup cost in
every container shard and leaves an avoidable critical-path expense.

### Upload browsers as a workflow artifact from the build job

Rejected for this change. It would add a producer and consumer artifact path
to every run, increase coupling to the build job, and still require transfer
before the container cohort starts. The cache can be warmed lazily and uses
the same explicit source key.

### Build a host image with browsers already installed

Deferred. Host-image ownership would couple the Docker executor to runner
provisioning and would not help arbitrary GitHub-hosted runners. Revisit only
if cache hit rates or cache transfer time do not produce a measured gain.
