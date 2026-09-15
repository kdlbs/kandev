---
id: "05-quota-and-pruning"
title: "Enforce journal accounting and terminal reserve"
status: done
wave: 5
depends_on: ["04-journal-batching"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-002
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.4
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
---

# Task 05: Enforce journal accounting and terminal reserve

## Summary

Enforce journal accounting and terminal reserve. Preserve the existing durable identity and admission boundaries.

## In scope

Count retained event records and submission payloads/metadata against logical quotas. Account for replacements and duplicate writes without double counting.
Rebuild or migrate counters for existing journals before admitting writes. Never trust old event-only counters as complete usage.
Replace cursor deletion with a traversal safe under bucket mutation. Test multiple leaf pages and already materialized nodes.
Use reserved capacity only for bounded health and terminal metadata; ordinary output and new submissions cannot consume it.
On capacity exhaustion, refuse admission, request cancellation of the exact active owner, and expose an out-of-band typed health failure.
If terminal persistence fails, retain uncertainty. A cancellation request is not proof of terminal execution.
Make health/status and Stop available after a writer failure. Do not downgrade or erase the journal.
Preserve exclusive ownership and idle compaction preflight. Distinguish logical usage from physical bbolt size.

## Out of scope

Other work orders, automatic prompt resend, native conversation replacement, and unrelated cleanup.

## Acceptance

- Submission-heavy storage reaches its quota; duplicates do not inflate accounting and pruning preserves all unacknowledged records.
- Ordinary capacity exhaustion leaves terminal/health reserve usable; exhausted reserve remains visible and blocks unsafe admission.
- Existing journals reopen with correct accounting; interrupted migration preserves records and fails closed.

## Regression evidence

Add TestJournalSubmissionQuota, TestJournalPruneMaterializedPages, TestJournalTerminalReserve, and TestJournalAccountingUpgrade. Inject quota and I/O failures without filling the host filesystem.

## Verification

Run from the repository root after implementation.

```bash
(cd apps/backend && go test -race ./internal/agentctl/journal ./internal/agentctl/server/process ./internal/agentctl/server/api -count=1)
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
- `apps/backend/internal/agentctl/server/api/durable_delivery.go`

Add named regression files beside the relevant production package.

## Dependencies

Task 04: Commit bounded journal batches before publication.

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

Implemented logical journal accounting for submissions and events, reserved
terminal capacity, migration repair, safe payload pruning, and stream submission
limits.

Validation passed:

- `(cd apps/backend && go test -race ./internal/agentctl/journal ./internal/agentctl/server/process ./internal/agentctl/server/api -count=1)`
- `make -C apps/backend lint`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`
