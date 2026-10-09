---
id: "13-prompt-submissions"
title: "Persist prompt submission outcomes"
status: complete
wave: 13
depends_on: ["12-kubernetes-journal"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.2
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
---

# Task 13: Persist prompt submission outcomes

## Summary

Make backend prompt retries idempotent without claiming exactly-once harness execution.

## In scope

- Extend queue_dispatch_claims with protocol mode and submission association. Reuse its dispatch attempt and WithSessionAdmission boundary.
- Add durable backend submission preparation and immutable payload hashing. Repair partial cross-repository preparation before any network dispatch.
- Implement authenticated admission and state-query endpoints with incarnation and owner validation.
- Commit accepted and dispatching states before harness calls. Preserve interrupted_unknown after ambiguous crashes.
- Update startup recovery to reconcile v1 claims and block legacy claims with unknown pre-crash delivery.
- Keep queue edits, transfers, and acceptedPromptDispatchError consistent with the original submission identity.
- Own agent_delivery_submissions_total and agent_delivery_duplicate_submissions_total at agentctl admission.
- Connect snapshot delivery checkpoints to submission identity. Fence terminal outcomes and cancellation races.

## Out of scope

Event replay, inbox projection, and automatic resend of uncertain work.

## Acceptance

- Same ID and hash returns the saved state without another dispatch. A hash conflict fails before dispatch.
- A process kill after dispatching cannot cause automatic resend, even if no harness response exists.
- An accepted but undispatched submission requires current-owner reconciliation before dispatch.

## Verification

Run from the repository root. New test names describe required evidence, not existing passing tests.
Use TDD for implementation. Record the failing assertion before the implementation result.

```bash
(cd apps/backend && rtk go test -tags fts5 -race ./internal/agentctl/server/api ./internal/agentctl/server/process ./internal/agentctl/journal ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle ./internal/task/repository/... ./internal/orchestrator/... -count=1)
(cd apps/backend && rtk go run ./cmd/sqlguard ./internal)
(cd apps/backend && rtk go test -tags fts5 -race ./internal/persistence ./internal/persistence/storeconformance -count=1)
rtk make -C apps/backend lint
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
```

Target evidence in `apps/backend/internal/agentctl/server/process/submission_delivery_test.go`:

- `TestSubmissionRetryDispatchesOnce`: `AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.1`.
- `TestSubmissionCrashWindowAndHashConflict`: `AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.2`.

Run conformance with a provisioned PostgreSQL test database and `KANDEV_TEST_POSTGRES_DSN` set.
A skipped PostgreSQL suite does not satisfy acceptance.
Include fresh, migration replay, and previous-stable upgrade results.

Add `TestPendingQueueDispatchReconcilesOriginalSubmission` and `TestLegacyUnknownDispatchRequiresRecovery` to `queue_dispatch_recovery_test.go`.
Extend transfer, edit-save, Send Now, and accepted-dispatch-error regressions.

## Files likely touched

- `apps/backend/internal/agentctl/server/api/agent.go`
- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/journal/` (new in Task 08).
- `apps/backend/internal/agent/runtime/agentctl/agent.go`
- `apps/backend/internal/agent/runtime/lifecycle/session.go`
- `apps/backend/internal/task/repository/sqlite/`
- `apps/backend/internal/orchestrator/messagequeue/repository_dispatch_recovery.go`
- `apps/backend/internal/orchestrator/messagequeue/service.go`
- `apps/backend/internal/orchestrator/queue_dispatch_recovery.go`
- `apps/backend/internal/orchestrator/queue_dispatch_recovery_test.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/agentctl/server/process/submission_delivery_test.go` (new tests or extensions).

## Dependencies

[Task 12](task-12-kubernetes-journal.md).

## Risks

- A durable dispatching marker precedes the external call. That unavoidable crash window must remain explicitly uncertain.

## Parallelism

`sequential`

The primary session owns integration. This work order does not authorize subagents.
Preserve existing user edits and unrelated changes.

## Inputs

- [Owned system design](../../specs/platform/system-design/durable-agent-delivery.md).
- [Package manifest](plan.md), including shared regression gates and test prerequisites.
- Existing source and adjacent tests in the listed files.
- [Boundary decision](../../decisions/2026-09-10-durable-harness-session-boundaries.md).

## Results

Implemented immutable backend and agentctl prompt admission, stable submission identity and
payload hashes, interrupted-unknown blocking, queue and Send Now association, response identity
propagation, and bounded payloads. Backend-owned dispatch reconciliation and legacy claim fences
are wired through the queue recovery path. Focused race tests, SQL guard, persistence conformance,
lint, and schema upgrade checks pass. PostgreSQL and live executor replacement evidence remain
environment-dependent.
