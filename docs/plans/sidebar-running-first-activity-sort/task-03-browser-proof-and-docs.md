---
id: "03-browser-proof-and-docs"
title: "Prove configurable sorting on desktop and phone"
status: done
wave: 3
depends_on:
  - "02-color-ranking"
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-RUNNING-ACTIVITY-001
  - REQ-UI-SIDEBAR-GROUP-INDENT-001
acceptance_criteria:
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.1
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.2
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.3
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.4
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.5
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.6
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.7
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.8
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.9
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.10
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.11
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.12
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.13
  - AC-UI-SIDEBAR-GROUP-INDENT-001.1
  - AC-UI-SIDEBAR-GROUP-INDENT-001.2
  - AC-UI-SIDEBAR-GROUP-INDENT-001.3
  - AC-UI-SIDEBAR-GROUP-INDENT-001.4
  - AC-UI-SIDEBAR-GROUP-INDENT-001.5
  - AC-UI-SIDEBAR-GROUP-INDENT-001.6
system_design:
  - ../../specs/ui/system-design/sidebar-running-first-activity-sort.md
---

# Task 03: Prove configurable sorting on desktop and phone

## Summary

Prove the user's three-rule chain and configurable precedence through actual
browser controls. Update public instructions and record the final package evidence.

## In scope

- The plan's desktop and phone E2E matrix, including add/edit/reorder/remove,
  save/reload, workspace scope, color priority, runtime changes, and activity time.
- Server paging beyond 100 rows with off-page color/running contributors,
  preserved conversation selection, and included child navigation.
- Phone stacked cards, visible reorder controls, labelled color options, touch
  hitboxes, internal scrolling, safe areas, and viewport containment.
- Update the sidebar organization section in `docs/public/tasks-and-workflows.md`.
  Include the example chain, precedence, displayed effective color, and phone entry.
- Update related compact-editor/color summary wording if it still assumes one
  field and direction. Promote paired draft specs only after all task checks pass.

- Group indentation E2E: default enabled, toggle absent while Group by is
  collapsed, explicit false saved/reloaded, group inset removed, child nesting
  preserved, and current page/conversation retained on both viewport paths.

## Out of scope

Unrelated flows, full-suite audits, changing other task lists, publication,
commits, pushes, PR creation, and additional color automation features.

## Acceptance

- Desktop and phone configure/save/reload the three-rule chain and show the
  expected order. Moving Color above Running changes precedence; removing it
  restores ordering by the remaining rules.
- Runtime/color/settings updates reorder complete eligible work without navigating;
  global pages retain subtree/priority ranking and bounded response behavior.
- Phone composition matches UI-02, public guidance matches behavior, and all
  required commands have recorded results with skips/blockers stated accurately.
  Group indentation defaults to checked; hiding its disclosure retains the preference.
  Disabled indentation survives reload and preserves child depth and paging.

## ASCII UI preview

UI-01/UI-02 example chain, [full previews](plan.md#ascii-ui-preview):

```text
Desktop: Running first > Red first > Newest activity first
Phone drawer:
  1 Running: Running first       [Move up] [Move down]
  2 Color: Red, matching first   [Move up] [Move down]
  3 Last activity: Newest first  [Move up] [Move down]
  [+ Add sort]
```

Phone uses stacked rule cards with at least 44px touch controls and one scroll
owner. Map rendered checks to AC .1, .8, .11, and .13.

UI-03 excerpt, both desktop and phone:

```text
GROUP BY v
[State v]
[x] Indent grouped tasks
```

The control is checked by default and absent while collapsed. See the
[full grouping preview](plan.md#ui-03-group-by-disclosure-desktop-and-phone),
covering group-indent AC .1-.6. Compare both enabled and disabled group-row geometry.

## Verification

Run from repository root. Managed E2E commands rebuild and run headless with
repository resource limits. Run desktop and phone commands sequentially.

```bash
(cd apps/web && pnpm e2e:run --project=chromium e2e/tests/task/sidebar-running-first-activity-sort.spec.ts)
(cd apps/web && pnpm e2e:run --project=mobile-chrome e2e/tests/task/mobile-sidebar-running-first-activity-sort.spec.ts)
(cd apps/web && pnpm e2e:run --project=chromium e2e/tests/task/workspace-sidebar-views.spec.ts)
(cd apps/web && pnpm e2e:run --project=mobile-chrome e2e/tests/task/mobile-workspace-sidebar-views.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Inspect the rendered phone flow or Playwright screenshot against UI-02. Assert
selector/action touch targets, full text accessibility, viewport containment,
internal scroll reachability at ten rules, and no document horizontal overflow.

## Files likely touched

- `apps/web/e2e/tests/task/sidebar-running-first-activity-sort.spec.ts`.
- `apps/web/e2e/tests/task/mobile-sidebar-running-first-activity-sort.spec.ts`.
- Shared `sidebar-running-first-activity-sort-helpers.ts` in that directory.
- `apps/web/e2e/tests/task/workspace-sidebar-views.spec.ts` and
  `mobile-workspace-sidebar-views.spec.ts`, with the shared
  `sidebar-workspace-view-sort-helpers.ts` fixture.
- `docs/public/tasks-and-workflows.md` and affected summary wording in related UI specs.
- This package's results/status fields and paired specification lifecycle fields.

## Dependencies

Tasks 01 and 02 complete. Load `/e2e`, its fixture/cleanup references,
`/mobile-parity`, and `/docs-maintainer` before implementation.

## Risks

Uncontrolled agent timing makes fixtures unstable. Await accepted projections
instead of arbitrary delays. Restore worker settings baselines after each test.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/ui/requirements/sidebar-running-first-activity-sort.md)
- [Design](../../specs/ui/system-design/sidebar-running-first-activity-sort.md)
- [Scenario matrix and previews](plan.md)

## Results

Completed. Managed Chromium and mobile-Chromium sort-chain E2Es each passed,
including group-indent defaults, expanded-only control, disabled inset, nesting,
save/reload, activity/color/runtime changes, paging, and conversation continuity.
Additional managed desktop and phone workspace-view E2Es passed with the
three-rule chain and `group_indent: false` retained in one workspace and default
view state retained in the other. Every managed run rebuilt backend and web
assets. Public documentation tests passed (62), all 47 published docs validated,
spec catalog validation passed (356 decisions, 1396 specifications), all 36
specification-linter tests passed, full specification lint passed, and
`git diff --check` passed. No PostgreSQL test DSN was configured for the focused
repository test batches. No stale compact-editor wording remained to update.
After review remediation, the desktop sort-chain, phone sort-chain, desktop
workspace-view, and phone workspace-view managed E2E specs were rerun against
fresh backend and Vite builds; each passed (one test per spec). The public guide
continues to describe the configurable sort chain and default-on group indentation.
The desktop and phone sort-chain specs also verify that changing a preferred
color retains focus on its control while the rule's stable editor identity moves
with it during reordering.
