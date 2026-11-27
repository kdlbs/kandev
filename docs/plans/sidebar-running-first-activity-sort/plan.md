---
created: 2026-10-06
updated: 2026-10-06
status: done
requirements:
  - REQ-UI-SIDEBAR-RUNNING-ACTIVITY-001
  - REQ-UI-SIDEBAR-GROUP-INDENT-001
system_design:
  - ../../specs/ui/system-design/sidebar-running-first-activity-sort.md
legacy_specs: []
---

# Implementation plan: Sidebar sort chains and color ranking

## Follow-up running predicate amendment

The [task-wide running fix](../sidebar-task-wide-running-rank/plan.md) replaces
this package's primary-only predicate with any-session runtime evidence.
This package remains a completed historical delivery record. Its recorded
checks cover the original predicate. The follow-up owns mixed-session,
absent-primary, upgrade, and desktop/phone regression evidence.

## Overview

Deliver configurable sorting with several rules. The user's example is Running
first, Red first, and Last activity newest first. Each later rule resolves ties
from earlier rules. Reordering the rules changes which preference takes precedence.

This revision replaced the fixed-preset package. The partial backend work was
reconciled with the chain contract, then extended through color ranking, group
indentation, browser proof, and public guidance. The fixed preset is not exposed
as the final interface.

The sorting comparison is small. Full delivery is moderate to substantial because
effective color currently resolves in the frontend, while global paging ranks
in SQL. Three sequential work orders cover a complete basic sort chain, color
ranking, then browser proof and user documentation.

## Scope

### In scope

- [Requirements](../../specs/ui/requirements/sidebar-running-first-activity-sort.md):
  configurable rules, runtime/activity subtree ranking, own-row effective color,
  persistence, compatibility, and desktop/phone editing.
- Additive `sort.then_by` contract, all-rule query needs, server/local comparators,
  legacy single-sort compatibility, and fixed-preset continuation cleanup.
- Authenticated color preference projection, SQL/TS color conformance, complete
  local coverage, settings-aware cache invalidation, and bounded paging.
- Per-view grouped-task indentation, enabled by default; its toggle appears only
  in the expanded Group by section. Turning it off removes the group inset only.

### Out of scope

Other task listing surfaces, shared priority changes, color inheritance from
children, arbitrary scoring, lifecycle changes, database migrations, and default-view changes.

## Technical approach

1. Normalize primary sort plus flat secondary rules. Retain existing single-sort
   behavior; add independent Running criteria. Extend store/API/user settings types,
   conversions, query validation, and migrations. Adapt partial fixed-preset work.
2. Make every SQL projection/ancestor/join gate inspect the whole chain. Compose
   per-criterion ORDER BY before canonical fallback, pins, manual siblings, and paging.
   Local and generic complete-data evaluators follow the same lexicographic order.
3. Add own-row effective color ranking using authenticated manual/automatic settings.
   Evaluate the same facts and first-match rule precedence as the existing frontend.
   Shared fixtures cover normalization and every condition dimension in both paths.
4. Add a compact ordered rule editor. Reuse the current anchored desktop popover
   and touch drawer, with explicit move buttons and stacked phone cards.
5. Add `groupIndent`/`group_indent` presentation state to saved views and drafts.
   Default to true; preserve false. Render its toggle only inside expanded Group by.
   Gate the shared group-body `ml-5` without changing child depth or query identity.
6. Verify save/reload, rule reordering, color updates, running/waiting transitions,
   included children, and global ordering beyond page one. Update public guidance.

The [proposed decision](../../decisions/2026-10-06-composable-sidebar-sort-rules.md)
records the additive wire format and effective-color boundary. Exact sources and
commands appear in the work orders. Keep existing SQLite scratch-table cleanup,
query limits, settings revisions, and response-generation guards.

## ASCII UI preview

### UI-01: Desktop expanded Sort section

Entry: Tasks filter gear, Sort expanded. Example three-rule draft:

```text
SORT
1 [Running v]       [Running first v]  [Up] [Down] [Remove]
2 [Color v] [Red v]  [Matching first v] [Up] [Down] [Remove]
3 [Last activity v] [Newest first v]   [Up] [Down] [Remove]
[+ Add sort]
GROUP BY [None v]

Running red task           20m
Running blue task          10m
Idle red task               1m
Idle red task               5m
Idle blue task             30s
```

Running blue outranks idle red because Running is first. Idle red outranks the
more recent idle blue because Color is second. Swatches include text labels.
The collapsed summary lists these rules in the same order. Use 28px fine-pointer
controls and existing Select/Button primitives.

### UI-02: Phone view-editor drawer

Entry: task picker trigger, filter gear, Sort expanded.

```text
View editor drawer             [Close]
SORT
+----------------------------------+
| 1  [Running v]                   |
|    [Running first v]             |
|    [Move up] [Move down] [Remove] |
+----------------------------------+
+----------------------------------+
| 2  [Color v] [Red swatch + Red v] |
|    [Matching first v]            |
|    [Move up] [Move down] [Remove] |
+----------------------------------+
+----------------------------------+
| 3  [Last activity v]             |
|    [Newest first v]              |
|    [Move up] [Move down] [Remove] |
+----------------------------------+
[+ Add sort]
GROUP BY [None v]
```

Use the existing inset drawer, not a new overlay. The header stays fixed; the
editor body owns scrolling. Cards keep labels and actions reachable by touch.
Use at least 44px touch targets, safe-area clearance, dynamic viewport containment,
and zero document horizontal overflow. Phone task selection returns to its
existing single conversation surface.

Required structure: ordered rules, field-specific options, labelled colors,
explicit reorder/remove controls, Add sort, and shared saved state. Example
spacing/titles are illustrative. UI-01/UI-02 map to AC .1, .8, .11, and .13.

At one rule, Remove is unavailable. At ten, Add sort is disabled with a localized
limit. Custom shows a standalone manual-order mode. Empty and query-error list
states reuse the current sidebar presenter; rule validation errors identify the rule.

### UI-03: Group by disclosure, desktop and phone

Entry: existing view editor. This small region has the same control order in
both viewport compositions; phone uses the existing inset drawer.

```text
Collapsed:
GROUP BY                              >
State

Expanded (default):
GROUP BY                              v
[State                                v]
[x] Indent grouped tasks

Expanded (user disables indentation):
GROUP BY                              v
[State                                v]
[ ] Indent grouped tasks
```

The checkbox exists only in the expanded content. The preference defaults to
checked and persists per view. Closing the section does not change it. All
phone hit targets remain at least 44px; the existing editor body owns scrolling.

```text
Enabled (default):             Disabled:
v In progress                  v In progress
    (*) Parent                 (*) Parent
        -> Child                   -> Child
```

Both columns illustrate that the disabled mode removes one group-body inset.
The actual row insets come from existing task-row primitives; subtask depth
remains relative to its parent. Group headers and collapse remain available.
This preview maps to `AC-UI-SIDEBAR-GROUP-INDENT-001.1` through `.6`.

## Tests

The sort criteria table uses `AC-UI-SIDEBAR-RUNNING-ACTIVITY-001`.

| Criteria | Evidence |
| --- | --- |
| .2, .3, .6 | Go query tests on SQLite/PostgreSQL; shared local conformance fixtures; running/activity subtree tests with secondary-key permutations |
| .4, .6 | Pins, manual children, groups, collapse, orphan promotion, ties, nanosecond timestamps, and concatenated pages |
| .5, .9, .13 | Query validation and safe error metadata; store/draft wire and migration tests; settings round trips; complete-coverage fallback |
| .1, .8, .11, .12, .13 | Ordered editor component tests; desktop and phone Playwright add/edit/reorder/remove/save/reload flows |
| .7, .10 | Shared SQL/TS color fixtures, every rule dimension, automation/manual precedence, and live color/settings invalidation |
| .2, .6, .7, .11 | Mixed running/color/activity E2E order, changed precedence, global server pages, preserved conversation |

Group-indent evidence: `ui-slice-migration.test.ts` and
`sidebar-view-wire.test.ts` cover absent=true and explicit false round trips.
`sidebar-filter-popover.test.tsx` covers default checked and expanded-only rendering.
`task-switcher.test.tsx` covers group-wrapper indentation without changing child depth.
Desktop and phone browser tests cover persisted false, default true, collapse,
selected rows, and retained page/conversation. These map to all six group-indent criteria.

## E2E tests

- `sidebar-running-first-activity-sort.spec.ts`, `chromium`: build the three-rule
  chain through UI, save/reload, and prove the UI-01 ordering. Move Color above
  Running, remove Color, and verify changed precedence. Test runtime and color
  updates while the current conversation remains selected.
- `mobile-sidebar-running-first-activity-sort.spec.ts`, `mobile-chrome`: configure
  the same chain through the drawer, reorder by touch buttons, save/reload/reopen,
  and prove equivalent ordering plus child navigation. Verify UI-02 geometry.
- `workspace-sidebar-views.spec.ts`, `chromium`, and
  `mobile-workspace-sidebar-views.spec.ts`, `mobile-chrome`: verify that the
  configured chain and explicit disabled group inset remain on one workspace
  after switching/reload while the other workspace keeps its defaults.
- Desktop server-boundary case: more than 100 matching rows, running descendant
  outside the returned page, and color-priority task on a later page before ranking.
  Verify global order, continuation, and stable navigation while paging.
- Both desktop and phone specs also verify UI-03: toggle absent when collapsed,
  checked by default when expanded, false saved/reloaded, group inset removed,
  and child nesting retained. A presentation-only toggle keeps the current page.
- Reuse existing sidebar/tree activity APIs and test fixtures. Shared setup belongs
  in `sidebar-running-first-activity-sort-helpers.ts`. Restore worker settings after tests.

## Work orders

- [x] [Task 01: Sort chain contract, evaluation, and editor](task-01-sort-preset.md) (`done`)
- [x] [Task 02: Effective color ranking before pagination](task-02-color-ranking.md) (`done`)
- [x] [Task 03: Browser proof and public documentation](task-03-browser-proof-and-docs.md) (`done`)

Order: 01 -> 02 -> 03. Sequential work; no delegation is authorized.
Fresh worktrees need `(cd apps && pnpm install --frozen-lockfile)` once before package commands.
The old Task 02 browser work order moves to Task 03 because color ranking is a
new dependency. Implementation was authorized against this revised package and
completed in the three sequential work orders below.

## Verification results

Implementation verification passed on 2026-10-06:

- Task 01 and 02 focused Go tests passed for task models, handlers, SQLite, and
  user settings. PostgreSQL-only cases were skipped because
  `KANDEV_TEST_POSTGRES_DSN` was unset.
- The original combined focused web regression batch passed: 23 files, 342
  tests. After review remediation, the final seven-file regression batch passed
  (235 tests), covering status-summary refresh on page 2, equal settings
  broadcasts, group-indent write/ack stability, and shared local/SQL handling of
  a missing primary-session summary.
- `pnpm run typecheck` and the targeted ESLint checks passed. Translation
  synchronization, pseudo-locale, completeness, and changed-line ratchet checks
  passed.
- Managed Chromium sort-chain and workspace-scope E2Es passed. Managed
  mobile-Chromium sort-chain and workspace-scope E2Es passed. Each managed run
  rebuilt the Go backend, Vite assets, and fixture plugin.
- After review remediation, all four managed sort-chain and workspace-scope E2E
  specs were rerun with fresh builds; each passed (one test per spec). The final
  work-order Go test command, `make -C apps/backend build`, and the web Vite
  build also passed. PostgreSQL-only cases remain skipped because
  `KANDEV_TEST_POSTGRES_DSN` was unset.
- Public-doc tests and validation passed (62 tests; 47 published pages).
- Specification catalog validation passed (356 decisions, 1396 specifications),
  all 36 specification-linter tests passed, and full specification lint passed.
- Final package links and `git diff --check` passed. Local Running now uses the
  authoritative status-summary primary session, matching SQL when that summary
  omits the primary-session member.
- PR follow-up on 2026-10-07 passed the focused sidebar repository suite with
  PostgreSQL 16 enabled, the backend build, and targeted SQLite pool-memory cases
  for activity/state, running/activity/state, and state sorts in both directions.
  Shared recursive ancestor rows are force-materialized only when multiple sort
  projections consume them, keeping the single activity/state case below the
  configured memory limits.

The revised design/package checks also passed before implementation on 2026-10-06:

- `python3 scripts/list-docs.py validate`: passed (356 decisions, 1396 specifications).
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Local `validateCoverage` preflight: all three work orders passed, with a planned
  runtime source path included to exercise implementation-package coverage.
- All seven package files have resolving document links and valid whitespace.
- `git diff --check` passed. Existing partial backend edits remain intact;
  this revision changed only documentation and task-plan tracking.
- Group indentation revision: specification validation, coverage preflight,
  package links, and whitespace checks passed before implementation.

## Risks

- Color rank computed only for returned rows violates global ordering.
- Checking only the primary key can omit state/activity/color projections.
- An implicit first-field tiebreak can hide later rules entirely.
- Effective-color SQL must match all frontend rule dimensions and normalization.
- Personal color updates need cache invalidation across affected views and pages.
- Maximum color settings must remain within native query-memory budgets.
- PostgreSQL behavior requires `KANDEV_TEST_POSTGRES_DSN`; report skipped cases accurately.
- Existing partial fixed-preset changes need reconciliation before further implementation.

Grouping update: enabled by default and expanded-only toggle are the confirmed
user choices. Add their verification to Tasks 01 and 03; Task 02 is unchanged.
