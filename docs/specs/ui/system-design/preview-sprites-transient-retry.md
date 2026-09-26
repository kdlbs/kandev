---
status: current
system: ui
created: 2026-09-21
requirements:
  - REQ-UI-PREVIEW-SPRITES-TRANSIENT-RETRY-001
---

# Preview Sprites transient retry system design

## Purpose and boundaries

The preview CLI owns the Sprites control-plane calls used to deploy a pull
request preview. It retries recoverable control-plane failures inside the CLI,
so a transient provider failure does not repeat the frontend build or the whole
GitHub Actions job. Bundle creation, service commands, health checks, and
GitHub description writes remain outside this retry boundary.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| REQ-UI-PREVIEW-SPRITES-TRANSIENT-RETRY-001 | Retry policy and deployment flow |

## Components and responsibilities

- `apps/backend/cmd/preview/sprite_ops.go` classifies transient errors and
  provides bounded, context-aware control-plane retries. It recognizes
  transport timeouts, temporary network errors, HTTP 429, and HTTP 5xx errors.
- `retrySpriteControl` creates a fresh two-minute operation context for each
  attempt, emits the operation and backoff to stderr, honors a positive Sprites
  SDK `Retry-After` value up to the 30-second retry-delay cap, and stops after
  the shared control retry budget.
- `apps/backend/cmd/preview/deploy.go` uses the helper for the idempotent public
  URL update and for the sprite URL lookup that follows it.

## Retry policy and deployment flow

The deploy command first uploads and extracts the bundle. It then updates the
deterministic PR sprite's URL settings to public mode. A transient failure in
that update is retried before deployment proceeds. After a successful update,
the CLI retrieves the sprite URL through the same policy. Repeating the URL
update is safe because the requested state is the same public setting.

Each operation receives at most three attempts. Exponential backoff starts at
700 milliseconds and doubles between attempts. A positive provider
`Retry-After` delay replaces the calculated backoff, including when it is
shorter, and is capped at 30 seconds. Parent cancellation stops the backoff
and the operation. Permanent client errors return immediately with the
provider error.

## Failure and recovery behavior

If all attempts fail, the final Sprites error is returned to the deploy command
and the job fails with the operation context in its diagnostic. The CLI never
rebuilds the artifact, repeats the upload, or retries the GitHub workflow as a
consequence of a control-plane retry.

## Verification

The preview package tests use an HTTP test server with the real Sprites SDK to
verify transient update and lookup recovery, retry-budget exhaustion, permanent
update failure without an extra request, bounded `Retry-After` handling, and
the existing get-or-create retry behavior.
