---
spec: docs/specs/tasks/requirements/title-length-limit.md
created: 2026-08-11
status: complete
---

# Implementation Plan: Keep the Caret When Task Title Fields Clamp at the 60-Character Limit

## Overview

Every editable task-title field runs `clampTaskTitleInput` inside its `onChange`
handler. When the typed text exceeds 60 code points (that is, the field's value
is already at the cap), the clamp truncates the value, so the value React
commits differs from the DOM's current value. React writes the DOM value and the
browser resets the caret to the end of the field. This repeats on every
keystroke, so editing a title that is at the 60-character limit becomes
unusable: the caret jumps to the end after each keypress in the task "Rename"
dialog and the task-edit dialog.

The fix preserves the caret after a clamping commit without changing the clamp
semantics: a small hook captures the caret when truncation occurs and restores
it (pinned to the clamped length) after React commits the truncated value.

## Confirmed root cause

- `clampTaskTitleInput` (identity below 60 code points, tail-dropping at the
  cap) is applied inside `onChange` in every task-title input.
- At the cap, typing inserts into the DOM (61+ code points), the clamp drops
  the tail, and the committed value differs from the DOM value. React writes
  the DOM, and the browser places the caret at the end. Every keystroke
  repeats the cycle.
- Any title at exactly 60 code points triggers this, including every
  remote-imported title shortened with `…` by `truncateRemoteTaskTitle`
  (GitHub review/issue, Jira, Linear, GitLab, Azure DevOps watchers) and any
  title that reached the cap while typing.
- The value never differs below the cap, which is why short titles behave
  correctly.

### Reproduction evidence (uncommitted scratch specs, removed after diagnosis)

- `apps/web/e2e/tests/kanban/scratch-cursor.spec.ts` — short titles: typing
  mid-title in the rename and edit dialogs keeps the caret (passed).
- `apps/web/e2e/tests/kanban/scratch-cursor-60.spec.ts` — 60-character titles:
  typing mid-title in both dialogs ends with `caret=60` after every keystroke
  (failed, reproduced the report).
- `apps/web/e2e/tests/task/mobile-scratch-cursor.spec.ts` — phone drawer
  rename and edit, short titles (passed; same components on mobile).

## Fix

### New hook

- `apps/web/hooks/use-task-title-selection-restore.ts`: returns `{ inputRef,
  clampChange }`.
  - `clampChange(e)` returns `clampTaskTitleInput(e.target.value)`; it records
    the DOM caret (`selectionStart`/`selectionEnd`) in a ref when the clamp
    actually truncated (`next !== e.target.value`) and clears the ref on any
    non-truncating change, so a stale record from a keystroke that never
    committed (typing at the very end at the cap) cannot be replayed by a later
    commit.
  - A `useLayoutEffect` on `value` restores the recorded selection with
    `setSelectionRange(min(start, value.length), min(end, value.length))` after
    React commits, skipping when the input is not focused.
  - Works for `<input>` and `<textarea>` (both expose
    `selectionStart`/`selectionEnd`/`setSelectionRange`).
- The hook lives in `apps/web/hooks/` next to the other behavior hooks; the
  pure `clampTaskTitleInput` stays in `apps/web/lib/task-title.ts` unchanged.

### Reported surfaces (required)

- `apps/web/components/task/task-rename-dialog.tsx` — the "Rename task" dialog
  title input.
- `apps/web/components/task-create-dialog-selectors.tsx` (`InlineTaskName`) —
  the task-create/task-edit dialog title input (`task-title-input`).

### Sibling surfaces with the same latent bug (consistency wave)

- `apps/web/components/task/task-top-bar-title.tsx` — inline breadcrumb title
  editor (`task-title-rename-input`).
- `apps/web/components/task/new-subtask-form-parts.tsx` — subtask title input
  (`subtask-title-input`).
- `apps/web/app/office/components/new-task-dialog.tsx` — Office new-task title
  textarea.
- `apps/web/app/office/setup/step-task.tsx` — onboarding task title input.
- `apps/web/components/automations/automation-editor-sections.tsx` — automation
  `taskTitleTemplate` input.

These use the identical clamp-in-`onChange` pattern and exhibit the same caret
jump at the cap. Each is a one-line swap to `clampChange(e)` plus the hook call.
They are a separate wave so the reviewer can drop them without touching the
reported fix.

## Behavior after the fix

- Typing mid-title at the cap inserts the characters and drops the tail (as
  today), but the caret stays immediately after the inserted text.
- Typing below the cap is unchanged (no truncation, no hook activity).
- The cap is still enforced: the field value never exceeds 60 code points and
  submission still clamps/validates.
- No `maxLength` attribute is added (the codebase deliberately avoids it
  because it counts UTF-16 units, not code points; the create dialog's
  `pr-action-create-task-dialog.spec.ts` asserts its absence).

## Tests

- **What:** the hook returns the clamped value and restores the caret after a
  truncating change; it does not touch the caret when no truncation occurs and
  skips restore when the input is not focused.
  **File:** `apps/web/hooks/use-task-title-selection-restore.test.tsx`.
  **How:** render a tiny controlled input through the hook with
  `@testing-library/react` + happy-dom; set the selection, dispatch a change
  with a 61-code-point value inside `act`, and assert `selectionStart` is
  pinned to the recorded position rather than the end.
- **What:** the rename and edit dialogs keep the caret when typing mid-title at
  the 60-character cap.
  **File:** `apps/web/e2e/tests/task/task-title-caret.spec.ts` (desktop
  `chromium` project).
  **How:** seed a task with a 60-character title; open the edit dialog and the
  rename dialog; click the title input, set the selection to position 6, type
  `XY`, and assert the value length stays 60 and `selectionStart` is 8 (the
  characters after the insert), not 60.
- **What:** the same user value on a phone.
  **File:** `apps/web/e2e/tests/task/mobile-task-title-caret.spec.ts`
  (`mobile-chrome` project).
  **How:** open the phone task drawer, right-click the task row, and repeat the
  rename + edit assertions. The mobile sheet renders the same dialogs, so this
  proves viewport parity of the fix.

## E2E scenarios

- **GIVEN** the task-edit dialog with a title at the 60-character limit, **WHEN**
  the user places the caret mid-title and types `XY`, **THEN** the value stays
  60 characters, `XY` appears in place, and the caret is immediately after `XY`.
- **GIVEN** the task "Rename" dialog with a title at the 60-character limit,
  **WHEN** the user places the caret mid-title and types `XY`, **THEN** the value
  stays 60 characters, `XY` appears in place, and the caret is immediately
  after `XY`.
- **GIVEN** a title below the limit, **WHEN** the user types mid-title, **THEN**
  the caret behaves as before (no regression).

## Mobile parity note

The change is caret preservation inside existing inputs; it does not alter
layout, touch behavior, scrolling, or viewport-dependent interaction. The
phone drawer renders the same `TaskRenameDialog` and `SidebarTaskEditDialog`
components, so the desktop E2E assertion covers the mechanism and the mobile
spec proves the same user value on the phone viewport.

## Files

- `apps/web/hooks/use-task-title-selection-restore.ts` (new)
- `apps/web/hooks/use-task-title-selection-restore.test.tsx` (new)
- `apps/web/components/task/task-rename-dialog.tsx`
- `apps/web/components/task-create-dialog-selectors.tsx`
- `apps/web/components/task/task-top-bar-title.tsx`
- `apps/web/components/task/new-subtask-form-parts.tsx`
- `apps/web/app/office/components/new-task-dialog.tsx`
- `apps/web/app/office/setup/step-task.tsx`
- `apps/web/components/automations/automation-editor-sections.tsx`
- `apps/web/e2e/tests/task/task-title-caret.spec.ts` (new)
- `apps/web/e2e/tests/task/mobile-task-title-caret.spec.ts` (new)
- `docs/specs/tasks/requirements/title-length-limit.md` (amended: caret scenario)

## Verification

- Unit: `cd apps && pnpm --filter @kandev/web test -- --run apps/web/hooks/use-task-title-selection-restore.test.tsx`
- E2E desktop: `cd apps/web && pnpm e2e:raw tests/task/task-title-caret.spec.ts`
- E2E mobile: `cd apps/web && pnpm e2e:raw --project=mobile-chrome tests/task/mobile-task-title-caret.spec.ts`
- Related suite: `cd apps && pnpm --filter @kandev/web test -- --run apps/web/lib/task-title.test.ts` and
  `cd apps/web && pnpm e2e:raw tests/github/pr-action-create-task-dialog.spec.ts` (asserts no `maxlength`)
- Full gate (after the final task): `make fmt`, then `make typecheck test lint`

## Implementation Waves And Parallel Candidates

Wave 1 (sequential):

- [x] [task-01-regression-e2e](task-01-regression-e2e.md) — write the failing
  E2E regression specs and the failing hook unit test (RED).

Wave 2 (depends on Wave 1):

- [x] [task-02-caret-preserving-clamp](task-02-caret-preserving-clamp.md) —
  implement the hook; unit test green.

Wave 3 (depends on Wave 2):

- [x] [task-03-rename-edit-dialogs](task-03-rename-edit-dialogs.md) — apply the
  hook to the reported rename and edit dialogs; E2E green.

Wave 4 (depends on Wave 3):

- [x] [task-04-sibling-title-inputs](task-04-sibling-title-inputs.md) —
  consistency across the sibling title inputs; full verification gate.

Parallel delegation is not authorized by this plan. The work stays in the
primary session and follows the dependency order above.

## Verification Results

- `cd apps && pnpm --filter @kandev/web test -- --run hooks/use-task-title-selection-restore.test.tsx`
  — passed (12 tests).
- `cd apps/web && pnpm e2e:raw tests/task/task-title-caret.spec.ts` — passed
  (3 tests) before the fix (RED: caret at 60) and after (GREEN: caret at 8;
  the third case covers the same-char bail-out).
- `cd apps/web && pnpm e2e:raw --project=mobile-chrome tests/task/mobile-task-title-caret.spec.ts`
  — passed (3 tests) RED then GREEN.
- `cd apps/web && pnpm e2e:raw tests/github/pr-action-create-task-dialog.spec.ts`
  — passed (guards the `maxlength`-absence contract).
- `make fmt` — passed.
- `make typecheck` — passed.
- `make lint` — passed (0 issues).
- `make test-web` — 10410+ passed; only `lib/http-git-server.test.ts` fails,
  reproduced identically on the clean tree (pre-existing, unrelated).
- `make test-backend` — pre-existing failures in
  `internal/agentctl/server/{process,api,config}` reproduced identically on the
  clean tree (unrelated to this web-only change).

## Adversarial Review Loop

Round 1 (DeepSeek V4 Pro sub-task, read-only): no blockers, no majors. Two
minors and one nit, all fixed in `30e6c647`:

- **Minor** — stale `pendingSelectionRef` when a same-value clamp makes React
  bail out of the render: the recorded caret could be replayed by an unrelated
  later commit. Fixed by tracking `lastCommittedRef` and only recording when
  the clamp result differs from it.
- **Minor** — unit tests covered only `<input>`, not the `<textarea>` path
  used by the Office new-task dialog. Added `HarnessTextarea` coverage.
- **Nit** — plan referenced `tests/kanban/` instead of `tests/task/` for the
  E2E file. Corrected.

Round 2 (same reviewer): **NO FINDINGS** — the `lastCommittedRef` fix was
verified complete (no desync path; the guard test genuinely exercises the
bail-out), the new coverage is sound, and the earlier-passing areas remain
intact.

Rounds 3-4 (OMP GPT Luna, fresh reviewer): one major + two minors + two nits,
all fixed in `f8574cf6` and `6740e3e9`:

- **Major (round 3)** — same-result truncations (e.g. typing the identical
  character into an all-same-char 60-char title) still reset the caret: the
  render bails out, but React's controlled-state restoration writes the value
  back after the event with no commit to hook into. Fixed with an immediate
  microtask restore in the bail-out path, guarded by element connection and
  focus. Unit + desktop/mobile E2E coverage added.
- **Minor (round 4)** — the bail-out microtask could restore a stale caret
  after a same-turn value or identity change. Fixed with an epoch token
  invalidated by newer changes and commits, plus ownership (`inputRef.current
  === el`) and value (`el.value === next`) guards. Regression test added.
- **Minor (round 4)** — bail-out path untested for textarea. Added.
- **Nits** — verification counts corrected (unit 12, E2E 3 per project).

Round 5 (OMP GPT Luna): **no correctness findings** — the bail-out microtask
guards (epoch, ownership, value, connection, focus) verified correctly ordered,
all seven call sites sound, input and textarea paths covered. One nit (stale
verification count) fixed here.

Round 6 (OMP GPT Luna, final round): one minor, fixed in `e071d1a5` — the
commit-path record stored only the caret, so a parent that delayed, rejected,
or superseded the clamped update could commit an unrelated value and apply the
stale caret to it (reachable in the automation editor's async form load).
The pending selection now stores the clamped value it belongs to, the
same-result bail-out clears any lingering record, and the layout effect
discards the record when the committed value does not match. Regression test
added (controlled harness that rejects the clamped update, then commits an
unrelated value). This was the 6-round cap; the final finding is addressed and
the full gate is green (13 unit tests, 3 E2E per project).

## Open Questions

None. The root cause is reproduced with deterministic E2E evidence and the fix
is behavior-preserving apart from the caret position.
