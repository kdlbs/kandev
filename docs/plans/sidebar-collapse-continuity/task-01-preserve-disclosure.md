---
id: "01-preserve-disclosure"
title: "Preserve sidebar disclosure continuity"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-ARCHIVED-FILTER-002
acceptance_criteria:
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.7
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.9
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.15
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.16
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.18
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.22
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.24
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.25
system_design:
  - ../../specs/ui/system-design/sidebar-archived-filter.md
---

# Task 01: Preserve sidebar disclosure continuity

## Summary

Keep an eligible bounded accepted display during collapse-only replacement
reads. Preserve the full request/cache identity and all context safeguards.
Implemented with TDD after the user authorized execution of the design package.

## In scope

Shared hook continuity, collapse rendering, paging/error recovery, safety tests,
desktop and phone Playwright evidence, and documentation/delivery results.

## Out of scope

Server query changes, all-task fetches, new caches or prefetch, layout redesign,
new user-facing copy, changes to the user's live instance, and subagents.

## Acceptance

1. The red hook test `keeps eligible rows while an uncached repository collapse
   loads` fails because the current hook returns a null page and initial loading.
   After the fix, groups/subtasks collapse immediately, unaffected rows and
   headings stay visible, all-collapsed headings do not show task placeholders,
   and expansion never fabricates unseen rows.
2. Held-response tests prove page-one replacement and Retry from a later source
   page, no navigation with transitional bounds, transient-error retention,
   latest-request settlement after rapid disclosures, deletion reconciliation,
   and no continuity across changed content/context identity. Existing initial
   loading, denial, deletion-recovery, cached return, and local-coverage behavior
   continue to pass. No retained membership is treated as complete coverage.
3. Desktop and phone isolated Playwright regressions prove continuity before
   a gated HTTP response settles, unchanged conversation selection, reachable
   disclosure controls and contained phone scrolling. Capture fresh desktop
   and phone screenshots, verify the exact published PR head and every review
   disposition, and merge only after all required checks and reviews permit it.

## ASCII UI preview

UI-01 and UI-02 excerpts; see [full plan](plan.md#ascii-ui-preview).

```text
Desktop sidebar                  Phone Tasks drawer body
> Repository A       (1)         > Repository A       (1)
v Repository B       (1)         v Repository B       (1)
  Task B                           Task B
```

The target disclosure applies immediately while the network is held. Retain
existing scroll owners, group headings, localized controls, and focus behavior.
Refresh status remains screen-reader-only. No required task workflow depends
on hover. Phone keeps existing 44px targets and safe-area containment.

## Owned files

- `apps/web/hooks/domains/kanban/use-sidebar-page-context.ts`
- `apps/web/hooks/domains/kanban/use-sidebar-task-page.ts`,
  `sidebar-task-page-display.ts`, `.disclosure.test.tsx`, and `.removal.test.tsx`
- `apps/web/components/task/sidebar-task-page-content.tsx`,
  `sidebar-task-pagination.tsx`, and their tests only if transitional page
  navigation needs an explicit presentation flag
- `apps/web/components/task/mobile/mobile-task-list.tsx` and
  `session-task-switcher-sheet.tsx` for shared transition presentation
- `apps/web/e2e/tests/task/sidebar-collapse-loading.spec.ts` (new)
- `apps/web/e2e/tests/task/mobile-sidebar-collapse-loading.spec.ts` (new)
- `apps/web/e2e/tests/task/sidebar-collapse-loading-fixtures.ts` (new)
- `apps/web/e2e/helpers/sidebar-disclosure-request.ts` and `.test.ts` for held-request cleanup
- This plan/work order and the linked requirement/system design

If return-shape changes affect mocks, find and update every affected consumer.
Do not weaken tests to avoid adding a consumed field.

## Dependencies

No other work order. Use the current branch and primary session. Refresh the
main branch before delivery and account for related concurrent UI changes.

## Verification

Install workspace dependencies from `apps/` once if absent. Run from the repo
root, sequentially for the browser projects:

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/kanban/use-sidebar-task-page.test.tsx hooks/domains/kanban/use-sidebar-task-page.disclosure.test.tsx hooks/domains/kanban/use-sidebar-task-page.removal.test.tsx hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts hooks/domains/kanban/use-sidebar-store-tasks.test.tsx lib/sidebar/sidebar-task-page-cache.test.ts components/task/sidebar-task-pagination.test.tsx components/task/sidebar-task-query-status.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 hooks/domains/kanban/use-sidebar-page-context.ts hooks/domains/kanban/use-sidebar-task-page.ts hooks/domains/kanban/sidebar-task-page-display.ts hooks/domains/kanban/use-sidebar-task-page.disclosure.test.tsx hooks/domains/kanban/use-sidebar-task-page.removal.test.tsx components/task/sidebar-task-page-content.tsx components/task/mobile/mobile-task-list.tsx components/task/mobile/session-task-switcher-sheet.tsx e2e/helpers/sidebar-disclosure-request.ts e2e/helpers/sidebar-disclosure-request.test.ts e2e/tests/task/sidebar-collapse-loading-fixtures.ts e2e/tests/task/sidebar-collapse-loading.spec.ts e2e/tests/task/mobile-sidebar-collapse-loading.spec.ts)
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/task/sidebar-collapse-loading.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/task/mobile-sidebar-collapse-loading.spec.ts
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Add any changed production presenter files to the targeted ESLint command.
Managed browser runs build the current frontend; do not reuse stale assets.
Capture via `prCapture` and preserve desktop captures before a separate mobile
runner invocation clears `.pr-assets`. Validate the merged manifest and images
before publication. No screenshot binaries belong on the merge branch.

## Delivery

The user requested delivery through merge in full AFK mode. After execution
is authorized at the required handoff, commit normally with hooks, push the
exact checked head, create/reuse a ready PR using the repository template,
publish validated screenshots, and use `scripts/pr-await` for CI/reviews.
Disposition every unresolved thread under the authorized fixup scope. Verify
head/base/mergeability and required review evidence before merging; use the
merge queue if protection requires it and verify actual `MERGED` state.
Do not bypass required checks, fabricate approval, or delete branches without
an explicit deletion request. Report any external human-approval gate precisely.

## Results

- Red: the original hook returned no display during a held collapse. The original
  desktop loader also removed repository headings while the HTTP request was held.
- Green: 73 focused tests passed, including disclosure, stale responses, page-one
  Retry, context barriers, deletion reconciliation, cache, and paging/error controls.
  Moving the disclosure cases into their own suite preserved all assertions and
  passed the three affected hook suites (37 tests).
- Typecheck and targeted ESLint with zero warnings passed.
- Fresh managed Chromium and mobile-chrome runs passed one test each. Phone covers
  both the Tasks picker and app-navigation outlet, 44px controls, unchanged
  conversation selection, and contained scrolling. Responses are gated causally.
- Public documentation validation passed 62 tests and 47 pages. Catalog validation,
  full specification lint, and whitespace checks passed.
- Three fresh synthetic screenshots were captured for the held refresh, inspected,
  and compressed for PR publication. Images stay off the merge branch.
- Production behavior, requirement 002.25, and the linked system design conform.
  No backend contract, new copy, cache budget, or complete-coverage semantics changed.

Implementation checks are complete. CI, review disposition, and protected merge
are delivery gates to verify against the published head.

## PR remediation

Greptile identified that an assertion failure could be replaced by the held
response's timeout, which also prevented route removal. Extracted the HTTP gate
into an isolated E2E helper: release and remove the route on every failure,
preserve the first error, and consume a late waiter rejection. The red unit
regression reproduced the masked assertion; four focused tests now pass,
including missing and failed responses, secondary cleanup failure, and success.
Targeted ESLint and web typecheck passed. Browser checks below validate the same
helper on desktop and both phone surfaces; the built production UI is unchanged.

```bash
(cd apps/web && pnpm exec vitest run e2e/helpers/sidebar-disclosure-request.test.ts)
(cd apps/web && pnpm exec eslint --max-warnings 0 e2e/helpers/sidebar-disclosure-request.ts e2e/helpers/sidebar-disclosure-request.test.ts e2e/tests/task/sidebar-collapse-loading-fixtures.ts)
(cd apps/web && pnpm run typecheck)
pnpm --dir apps/web e2e:run --host --no-build --shards 1 --project chromium tests/task/sidebar-collapse-loading.spec.ts
pnpm --dir apps/web e2e:run --host --no-build --shards 1 --project mobile-chrome tests/task/mobile-sidebar-collapse-loading.spec.ts
```

The first published head's architecture job was cancelled without steps because
GitHub failed to acquire a hosted runner (run 37374049388, job 111978000886).
This is runner evidence; the normal commit architecture hook passed. The new
published head must receive fresh CI/review evidence before merge.
