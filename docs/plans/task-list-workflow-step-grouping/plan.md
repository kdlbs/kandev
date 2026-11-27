---
created: 2026-09-30
status: done
requirements:
  - REQ-UI-LIST-STEP-GROUPING-001
system_design:
  - ../../specs/ui/system-design/task-list-workflow-step-grouping.md
legacy_specs: []
---

# Implementation Plan: Task List Workflow Step Grouping

## Overview

Replace List's State grouping with Workflow step grouping in one end-to-end
work order. Use actual workflow/step identity and names, and retain compatibility
with existing saved values and links. Implementation, screenshots, and PR
publication were explicitly requested on 2026-09-30.

## Scope

### In scope

- Shared desktop/phone grouping option, step metadata, section projection,
  portable preference compatibility, translations, and public help text.
- Targeted tests for grouping semantics, compatibility, and cold route loading.

### Out of scope

- Other task surfaces, lifecycle transitions, status icons, pagination redesign,
  and changing the current parent/child grouping policy.
- New settings pages, feature flags, or automatic task/session creation.

## Technical approach

The [system design](../../specs/ui/system-design/task-list-workflow-step-grouping.md)
defines canonical `workflow_step` preferences with `state` input compatibility,
step-only metadata reads, and sections keyed by workflow and step IDs.

Update `internal/user/models/tasks_list_preferences.go` and
`internal/user/service/service.go` with normalization at validation and
persistence boundaries. Frontend changes start in `lib/tasks/tasks-list-options.ts`,
then connect the existing `useWorkflowOptionPreviews` hook to `TasksPageClient`
and pass metadata through `TasksPageContent` to `TasksListView`. Extract a pure
section helper if necessary. Cover the live SPA preference parser and user
settings mapper. Update the shared phone explanation and catalogs.

## ASCII UI preview

### UI-01: Desktop List, grouped by workflow step

Entry: `/tasks`, toolbar Group control. Example data comes from one workflow.

```text
Sort [Updated newest v]   Group [Workflow step v]

BACKLOG  2
  Task A   (runtime: waiting for input)
  Task B   (runtime: completed)
BUILD  1
  Task C   (runtime: waiting for input)
```

### UI-02: Phone List, view-options drawer

Entry: List title dropdown -> View options -> Task list.

```text
+-----------------------------------+
| View options                  [X] |
| Task list                         |
| Sort                              |
| [Updated newest                v] |
| Group                             |
| [Workflow step                 v] |
| Group tasks by workflow step,     |
| workflow, repository, or none.    |
| [ ] Show archived                 |
+-----------------------------------+
```

The option label and shared grouping result are required (.1/.2/.7). The drawer
uses its existing scroll region and safe-area clearance; sections use the
existing page scroller. Example task names, status annotations, counts, and
spacing are illustrative. With multiple workflows, headings include both
workflow and step. Loading/unavailable headings preserve rows; a no-step section
appears last. Rendered checks cover both views through the named E2E files.

## Tests

| Criteria | Targeted evidence |
| --- | --- |
| .1, .5 | `tasks-list-options.test.ts`: canonical default, alias, and invalid input; `user-settings.test.ts`: legacy hydration; new `src/spa-routes.tasks-preferences.test.tsx`: route preference resolution |
| .2, .3, .4, .6 | `tasks-list-view.test.tsx`: same step/different state; different step/same state; repeated names; ordered sections; missing metadata; parent/child grouping |
| .4 | `use-workflow-option-previews.test.ts`: refresh/retry and stale workspace response rejection |
| .5 | New `internal/user/models/tasks_list_preferences_test.go`; existing user service and store suites: canonical writes, legacy reads, omitted patches, and reload |
| .1, .7 | Translation gates and rendered desktop/phone selection checks |

Names above identify test suites to extend; new scenarios belong in those
suites or a helper's adjacent test file if the projection is extracted.

## E2E tests

- `tests/task/task-list.spec.ts`, chromium: select Workflow step, assert actual
  configured step headings using contrasting runtime states, reload, and verify
  canonical persistence and old `group=state` compatibility (.1/.2/.3/.5/.6).
- `tests/task/mobile-task-list-search.spec.ts`, mobile-chrome: choose Workflow
  step in View options, assert sections and reload persistence, and check the
  translated label fits without horizontal document overflow (.1/.2/.5/.7).
- Include a cold direct visit with All Workflows and two workflows sharing a
  step name in the desktop spec, and a failed-then-retried metadata read in
  component/hook coverage (.2/.3/.4).

## Work orders

- [x] [Task 01: Replace State grouping with workflow steps](task-01-workflow-step-grouping.md)

One sequential work order owns all boundaries. No delegation is authorized.

## Verification results

Design-package validation on 2026-09-30:

- `python3 scripts/list-docs.py validate`: passed (333 decisions, 1264 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage` preflight against this package
  and its intended List source path: passed with one covered work order and
  zero errors. This is local package validation, not a live PR check.
- `git diff --check -- docs/specs docs/plans`: passed; status confirms all four
  package documents are unstaged/uncommitted.

Implementation validation on 2026-10-01:

- Backend user controller, DTO, handler, model, service, and store packages: passed.
- Nine targeted frontend test files: passed (116 tests, followed by the updated
  five-test metadata-hook suite including the new combined-refresh scenario).
- TypeScript typecheck and focused ESLint with zero warnings: passed.
- Traditional Chinese and pseudo generation, full localization checks, and the
  changed-code localization ratchet: passed.
- Desktop List E2E: six tests passed. Phone List E2E: two tests passed. The final
  desktop scenario also exercises cold-list step rename notifications.
- Public documentation validator: 47 pages passed; validator tests: 62 passed.
- Specification catalog, 36 linter tests, all-spec lint, local PR documentation
  coverage preflight, and whitespace checks: passed.
- Fresh desktop List, phone List, and phone View options screenshots captured
  with synthetic tasks, inspected, and compressed for PR publication. Phone
  checks verify a 44px grouping selector and no document horizontal overflow.

Task 01 is complete. Commit, push, screenshot embedding, and PR monitoring are
tracked in the current platform task plan.

PR review remediation on 2026-10-01:

- Workspace changes and newly created workflows now use the current workspace
  store collection for metadata authorization, workflow names, and ordering.
  Two regression tests failed with the initial route-data source and passed
  with the store projection; the metadata-hook suite now has seven tests.
- The desktop grouping Select grows from its 150px minimum. A Portuguese E2E
  assertion reproduced clipping before the fix and passes afterward.
- Four affected frontend suites passed 17 tests, with typecheck, focused ESLint,
  and localization checks passing. Desktop List E2E passed seven tests,
  including a new workflow becoming visible after foreground refresh. Phone
  List E2E passed two tests against the rebuilt frontend.
- Four refreshed screenshots were inspected and compressed: English and
  Portuguese desktop groups, phone groups, and phone View options. They replace
  the initial PR screenshots and use only synthetic test data.

## Risks

- Cold All Workflows visits lack names in Task DTOs; the active board's steps
  cannot substitute for every task's own workflow.
- Backend validation must accept the legacy input before canonical assignment.
- Longer localized labels need intrinsic desktop Select width and phone checks.
- Existing task hierarchy intentionally keeps children under a displayed
  parent; this can place a child's row outside its own step section.

## Inputs

- [Requirements](../../specs/ui/requirements/task-list-workflow-step-grouping.md).
- [System design](../../specs/ui/system-design/task-list-workflow-step-grouping.md).
