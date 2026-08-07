---
status: building
created: 2026-07-31
owner: kandev
---

# Preview Sprites Transient Retry

## Why

PR preview deployment can fail when the Sprites control plane briefly times
out, even though the source build and preview configuration are valid. A
maintainer should not need to rerun a whole workflow for a recoverable API
failure.

## What

- Preview deployment retries a bounded number of transient Sprites
  get-or-create failures with visible backoff.
- A retry is limited to transport timeouts, temporary network errors, HTTP
  `429`, and HTTP `5xx` responses. Authentication, authorization, validation,
  and other client errors fail immediately.
- After a transient create failure, deployment re-reads the deterministic PR
  sprite name before another create attempt. If the first request actually
  created the sprite, deployment resumes without creating a duplicate.
- Retries respect cancellation and do not retry the complete GitHub Actions
  workflow or repeat the frontend build.

## Failure modes

- After the retry budget is exhausted, deployment fails with the last Sprites
  operation error so the GitHub Actions log remains actionable.
- A Sprites-provided `Retry-After` delay takes precedence over exponential
  backoff when present.

## Scenarios

- **GIVEN** Sprites times out while creating a preview sprite, **WHEN** the
  next control-plane attempt succeeds, **THEN** the preview deployment
  continues without a workflow rerun.
- **GIVEN** Sprites creates a preview sprite but the response times out,
  **WHEN** deployment retries, **THEN** it finds and uses the existing named
  sprite instead of issuing another create.
- **GIVEN** Sprites rejects a create request with an authentication or other
  non-retryable client error, **WHEN** deployment receives it, **THEN** the
  deployment fails immediately.
- **GIVEN** all transient control-plane attempts fail, **WHEN** the retry
  budget is exhausted, **THEN** the deployment fails with the final error.

## Out of scope

- Retrying frontend builds, bundle packaging, or GitHub PR-description writes.
- Changing Sprites account quotas or retrying preview cleanup.
- Adding a GitHub Actions job-level retry wrapper.
