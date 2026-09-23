---
id: "02-change-dialog"
title: "Shared change form and end-to-end behavior"
status: done
wave: 2
depends_on:
  - "01-change-contract"
plan: "plan.md"
requirements:
  - REQ-TASKS-CHANGE-WORKFLOW-001
  - REQ-TASKS-CHANGE-WORKFLOW-002
acceptance_criteria:
  - AC-TASKS-CHANGE-WORKFLOW-001.1
  - AC-TASKS-CHANGE-WORKFLOW-001.2
  - AC-TASKS-CHANGE-WORKFLOW-001.3
  - AC-TASKS-CHANGE-WORKFLOW-001.4
  - AC-TASKS-CHANGE-WORKFLOW-001.5
  - AC-TASKS-CHANGE-WORKFLOW-001.6
  - AC-TASKS-CHANGE-WORKFLOW-001.7
  - AC-TASKS-CHANGE-WORKFLOW-001.8
  - AC-TASKS-CHANGE-WORKFLOW-002.1
  - AC-TASKS-CHANGE-WORKFLOW-002.2
  - AC-TASKS-CHANGE-WORKFLOW-002.3
  - AC-TASKS-CHANGE-WORKFLOW-002.4
  - AC-TASKS-CHANGE-WORKFLOW-002.5
  - AC-TASKS-CHANGE-WORKFLOW-002.6
system_design:
  - ../../specs/tasks/system-design/change-workflow.md
---

# Task 02: Shared change form and end-to-end behavior

## Summary

Expose the guarded contract through one task-scoped form on desktop and phone.
Rename single-task entry points and prove destination mapping through real mock
workflow execution, preserving navigation and the existing bulk path.

## In scope

- Shared draft hook and responsive dialog, loaded destination details, profile
  defaults/reset, read-only conversation relationships, and mapping-aware preview.
- Card, sidebar, task detail/preview, management drawer, and command-palette entry.
- In-flight protection, stale-response handling, conflict refresh, and uncertain-result reconciliation.
- All six locales and generated Traditional Chinese values.
- Unit/component tests and updated desktop/mobile E2E interactions and page objects.
- Public how-to sections in `docs/public/tasks-and-workflows.md` and the action
  list in `docs/public/sessions-and-review.md`. Explain defaults, task-only mapping,
  old-map replacement, entry behavior, and recovery. Do not add a new public page.
- Reconcile affected current requirements/designs: workflow-agent-overrides,
  task-actions-menu, task-actions-menu-outcomes, task-menu-grouping, and sidebar
  action wording. Preserve historical completed plan results and stable IDs.

## Out of scope

Bulk mapping, profile editing, executor changes, and unrelated task menu redesign.

## Acceptance

1. Every single-task entry opens the shared form for the correct task. Destination,
   mapping, preview, validation, and cancel work without navigation or silent fallback.
2. Desktop and phone E2E prove saved mappings and actual later-step recipients,
   plus original task context. Mobile also proves visible entry, scroll containment,
   safe-area footer, focus return, and measured touch targets.
3. Existing move/bulk behavior remains covered. All copy is localized, public docs
   match behavior, and the listed checks pass after failing tests establish the change.

## ASCII UI preview

Use [the combined previews](plan.md#ascii-ui-preview), including UI-03 failure states.
These excerpts retain the same structural labels and apply to AC-001.1 through
AC-001.8 under `REQ-TASKS-CHANGE-WORKFLOW`.

UI-01: Desktop form.

```text
Change workflow                                      [X]
Current: Kanban / Review
Workflow [Feature v]        Destination step [Analysis v]
Workflow agents (this task only)
Implement, PR: [Agent B v] [Reset]
Analysis, Review: Use initial conversation
Previous workflow overrides will be replaced.
On entry: Reuse initial conversation. Model: <effective>
[>] Entry options               [Cancel] [Change workflow]
```

UI-02: Phone full-height form.

```text
Change workflow                 [X]    fixed header
-----------------------------------
Current: Kanban / Review
Workflow [Feature                v]
Destination step [Analysis       v]
Workflow agents (this task only)
Implement, PR
[Agent B                         v]
[Reset to workflow profile]
Analysis, Review: Initial conversation
Previous overrides will be replaced.
On entry: Reuse initial conversation
Model: <effective> [Details]
[>] Entry options                      scrolling body
-----------------------------------
[Change workflow]                      fixed footer
             safe area
```

Use the mobile-menu-sheet Drawer anatomy and responsive agent picker. Share the
hook, not viewport-dependent mutation logic. Keep 28px desktop controls and
44px minimum touch targets. Labels are illustrative and must use locale keys.

## Verification

Run from the repository root. If dependencies are absent, first run
`(cd apps && pnpm install --frozen-lockfile)` once.
The new test paths below are deliverables of this work order.

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/kanban/use-change-workflow.test.ts hooks/domains/kanban/use-workflow-move-preview.test.ts hooks/domains/kanban/use-workflow-move-preview-revision.test.ts components/task/change-workflow-dialog.test.tsx lib/api/domains/kanban-api.test.ts components/task-create-dialog-workflow-agent-overrides.test.ts components/kanban-card-menu-items.test.tsx components/kanban-card-menu-grouping.test.tsx hooks/use-task-actions-menu-move-targets.test.tsx components/task/task-move-context-menu.test.tsx components/task/task-switcher-context-menu.test.tsx components/task/task-actions-menu-dialogs.test.tsx components/task/task-management-drawer.test.tsx components/task/task-management-menu.test.tsx components/task/task-management-surface.test.tsx components/task-commands.test.tsx components/task-command-choices.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/task/change-workflow.spec.ts tests/kanban/cross-workflow-task-move.spec.ts tests/task/sidebar-send-to-workflow.spec.ts tests/task/task-menu-grouping.spec.ts tests/kanban/task-actions-menu-preview.spec.ts tests/kanban/task-actions-menu-detail.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-change-workflow.spec.ts tests/task/mobile-sidebar-task-actions.spec.ts tests/task/mobile-command-palette-task-actions.spec.ts tests/task/mobile-threads-task-actions.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The managed E2E runner builds fresh artifacts and enforces resource limits. Run
desktop and phone commands sequentially. Inspect screenshots from the new tests
against UI-01/UI-02 and record differences. Add exact targeted commands for any
additional changed suites discovered by searching the removed submenu interactions.
Run scoped ESLint on changed TS/TSX files and record the actual file command.

## Files likely touched

- `apps/web/components/task/change-workflow-dialog.tsx` (new, split presentation as needed).
- `apps/web/hooks/domains/kanban/use-change-workflow.ts` (new) and its test.
- `apps/web/lib/api/domains/kanban-api.ts` and its test.
- `apps/web/hooks/domains/kanban/use-workflow-move-preview.ts` and tests.
- `apps/web/components/task-create-dialog-workflow-agent-overrides.ts` for shared row extraction.
- `apps/web/components/kanban-card-menu-items.tsx`, `kanban-card-menu.tsx`.
- `apps/web/components/task/task-move-context-menu.tsx`, task switcher/menu/dialog files.
- `apps/web/hooks/use-task-actions-menu.ts` and task management hooks.
- `apps/web/components/task-command-items.tsx`, `task-command-choices.tsx`.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja}/` affected namespaces.
- E2E files listed above and their existing page objects.
- Public docs and owning specs named in scope; plan/work-order result sections.

## Dependencies

Task 01. Keep new contract types aligned with the implemented backend schema.

## Risks

Menus unmount on selection, so the form needs a durable host. Stale board snapshots
must not overwrite a new mapping. Profile grouping must preserve explicit recipient
semantics. Bulk menu builders share code with single-task entries.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/change-workflow.md), all criteria.
- [Design](../../specs/tasks/system-design/change-workflow.md), preview through mobile/failure sections.
- Existing create-task override and cross-workflow E2E helpers.
- `/mobile-parity`, `/e2e`, `/tdd`, `/docs-maintainer`, and scoped frontend guidance.

## Results

Completed on 2026-09-23.

- Backend targeted workflow-change tests passed across models, service, handlers,
  SQLite repository, and orchestrator. The environment-gated PostgreSQL case was
  skipped because `KANDEV_TEST_POSTGRES_DSN` is not set.
- The 17-file frontend test command passed 131 tests. The three terminology
  regression suites passed 38 tests; the task-switcher context-menu suite passed
  22 tests after its state refactor. `pnpm run typecheck` passed.
- `pnpm run i18n:zh-hant`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet`
  passed. All changed TS/TSX files passed scoped ESLint with no warnings and
  Prettier checks.
- `pnpm run build:vite` passed. The managed E2E runner also built the backend
  and Vite assets. Vite reported the existing deprecated `advancedChunks`,
  ineffective dynamic-import, and large-chunk warnings.
- Desktop managed E2E ran 27 tests: 26 passed, and the remaining long-step
  screenshot case passed after fixing its Playwright fixture signature. Mobile
  managed E2E also ran 27 tests: 26 passed, and the remaining long-step form
  case passed after correcting its scroll-owner check and dismissal sequence.
  The mapped desktop and phone flows, including saved later-step routing, and
  both final long-step cases passed after the picker portal change.
- The desktop selection also covered thread actions, command-palette actions,
  and bulk moves after locating those entry points during the stale-label scan:

  ```bash
  pnpm e2e:run --project chromium tests/task/change-workflow.spec.ts tests/kanban/cross-workflow-task-move.spec.ts tests/task/sidebar-send-to-workflow.spec.ts tests/task/task-menu-grouping.spec.ts tests/kanban/task-actions-menu-preview.spec.ts tests/kanban/task-actions-menu-detail.spec.ts tests/task/threads-task-actions.spec.ts tests/task/command-palette-task-actions.spec.ts tests/kanban/cross-workflow-task-batch.spec.ts
  pnpm e2e:run --project mobile-chrome tests/task/mobile-change-workflow.spec.ts tests/task/mobile-sidebar-task-actions.spec.ts tests/task/mobile-command-palette-task-actions.spec.ts tests/task/mobile-threads-task-actions.spec.ts
  ```
- Desktop and phone screenshots were inspected against UI-01/UI-02. The source
  assignment, selectors, task-only mapping, and scrollable form body are clear;
  the phone keeps its header and safe-area-aware footer fixed. Long picker rows
  render above the form, and the phone case reports no document overflow.
- Scoped ESLint command, run from `apps/web`:

  ```bash
  { git -C ../.. diff --name-only; git -C ../.. ls-files --others --exclude-standard; } | sort -u | sed 's#^apps/web/##' | rg '\.(ts|tsx)$' | xargs -r pnpm exec eslint
  ```
- Public-doc tests passed (62), all 47 published pages validated, the spec
  catalog validated (299 decisions and 1129 specifications), and full spec lint
  passed. `gofmt` and `git diff --check` are clean.
- No live task was migrated during implementation.

### Review follow-up: destination agent source rows

- The hook regression runs the real `useChangeWorkflow` derivation with workflow
  steps and proves that `Implement` and `PR` share profile A when PR targets
  Implement. It also proves initial-target and step-target rows with leftover
  profile IDs do not create extra sources or block submission.
- Before the fix, that regression exposed three rows: Implement/profile A,
  PR/stale profile, and Initial context/stale profile. It passed after retaining
  `session_target` in the step shape passed to the shared grouping helper.
- `pnpm exec vitest run hooks/domains/kanban/use-change-workflow.test.ts`,
  `pnpm run typecheck`, scoped ESLint, and Prettier checks passed.
- The strengthened desktop cross-workflow test asserts that the preview
  predicts a new session with the replacement profile and `mock-slow` model
  before submit, then confirms the committed session uses profile B. The focused
  Chromium test and existing phone mapping test both passed in the managed
  runner:

  ```bash
  pnpm e2e:run --project chromium tests/task/change-workflow.spec.ts -- --grep "maps a task profile for a later destination step"
  pnpm e2e:run --project mobile-chrome tests/task/mobile-change-workflow.spec.ts
  ```
- For PR screenshots, reran those desktop and phone scenarios with
  `CAPTURE_PR_ASSETS=1`; both passed 1/1 with fresh builds. The captured assets
  were inspected and compressed before publication:

  ```bash
  CAPTURE_PR_ASSETS=1 pnpm e2e:run --project chromium tests/task/change-workflow.spec.ts -- --grep "maps a task profile for a later destination step"
  CAPTURE_PR_ASSETS=1 pnpm e2e:run --project mobile-chrome tests/task/mobile-change-workflow.spec.ts
  ```
