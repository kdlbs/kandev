---
id: "01-session-bound-lifecycle-visibility"
title: "Bind compact lifecycle visibility to one manual session"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-MANUAL-VISIBILITY-001
acceptance_criteria:
  - AC-COORDINATOR-MANUAL-VISIBILITY-001.1
  - AC-COORDINATOR-MANUAL-VISIBILITY-001.2
  - AC-COORDINATOR-MANUAL-VISIBILITY-001.3
  - AC-COORDINATOR-MANUAL-VISIBILITY-001.4
  - AC-COORDINATOR-MANUAL-VISIBILITY-001.5
system_design:
  - ../../specs/coordinator/system-design/manual-task-visibility.md
---

# Task 01: Session-bound lifecycle visibility

## Scope

- Persist an exact manual task/session/workspace grant and check it at profile
  discovery and dispatch.
- Expose only the compact task-visibility read; derive caller identity from the
  backend session and refuse foreign or unsupported reads.
- Maintain durable lifecycle receipts atomically with task transitions,
  including deletion, with SQLite and PostgreSQL parity.

## Exclusions

No managed-coordinator tool expansion, task relation or plugin read, general
manual-task grant, task content, or write operation. Activating the grant for a
particular deployed session is a separate operation.

## Acceptance

1. A matching persisted grant permits one same-workspace compact read; every
   absent or mismatched authority denies it.
2. The result and lifecycle receipts obey the five acceptance criteria without
   leaking foreign data or advancing on failed/replayed transitions.
3. SQLite and native PostgreSQL behavior matrices pass, and independent review
   and QA cover the final candidate.

## Verification

From `apps/backend`, run the focused manual visibility tests and receipt
matrices in `internal/task/repository/sqlite`, `internal/task/service`, and
`internal/mcp/server`. Repeat the `TestPostgres*LifecycleReceipt*`,
`TestPostgres*TaskVisibility*`, and PostgreSQL MCP handler cases with a fresh
task-owned PostgreSQL 16 DSN. Run `python3 scripts/list-docs.py validate`,
`python3 scripts/lint-spec-files.py --all`, and the PR documentation coverage
check from the repository root. Record real execution results separately from
this plan.

## Likely files and dependencies

The implementation spans `apps/backend/internal/task/models`,
`internal/task/repository/sqlite`, `internal/task/service`, `internal/mcp`,
`internal/orchestrator/executor`, and the paired public/decision documents.
Native PostgreSQL verification depends on a qualified isolated task fixture;
it must not borrow another task's database or host Docker access.

## Results

The published candidate includes the grant, compact read, receipts, and
regression tests. Focused SQLite verification passed. Native PostgreSQL and
final exact-head QA are pending the task-owned fixture. This work order was
added to satisfy PR documentation traceability after the scope correction.
