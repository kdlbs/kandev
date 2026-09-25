---
status: draft
system: tasks
created: 2026-09-25
owners:
  - kandev
---

# Guarded Exact-Task Retirement Requirements

## Overview

The task system owns a narrowly scoped retirement operation for one damaged
task only after a replacement task has accepted its preserved work. This is
separate from ordinary archive and delete because it must never broaden cleanup
authority or infer that missing state is safe to remove.

## Terminology

- **Old task:** The one exact task proposed for retirement.
- **Replacement task:** The distinct same-workspace task that has accepted the
  old task's preserved deliverables.
- **Receipt:** A bounded, stable result that records each retirement predicate.
- **Unknown:** Evidence that cannot be inspected or reconciled. Unknown blocks
  retirement.

## Requirements

### REQ-TASKS-EXACT-RETIREMENT-001: Authorize and preview an exact pair

**Intent:** Let an authorized operator discover why one exact task pair is or is
not eligible without changing either task.

#### Acceptance criteria

- **AC-TASKS-EXACT-RETIREMENT-001.1:** The system shall deny an unauthenticated
  or foreign-workspace caller through the existing denial contract without
  returning retirement inventory or mutating state.
- **AC-TASKS-EXACT-RETIREMENT-001.2:** The system shall require distinct old and
  replacement IDs in the same workspace and require their supplied generations
  to match the observed generations.
- **AC-TASKS-EXACT-RETIREMENT-001.3:** Preview shall return one stable receipt
  per predicate with its status (`PASS`, `BLOCKED`, or `UNKNOWN`), reason code,
  resource identity, observed generation, and evidence digest.
- **AC-TASKS-EXACT-RETIREMENT-001.4:** Preview shall not resume a session,
  consume or acknowledge a queue, repair a worktree, or otherwise mutate an old
  or replacement task resource.

### REQ-TASKS-EXACT-RETIREMENT-002: Preserve and hand off all unique state

**Intent:** Ensure retirement cannot discard state that the replacement has not
explicitly and durably accepted.

#### Acceptance criteria

- **AC-TASKS-EXACT-RETIREMENT-002.1:** The system shall block retirement when
  any old-session queue entry, pending move, dispatch claim, dependency,
  subtask, PR/watch, runtime, lease, consumer, or handoff lacks an exact
  terminal disposition.
- **AC-TASKS-EXACT-RETIREMENT-002.2:** The system shall block retirement unless
  an independently verified preservation receipt binds both task IDs and the
  exact repository, environment, worktree, archive, Git index, source-byte, and
  reachable-commit evidence.
- **AC-TASKS-EXACT-RETIREMENT-002.3:** The system shall require an ordered,
  hash-verified replacement intake receipt and durable acknowledgement for
  every handed-off old-session queue entry; it shall not replay or resume an old
  session implicitly.

### REQ-TASKS-EXACT-RETIREMENT-003: Fence and audit exact cleanup

**Intent:** Retire only proven old-owned resources while leaving the replacement
and every foreign resource unchanged.

#### Acceptance criteria

- **AC-TASKS-EXACT-RETIREMENT-003.1:** Before cleanup, the system shall compare
  all captured generations and claim one exact operation atomically; changed or
  missing evidence shall block before physical cleanup begins.
- **AC-TASKS-EXACT-RETIREMENT-003.2:** The system shall remove only recorded
  old-owned resources after exact physical identity revalidation. It shall not
  invoke ordinary delete/archive cleanup, forced worktree removal,
  repository-wide pruning, or broad filesystem removal.
- **AC-TASKS-EXACT-RETIREMENT-003.3:** The system shall remove the old task only
  after all exact resources have terminal receipts, retain the audit and
  preservation evidence, publish one task-deleted tombstone, and not transition
  the old task through Done.
- **AC-TASKS-EXACT-RETIREMENT-003.4:** Repeating an identical request ID shall
  return its stored receipt. A changed request or uncertain physical outcome
  shall conflict or remain `UNKNOWN`/`BLOCKED`; it shall never be presumed
  complete.

## Out of scope

- Retiring the ten reproduction tasks or archives, bulk retirement, or raw
  database surgery.
- Changing ordinary archive/delete behavior or reconstructing lost source,
  index, or commit state.
