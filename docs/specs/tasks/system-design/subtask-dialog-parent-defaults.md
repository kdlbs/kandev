---
status: current
system: tasks
created: 2026-10-14
owners:
  - kandev
requirements:
  - REQ-TASKS-SUBTASK-DIALOG-PARENT-DEFAULTS-001
---

# Subtask Dialog Parent Defaults System Design

## Purpose and boundaries

Tasks owns selected-parent context for subtask creation. This is a local
correction in `apps/web/components/task/new-subtask-dialog.tsx` and its existing
`use-subtask-parent-defaults.ts` helper, using existing task/session projections
and form controls. No store architecture, loading
policy, API, profile eligibility, persistence, or materialization change.
The [Office parent/child design](../../office/system-design/tasks-01.md#b-parent--child-handoffs)
is read-only workspace background, not acceptance coverage for this correction.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-SUBTASK-DIALOG-PARENT-DEFAULTS-001` | Selected context; Initial seeding; Failure and shared surfaces; Regression evidence |

## Selected context

`useSubtaskDialogState` currently reads only `kanban.tasks`, so a selected
sidebar parent held in `taskOverview.byId` resolves to null and its successfully
loaded sessions are ignored. Use `parentTaskId` in a narrow canonical-first
selector, following the existing `task-page-content.tsx` lookup pattern. Retain
board compatibility only for the same ID; do not fetch lists or activate a task.

Keep `resolveSubtaskParentContext` and its repository ordering/fallback rules.
Primary session resolution uses `primarySessionId`, otherwise `is_primary` in
the selected parent's sessions. Keep explicit-primary precedence even if that
session is not yet loaded; secondary sessions cannot fill the gap.
`useTaskSessions(parentTaskId)` remains the owner of reads, loaded/error flags,
refreshes, and session options. Shared task/session hooks remain read-only.

## Initial seeding

`useSubtaskParentDefaults` is the sole repository/profile seed owner, with
one-time delayed seeds for untouched fields. Repository context can seed as soon as it is known; reuse
`repositoriesDirty` and preserve existing or remote selections. Profile and
workspace-mode choices need only local initial-seed/touched tracking through
existing handlers, including deliberate clearing. Existing repository dirty,
remote mode, and remote-row state also settle the initial new-workspace choice;
late worktree data cannot switch to inheritance and discard an edited source. Resolve initial workspace
mode when the primary context is available or the session read settles
loaded/error; use `defaultSubtaskWorkspaceMode` without changing its rule.
Later refreshes cannot reseed completed fields. The helper wraps the existing profile change handler and returns an empty
submission fallback after an explicit profile choice. The dialog caller passes
that fallback to its submit hook, so clearing
the selector cannot silently submit the parent profile. `useSubtaskSubmit` stays
read-only; ordinary omitted-profile backend defaults and eligibility are unchanged.

The form stays mounted and editable during reads. Delayed seeding is smaller
than a full-context readiness gate and avoids a new spinner or rekey/remount
that discards title/prompt. Do not introduce a reusable revision/coordinator
policy or change `useSubtaskFormState` and shared form parts/handlers.

## Failure and shared surfaces

Loaded-empty/error flags settle absent session context without blocking the
form. A missing parent leaves existing empty/default controls and validation;
available task repository data does not require a session. No active-task
fallback or new copy is added. Desktop and phone reuse the existing dialog,
translated controls, repository chips, and submission path with no geometry
change. No public-documentation update is needed to restore these defaults.

## Regression evidence

Independently render the real dialog, form state, and controls in
`new-subtask-dialog.test.tsx` with a reactive store and external transport/data
stubs. RED must expose canonical-only non-active parent lookup and delayed
successful primary context; observe actual repository/profile/mode controls
and retained title/prompt. Controls cover active parent, primary versus
secondary, no-session/error/missing parent, deliberate choices, and refresh.
Use the existing context suite for primary-resolution cases.

The same sequential work order owns these tests, targeted lint/typecheck,
normal staged i18n, and one managed mobile E2E run of the existing
`mobile-sidebar-task-actions.spec.ts` selected-parent scenario with its original
assertion/timeouts, zero retries, and one worker. Keyboard regression evidence
remains unchanged. No additional plan/order or UI specification is introduced.
