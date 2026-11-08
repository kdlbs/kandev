---
id: 02-graphql-evidence
title: Classify structured GraphQL errors
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-INTEGRATIONS-GITHUB-RATE-001
  - REQ-INTEGRATIONS-GITHUB-RATE-004
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-RATE-001.1
  - AC-INTEGRATIONS-GITHUB-RATE-001.2
  - AC-INTEGRATIONS-GITHUB-RATE-001.4
  - AC-INTEGRATIONS-GITHUB-RATE-001.6
  - AC-INTEGRATIONS-GITHUB-RATE-004.3
system_design:
  - ../../specs/integrations/system-design/github-rate-limit-coordination.md
---

# Task 02: Classify structured GraphQL errors

## Summary

Use GraphQL `errors[]` as payload error evidence. Successful data containing
rate-related words must decode normally and must not establish a throttle.

## Confirmed root cause

`secondaryRateLimitResponse` scans the whole body for `rate_limited` at any
status. CLI ExecuteGraphQL invokes this classifier on successful stdout.
The existing fake-gh fixture reproduces the failure with a PR title.

## In scope

Add `TestGraphQLSuccessfulDataDoesNotThrottle` through `executeGraphQLPayload`
for PAT and CLI. Include absent/empty errors, unrelated errors plus rate-like data,
healthy/zero remaining, and malformed payload behavior. Keep the existing tests
for typed/message-only rate errors, partial data, and redacted failures.

## Out of scope

New UI, persistence changes, external publication, and unrelated CI failures.

## Acceptance

- PAT and CLI both accept successful payloads containing rate-related titles and text.
- Real typed and message-only GraphQL errors retain primary/secondary classification and retry details.
- Successful zero-remaining responses stay successful while updating quota observations.

## Verification

Run from the repository root. First add the named regression and record its
expected failure. Apply the correction, then run:

```bash
(cd apps/backend && go test -trimpath -race ./internal/github -run 'Test.*(GraphQL|Classif|GitHubAPIError|Stderr)' -count=1 -timeout=5m)
git diff --check
```

## Files likely touched

- `apps/backend/internal/github/rate_error.go`
- `apps/backend/internal/github/graphql.go`
- `apps/backend/internal/github/graphql_rate_error_test.go`
- `apps/backend/internal/github/rate_error_test.go`

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

RED: successful CLI data produced a secondary-rate error in all three cases.
GREEN: the specified GraphQL/classification race command passed (2.553s).
