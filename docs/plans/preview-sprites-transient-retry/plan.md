---
spec: docs/specs/ui/requirements/preview-sprites-transient-retry.md
created: 2026-07-31
status: complete
---

# Implementation Plan: Preview Sprites Transient Retry

## Overview

The failed preview job reached `CreateSprite` after a successful local build,
then failed on the SDK HTTP client's 30-second timeout. Add bounded,
context-aware retries inside the preview CLI's get-or-create boundary, where
the operation is safe to resume and can reconcile an ambiguous create. Keep
the GitHub Actions workflow unchanged so permanent build or credential errors
remain visible on their first attempt.

## Backend

### Sprites control-plane retry

- `apps/backend/cmd/preview/sprite_ops.go`: introduce transient-error
  classification and a bounded backoff helper. Retry discovery and creation
  with fresh per-attempt contexts; use `Retry-After` from `sprites.APIError`
  where it is supplied. Re-read the named sprite after a transient create
  error before another create request.
- Preserve the existing upload retry independently. Do not broaden retries to
  bundle build, extraction, service deployment, health checking, or GitHub
  metadata updates.

## Tests

- **What:** a transient get error retries and then returns the discovered
  sprite. **File:** `apps/backend/cmd/preview/sprite_ops_test.go`. **How:** a
  HTTP test server returns a transient failure followed by a valid SDK
  response.
- **What:** a transient create error is reconciled by a subsequent get rather
  than a second create. **File:** `apps/backend/cmd/preview/sprite_ops_test.go`.
  **How:** HTTP test server scripts get/create responses through the real SDK
  client.
- **What:** non-transient errors fail without retry, and exhausted transient
  failures return the final error. **File:**
  `apps/backend/cmd/preview/sprite_ops_test.go`. **How:** table-driven fake
  SDK-backed HTTP test cases, including typed `sprites.APIError` responses.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [task-01-sprites-control-plane-retry](task-01-sprites-control-plane-retry.md)

## Risks

- `CreateSprite` can complete remotely before its client receives a response;
  re-reading the deterministic sprite name is required before retrying it.
- Retry classification must unwrap SDK errors so wrapped timeout and API errors
  are handled correctly without masking permanent failures.
