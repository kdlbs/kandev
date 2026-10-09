---
created: 2026-09-26
status: planned
requirements:
  - REQ-OFFICE-GATE-COMMENT-001
  - REQ-OFFICE-GATE-COMMENT-002
  - REQ-OFFICE-GATE-COMMENT-003
  - REQ-OFFICE-GATE-COMMENT-004
  - REQ-OFFICE-GATE-COMMENT-005
system_design:
  - ../../specs/office/system-design/gate-comment-wake-01.md
legacy_specs: []
---

# Implementation Plan: Office Gate Comment Wake

## Overview

A comment on an Office card at `review` or `approval` wakes the task's runner
today, not the seated reviewer or approver whose verdict could move the card. The
scheduled coordinator's only lever on a parked gate is therefore dead. This
package makes the gate's `on_comment` fan out to the role's undecided seats,
never to the comment's author, with a prompt that asks for a verdict, and
backfills the handler onto existing system-owned Office workflows.

Requirements: [gate-comment-wake.md](../../specs/office/requirements/gate-comment-wake.md).
Design: [gate-comment-wake-01.md](../../specs/office/system-design/gate-comment-wake-01.md).

## Work orders

| Order | Work order | Wave | Depends on | Result |
|---|---|---|---|---|
| 01 | [task-01-fanout-skip-decided-and-author.md](task-01-fanout-skip-decided-and-author.md) | 1 | none | `queue_run_for_each_participant` gains `skip_decided` and on-comment author exclusion |
| 02 | [task-02-gate-comment-prompt.md](task-02-gate-comment-prompt.md) | 1 | none | `task_comment` runs at a review/approval stage get a verdict prompt |
| 03 | [task-03-template-and-reconcile.md](task-03-template-and-reconcile.md) | 2 | 01, 02 | Shipped template declares the gate `on_comment`; startup reconciler backfills system-owned workflows; end-to-end proof |

01 and 02 touch disjoint packages and can land in either order. 03 ships the
template change, which must not reach a workspace before 01 exists: without
`skip_decided` and author exclusion the new handler would wake every seat,
including the author.

## Risks

- **Runner no longer woken at gates.** Declaring any `on_comment` action makes
  the engine report the comment handled, which suppresses the legacy assignee
  wake. This is intended (AC-OFFICE-GATE-COMMENT-001.8) and an operator-visible
  change; @-mention remains the way to reach the runner.
- **Extra runs.** Distinct comments and a comment racing a decision can wake a
  seat more than once. Bounded by the existing self-trigger allowance and
  accepted in the requirements.
- **Fan-out failure at a gate.** A failed gate fan-out no longer falls back to
  waking the runner; it relies on the subscriber's single redispatch. A second
  failure leaves the remaining seats unwoken until the next comment or step
  re-entry. Accepted in AC-OFFICE-GATE-COMMENT-001.13; the warning log is the
  signal.
- **Engine infrastructure error at a gate.** Every failure the fan-out does
  not report (the operation-applied read, a state or step load, an unregistered
  action kind, a comment action declared before the fan-out, or a failed
  operation marker) keeps today's legacy runner wake plus one redispatch
  (AC-OFFICE-GATE-COMMENT-001.15). Accepted: these are storage, wiring or
  operator-configuration faults, and the dispatcher returns no step context to
  classify them. A preceding action that fails on both evaluations leaves the
  seats unwoken for that comment; only system-owned rows that already carried
  their own gate comment actions can reach it (Out of scope in the
  requirements).
- **Channel-inbound comments.** Comments from channel producers reach the
  engine only through the subscriber, so a fan-out failure there is logged and
  never retried (AC-OFFICE-GATE-COMMENT-001.16). Channel tasks carry no workflow
  step today.
- **Runner comment under self-review.** The runner's own gate comment wakes no
  other seat (AC-OFFICE-GATE-COMMENT-002.7, out of scope to change).
- **Session-less gated task.** The engine does not evaluate a comment for a task
  with no session; such a task keeps the legacy wake. Out of scope.
- **Template tests pinning `office-default.yml`.** Any golden test over the
  template's event lists must be updated deliberately, not loosened.

## Verification strategy

- Engine unit tests (01), prompt builder tests (02), reconciler tests and one
  orchestrator-level test through the real engine and the shipped template (03).
- No web UI changes, so no Playwright work order. The end-to-end evidence is the
  orchestrator-level test in 03 plus a restart of a dev backend against a
  materialized Office workflow, recorded in 03's Results.
- Backend lint: `make -C apps/backend lint`.
