---
id: "06-submission-rollover"
title: "Bound submission retention with safe idle rollover"
status: done
wave: 6
depends_on: ["05-quota-and-pruning"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-004
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.2
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
---

# Task 06: Bound submission retention with safe idle rollover

## Summary

Bound submission retention with safe idle rollover. Preserve the existing durable identity and admission boundaries.

## In scope

Enforce the existing 10000-submission stream limit. Keep accepted IDs non-executable after payload pruning.
Rollover only when the stream is idle, all outcomes are settled, and required projection/ACK evidence exists.
Seal the old stream and persist the new stream association through existing SQL generation and owner checkpoints.
Do not create a harness conversation or change its generation merely to rotate transport storage.
Retain compact tombstones and reject old IDs, stale ACKs, or dispatch attempts after rollover.
Validate stored submission session, incarnation, and generation before every retirement or sealing mutation, including the existing retirement endpoint.
Make the journal/SQL handoff restart-reconcilable with a durable intent or equivalent existing checkpoint.
Reconcile old journals already above the limit without deleting history or interrupting active work. Block new admission until safe rollover.
Version any incompatible journal representation. Document supported upgrade and rollback; an old binary must not reopen new identities for dispatch.

## Out of scope

Other work orders, automatic prompt resend, native conversation replacement, and unrelated cleanup.

## Acceptance

- The 10001st submission cannot enter the old stream; safe rollover admits a new prompt once without native session replacement.
- Crashes before and after each journal/SQL checkpoint recover one authoritative active stream.
- Old IDs remain rejected after pruning, restart, upgrade, and attempted supported rollback.

## Regression evidence

Add TestDeliveryStreamRolloverLimit, TestDeliveryStreamRolloverCrashCheckpoints, and TestDeliveryStreamRolloverRejectsOldIDs. Add TestDeliveryRetirementRejectsForeignOwner for the retirement API and process boundary. Cover mixed historical/completed/active submissions and real SQL plus journal reopening.

## Verification

Run from the repository root after implementation.

```bash
(cd apps/backend && go test -race ./internal/agentctl/journal ./internal/agentctl/server/process ./internal/agent/runtime/lifecycle ./internal/task/repository/sqlite ./internal/orchestrator -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -race ./internal/persistence/storeconformance -count=1)
make -C apps/backend lint
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Persistence gate: cover fresh boot, close/reopen, previous-format upgrade, interrupted migration, and unsupported newer formats. Run PostgreSQL conformance when SQL changes; record missing infrastructure as a release evidence gap. Never report skipped database cases as passed.

## Files likely touched

- `apps/backend/internal/agentctl/journal/journal.go`
- `apps/backend/internal/agentctl/journal/recovery.go`
- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agent/runtime/lifecycle/durable_adoption.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery.go`

Add named regression files beside the relevant production package.

## Dependencies

Task 05: Enforce journal accounting and terminal reserve.

## Risks

Partial failure must preserve ownership, original submission identity, and replay evidence. Do not clear uncertainty solely because transport reconnects.

## Parallelism

`sequential`

## Inputs

- [Plan and source evidence](plan.md).
- [Delivery requirements](../../specs/platform/requirements/durable-agent-delivery.md).
- [Delivery design](../../specs/platform/system-design/durable-agent-delivery.md).
- Existing tests beside the listed files.

## Results

Implemented authenticated idle stream rollover and submission tombstone
retirement. Rollover preserves session, incarnation, and harness ownership and
rejects unsettled submissions.

Validation passed:

- `(cd apps/backend && go test -race ./internal/agentctl/journal ./internal/agentctl/server/process ./internal/agent/runtime/lifecycle ./internal/task/repository/sqlite ./internal/orchestrator -count=1)`
- `(cd apps/backend && go run ./cmd/sqlguard ./internal)`
- `(cd apps/backend && go test -race ./internal/persistence/storeconformance -count=1)`
- `make -C apps/backend lint`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`
