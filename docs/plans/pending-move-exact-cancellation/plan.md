---
requirements:
  - REQ-TASKS-PENDING-MOVE-CANCELLATION-001
  - REQ-TASKS-PENDING-MOVE-CANCELLATION-002
  - REQ-TASKS-PENDING-MOVE-CANCELLATION-003
  - REQ-TASKS-PENDING-MOVE-CANCELLATION-004
system_design:
  - ../../specs/tasks/system-design/pending-move-exact-cancellation.md
created: 2026-09-14
status: integration-required
---

# Implementation Plan: Exact Pending-Move Cancellation

## Outcome

Give the automation Coordinator one administrative cancellation and one
read-only census for deferred task moves. The exact cancellation compares the
complete pending-move tuple inside one repository transaction, deletes exactly
one row generation when every predicate matches, and collapses every denial into
one non-leaking result with an atomic audit record. The census never mutates
state. Ordinary-agent self-only move behavior is unchanged.

## Requirements and design

- Requirements: `docs/specs/tasks/requirements/pending-move-exact-cancellation.md`
  (REQ-TASKS-PENDING-MOVE-CANCELLATION-001 through -004).
- System design: `docs/specs/tasks/system-design/pending-move-exact-cancellation.md`.
- Decision: `docs/decisions/2026-08-30-exact-pending-move-cancellation.md`.

## Work packages

| Order | Work order | Outcome |
| ----- | ---------- | ------- |
| 1 | [task-01](task-01-exact-cancel-and-census.md) | Exact compare-and-delete, non-mutating census, Coordinator MCP catalog wiring, persistence and grant migration |

No frontend work package exists: both surfaces are backend MCP tools and the
diff has no UI-visible change.

## Dependency order

task-01 is the single work package. It is integrable on its own; no intra-plan
dependencies exist.

## Risks

- Exact cancellation serializes with the lifecycle refresh/stop persistence
  paths; the work order's verification includes the race-enabled focused suites
  that cover concurrent cancellers, successor-generation survival, retry
  idempotence, and terminal status CAS.
- Main advanced while this branch was open and reshaped messagequeue
  persistence (one-shot step-entry move overrides); the work order directs
  implementation against the current merged tree and its `PendingMove`/
  `pending_moves` seams.

## Verification strategy

Each work order owns its focused commands. The plan-level gate is:

    cd apps/backend
    go test -count=1 ./internal/backendapp

plus `python3 scripts/list-docs.py validate` and
`python3 scripts/lint-spec-files.py --all` at the repository root after any
specification-adjacent documentation change.

## Results

- task-01: implemented and verified. Focused race-enabled suites pass for
  messagequeue, MCP handlers/server/scope, SQLite task repository, and
  lifecycle; twenty repeated race-enabled runs cover the adversarial
  cancellation/retry/CAS cases; backend composition tests pass. See the work
  order for the exact commands and remaining notes.
