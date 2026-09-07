---
id: "02-bound-clarification-response-resolution"
title: "Bound clarification response resolution"
status: done
wave: 2
depends_on:
  - "01-index-pending-id-bundle-access"
plan: "plan.md"
spec: "../../specs/tasks/requirements/clarification-response-reliability.md"
---

# Task 02: Bound clarification response resolution

## Outcome

The backend stops pre-claim clarification work after five seconds. It returns
an explicit retryable result and preserves all post-claim recovery guarantees.

## In scope

- Apply one fresh five-second context to identity resolution, authorization
  inputs, validation, and the atomic bundle claim.
- Introduce a typed resolver classification for internal pre-claim budget
  exhaustion that remains distinguishable from caller cancellation.
- Map that classification to HTTP 503 and code `temporarily_unavailable`.
- Prove timeout/rollback leaves the bundle pending and performs no live or
  detached delivery.
- Preserve 200 winner reconstruction, 400 validation, 404 authorization-safe
  not-found, 409 `not_active`, and unexpected 500 behavior.
- Add phase-duration structured logs and a low-cardinality timeout counter.

## Exclusions

- No change to the existing 30-second post-claim delivery budget.
- No weakening of response-delivery intents, detached-resume reservations,
  compensation, startup recovery, or current-turn authority.
- No retry loop inside the backend handler.

## Traceability

- `REQ-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001`
- `AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001.1`
- `AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001.2`
- `AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001.3`
- `docs/specs/tasks/system-design/clarification-response-reliability.md`
- `docs/specs/tasks/system-design/clarification-active-lifecycle.md`

## Implementation acceptance

- A blocked identity read or claim returns the typed retryable outcome within
  the pre-claim budget. It writes no terminal state and invokes no delivery
  seam.
- The HTTP route returns status 503 with machine code
  `temporarily_unavailable` only for internal pre-claim exhaustion. Existing
  route outcomes remain compatible.
- Timeout metrics use phase-only labels. Structured logs include duration and
  outcome. Focused race tests preserve post-claim ownership.

## TDD sequence

1. Add channel-controlled resolver tests for blocked identity and claim work.
   assert deadline presence, no delivery, and pending state before adding the
   new classification.
2. Add handler tests for the 503 envelope, caller cancellation, and unchanged
   409/500 mappings.
3. Add metrics/log tests with low-cardinality labels.
4. Implement the shared pre-claim context, typed error mapping, and telemetry.
5. Run existing delivery/recovery tests under the race detector before
   refactoring.

## Likely files

- `apps/backend/internal/clarification/resolver.go`
- `apps/backend/internal/clarification/handlers.go`
- `apps/backend/internal/clarification/metrics.go`
- `apps/backend/internal/clarification/resolver_delivery_test.go`
- `apps/backend/internal/clarification/handlers_conflict_test.go`
- `apps/backend/internal/clarification/handlers_stub_test.go`
- `apps/backend/internal/clarification/metrics_test.go`

## Dependencies

- Task 01 must land first so healthy pre-claim work has the required indexed
  access path before the new deadline becomes a user-visible contract.

## Verification

- `cd apps/backend && go test ./internal/clarification -run 'PreClaim|TemporarilyUnavailable|Timeout|Resolution' -count=1`
- `cd apps/backend && go test -race ./internal/clarification -count=1`

## Results

Implemented the shared five-second pre-claim context, typed internal timeout
classification, HTTP 503 `temporarily_unavailable` response, and phase
telemetry. Controlled identity/claim tests prove the pending bundle and
delivery boundary; handler tests preserve caller-cancellation and existing
conflict behavior. `go test ./internal/clarification -count=1` passed 122
tests and the focused pre-claim/timeout suite passed. The PostgreSQL test path
remains environment-gated and was skipped without a configured DSN.
