---
created: 2026-10-12
status: in_progress
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-001
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-002
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
  - ../../specs/platform/system-design/session-recovery-convergence.md
legacy_specs: []
---

# Implementation plan: Session recovery convergence

## Outcome and ownership

This original task/session solely owns permanent recovery through merge. No
parallel tasks, delegation, new sessions, local E2E or production mutation.
PR #4425 merged as 179fc69cc6838878c6db96f8743a42f9bb0041fc; its receipt/gap
correction remains the base. This follow-up closes producer shutdown ordering
and completes evidence-based native retirement and admission convergence.

## Confirmed reproduction

TestSubmissionGracefulStopPreservesUnknownRequestOutcome starts a real child,
admits an immutable prompt, invokes real Stop while RPC is active, and releases
a typed RequestError from adapter Close. Before the fix it expected unknown but
observed failed, without retained terminal. Existing process-manager code closed
stopCh after Close; dispatch used Background and classified every RequestError
as known. Private incident recovery also required filling an empty bound stream.
No private payloads or live native conversations belong in fixtures.

## Sequence

1. [Producer settlement](task-01-shutdown-settlement.md).
2. [Retirement and fence convergence](task-02-native-convergence.md).
3. [State/boundary contract and delivery](task-03-recovery-contract.md).
4. [Prepared prompt admission](task-04-prepared-prompt-admission.md).

Each work order is executed here sequentially. Native restoration and retirement
are never prompt authority. Historical labels, hashes, cursors and independent
fences remain evidence. Unknown ownership and true missing dependencies may
remain blocked with a specific supported action; they are not fabricated away.

## Validation and delivery

Focused Go tests use GOMEMLIMIT=2048MiB GOMAXPROCS=2, -p 1, -race, -trimpath,
-tags fts5; only one heavy local operation runs at once. Required PostgreSQL and
all desktop/phone browser suites run in CI. Push and open a follow-up PR promptly,
address every actionable CI/review finding, and merge the actual passing head.
No completion claim follows merely from recovering the four observed records.

## Results

PR #4432 implements all three work orders. Focused producer, journal, SQL,
lifecycle and orchestrator race checks pass, including the 49-state-pair matrix,
legacy shutdown history, prepared checkpoints, missing/stale fences and repeated
recovery. The four established recovery regressions reported by CI now pass
after separating stored-record inspection from retired-record proof.
All eight review threads were addressed and resolved. All 55 final-head CI
checks passed, and the PR merged. No local E2E ran. Task 04 below remains open.

PR #4432 merged as bc0d3b9047 after all 55 CI checks passed and all review
threads were resolved. A subsequent other-machine fresh-task report confirmed
attachments were present. Task 04 repairs a separately reproduced pre-dispatch
payload divergence: SQL admission occurred before attachment materialization.
