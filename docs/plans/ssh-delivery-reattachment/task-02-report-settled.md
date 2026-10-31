---
id: "02-report-settled"
title: "Report committed terminal settlement on the first Retry"
status: done
wave: 2
depends_on:
  - "01-restore-transport"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.16
system_design:
  - ../../specs/platform/system-design/interrupted-prompt-recovery.md
---

# Task 02: Report committed terminal settlement on the first Retry

## Summary

Return settled from the first Retry when the same submission's authoritative terminal state committed during runtime reconciliation. Preserve all original ownership and independent-block checks.

## In scope

A fresh canonical evidence check after runtime recovery, deterministic real-SQL race regression, negative fencing tests, and new isolated macOS SSH fault acceptance.

## Out of scope

Installed Kandev restarts or updates, changes to existing sessions, prompt replay, new conversations, new UI, and broader recovery policy changes.

## Acceptance

1. A terminal event committed during runtime recovery produces settled with the current recovery revision from that same Retry, even if the runtime returns owner_mismatch.
2. Changed ownership/generation, missing terminal proof or an independent recovery block remains blocked, without prompt dispatch or unrelated mutation.
3. A new isolated SSH task recovers on its first Retry after a 15-second transport outage, preserves native conversation and command counts, and leaves all existing sessions and the installed backend intact.

## Verification

From repository root:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator -run 'Test.*RetrySessionDelivery' -count=1)
(cd apps/backend && go test -race -trimpath -tags fts5 ./internal/orchestrator -run 'Test.*RetrySessionDelivery' -count=1)
```

Run `scripts/lint-go-changed` for the changed orchestrator files, specification catalog and lint checks, and diff whitespace checks. Build a separately named binary, use a fresh home/database/port and a new task, inject an exact-connection 15-second fault, and record the first Retry response, native/process identity, exactly one BEGIN/END, follow-up memory, existing-session comparison and cleanup. Browser rendering is unchanged and not part of this acceptance.

## Files likely touched

- `apps/backend/internal/orchestrator/session_delivery_recovery.go`
- New focused settlement response helper and tests next to that file.
- `apps/backend/internal/orchestrator/session_delivery_terminal_retry_test.go` for shared real-SQL setup.

## Dependencies

Task 01, completed.

## Risks

Converting a real owner change to success, accepting metadata without terminal proof, and hiding independent restrictions.

## Parallelism

sequential

## Inputs

Scoped requirements, the interrupted-prompt recovery design, and the preserved first-response timing evidence from Task 01.

## Results

Completed on 2026-10-31. The real-SQL regression first returned blocked despite committing the same submission's terminal event during the runtime callback. The repaired path returns settled with the canonical post-settlement revision.

- Exact work-order regression command passed, 2.467 s.
- The same Retry scope with `-race` passed, 4.491 s.
- Changed-package Go lint passed with zero issues. Go vet passed.
- Specification catalog validated 399 decisions and 1718 specifications; 36 linter self-tests, full specification lint and diff whitespace checks passed.
- Negative tests preserved no-terminal-proof, independent block, new submission, new generation and canonical execution replacement cases. Unreadable canonical evidence returned an error rather than success. No prompt dispatch occurred.
- Real macOS SSH acceptance started with the target SQL submission interrupted_unknown and terminal sequence zero. After an abrupt disconnect and 15-second refusal, its first explicit Retry returned settled at 2026-10-31T09:56:53.947249Z, revision 3 matching canonical SQL. The runtime still returned owner_mismatch at 09:56:53.943Z, so this exercised the exact response correction rather than a pre-settled fast path.
- The same controller/native conversation/incarnation/execution survived. The original command produced one BEGIN and one END. A repeated Retry remained settled; the distinct follow-up recalled the conversation-only phrase and wrote one FOLLOWUP, without repeating the original command.
- Existing task/session identities, states and executor/profile settings matched the baseline. The original backend process stayed ready. New test tasks, relay and test services were stopped; a scoped remote working-directory scan found no residual test runtimes.

The source and tested binary remain independent of the installed app. No existing-session control or database override was performed. Browser rendering was not separately tested.
