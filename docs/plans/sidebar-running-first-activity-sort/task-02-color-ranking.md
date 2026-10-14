---
id: "02-color-ranking"
title: "Rank effective task colors before global paging"
status: done
wave: 2
depends_on:
  - "01-sort-preset"
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-RUNNING-ACTIVITY-001
acceptance_criteria:
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.2
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.4
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.5
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.6
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.7
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.9
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.10
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.11
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.13
system_design:
  - ../../specs/ui/system-design/sidebar-running-first-activity-sort.md
---

# Task 02: Rank effective task colors before global paging

## Summary

Add preferred-color rules to the implemented sort chain. Match each row's
visible effective marker in SQL and local evaluation before global paging.

## In scope

- Typed read-only color preference snapshot from authenticated user settings;
  use existing automatic/manual contracts and bounds.
- Backend candidate color facts, parameterized ordered automatic-rule evaluation,
  manual fallback, workflow color normalization, and own-row preferred-token rank.
- Shared SQL/TS color fixtures for every condition dimension and normalization;
  test automation override, clear tombstones, incomplete rules, and missing facts.
- Local source-coverage requirements and color projections. Unknown coverage
  falls back to the server; known absence uses normal matching/fallback rules.
- Full-chain/cache identity and invalidation on relevant settings/fact changes,
  including off-page tasks. Use bounded preference identity rather than full maps.
- Enable Color in the editor with a labelled palette selector, allow multiple
  distinct preferred colors, and verify save/draft round trips.

## Out of scope

Custom hexadecimal similarity, subtree color inheritance, shared task priority,
new color automation features, and browser workflow proof.

## Acceptance

- SQL and frontend fixtures agree on effective marker tokens for all rule
  dimensions; automatic blue over manual red ranks as non-red.
- The user's Running/Red/Activity chain works globally across pages and all
  covered local data, respecting own-root color and existing overrides.
- Color edits, clears, automation edits, and source-fact changes invalidate rank;
  maximum settings retain bounded query preparation and page responses.

## ASCII UI preview

UI-01/UI-02 color rule from [the plan](plan.md#ascii-ui-preview), AC .1, .8, .10, and .11:

```text
Desktop: 2 [Color v] [Red swatch + Red v] [Matching first v] [Up] [Down] [Remove]
Phone:
  2 [Color v] [Red swatch + Red v]
    [Matching first v]
    [Move up] [Move down] [Remove]
```

Color is a labelled token, never an unlabelled swatch. Its priority depends on
its position in the chain. Reuse the existing inset phone editor and touch targets.

## Verification

Run from repository root. Use the same isolated PostgreSQL environment as Task 01.
Add proposed focused files as part of implementation and extend existing memory
cases for color ranking with maximum personal settings.

```bash
(cd apps/backend && go test -trimpath ./internal/task/models ./internal/task/handlers ./internal/task/repository/sqlite -run 'Sidebar|QuerySidebar' -count=1)
(cd apps/web && pnpm exec vitest run lib/sidebar/sidebar-color-rank.test.ts lib/sidebar/task-color-rules.test.ts lib/sidebar/task-color-projection.test.ts lib/sidebar/repository-rule-identity.test.ts lib/task-color-presentation.test.ts lib/sidebar/sidebar-local-conformance.test.ts lib/sidebar/sidebar-local-view.test.ts lib/sidebar/sidebar-task-source.test.ts lib/sidebar/sidebar-task-page-cache.test.ts lib/state/slices/task-overview-coverage.test.ts lib/state/slices/ui/sidebar-view-wire.test.ts components/task/sidebar-filter/sort-picker.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 lib/sidebar/sidebar-color-rank.ts lib/sidebar/sidebar-local-order.ts lib/sidebar/sidebar-local-view.ts lib/sidebar/sidebar-local-projection.ts lib/sidebar/sidebar-task-source.ts lib/sidebar/sidebar-task-page-cache.ts lib/state/slices/task-overview-coverage.ts components/task/sidebar-filter/sort-picker.tsx)
(cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:pseudo && pnpm run i18n:check && pnpm run i18n:ratchet)
git diff --check
```

Record any skipped PostgreSQL cases. Add every additional changed test/source
file to these focused commands. Confirm SQLite scratch cleanup after cancellation
and that color metadata does not enter shared task records.

## Files likely touched

- `apps/backend/internal/task/models/sidebar_task_view.go`: typed color read preferences.
- `apps/backend/internal/task/handlers/sidebar_task_http.go` and tests.
- `apps/backend/internal/task/repository/sqlite/sidebar_task_query_base.go`,
  page SQL, scratch preference staging, color SQL helper/tests, and memory tests.
- New color conformance JSON beside `sidebar-local-conformance.json`.
- `apps/web/lib/sidebar/sidebar-color-rank.ts`/test and fixture tests using existing color resolvers.
- Local projection/order/source, coverage tests, page cache/invalidation paths.
- Sidebar color selector/rule-editor components and all task locale catalogs.

## Dependencies

Task 01 complete for ordinary chains. Existing task color rules/presentation and
backend user settings supply the color source contract.

## Risks

Repository identity and workflow-color normalization can diverge across runtimes.
Ranking only loaded page rows or caching without color settings identity is incorrect.
Maximum map/rule bounds must not inflate native SQLite memory beyond existing budgets.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/ui/requirements/sidebar-running-first-activity-sort.md)
- [Design](../../specs/ui/system-design/sidebar-running-first-activity-sort.md)
- [Existing color design](../../specs/ui/system-design/sidebar-automatic-task-colors.md)
- Frontend `task-color-rules.ts`, `task-color-presentation.ts`, and `repository-rule-identity.ts`.

## Results

Completed. Focused backend tests passed for task models, handlers, and SQLite;
PostgreSQL-only tests were skipped because `KANDEV_TEST_POSTGRES_DSN` was unset.
The focused color frontend batch passed (11 files, 188 tests), and the combined
sort/color/local-projection regression batch passed (23 files, 342 tests).
Typecheck, targeted ESLint, translation and i18n checks passed. Managed desktop
and phone E2Es prove color rules rank globally, respect automatic-over-manual
effective color, and preserve order when criteria move or are removed. Review
remediation expanded the shared SQL/TypeScript fixture to cover provider-host
defaults and remote fallback, provider identity precedence over local paths,
workspace eligibility, normalized paths, and primary-session executor-profile
selection with metadata fallback. The final work-order Go test command passed;
the PostgreSQL-only cases remained skipped because no test DSN was configured.
