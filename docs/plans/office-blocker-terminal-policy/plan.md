---
created: 2026-11-11
status: done
requirements:
  - REQ-OFFICE-BLOCKER-POLICY-001
  - REQ-OFFICE-BLOCKER-POLICY-002
system_design:
  - ../../specs/office/system-design/blocker-terminal-policy.md
legacy_specs: []
---

# Implementation Plan: Office blocker terminal policy

## Overview

This plan resolves [issue #4548](https://github.com/kdlbs/kandev/issues/4548). Only
`COMPLETED` satisfies an Office blocker. A blocker that fails or is cancelled holds
its dependents and wakes one responsible agent once. The authoritative pair is
[blocker terminal policy requirements](../../specs/office/requirements/blocker-terminal-policy.md)
and [design](../../specs/office/system-design/blocker-terminal-policy.md).
`REQ-OFFICE-SCHEDULER-002` (AC .5, .6 and .9) now defers its classification to that
pair.

Decisions confirmed by the user on 2026-11-11:

- Option A: match Kanban. Failed and cancelled blockers hold their dependents.
- Escalation goes to the runner of the blocker's parent task. When there is none,
  it goes to the workspace CEO.
- Both `FAILED` and `CANCELLED` escalate. When the canceller is the recipient, the
  existing claim-time merge absorbs the wake into its in-flight run.
- Avoid double alerts through occurrence keys and the existing claim-time merge. No
  cross-reason correlation is added.

## Scope

In scope:

- One Office blocker classifier (satisfied, failed, pending) replacing
  `IsTaskInTerminalStep` in admission, the resolution cascade, and the legacy
  step-move path.
- Confirmation that every runtime `FAILED` writer already publishes
  `task.state_changed` (through `backendapp.taskRepositoryAdapter` and the task
  service). No new publisher is needed.
- A `task_blocker_failed` wake: a `task.state_changed` subscriber, the Office
  status-change trigger in `reactToStatusChange`, a tree-cancel trigger, recipient resolution, key,
  registry entries, and a prompt branch.
- The scheduler requirement and design cross-references, already updated in this
  package.

Out of scope:

- Kanban dependency policy, Office `task_children_completed` classification, the
  `agent_error` producer, and the Inbox Failed query.
- Sweeps for unpublished transitions, automatic retry or rewiring, UI or copy,
  schema, runtime flags, and metrics labels.

## Delivery order

| Wave | Work order | Depends on |
| --- | --- | --- |
| 1 | [Task 01: Success-only blocker classification](task-01-success-only-classification.md) | none |
| 2 | [Task 02: Escalate blocker failures once](task-02-blocker-failure-escalation.md) | 01 |

Both work orders ship in one PR. Task 01 alone would hold dependents behind a failed
blocker without the Task 02 escalation, which brings back the silent stall that the
requirement forbids.

## Verification strategy

The tests extend the real-schema fixtures in
`apps/backend/internal/office/service/assignment_blocker_*_test.go`, which use real
writers, the real bus, and actual run admission. New files keep the package under
the 800-line test-file limit. The design's [Testing](../../specs/office/system-design/blocker-terminal-policy.md#testing)
section is the matrix. Each work order lists its exact commands. Finish with the
scoped lint on the changed Go files:

```bash
bash scripts/lint-go-changed $(git diff --name-only origin/main -- '*.go')
```

Spec checks:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

No rendered UI changes, so no Playwright or ASCII preview is required.

## Documentation impact

Internal specs changed in this package. After implementation, add a row for the
pair to the spec table in `apps/backend/internal/office/AGENTS.md` and update its
row count. Public Office docs do not describe blocker failure semantics. Task 02
runs `/docs-maintainer` to confirm that no public page changes.

## Risks

- Behavior change: Office chains that relied on a failed blocker releasing its
  dependent now stay held, and their coordinator is woken.
- An archived failed blocker holds its dependents until the edge is removed. This
  matches Kanban, and the escalation tells the coordinator what to remove.
- A queued escalation is rechecked before replay after a pause. If the blocker
  recovered or no longer holds an Office dependent, replay skips the stale request.
  An alert already delivered into a run cannot be recalled.
- A failed `task.state_changed` publication leaves its transition unescalated. The
  dependent stays held, and a `FAILED` blocker remains in the Inbox Failed bucket.
- Agent sessions outside any Office run that cancel a blocker still wake the
  recipient. This is accepted, because no actor travels on the event.
