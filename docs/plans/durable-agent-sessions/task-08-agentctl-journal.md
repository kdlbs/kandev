---
id: "08-agentctl-journal"
title: "Add the agentctl delivery journal"
status: complete
wave: 8
depends_on: ["07-automation-recovery"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.2
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
---

# Task 08: Add the agentctl delivery journal

## Summary

Add a bounded transactional journal with crash recovery, explicit health, and CGO-free builds.

## In scope

- Pin bbolt and implement schema, stream sequence, atomic event batches, submission records, and retained identity tombstones.
- Add exclusive ownership, file permissions, logical quotas, reserved control space, and idle compaction.
- Own agent_delivery_journal_bytes, agent_delivery_unacknowledged_bytes, and agent_delivery_journal_errors_total with the shared delivery metrics contract.
- Use subprocess kill tests to cover committed recovery and interrupted writes. Keep corruption intact rather than replacing the journal.

## Out of scope

Production executor paths, network replay, and prompt admission wiring.

## Acceptance

- Committed records survive process termination. Uncommitted records never appear as acknowledged data.
- Full, corrupt, locked, or newer-format stores fail closed without silent record loss.
- Native and remote agentctl builds remain compatible with CGO-disabled targets.

## Verification

Run from the repository root. New test names describe required evidence, not existing passing tests.
Use TDD for implementation. Record the failing assertion before the implementation result.

```bash
(cd apps/backend && rtk go test -race ./internal/agentctl/journal -count=1)
rtk make -C apps/backend build-agentctl build-agentctl-remote
rtk make -C apps/backend lint
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
```

Target evidence in `apps/backend/internal/agentctl/journal/journal_test.go`:

- `TestJournalCommittedRecordsSurviveKill`: `AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.1`.
- `TestJournalStorageFailuresFailClosed`: `AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.2`.

## Files likely touched

- `apps/backend/internal/agentctl/journal/ (new)`
- `apps/backend/go.mod`
- `apps/backend/go.sum`
- `apps/backend/Makefile`
- `AGENTS.md` (journal metric producer boundaries).
- `apps/backend/internal/agentctl/journal/journal_test.go` (new tests or extensions).

## Dependencies

[Task 07](task-07-automation-recovery.md).

## Risks

- Logical quota does not cap bbolt file size. Compaction must have a crash-safe replacement and disk-space preflight.

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

Implemented the versioned bbolt journal, sequence/cursor fencing, ownership checks, logical event/submission limits, metrics, corruption handling, and crash/lifetime tests. Focused journal suites pass.
