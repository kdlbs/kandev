---
created: 2026-09-11
status: implemented
requirements:
  - REQ-TASKS-PLAN-COMMENTS-001
  - REQ-TASKS-PLAN-COMMENTS-002
  - REQ-TASKS-PLAN-COMMENTS-003
  - REQ-TASKS-PLAN-COMMENTS-004
system_design:
  - ../../specs/tasks/system-design/plan-comments.md
legacy_specs: []
---

# Implementation Plan: Plan comment recovery

## Overview

Keep ordinary chat usable when background plan-comment reads fail, and recover
actual legacy feedback without a manual Retry action. One focused work order
owns the frontend correction and its desktop/mobile regression evidence. The
task system owns this repair because it controls feedback persistence and
delivery; the existing Plan and composer surfaces present that state.

This follows the completed
[task-owned plan-comment package](../task-owned-plan-comments/plan.md), especially
its [migration work](../task-owned-plan-comments/task-04-migrate-legacy-browser-drafts.md)
and [responsive scenarios](../task-owned-plan-comments/task-05-prove-responsive-multi-session-behavior.md).
The earlier verification results are historical and do not certify this repair.

## Confirmed defect

PR #3332, commit `f45450fe8`, added an unconditional comment-list request after
legacy migration, including when the legacy collection is empty. Its failure
marks the task's migration failed. Session discovery and ordinary plan/comment
read errors can set the same failure before a single legacy row is identified.
The effect then exits on `failed`, so a later successful read cannot release
the gate. Structured and passthrough Send reject every message on that gate.

A temporary harness executed the actual migration source with simulated
transport and hook/state boundaries. It reproduced empty-storage failure,
successful refresh leaving Send blocked, explicit Retry recovery, and failed
plan discovery staying blocked after confirming there is no plan. These are
source-level reproductions, not a completed React or browser regression suite.
The live frontend diagnostic bundle contained no browser logs, so the exact
network event on the reporting phone remains unconfirmed.

## Scope

### In scope

- Background task-scoped legacy discovery, exact acknowledgement cleanup,
  quiet transient recovery, and bounded retries.
- Separate ordinary snapshot loading from legacy recovery restrictions.
- Standard and passthrough Send, selected-comment Run, and inline actionable
  recovery feedback in Plan and chat.
- Shared desktop/phone behavior, translated pending-action copy, focused tests,
  and one recovery paragraph in the existing public task guide.

### Out of scope

- Backend schema, WebSocket payloads, queue admission, primary routing, or
  comment ownership changes.
- Global WebSocket timeout/retry changes, browser-wide storage migration,
  recovery modals, new settings, or layout redesign.
- Reconstructing drafts already deleted by older releases or changing other
  comment types' persistence.
- Creating platform tasks/sessions, delegation, publication, or changes to the
  user's running instance.

## Technical approach

- Replace the task-plan status-only migration map with the recovery projection
  described in the [system design](../../specs/tasks/system-design/plan-comments.md#recovery-state-and-lifetime).
  Remove all consumers of the old map; do not keep parallel readiness flags.
- Keep orchestration behind `usePlanCommentMigration`. A small adjacent
  coordinator can hold store-scoped promises, timers, known pending records,
  and subscriptions. Use the existing `useForegroundRefresh` pattern and
  connection state; coalesce duplicate consumers and foreground signals.
- Stop importing `getTaskPlanComments` into migration merely for a final
  refresh. Reconcile acknowledged create snapshots, and finish empty scans
  without a migration network request. `usePlanComments` remains the read owner.
- Preserve identified pending records across failed storage reads, partial
  upload, task/plan changes, and retry. Never treat another session's row or a
  generic load error as proof of drafts belonging to this task.
- Share the ordinary-Send restriction predicate between structured and
  passthrough consumers. Keep displayed persisted refs and backend validation.
  Run checks only its selected persisted comment and primary destination.
- Render `PlanCommentMigrationNotice` only for actionable identified draft
  recovery. Background phases have no banner. Pending-Send feedback preserves
  the draft and explains recovery without requiring migration terminology.

The accepted ownership/admission
[ADR](../../decisions/2026-09-02-task-owned-plan-comments.md) still applies.
The recovery rationale fits the requirement/design; no new ADR is needed.

## ASCII UI preview

### UI-01: Empty task, or no identified legacy feedback

Entry: open any task composer, including after a failed background read.
The current failure and corrected shared composition are:

```text
Before (source-confirmed):
[Some saved plan comments could not be restored.] [Retry]
[Your message                                      ] [Send]
                                               -> rejected

After (desktop and phone):
[Existing context, when present]
[Your message                                      ] [Send]
                                               -> delivered
```

Required: no restoration banner or migration restriction solely from a pending
or failed read. This does not override actual offline/session input constraints.
The phone uses the current task chat layout and its existing reachable Send
control; the diagram describes hierarchy, not a new horizontal phone layout.
Maps to `AC-TASKS-PLAN-COMMENTS-004.5`, `.6`, and `.8`.

### UI-02: Actual legacy feedback requires attention

Entry: the same Plan or chat surface, after known draft recovery exhausts its
initial retry burst or encounters an actionable rejection.

```text
Desktop:
[Saved plan feedback needs attention.              ] [Retry]
[Your retained message                             ] [Send]

Phone (same inline region, wrapping when needed):
[Saved plan feedback needs attention.]
[Retry: touch target >=44px           ]
[Your retained message               ]
[Existing composer actions + Send    ]
```

Required: only genuine unresolved task feedback can produce this row; Send
preserves the draft while that feedback would be omitted. Automatic transient
attempts do not mount the row. A persisted selected-comment Run remains usable
if its other eligibility conditions pass. Copy is illustrative and localized;
reuse the existing actionable copy where it already describes the state.
Maps to `AC-TASKS-PLAN-COMMENTS-004.1`, `.3`, `.4`, `.7`, and `.8`.

The closest shipped composition is `task-layout.tsx` with its dedicated mobile
chat and Plan Drawer; the existing `PlanCommentMigrationNotice` is the inline
region being corrected. Inline feedback fits a brief recovery action without
displacing chat. Preserve its scroll owner, keyboard focus, and safe-area
clearance. Keep desktop Retry at 28 px and coarse-pointer Retry at least 44 px;
do not create an overlay, navigation destination, or extra scroll region.

## Tests

| Regression evidence | Acceptance criteria |
| --- | --- |
| `use-plan-comment-migration.test.tsx`: no legacy rows, unknown/absent/current plan, failed discovery, later successful empty read, unrelated task records | `004.5`, `004.6` |
| Migration coordinator and persistence tests: partial success, same-UUID replay, unreadable storage after discovery, conflict, missing plan, plan change, unmount, simultaneous consumers | `004.1`, `004.2`, `004.3`, `004.6`, `004.7`, `004.8` |
| `use-plan-comments.test.tsx`: connected/foreground retries, coalescing, empty success, preserved displayed snapshot, no migration failure from a read error | `001.3`, `004.5`, `004.6`, `004.7` |
| Structured/passthrough submission tests and Run tests: plain Send allowed, identified unresolved row blocks Send, persisted selected Run allowed, retained text and refs | `002.2`, `002.6`, `003.1`, `003.6`, `004.7` |
| Notice component tests: quiet background phases, actual actionable state, manual Retry, focus and draft preservation | `004.4`, `004.8` |

All abbreviated IDs in this table use the prefix `AC-TASKS-PLAN-COMMENTS-`.
The [work order](task-01-recover-plan-comment-context.md) names the mandatory
RED cases and exact commands. Preserve existing scenario coverage.

## E2E tests

Extend the existing files, with shared failure-injection helpers beside them:

- `apps/web/e2e/tests/session/task-plan-comments.spec.ts`, project `chromium`:
  keep the existing Send/Run routing scenario. Add no-draft failures for a
  present plan and a lookup eventually confirming no plan. Leave chat transport
  healthy and prove ordinary Send works without Retry. Add actual mixed legacy
  recovery, preserving a colocated diff comment and preventing omission.
- `apps/web/e2e/tests/session/mobile-task-plan-comments.spec.ts`, project
  `mobile-chrome`: retain the current session-picker/Plan-Drawer migration and
  routing scenario. Exercise a foreground/reconnect sequence with no drafts,
  then send through the visible phone control without manual recovery. In a
  separate scenario, fail one real legacy upload, allow recovery, and prove the
  comment appears and is delivered once. Check the genuine failure Retry target
  and absence of horizontal overflow.

Use a correlated `page.routeWebSocket` interceptor following `ws-response-hold.ts`
and `ws-drop.ts`. Target the exact task and action, leaving unrelated frames
live; cover session-discovery failure separately at the `useTaskSessions` hook
boundary. Record the injected
failure before allowing success. Prove automatic requests and passive backend
state before any helper that can reload or click Retry. Arm causal waits before
the stimulus; no wall-clock success sleeps. Scope controls to the active chat.

These flows cover `AC-TASKS-PLAN-COMMENTS-004.1` through `.8` and preserve the
existing `001`/`002`/`003` routing coverage. Rendered phone checks compare UI-01
and UI-02 with the existing composition, not a new layout.

## Work orders

- [x] [Task 01: Recover plan comment context](task-01-recover-plan-comment-context.md)

Execution is sequential in the primary conversation. No subagents authorized.

## Verification results

Design-package checks on 2026-09-11:

- `python3 scripts/lint-spec-files.test.py`: passed all 36 tests.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- Read-only local-link and work-order reference audit: 31 local document links
  and all 18 requirement/acceptance references passed.
- `git diff --check -- docs/specs docs/plans`: passed.
- `git status --short -- docs/specs docs/plans`: confirmed both new package
  files and the intended existing specification/companion-plan edits.

Implementation is complete. The final focused suite passes all 130 tests across
13 files. Changed-file ESLint, TypeScript, i18n checks and ratchet, specification
validation, and public-doc validation pass. The final desktop build passes all
four browser scenarios. The final phone rerun also passed all four scenarios
against that fresh build after a rebuild was terminated before tests started.
The states captured on desktop and phone were inspected; draft retention, partial
recovery, Retry sizing, and absence of horizontal overflow were verified.
The work order records the final results and the scoped public-guide update.

PR review follow-up on 2026-09-12 adds regressions for unknown-plan failures,
known-session discovery, concurrent local edits, resume coalescing, retained
locales, and stale loading flags. The focused suite now passes 145 tests across
the same 13 files; 24 locale-generator tests also pass. Changed-file lint,
typecheck, i18n, specification and public-doc checks pass. The
[work order](task-01-recover-plan-comment-context.md#pr-review-follow-up-2026-09-12)
records review dispositions and scoped validation. Remote CI/review completion
remains an exact-head delivery check, not a claim made by this tracked plan.

## Risks

- Unknown ownership must not be mistaken for task-owned feedback. Plain Send
  before legacy discovery includes only visible persisted context; later
  recovery never amends or resends that prompt.
- A successful subset must not release a Send that would omit the remaining
  legacy rows. A failed subsequent storage read must not forget known rows.
- Retry promises and timers must remain shared across mounted Plan/composer
  consumers and stop when their scope is gone.
- New retries apply to reads and idempotent draft promotion only. They must not
  replay user messages, Run actions, or accepted queue entries.
- A fresh worktree needs its own frozen-lockfile install. The interrupted
  diagnostic install was completed successfully before product tests.
