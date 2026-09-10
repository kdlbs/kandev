---
id: "03-phone-task-actions"
title: "Make task choices touch accessible"
status: done
wave: 3
depends_on:
  - "02-desktop-threads-actions"
plan: "plan.md"
requirements:
  - REQ-TASKS-THREADS-ACTIONS-001
  - REQ-TASKS-THREADS-ACTIONS-002
  - REQ-TASKS-THREADS-ACTIONS-003
  - REQ-TASKS-THREADS-ACTIONS-004
acceptance_criteria:
  - AC-TASKS-THREADS-ACTIONS-001.1
  - AC-TASKS-THREADS-ACTIONS-001.2
  - AC-TASKS-THREADS-ACTIONS-001.3
  - AC-TASKS-THREADS-ACTIONS-001.4
  - AC-TASKS-THREADS-ACTIONS-001.5
  - AC-TASKS-THREADS-ACTIONS-001.6
  - AC-TASKS-THREADS-ACTIONS-002.1
  - AC-TASKS-THREADS-ACTIONS-002.2
  - AC-TASKS-THREADS-ACTIONS-002.3
  - AC-TASKS-THREADS-ACTIONS-002.4
  - AC-TASKS-THREADS-ACTIONS-002.5
  - AC-TASKS-THREADS-ACTIONS-002.6
  - AC-TASKS-THREADS-ACTIONS-003.1
  - AC-TASKS-THREADS-ACTIONS-003.2
  - AC-TASKS-THREADS-ACTIONS-003.3
  - AC-TASKS-THREADS-ACTIONS-003.4
  - AC-TASKS-THREADS-ACTIONS-003.5
  - AC-TASKS-THREADS-ACTIONS-003.6
  - AC-TASKS-THREADS-ACTIONS-004.1
  - AC-TASKS-THREADS-ACTIONS-004.2
  - AC-TASKS-THREADS-ACTIONS-004.3
  - AC-TASKS-THREADS-ACTIONS-004.4
  - AC-TASKS-THREADS-ACTIONS-004.5
  - AC-TASKS-THREADS-ACTIONS-004.6
  - AC-TASKS-THREADS-ACTIONS-004.7
system_design:
  - ../../specs/tasks/system-design/threads-task-actions.md
---

# Task 03: Make Task Choices Touch Accessible

## Summary

Add an inset drawer presentation for the shared task choices, with visible
overflow in the parent's existing task header. Prove the six task operations
on phones while retaining compact page chrome, session controls, and native
horizontal swiping.

## In scope

- Add `task-management-drawer.tsx` with one page state, fixed header, visible
  Back, single internal body scroller, safe-area padding, and dynamic viewport
  bounds. Reuse task-owned option derivation and callbacks from Task 01.
- Render task overflow beside Open task in `MobileThreadColumnHeader`; retain
  its title picker, two-line title, status and session control, plus the page's
  inline pagination. Use the drawer for phone/coarse-pointer entries and compact
  menus for fine-pointer desktop.
- Coordinate menu, simple inline archive confirmation, full archive/delete
  dialogs, provider forms, and plugin link handoffs as one flow. Keep A's
  identity and return focus only after the final surface closes.
- Add mobile TDD/Playwright for every action, nested navigation, cancellation,
  failures, long labels/lists, removal fallback, focus, viewport transitions,
  safe-area containment and native swipes. Inspect new phone screenshots.
- Reuse translations and add only necessary new labels in all five real
  languages plus pseudo; run the Traditional Chinese generator.

## Out of scope

- New mutations, integration-specific forms, gesture recognition, header rows,
  saved view/default Home changes, or broad global menu CSS changes.

## Acceptance

1. Each action completes for the task whose visible 44px control was tapped.
   Nested choices use one drawer and Back navigation; provider and destructive
   confirmation handoffs preserve the target and have one interactive surface.
   Touch rows meet the 44px floor while fine-pointer desktop remains compact.
2. Phone tests prove all six persisted outcomes with an unaffected sibling,
   cancellation and failure behavior, original/replaced/removed-opener focus,
   and successor/predecessor/empty recovery. Long content stays within the
   viewport, scrolls in one region, and leaves final rows reachable.
3. The compact parent topbar and inline swiper remain intact. Native held
   swipes, reverse swipes, nested chat horizontal content, and session switching
   work after menu dismissal. Inspect and record 360px and 320px rendered phone
   states, including the deepest workflow choice and a confirmation.

## Verification

Run from `apps/web`. Add the component and browser failures before implementing
the drawer or phone trigger changes:

```bash
pnpm exec vitest run components/task/task-management-drawer.test.tsx components/threads/thread-task-actions.test.tsx
pnpm e2e:run --project mobile-chrome tests/task/mobile-threads-task-actions.spec.ts -- --retries=0
```

The mobile spec uses the configured Pixel 5 device and `.tap()`. Set viewport
widths to 360, 320, and 700 CSS pixels within that project; include an 820px
coarse-pointer transition to cover the tablet interaction. Assert bounding
boxes, center-point hit testing, document `scrollWidth <= clientWidth`, one
body scroller, and bottom safe-area clearance for root, every nested level,
link dialog, and archive/delete confirmation. Include long unbroken names,
many workflow steps/provider rows, and page heights short enough to scroll.

Use the parent `mobile-threads-swipe-helpers.ts` to send actual touch gestures.
Assert position before touch release and before destination chat hydration,
then verify one active phone conversation after snap. Do not force interactions
through an open modal. For a flow whose target leaves the admitted set, change
task state from an independent client while the flow remains open.

After GREEN, run the affected parent and shared-menu regressions sequentially:

```bash
pnpm exec vitest run hooks/use-task-menu-actions.test.ts components/task/task-management-menu.test.tsx components/task/task-management-drawer.test.tsx components/task/task-switcher-context-menu.test.tsx components/task/task-archive-confirmation.test.tsx components/task/task-delete-confirm-dialog.test.tsx components/threads/thread-column-activation.test.tsx components/threads/threads-board.test.tsx components/kanban/kanban-header-mobile.test.tsx
pnpm e2e:run --project mobile-chrome tests/task/mobile-threads-task-actions.spec.ts tests/task/mobile-threads-view.spec.ts tests/task/mobile-threads-swipe.spec.ts tests/task/mobile-sidebar-task-actions.spec.ts -- --retries=0
pnpm e2e:run --project chromium tests/task/threads-task-actions.spec.ts -- --retries=0
pnpm run typecheck
pnpm run i18n:check
pnpm run i18n:ratchet
pnpm exec eslint --max-warnings 0 components/task/task-management-drawer.tsx components/task/task-management-menu.tsx components/task/task-item-menu-button.tsx components/threads/thread-task-actions.tsx components/threads/mobile-thread-column-header.tsx e2e/tests/task/mobile-threads-task-actions.spec.ts e2e/tests/task/threads-task-actions-helpers.ts
```

If copy is added, run these before the locale checks above:

```bash
pnpm run i18n:zh-hant
pnpm run i18n:pseudo
```

Save screenshots through the new Playwright spec using `testInfo.outputPath`
for the 360px root menu, deepest workflow step page, and delete confirmation,
plus the narrow 320px header/menu. Open the resulting images with the runtime's
image viewer. Record their paths and visual findings below. Screenshots from
the parent are not evidence that this new menu works.

If any parent test path changed when its final commits were integrated, update
the exact command here before execution and identify the preserved scenario.

## Files likely touched

- `apps/web/components/task/task-management-drawer.tsx` and `.test.tsx` (new),
  shared management-menu model/items, and `task-item-menu-button.tsx`.
- `apps/web/components/task/task-archive-confirmation.tsx` and its adapter only
  if a shared surface-handoff hook is required; preserve classification policy.
- `apps/web/components/threads/thread-task-actions.tsx`, the parent's
  `mobile-thread-column-header.tsx`, and their focused tests.
- `apps/web/e2e/tests/task/mobile-threads-task-actions.spec.ts` (new) and shared
  `threads-task-actions-helpers.ts`; parent swipe helpers are reused.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,pseudo}/task.json` or
  `threads.json` only for required new copy.

## Dependencies

Task 02 and the integrated parent mobile layout. Re-read the final parent
`mobile-menu-sheet.tsx`, `mobile-picker-sheet.tsx`, and `MobileThreadColumnHeader`
before choosing geometry or focus APIs.

## Risks

The global below-640px Radix overrides can stack parent and child portals.
Phone drawer navigation must share data/actions without rendering those nested
portals. A modal can keep focus trapped in a disappearing origin, and a default
focus restore can scroll to an offscreen task. Preserve the target but resolve
focus against the current surviving viewport after handoff.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/threads-task-actions.md).
- [Design](../../specs/tasks/system-design/threads-task-actions.md): Phone
  composition, dismissal and focus, target lifetime.
- `/mobile-parity` and its mobile UI language reference, `/e2e`, `/tdd`, and the
  integrated parent mobile Threads and shared-header work orders.
- Task 02's action fixtures and assertions; existing mobile sidebar and
  Threads session/swipe tests.

## Results

Completed. The existing header gains one 44px overflow beside Open task; no new
row, global menu CSS, gesture handler or page-header replacement was introduced.
Phone and coarse pointers use a single inset task drawer with nested pages and
Back. Shared provider/full-confirmation dialogs replace it; simple archive
confirmation uses its inline body without duplicating classification.

RED/GREEN: the nested-navigation regression initially failed to return Back
focus to the originating choice. Page transitions now reset scroll and focus
the corresponding row. Shared surface tests also protect removed opener and
old-response handoffs. The final combined unit command in `plan.md` passed
**217 tests in 25 files**.

After the current production build, this command ran from `apps/web` with one
worker and no retries:

```bash
pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-threads-task-actions.spec.ts tests/task/mobile-threads-view.spec.ts tests/task/mobile-threads-swipe.spec.ts tests/task/mobile-sidebar-task-actions.spec.ts -- --retries=0
```

The **29-test suite passed in four files**: five new outcome/edge/geometry tests
and 24 parent/shared-menu cases. Discovery separately confirmed all 29 tests.
Both desktop and mobile exercise all six actions, canceled archive/delete,
failures/retry, target identity through filtering, and late archive completion
while a newer menu remains open. Existing sidebar and session-picker behavior
is retained. The desktop follow-up passed all six new tests as recorded in
Task 02. Typecheck, affected-file ESLint, formatting, `i18n:check` and
`i18n:ratchet` passed; existing copy covers every label, so no catalogs changed.

Rendered phone evidence was inspected from the final mobile test and copied to
`/tmp/kandev-thread-actions-F8Jynk/` before test cleanup. Files are local,
disposable verification artifacts, not production-data screenshots:

- `verified-phone-header-360.png`: compact inline topbar and two-line title
  remain intact; Open task and overflow share the existing title row.
- `verified-phone-root-360.png` and `verified-phone-root-320.png`: all six
  groups are reachable in an inset card, with a visible separator and red
  Delete. The long task label wraps inside the surface.
- `verified-phone-deep-360.png`: the deepest workflow step page retains fixed
  Back/Close controls, bounds the long workflow title, and exposes the final
  long step within one internal scroll region.
- `verified-phone-delete-360.png`: the existing consent-gated destructive
  dialog is fully contained, with reachable Cancel and Delete controls.

Browser geometry assertions also cover 700px phones and an 820px coarse-pointer
transition, minimum 44px controls/rows, center-point hit testing, safe-area
insets and no document horizontal overflow. Escape/Back restores the proper
choice, backdrop dismissal returns focus, and actual held touch swipes update
the parent's inline counter before release and still work after menu use.

### Header spacing correction (2026-09-10)

User screenshot feedback exposed unequal center-to-center gaps: 38px from the
picker chevron to Open task, versus 48px from Open task to overflow. Increasing
the existing picker button's trailing padding gives both gaps the same 48px
spacing without shrinking or overlapping touch targets or adding a header row.

The existing narrow/long-label/swipe regression now checks rendered icon
alignment, equal phone spacing, minimum 44px targets, and non-overlapping real
center-point hits. It reproduced the 10px mismatch before the change. After
the fresh production E2E build, this command from `apps/web` passed one test
without retries, including 320px/360px phone geometry and 700px/820px
coarse-pointer alignment:

```bash
pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-threads-task-actions.spec.ts -- --grep 'contains nested choices' --retries=0
```

Nested long-label containment, Back/Escape, canceled deletion, backdrop focus
return and native swiping still pass in the same regression. No copy or action
logic changed.

### PR feedback: drawer control cursor (2026-09-10)

Greptile identified missing pointer cursors on the native Back and Close
buttons. Both now follow the existing web interactivity rule. The shipped
`MobilePickerSheet` remains the inset geometry/focus exemplar; no composition,
44px hitbox, navigation, mutation, or localization contract changed.

The existing rendered long-label regression now checks the computed cursor of
both controls. Its RED run reported Close as `default` instead of `pointer`.
After the two class changes and a fresh managed production build, all five
phone task-action tests passed with one worker and no retries:

```sh
pnpm e2e:run --host --project mobile-chrome tests/task/mobile-threads-task-actions.spec.ts -- --retries=0
pnpm exec vitest run --maxWorkers=2 components/task/task-management-drawer.test.tsx components/task/task-management-surface.test.tsx components/threads/thread-task-actions.test.tsx
pnpm exec eslint --max-warnings 0 components/task/task-management-drawer.tsx e2e/tests/task/mobile-threads-task-actions.spec.ts
pnpm run typecheck
```

The three focused component files passed five tests; ESLint and typecheck
passed. The new 360px nested-drawer rendering was inspected, including fixed
Back/Close controls, long labels, and the reachable last workflow step.

The shared GitLab link option also regained its pre-existing phone-specific
48px minimum after review found it missing from the options renderer. The
existing GitLab browser regression now asserts computed `min-height: 48px`;
RED measured 44px. A fresh build passed that test and all five Threads phone
action tests together (six tests, one worker, no retries):

```sh
pnpm e2e:run --host --project mobile-chrome tests/task/mobile-threads-task-actions.spec.ts tests/gitlab/mobile-gitlab-parity.spec.ts -- --grep 'Mobile GitLab parity.*links a GitLab MR from the visible task actions menu|^(?!.*Mobile GitLab parity)' --retries=0
```

After the parent merged, main-base integration passed all 15 phone cases below
against the freshly rebuilt backend/web pair. The shared control-sizing change
preserves the approved alignment, 44px targets, drawer containment, all six
actions, and native picker/swipe/focus behavior. One worker, no retries:

```sh
pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-threads-task-actions.spec.ts tests/task/mobile-threads-view.spec.ts tests/task/mobile-threads-swipe.spec.ts tests/gitlab/mobile-gitlab-parity.spec.ts -- --grep 'Mobile GitLab parity.*links a GitLab MR from the visible task actions menu|^(?!.*Mobile GitLab parity)' --retries=0
```
