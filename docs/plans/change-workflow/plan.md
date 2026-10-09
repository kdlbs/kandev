---
created: 2026-09-23
status: done
requirements:
  - REQ-TASKS-CHANGE-WORKFLOW-001
  - REQ-TASKS-CHANGE-WORKFLOW-002
system_design:
  - ../../specs/tasks/system-design/change-workflow.md
legacy_specs: []
---

# Change workflow implementation plan

## Overview

Replace the single-task Send to workflow submenu with a shared form. Users choose
a destination step and task-specific workflow agents before one atomic transition.
Deliver the guarded move and draft preview contract first. Then connect all
single-task entry points and prove desktop and phone behavior with mock agents.

## Intent and assumptions

The user requested the rename, agent mapping, and related improvements. Source
inspection confirms that cross-workflow moves, agent overrides, and entry previews
already exist. The chosen improvements are explicit step selection, shared entry
points, replacement/reset controls, error recovery, and entry feedback.

The design preserves manual-move lifecycle behavior, including the HTTP endpoint's
active-primary exception. The earlier feasibility answer overstated its idle gate.
Replacement-map reset and bulk separation are design choices within the user's
request for improvements. The form explains reset before submission.

## Scope

In scope: normal tasks, same workspace, fixed-profile replacements, grouped
selectors, read-only recipient relationships, atomic persistence, and six locales.
The task ID, conversation history, and workspace resources remain attached.

Out of scope: bulk mapping, Office conversion, cross-workspace moves, Initial
Agent replacement, executor changes, automatic undo, and MCP/plugin expansion.
No live task is migrated while preparing or implementing this package.

## Technical approach

The [requirements](../../specs/tasks/requirements/change-workflow.md) own behavior.
The [design](../../specs/tasks/system-design/change-workflow.md) owns the proposed
`workflow_change` object, source/version guard, candidate-map preview, and routing.

Task 01 extends the existing HTTP/WS adapters, `MoveTaskWithOptions`, profile
validation, transactional admission, and `PreviewWorkflowMove`. Existing callers
without the object retain their behavior. The persisted override column is reused.

Task 02 adds a shared form and domain hook, reuses create-form mapping derivation,
updates menus/command palette, and extends the move-preview hook's request key.
It includes frontend unit tests, desktop/mobile E2E, locales, and public guidance.
Both work orders use TDD. UI E2E failures are established before changing the UI.

## Companion packages

- `docs/plans/task-workflow-agent-overrides/`: completed create-time mapping foundation.
- `docs/plans/workflow-move-preview/`: implemented advisory preview foundation.
- `docs/plans/workflow-session-targeting/`: completed explicit-recipient foundation.
- `docs/plans/task-menu-grouping/`: completed grouping foundation.

Keep their recorded results as historical evidence. Link this extension from the
current owning specifications. Do not reopen completed work orders or claim their
old test counts prove this change. Task 02 reconciles single-task submenu wording;
this package owns the new scenarios and results.

## ASCII UI preview

### UI-01: Desktop, task menu and populated form

Entry: task card, sidebar, detail, preview, or command palette.

```text
Move to >
Change workflow...

+------------------------------------------------------+
| Change workflow                                  [X] |
| Current: Kanban / Review                             |
| Workflow          [Feature                        v] |
| Destination step  [Analysis                       v] |
|                                                      |
| Workflow agents                                      |
| Changes apply only to this task.                      |
| Implement, PR   Default: Agent A                      |
|                 [Agent B              v] [Reset]      |
| Analysis, Review: Use initial conversation            |
|                                                      |
| Previous workflow agent overrides will be replaced.  |
| On entry: Reuse initial conversation                  |
| Model: <effective model>           [Details]           |
| Step prompt will run.                                |
| [>] Entry options                                    |
|                         [Cancel] [Change workflow]    |
+------------------------------------------------------+
```

Agent/step names and preview outcome are illustrative. Actual workflow data owns
relationships. Do not hardcode Feature, Analysis, or a provider profile.

### UI-02: Phone, full-height change form

Entry: visible task overflow action. Header and footer stay fixed.

```text
+--------------------------------------+
| Change workflow                  [X] |
+--------------------------------------+
| Current: Kanban / Review             |
| Workflow                            |
| [Feature                          v] |
| Destination step                    |
| [Analysis                         v] |
|                                     |
| Workflow agents                     |
| Changes apply only to this task.    |
| Implement, PR                       |
| Default: Agent A                    |
| [Agent B                          v] |
| [Reset to workflow profile]         |
| Analysis, Review                    |
| Use initial conversation            |
|                                     |
| Previous overrides will be replaced.|
| On entry: Reuse initial conversation|
| Model: <effective model> [Details]  |
| [>] Entry options                   |
|             (scrolling body)        |
+--------------------------------------+
| [Change workflow]                   |
|              safe area              |
+--------------------------------------+
```

Reuse the inset full-height Drawer anatomy of `mobile-menu-sheet.tsx`, with one
body scroll owner. Source context, field order, relationship labels, and a fixed
primary action are structural requirements. Spacing is illustrative. Use 28px
desktop controls and at least 44px touch targets; do not enlarge desktop controls.
Map these previews to AC-001.1 through AC-001.8 under `REQ-TASKS-CHANGE-WORKFLOW`.

### UI-03: Loading, empty, invalid, and conflict states

These states occupy the same form on both viewports.

```text
Workflow data: Loading...                 [Change disabled]
No steps in this workflow.                [Change disabled]
No fixed workflow agents.                 [Change enabled*]
Agent B is unavailable. [Choose agent]    [Change disabled]
Preview unavailable. [Retry]              [Change enabled*]
Task changed elsewhere. [Refresh]         [Change disabled]
* A valid workflow, step, and mapping are still required.
```

Failed data loads show Retry and preserve selections. A conflict refreshes the
source version before another submit. Preview failure alone remains advisory.
Map UI-03 to AC-001.2, .5, .6, .8 and AC-002.5.

## Tests

Test names below are proposed additions. Each maps to the full
`AC-TASKS-CHANGE-WORKFLOW-` prefix.

| Acceptance | File / test evidence |
| --- | --- |
| 001.2-.4, 002.2 | `internal/task/service/change_workflow_test.go`: `TestWorkflowChangeValidation`, table cases for scope, fixed membership, unavailable defaults/replacements, executor mismatch, and explicit targets |
| 002.1-.3, .5 | `internal/task/repository/sqlite/change_workflow_test.go`: `TestWorkflowChangeAtomicWrite`, `TestWorkflowChangeConflict`, `TestWorkflowChangePreservesTaskContext`; include rollback, reload, queued admission, and write-time races |
| 002.2, .5-.6 | `internal/task/handlers/change_workflow_test.go`: `TestWorkflowChangeAdapters`; HTTP/WS malformed, unauthorized, defaults, legacy omission, duplicate/stale request |
| 001.6, 002.4-.6 | `internal/orchestrator/change_workflow_test.go`: `TestWorkflowChangePreview`, `TestWorkflowChangeRouting`; candidate map, no preview writes, current/other/new/unknown recipient, entry policy, WIP promotion, stale source completion, future missing profile |
| 001.2-.5, .8 | `hooks/domains/kanban/use-change-workflow.test.ts`: defaults/reset, unavailable rows, workflow switch, load failure, task identity freeze, stale/uncertain submit recovery |
| 001.6 | `hooks/domains/kanban/use-workflow-move-preview.test.ts`: mapping-sensitive key, late responses, no stale preview, bounded requests |
| 001.1, .7-.8, 002.6 | `components/task/change-workflow-dialog.test.tsx` and existing menu/command tests: shared trigger, cancel, focus, duplicate submit, bulk separation |

## E2E tests

Use existing isolated mock-agent fixtures, causal network/event waits, and API
assertions. New files are explicitly planned, not existing evidence.

| File and project | Scenario / acceptance |
| --- | --- |
| `tests/task/change-workflow.spec.ts`, chromium | Kanban to Feature/Analysis, grouped custom Implement/Review profiles, actual session identities through later steps, PR reuse, unchanged task/plan/worktree, reload; 001.1-.6, .8 and 002.1-.4 |
| Same file | Initial/earlier-step labels, no-profile workflow, invalid profile, cancel, slow/stale response, conflict/uncertain result, active primary and blocking sibling; 001.4-.6, .8 and 002.5 |
| `tests/task/mobile-change-workflow.spec.ts`, mobile-chrome | Overflow entry, select destination and replacement, move, reload; long labels, contained scroll, safe-area footer, measured targets, focus return; 001.1-.8 |
| Existing cross-workflow, sidebar, menu-grouping, preview/detail and mobile action suites | Update old submenu interactions and retain navigation, single-task subject, same-workflow moves, and bulk behavior; 001.1, .8 and 002.6 |

## Work orders

- [x] [Task 01: Guarded workflow change and preview](task-01-change-contract.md) (done)
- [x] [Task 02: Shared change form and end-to-end behavior](task-02-change-dialog.md) (done)

Run sequentially: 01 then 02. No subagents are authorized. Each work order contains
commands rooted independently in the repository. Bootstrap dependencies once when
the workspace has no install. Do not overlap E2E runs.

## Verification results

Design validation completed on 2026-09-23:

- `python3 scripts/list-docs.py validate`: passed, 299 decisions and 1129 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs docs/plans/change-workflow`: passed.
- Work-order acceptance IDs, local links, and existing unit-test paths: verified.
- Catalog discovery includes both new task-system documents.

Task 01 implementation verification:

- Targeted backend workflow-change tests: passed across models, service,
  handlers, SQLite repository, and orchestrator.
- PostgreSQL variant: added, but skipped because `KANDEV_TEST_POSTGRES_DSN` is
  not set in this environment.
- `python3 scripts/list-docs.py validate`: passed, 299 decisions and 1129 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.

Task 02 implementation verification:

- Backend targeted workflow-change tests passed in models, service, handlers,
  SQLite repository, and orchestrator; PostgreSQL coverage was skipped because
  `KANDEV_TEST_POSTGRES_DSN` is unset.
- Frontend unit suites passed, including 131 planned tests and 38 terminology
  regression tests. Typecheck, scoped ESLint, Prettier, both i18n gates, and the
  production Vite build passed.
- Desktop and mobile managed E2E selections each ran 27 tests. Each initial run
  had one test-only failure, and the corrected test passed in a focused rerun.
  Final desktop and phone mapped flows and long-step picker checks passed.
- Public docs validation passed for all 47 pages; the spec catalog and full spec
  lint passed. Screenshot review confirmed the desktop source context and the
  phone's fixed header/footer, contained scrolling, and usable step picker.

No live task was migrated during implementation.

## Risks

- Candidate-map loss in preflight or preview task reloads can select the old agent.
- Admission has separate queue/CAS paths; a partial update can lose lifecycle metadata.
- Existing explicit recipient bindings can correctly override a proposed fixed-profile expectation.
- Old submenu interactions occur in multiple E2E page objects and phone surfaces.
- A committed move can outlive a lost HTTP response; retries must not replay entry effects.
- Timestamp guards can reject unrelated concurrent task edits; refresh is intentional and recoverable.
