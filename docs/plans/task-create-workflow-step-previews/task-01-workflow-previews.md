---
id: "01-workflow-previews"
title: "Load and render complete workflow previews"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-CREATE-WORKFLOW-STEPS-001
acceptance_criteria:
  - AC-TASKS-CREATE-WORKFLOW-STEPS-001.1
  - AC-TASKS-CREATE-WORKFLOW-STEPS-001.2
  - AC-TASKS-CREATE-WORKFLOW-STEPS-001.3
  - AC-TASKS-CREATE-WORKFLOW-STEPS-001.4
  - AC-TASKS-CREATE-WORKFLOW-STEPS-001.5
  - AC-TASKS-CREATE-WORKFLOW-STEPS-001.6
system_design:
  - ../../specs/tasks/system-design/task-create-workflow-step-previews.md
---

# Task 01: Load and render complete workflow previews

## Summary

Load task-create option previews from the step API when the selector opens.
The loader, UI states, and targeted tests are complete as one vertical slice.

## In scope

- First add the failing partial-cache regression named in the plan. Run it red
  before production edits, then implement through TDD.
- Add the local scoped hook and optional selector prop from the design.
- Render independent status feedback and sibling retry controls. Keep one
  translated status region, stable option names, keyboard retry focus, and
  workflow metadata.
- Add hook, component, desktop, and phone tests from the plan's scenario matrix.
- Use existing localized keys for preview states and add one translated status
  announcement frame. Add brief public guidance on preview loading and retry.

## Out of scope

Backend changes, workflow mutation, launch resolution, selection memory,
automation loading changes, and task snapshot/cache restructuring.

## Acceptance

1. All option previews load without board cache dependencies; mixed outcomes
   and retries satisfy AC-001.1 through AC-001.3.
2. Scope changes and closing reject old results. Metadata, existing selection,
   launch behavior, and automation fallback satisfy AC-001.4 and AC-001.5.
3. Desktop and phone rendering, localized states, and causal E2E checks satisfy
   AC-001.6. Every listed command passes before marking this order done.

The AC prefix is `AC-TASKS-CREATE-WORKFLOW-STEPS` throughout this work order.

## ASCII UI preview

[Full previews and annotations](plan.md#ascii-ui-preview).
UI-01: Desktop option states; AC-001.1 through AC-001.6:

```text
Feature [selected]
  Analysis > Implement > Review > Done
Review workflow
  Loading steps...
Failed workflow
  Could not load steps.  [Retry]
Empty workflow
  No steps in this workflow
```

UI-02: Phone option states; same ACs:

```text
+------------------------------+
| Feature [check]              |
|   Analysis > Implement >     |
|   Review > Done              |
| Failed workflow              |
|   Could not load steps.      |
|   [Retry]                    |
+------------------------------+
```

Keep the click picker contained. The list owns vertical scrolling; step groups
wrap. Selection and retry are separate controls with phone/coarse-pointer
hit areas of at least 44px. Compare rendered results with these structures.

## Verification

Run from the repository root. Install dependencies once if this worktree has
no `apps/node_modules`: `(cd apps && pnpm install --frozen-lockfile)`.
All new paths below must exist before final verification.

```bash
(cd apps/web && pnpm exec vitest run hooks/use-workflow-option-previews.test.ts components/workflow-selector-row.test.tsx components/task-create-dialog-form-body.test.tsx components/task-create-dialog-launch-preview.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint hooks/use-workflow-option-previews.ts components/workflow-selector-row.tsx components/task-create-dialog-form-body.tsx components/task-create-dialog.tsx)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/task/task-create-workflow-step-previews.spec.ts tests/task/create-task.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-task-create-workflow-step-previews.spec.ts tests/task/mobile-create-task-launch-preview.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Managed E2E commands build current assets. Do not overlap suites or override
worker limits. Record discovered test counts and results. If implementation
changes another test suite, add its exact command before marking completion.

## Files likely touched

- `apps/web/hooks/use-workflow-option-previews.ts` and `.test.ts` (new).
- `apps/web/components/workflow-selector-row.tsx` and `.test.tsx`.
- `apps/web/components/task-create-dialog.tsx`.
- `apps/web/components/task-create-dialog-form-body.tsx` and `.test.tsx`.
- `apps/web/e2e/tests/task/task-create-workflow-step-previews.spec.ts` (new).
- `apps/web/e2e/tests/task/mobile-task-create-workflow-step-previews.spec.ts` (new).
- `apps/web/e2e/tests/task/workflow-step-previews-helpers.ts` (new shared E2E setup and assertions).
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja}/workflows.json`.
- `docs/public/tasks-and-workflows.md`.
- This plan, work order, and paired requirement/design lifecycle fields.

## Dependencies

None. Use existing API and UI primitives. No new package is needed.

## Risks

See the plan's risk list. In particular, never insert step-only data into the
Kanban snapshot store and never nest retry inside a selection button.

## Parallelism

`sequential`. This package does not authorize delegation.

## Inputs

- [Requirements](../../specs/tasks/requirements/task-create-workflow-step-previews.md).
- [System design](../../specs/tasks/system-design/task-create-workflow-step-previews.md).
- Existing `hooks/use-workflow-steps.ts` and
  `hooks/domains/kanban/use-change-workflow-data.ts` for request lifecycle patterns.
- `components/task-create-dialog-effects.ts` and
  `components/task-create-dialog-prop-builders.ts` for the separate launch path.
- Existing desktop/mobile launch-preview E2E suites and `e2e/helpers/causal-waits.ts`.
- Scoped web guidance, `/tdd`, `/e2e`, `/mobile-parity`, and `/docs-maintainer`.

## Results

Completed. All six acceptance criteria are covered. Existing localized keys
provide the loading, empty, failure, and retry copy in all supported languages.
A new status-announcement frame is translated in all supported languages. See
the plan's verification results for test counts and commands.

Review remediation is complete. The picker wraps unbroken step names inside the
step group. Retry sizing uses the standard desktop control size and applies the
48px minimum on coarse pointers. Accessibility behavior keeps option names
stable, announces row states in one translated region, and retains keyboard
focus during retry. Successful retry returns focus to the workflow option, and
Escape returns focus to the selector trigger on phone. The plan records browser
geometry and focused verification.
