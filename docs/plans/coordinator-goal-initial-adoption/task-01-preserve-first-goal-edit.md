---
id: "01-preserve-first-goal-edit"
title: "Preserve the first goal edit"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-GOALS-001
acceptance_criteria:
  - AC-COORDINATOR-GOALS-001.1
  - AC-COORDINATOR-GOALS-001.2
  - AC-COORDINATOR-GOALS-001.3
  - AC-COORDINATOR-GOALS-001.5
  - AC-COORDINATOR-GOALS-001.9
  - AC-COORDINATOR-GOALS-001.10
system_design:
  - ../../specs/coordinator/system-design/goals.md
---

# Task 01. Preserve the first goal edit

## Summary

Initialize the goal draft and its saved snapshot before React paints the
editable Goal section. A manager can enter a name at the first opportunity
and submit it with **Set goal**.

## Scope

- Add the component case `keeps the first goal name edit after load`. Hold the
  initial empty goal response and observe the first enabled `goal-name` input.
  Dispatch the input event with the native value setter from the observer.
  Let the scheduler process the event and adoption before checking the value.
  Use real scheduling for this window and restore `IS_REACT_ACT_ENVIRONMENT`
  and disconnect the observer on cleanup. Check that `goal-set` is enabled and
  that PUT receives `{ name: "Ship", due_on: null, criteria: [] }`.
- Add `keeps goal text entered as the first load finishes` to the desktop and
  phone coordinator settings specs. Share the browser probe between them.
  Hold the first real GET until the loading skeleton is visible. Arm the DOM
  observer before releasing the response. Enter the name from the observer
  and capture the value and button state after paint. Arm `waitForHttp` before
  clicking **Set goal**. Check the PUT body, success status, and active goal's
  displayed name. Release the request gate and remove the route on cleanup.
- Run the component and browser cases before the repair. Expect the name to
  be empty and the button to be disabled. Move adoption to `useLayoutEffect`
  and rerun the checks. Keep the first-edit stimulus tied to the form commit.

### Scope boundaries

The affected entry point is the Goal section of coordinator settings. The
[guided setup draft](../../specs/coordinator/system-design/coordinators.md#guided-setup)
and [goal routes](../../specs/coordinator/system-design/goals.md#routes) have
their own contracts. The editor uses its current shared form fields.

## Acceptance

- Text entered when the form first becomes editable stays visible. **Set goal**
  enables and submits the entered name in desktop and phone browsers.
- An existing goal starts with its stored draft and saved snapshot. The Goal
  section tests pass for saving an active goal, toggles, and conflict reload.

## ASCII UI preview

### UI-01 Goal name after the first edit

The [plan preview](plan.md#ui-01-goal-name-after-the-first-edit) shows the
same state in context. This state applies to desktop and phone under
`AC-COORDINATOR-GOALS-001.9`.

```text
Milestone
[ Ship                  ]
[ Set goal ] enabled
```

## Verification

Run from the repository root. In a fresh worktree, first install the workspace
dependencies from `apps/` with `pnpm install --frozen-lockfile`. Run the two
browser commands in order. The desktop command builds the app, and the phone
command uses that build. Repeat the checks after the hook change.

```bash
pnpm --dir apps/web exec vitest run components/coordinators/sections/goal-section.test.tsx -t 'keeps the first goal name edit after load'
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/coordinator/configure-sections.spec.ts -- --grep 'keeps goal text entered as the first load finishes'
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --shards 1 --project mobile-chrome tests/coordinator/mobile-configure-sections.spec.ts -- --grep 'keeps goal text entered as the first load finishes'
```

After the repair, run the full Goal section component suite and lint the
changed web files.

```bash
pnpm --dir apps/web exec vitest run components/coordinators/sections/goal-section.test.tsx
pnpm --dir apps/web exec oxlint -c oxlint.config.mjs --deny-warnings components/coordinators/sections/use-goal-editor.ts components/coordinators/sections/goal-section.test.tsx e2e/tests/coordinator/goal-adoption-fixture.ts e2e/tests/coordinator/configure-sections.spec.ts e2e/tests/coordinator/mobile-configure-sections.spec.ts
```

## Files likely touched

- `apps/web/components/coordinators/sections/use-goal-editor.ts`
- `apps/web/components/coordinators/sections/goal-section.test.tsx`
- `apps/web/e2e/tests/coordinator/goal-adoption-fixture.ts`
- `apps/web/e2e/tests/coordinator/configure-sections.spec.ts`
- `apps/web/e2e/tests/coordinator/mobile-configure-sections.spec.ts`

## Dependencies

This is the only work order in the plan.

## Risks

- If the test waits for passive effects before entering text, it misses the
  defect. Check the failing assertion before editing the hook.

## Parallelism

Sequential.

## Inputs

- [Goal requirements](../../specs/coordinator/requirements/goals.md#req-coordinator-goals-001-setting-a-goal)
- [Goal UI design](../../specs/coordinator/system-design/goals.md#goal-ui)
- [Root cause and evidence](plan.md#root-cause-and-evidence)

## Results

- **RED:** The component, Chromium, and `mobile-chrome` first-edit regressions
  observed an empty name and disabled **Set goal** before the hook change.
- **GREEN:** The Goal section component suite passed all 17 tests, including
  cleanup coverage with an unresolved probe promise.
- **GREEN:** The managed Chromium run rebuilt the web app and passed the new
  browser case. The subsequent `mobile-chrome` run passed the phone case.
- **GREEN:** `oxlint` passed for the five changed web files. Prettier and
  `git diff --check` passed.
- **Review remediation:** The first-edit test registers observer disconnection
  and act-environment restoration with `onTestFinished` before changing the
  act flag, so timeout cleanup does not depend on the probe promise settling.
