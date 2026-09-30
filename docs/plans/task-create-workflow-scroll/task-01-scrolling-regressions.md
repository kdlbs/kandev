---
id: "01-scrolling-regressions"
title: "Protect workflow picker scrolling"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-CREATE-WORKFLOW-STEPS-001
acceptance_criteria:
  - AC-TASKS-CREATE-WORKFLOW-STEPS-001.5
  - AC-TASKS-CREATE-WORKFLOW-STEPS-001.6
  - AC-TASKS-CREATE-WORKFLOW-STEPS-001.7
system_design:
  - ../../specs/tasks/system-design/task-create-workflow-step-previews.md
---

# Task 01: Protect workflow picker scrolling

## Summary

Add permanent browser coverage for the workflow menu reported in issue #4073.
PR #4058 already corrected the production menu. This work order protects that behavior with ten-workflow native-input tests.

## In scope

- Add desktop case `scrolls ten workflow options in both directions without losing the task draft`.
- Add phone case `touch scrolls ten workflow options and selects either end`; prove real movement down and up in the same loaded picker.
- Seed ten custom workflows, each with at least fifteen steps and representative
  long stage names. Wait for loaded previews and assert wrapping without
  horizontal overflow.
- Assert native input scrolling, end-option reachability, viewport bounds, keyboard reachability, focus return, and draft preservation.
- On each picker reopen, await every seeded workflow's response and rendered ordered steps before keyboard, scrolling, geometry, or hit-test assertions.
- Keep common geometry and Chromium touch helpers in the existing helper file.

## Out of scope

Production components, picker redesign, workflow loading changes, new dependencies, localization, release publication, and issue closure.

## Acceptance

1. Desktop wheel input reaches both ends at 1682x768 and 1280x600. Small and diagonal deltas work over step content. Keyboard focus reveals end options. Selection preserves the draft and updates the workflow.
2. Phone touch gestures at 390x640 reach both ends in one loaded picker session. The upward gesture starts at a positive scroll position and must move the list upward. Each claimed direction performs at least one gesture with observed movement. End options receive real pointer hits and remain selectable with 44px targets.
3. Both suites prove actual overflow, viewport containment, and unchanged selection during scrolling. Existing preview cases continue to pass.

The applicable criteria are AC-001.5, AC-001.6, and AC-001.7, with the full `AC-TASKS-CREATE-WORKFLOW-STEPS` prefix.

## ASCII UI preview

[Full previews and annotations](plan.md#ascii-ui-preview).

UI-01: Desktop picker, task dialog open, overflow state:

```text
+-------------------------------------+
| Workflow                            | <- fixed
|-------------------------------------|
| Workflow 01                         |
|   Start > Implement > Review        |
| ...                            [|]  | <- wheel up/down
+-------------------------------------+
[Selected workflow v]  [Launch step]
```

UI-02: Phone picker, task dialog open, overflow state:

```text
+------------------------------+
| Workflow                     | <- fixed
|------------------------------|
| Workflow 01                  |
|   Start > Implement >        |
|   Review                     |
| ...                     [|]  | <- touch up/down
+------------------------------+
[Selected workflow v]
```

AC-001.6 and AC-001.7 require viewport containment, one vertical scroll owner, wrapped previews, and reachable options. Example names and spacing are illustrative. The work order retains the current composition.

## Verification

Run from the repository root. If dependencies are absent, run `(cd apps && pnpm install --frozen-lockfile)` once first.

```bash
(cd apps/web && pnpm exec vitest run hooks/use-workflow-option-previews.test.ts components/workflow-selector-row.test.tsx)
(cd apps/web && pnpm exec eslint e2e/tests/task/task-create-workflow-step-previews.spec.ts e2e/tests/task/mobile-task-create-workflow-step-previews.spec.ts e2e/tests/task/workflow-step-previews-helpers.ts)
(cd apps/web && pnpm e2e:run --host --project chromium tests/task/task-create-workflow-step-previews.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/task/mobile-task-create-workflow-step-previews.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run E2E commands sequentially. Each managed run builds current assets and owns its isolated instance. Record every command and actual result.

## Files likely touched

- `apps/web/e2e/tests/task/task-create-workflow-step-previews.spec.ts`.
- `apps/web/e2e/tests/task/mobile-task-create-workflow-step-previews.spec.ts`.
- `apps/web/e2e/tests/task/workflow-step-previews-helpers.ts`.
- This work order and sibling `plan.md` for status and results.

## Dependencies

PR #4058, already merged as `ff917a370ac`. No prior pending work order.

## Risks

- Programmatic scrolling does not verify wheel or touch event delivery.
- Focus changes can mask missing input scrolling. Measure the settled baseline first.
- CDP touch input is Chromium-specific. Detach the session in `finally` and keep gesture coordinates inside the option list.

## Parallelism

`sequential`. No delegation is authorized.

## Inputs

- [Requirement](../../specs/tasks/requirements/task-create-workflow-step-previews.md), AC-001.5 through AC-001.7.
- [Design](../../specs/tasks/system-design/task-create-workflow-step-previews.md), Responsive behavior and Vertical scrolling.
- [Plan evidence](plan.md#evidence-and-root-cause).
- Existing workflow preview tests and `seedWorkflowStepPreviewScenario` with `extraWorkflowCount: 7`.
- `apps/web/components/workflow-selector-row.tsx`, `WorkflowSelectorOptionList`.
- `apps/packages/ui/src/popover.tsx` and the task dialog portal context.
- Scoped web guidance and the `/e2e` and `/mobile-parity` skills.

## Results

Complete. The phone test now swipes down to the bottom and swipes up from that nonzero position while the same picker remains open. The boundary helper requires a gesture and observed movement for each direction, so an existing boundary cannot satisfy the test. The case checks that selection and the task draft remain unchanged while scrolling, then selects each endpoint.

Desktop keyboard navigation and final-option selection, plus the phone's final-option selection after reopening, now arm waits before reopening. Each phase awaits all seeded workflow responses and retries ordered-content assertions until all preview steps render before keyboard, scrolling, geometry, or hit-testing checks.

The opt-in scenario now seeds ten owned workflows with fifteen representative long stages each. Existing preview tests retain their smaller fixtures. No production component changes were needed.

Verification from the repository root:

- `(cd apps/web && pnpm exec vitest run hooks/use-workflow-option-previews.test.ts components/workflow-selector-row.test.tsx)`: 2 files and 15 tests passed.
- `(cd apps/web && pnpm exec eslint e2e/tests/task/task-create-workflow-step-previews.spec.ts e2e/tests/task/mobile-task-create-workflow-step-previews.spec.ts e2e/tests/task/workflow-step-previews-helpers.ts)`: passed.
- `(cd apps/web && pnpm e2e:run --host --project chromium tests/task/task-create-workflow-step-previews.spec.ts)`: managed backend, Vite asset, and fixture-plugin builds passed; 2 tests passed.
- `(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/task/mobile-task-create-workflow-step-previews.spec.ts)`: managed backend, Vite asset, and fixture-plugin builds passed; 2 tests passed.
- Review remediation rerun of the targeted ESLint command above: passed.
- Review remediation rerun of the managed desktop command above: backend, Vite asset, and fixture-plugin builds passed; 2 tests passed.
- Review remediation rerun of the managed phone command above: backend, Vite asset, and fixture-plugin builds passed; 2 tests passed.
- `python3 scripts/list-docs.py validate`: validated 333 decisions and 1260 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.

During implementation, the new fixture first exposed its hard-coded `Backlog` starting-step assumption, and the short desktop viewport showed that a long end option can be taller than the list. The fixture now selects its configured first stage, and end-option checks measure the visible intersection and verify a real hit within it.

Review remediation closed the false-positive upward-scroll path and the preview-loading race on reopened pickers. The final managed test runs above passed.
