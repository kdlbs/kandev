---
spec: docs/specs/tasks/requirements/task-dependencies-create-dialog-advanced-settings.md
created: 2026-08-13
status: implemented
---

# Implementation Plan: Task-create advanced settings disclosure

## Overview

Wrap the existing task-create dependency selector in a compact, collapsed
advanced-settings disclosure. Keep dependency state, selector behavior, and
the `blocked_by` payload unchanged while reducing the default form height and
leaving a contained region for future uncommon create options.

This is a frontend-only presentation change. It builds on the implemented
dependency selector refinement in
`docs/specs/tasks/requirements/task-dependencies-create-dialog-dependency-selector.md` and does
not change the backend, API client, persistence, or WebSocket contracts.

## Frontend composition

### Advanced settings component

- Add a focused `TaskCreateAdvancedSettings` component in
  `apps/web/components/task-create-dialog-advanced-settings.tsx`.
- Use the existing `@kandev/ui/collapsible` primitive. Keep the disclosure
  uncontrolled or locally controlled with a collapsed initial value so its
  open state is presentation-only and does not enter `DialogFormState`.
- Render a semantic trigger with localized `Advanced settings` copy, muted
  styling, a very subtle text size, a direction indicator, `aria-expanded`,
  and a mobile hitbox of at least 44 CSS pixels.
- Render the existing `TaskCreateDependencies` in a responsive option grid
  with the localized `Depends on` label and contextual help to the left of the
  selector in the same column. Use two columns on desktop so future advanced options can
  share a row, and collapse to one column on narrow screens. Keep the content
  wrapper ready for future sibling controls without introducing a speculative
  registry or new state shape.
- Preserve the dependency selector's `value`, `onChange`, and disabled props.

### Dialog placement

- Update `apps/web/components/task-create-dialog.tsx` so the advanced section
  renders at the bottom of the create-task form, below the model, executor, and
  workflow selector controls.
- Keep `renderWorkflowSection` and all workflow visibility, locking, and
  single-workflow override behavior unchanged. Refactor the old paired
  workflow/dependency wrapper into a workflow-only render path if needed, then
  mount the advanced section where the dependency slot was previously shown.
- Keep the advanced component out of session mode, edit mode, and started-task
  rendering.
- Keep the dialog body as the only outer form scroll owner. Do not add a nested
  scrolling container around the disclosure.

### Localization

- Add the new trigger label to
  `apps/web/src/locales/en/task.json`.
- Regenerate or update `apps/web/src/locales/pseudo/task.json` using the
  repository's i18n tooling.
- Add other locale values only through the repository's established catalog
  workflow. Do not hardcode the label in JSX or tests that exercise translated
  rendering.

## Tests

### Component tests

- Add `apps/web/components/task-create-dialog-advanced-settings.test.tsx` for
  the default collapsed state, expanded state, semantic attributes, touch-safe
  trigger class, dependency label/help, dependency mounting, and state
  preservation across collapse and reopen.
- Update
  `apps/web/components/task-create-dialog-form-body.test.tsx` to cover the
  workflow-only path and ensure the advanced dependency section does not remove
  workflow visibility rules.
- Keep the existing
  `apps/web/components/task-create-dialog-dependencies.test.tsx` coverage
  green. It remains the source of truth for selector internals.
- Preserve existing payload tests for multiple `blockedBy` IDs and the empty
  array case.

## E2E tests

- Extend `apps/web/e2e/tests/task/create-task-dependency-selector.spec.ts` to
  assert that the create dialog starts with a collapsed advanced row below the
  model, executor, and workflow controls, the dependency trigger is hidden
  until expansion, the labeled setting help is available, and the existing
  selection and create-payload behavior remains intact.
- Extend `apps/web/e2e/tests/task/mobile-create-task-dependency-selector.spec.ts`
  to tap the advanced row before opening the dependency selector, check the
  row's touch box and expanded state, and retain picker containment, help,
  touch selection, and horizontal-overflow assertions.
- Use existing managed `chromium` and `mobile-chrome` projects. No container
  or backend behavior is required for this presentation change beyond the
  existing task-create fixture.

## Implementation waves

Wave 1 (sequential):

- [x] [Task 01: Add the advanced settings disclosure](task-01-advanced-settings-ui.md)

Wave 2 (sequential, depends on Task 01):

- [x] [Task 02: Verify disclosure behavior on desktop and mobile](task-02-advanced-settings-e2e.md)

No task is parallel-safe because the browser selectors and placement depend on
the final component composition.

## Verification commands

The implementation task owns focused component, typecheck, lint, i18n, and
diff checks. The E2E task owns the managed desktop and mobile browser runs.
The exact commands are recorded in the individual task files.

## Open questions

None. The disclosure is intentionally limited to presentation state, and the
existing dependency selector remains the only advanced option in this change.

## Verification results

- Component tests: 40 tests passed across the advanced-settings, form-body,
  dependency-selector, and helper suites.
- TypeScript typecheck, focused ESLint, `i18n:check`, `i18n:ratchet`, and
  `git diff --check` passed. `i18n:check` still reports the repository's
  existing advisory pt-pt and zh-cn catalog parity findings.
- Managed Chromium E2E passed for collapsed-by-default rendering, expansion,
  dependency selection, clearing, help, search, and persistence.
- Managed mobile-chrome E2E passed for touch expansion, the 44 CSS pixel
  disclosure hitbox, picker containment, help, selection, and horizontal
  overflow.
