---
id: "01-clarification-admission-and-recovery"
title: "Persist detached-answer delivery and expose interrupted-work recovery"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-006
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.2
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-006.2
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
  - ../../specs/agents/system-design/harness-session-continuity.md
---

# Task 01: Durable clarification admission and recovery

## Summary

Give each detached-answer dispatch attempt a backend-owned identity, shared by
SQL and agentctl. Return the existing bounded history-continuation action when
unresolved durable work prevents a session launch or recovery.

## Scope

- Synchronous answers, rejections, and legacy event/watchdog fallback share
  durable admission before dispatch.
- Retain the exact accepted prompt payload and existing clarification claim,
  reserved-turn and asynchronous acceptance semantics.
- Surface only the allowlisted interrupted-work reason and existing action;
  internal recovery blocks and journal contents remain server-side.

## Exclusions

Automatic replay, conversation replacement, live database repair, schema changes,
new recovery actions, and frontend rendering changes.

## Acceptance

1. SQL contains the same immutable prompt identity and payload agentctl receives
   before dispatch; asynchronous acceptance does not settle it as completed.
2. An unresolved peer blocks another clarification prompt before provider dispatch.
3. Session launch/recovery returns the existing explicit history-continuation
   contract for this bounded reason while unrelated errors remain excluded.

## Likely files

- `apps/backend/internal/orchestrator/event_handlers_clarification.go`
- `apps/backend/internal/orchestrator/event_handlers_clarification_delivery_test.go`
- `apps/backend/internal/orchestrator/handlers/handlers.go`
- `apps/backend/internal/orchestrator/handlers/handlers_durable_recovery_test.go`

## Verification

Run from `apps/backend`:

```bash
go test -trimpath -race ./internal/orchestrator ./internal/orchestrator/handlers -run 'TestDetachedClarification|TestResumeDetachedClarification|TestRestoreRequiredRecoveryResponse|TestWSLaunchSession_DurableRecovery' -count=1
golangci-lint run ./internal/orchestrator/... --new-from-rev=770ba303fd07f26edab2f7edb84ad62684111813 --timeout=5m
```

Run `python3 scripts/list-docs.py validate` and `git diff --check` from the root.

## Dependencies and risks

Uses the existing durable delivery admission, clarification claim authority and
explicit generation transition. Preserve the distinction between dispatch
acceptance and completion. Unknown results cannot authorize automatic resend.

## Results

- Observed the missing submission identity, unresolved-peer admission and
  generic launch error before changing production logic.
- Targeted orchestrator and handler tests passed with `-trimpath -race`.
- Changed-code Go lint passed with zero issues.
- Documentation coverage, catalog validation and `git diff --check` passed.
- New tests cover answer, rejection and legacy fallback admission, asynchronous
  acceptance without terminal settlement, unresolved-work blocking, bounded
  recovery details and launch behavior that leaves explicit recovery pending.
