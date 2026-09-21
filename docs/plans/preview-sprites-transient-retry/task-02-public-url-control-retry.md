---
id: "02-public-url-control-retry"
title: "Retry transient public URL control calls"
status: done
wave: 1
depends_on:
  - "01-sprites-control-plane-retry"
plan: "plan.md"
requirements:
  - REQ-UI-PREVIEW-SPRITES-TRANSIENT-RETRY-001
acceptance_criteria:
  - AC-UI-PREVIEW-SPRITES-TRANSIENT-RETRY-001.2
  - AC-UI-PREVIEW-SPRITES-TRANSIENT-RETRY-001.4
  - AC-UI-PREVIEW-SPRITES-TRANSIENT-RETRY-001.8
  - AC-UI-PREVIEW-SPRITES-TRANSIENT-RETRY-001.9
  - AC-UI-PREVIEW-SPRITES-TRANSIENT-RETRY-001.10
system_design:
  - ../../specs/ui/system-design/preview-sprites-transient-retry.md
---

# Task 02: Retry transient public URL control calls

## Summary

Extend the existing Sprites control-plane retry policy to the idempotent public
URL update and the sprite URL lookup that follows it. Keep permanent client
errors fail-fast and leave build, upload, service, health, and GitHub metadata
operations outside this retry boundary.

## Acceptance

- A transient 500 or 502 during public URL configuration or URL lookup retries
  and then returns the preview URL without rebuilding the artifact.
- A permanent 401 during public URL configuration makes one request, returns
  the provider error, and does not request the sprite URL.
- The focused preview package test passes with race detection.

## Verification

```bash
cd apps/backend && go test -race ./cmd/preview -count=1
```

## Files likely touched

- `apps/backend/cmd/preview/deploy.go`
- `apps/backend/cmd/preview/sprite_ops.go`
- `apps/backend/cmd/preview/sprite_ops_test.go`

## Dependencies

Task 01 provides the shared Sprites transient-error classification and backoff
policy.

## Risks

The URL update is retried only because each attempt requests the same public
state. Do not reuse this helper for non-idempotent operations without defining
their reconciliation behavior.

## Results

Implemented the shared control retry wrapper for public URL configuration and
sprite URL lookup. Added real SDK-backed HTTP regression tests for transient
500/502 recovery and permanent 401 fail-fast behavior.

Verification passed:

- `cd apps/backend && go test -race ./cmd/preview -count=1`
