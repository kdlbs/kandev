---
id: 01-secondary-deadlines
title: Preserve secondary refusal deadlines
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-INTEGRATIONS-GITHUB-RATE-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-RATE-001.4
  - AC-INTEGRATIONS-GITHUB-RATE-001.5
system_design:
  - ../../specs/integrations/system-design/github-rate-limit-coordination.md
---

# Task 01: Preserve secondary refusal deadlines

## Summary

Keep the longest active secondary retry boundary under the tracker mutex.
A later refusal extends it. A shorter refusal preserves its deadline and source.
Successful response clearing remains a separate operation.

## Confirmed root cause

The tracker unconditionally replaces `secondary[resource]`. Delayed overlapping
responses can therefore replace ten minutes of Retry-After with one minute.
Recreate the review sequence with `testing/synctest` and real admission.

## In scope

Add `TestSecondaryRefusalsPreserveLatestBoundary`, including both orders and
concurrent writers. After a shorter refusal, attempt admission before the longer
boundary. Assert refusal and the retained retry source. Also test expiry and
successful-response clearing.

## Out of scope

New UI, persistence changes, external publication, and unrelated CI failures.

## Acceptance

- Both response orders retain the latest deadline and its source. Equal deadlines do not weaken a provider-derived source.
- Successful-response clearing, expiry, and independent resource state still work.
- Concurrent updates pass the race detector and wake admission consumers safely.

## Verification

Run from the repository root. First add the named regression and record its
expected failure. Apply the correction, then run:

```bash
(cd apps/backend && go test -trimpath -race ./internal/github -run 'Test.*(Secondary|RateTracker|Coordinator|Admission)' -count=1 -timeout=5m)
git diff --check
```

## Files likely touched

- `apps/backend/internal/github/ratelimit.go`
- `apps/backend/internal/github/ratelimit_secondary_order_test.go`
- `apps/backend/internal/github/rate_coordinator_test.go`

## Dependencies

No implementation dependency. Follow the plan order in this single session.

## Risks

Preserve the existing admission, workspace identity, and cancellation contracts.
Use bounded deterministic fixtures and retain provider-call assertions.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/github-rate-limit-coordination.md).
- [System design](../../specs/integrations/system-design/github-rate-limit-coordination.md).
- [Plan evidence and compatible paths](plan.md).
- ADR 0047 and ADR-2026-08-29-github-provider-rate-coordination.

## Results

RED: longest-boundary and equal-deadline cases failed before the correction.
GREEN: the specified race-enabled focused test command passed (4.392s).
