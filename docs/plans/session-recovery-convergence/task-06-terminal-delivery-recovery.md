---
id: "06-terminal-delivery-recovery"
title: "Reconcile projected terminal deliveries after restart"
status: completed
wave: 6
depends_on:
  - 05-legacy-resume-controls
plan: plan.md
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-008
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.8
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-008.5
system_design:
  - ../../specs/platform/system-design/session-recovery-convergence.md
---

# Task 06: Projected terminal delivery recovery

## Evidence and implementation

Two native-restored sessions retained a generic empty delivery fence after a
journal-capacity failure. SQL had a projected failed terminal and matching
receipt/projection cursors. The journal retained the same terminal sequence,
but restart classified its dispatching submission as interrupted unknown.
Terminal error append, unlike completion append, did not settle dispatching
atomically. Explicit Resume rejected terminal SQL before authenticated recovery.

Settle dispatching to failed in the same transaction as a terminal error event.
Preserve already recorded outcomes and nonterminal errors. For existing records,
allow authenticated retirement of an unknown journal submission only when exact
owner, hash, stream and terminal sequence match a projected terminal SQL record.
Keep both historical states and outcomes intact. Revalidate that evidence under
SQL admission locks before resolving the matching fence. Independent active
work, incomplete projection and mismatched evidence must still block recovery.
Idle native restoration and explicit Resume use the same reconciliation; neither
replays an old prompt. Investigate capacity separately without assuming small
journal files prove or disprove a physical storage shortage.

## Validation and delivery

Focused journal tests interrupt between terminal commit and dispatch return,
then reopen and classify abandoned dispatch. Lifecycle tests use a real journal
and authenticated client, including repeated retirement, terminal SQL conflicts,
projection lag, and mixed active/terminal inventory. Repository tests verify
transactional proof and preserve outcomes; orchestrator tests exercise explicit
Resume and normal admission. Run resource-capped focused Go tests and scoped
lint; browser and PostgreSQL execution stay in CI. Push/open PR promptly, fix
CI/reviews, and merge the passing head. No production mutation or local E2E.
