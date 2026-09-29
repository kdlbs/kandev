---
id: "01-move-shared-ui-contracts"
title: "Move shared UI contracts"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-HOVER-001
  - REQ-UI-PR-ONLY-COMMIT-DETAILS-001
acceptance_criteria:
  - AC-UI-SIDEBAR-HOVER-001.2
  - AC-UI-SIDEBAR-HOVER-001.4
  - AC-UI-SIDEBAR-HOVER-001.5
  - AC-UI-PR-ONLY-COMMIT-DETAILS-001.1
  - AC-UI-PR-ONLY-COMMIT-DETAILS-001.2
  - AC-UI-PR-ONLY-COMMIT-DETAILS-001.3
  - AC-UI-PR-ONLY-COMMIT-DETAILS-001.6
system_design:
  - ../../specs/ui/system-design/sidebar-hover-reveal.md
  - ../../specs/ui/system-design/commit-detail-target-types.md
---

# Task 01: Move Shared UI Contracts

## Summary

Move the diff target types and sidebar geometry constants into neutral modules.
Migrate all consumers and remove the four exact state-to-UI baseline entries.
Preserve runtime values, source identity, saved preferences, and responsive
behavior.

## In scope

- Move `DiffSource`, `ChangeLayer`, `CommitDetailTarget`, `OpenDiffOptions`, and
  `DiffSheetMode` into `lib/state/diff-target-types.ts`.
- Move `APP_SIDEBAR_EXPANDED_WIDTH` and `APP_SIDEBAR_COLLAPSED_WIDTH` into
  `lib/layout/app-sidebar-geometry.ts`.
- Migrate direct, re-export, and inline type consumers; keep section IDs and
  CSS class strings in `app-sidebar-constants.ts`.
- Remove the four entries below from the frontend state/UI baseline.

| State path                                            | Removed UI import                                |
| ----------------------------------------------------- | ------------------------------------------------ |
| `apps/web/lib/state/dockview-panel-actions.ts`        | `@/components/task/changes-diff-target`          |
| `apps/web/lib/state/dockview-store.ts`                | `@/components/task/changes-diff-target`          |
| `apps/web/lib/state/slices/ui/app-sidebar-actions.ts` | `@/components/app-sidebar/app-sidebar-constants` |
| `apps/web/lib/state/slices/ui/ui-slice.ts`            | `@/components/app-sidebar/app-sidebar-constants` |

## Out of scope

- Layout, sidebar, mobile navigation, or commit navigation behavior changes.
- Root store composition, System server-state migration, scanner changes, or
  other architecture baselines.
- New product requirements, requirement status changes, or lint waivers.

## Acceptance

- State and component consumers use the neutral modules; no forbidden
  state-to-UI edge or type cycle remains.
- The four exact grandfathered entries are removed and the baseline contains
  zero entries.
- The existing source identity, sidebar dimensions, saved collapse behavior,
  and phone navigation remain unchanged.

## Verification

```bash
make lint-architecture
python3 scripts/lint-architecture.test.py
(cd apps/web && pnpm exec vitest run \
  components/app-sidebar/app-sidebar.test.tsx \
  lib/state/slices/ui/ui-slice.test.ts \
  lib/state/slices/ui/app-sidebar-actions.test.ts \
  lib/state/dockview-panel-actions.test.ts \
  lib/state/dockview-store.test.ts \
  components/task/changes-panel-helpers.test.ts \
  components/task/changes-panel.test.ts \
  components/task/commit-detail-request.test.ts \
  components/task/commit-detail-panel.test.tsx \
  components/task/mobile/mobile-changes-panel.test.tsx \
  hooks/domains/session/use-commit-detail.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test .github/scripts/pr-docs.test.cjs
(cd apps/web && pnpm exec prettier --check \
  ../../docs/plans/frontend-state-ui-ownership/plan.md \
  ../../docs/plans/frontend-state-ui-ownership/task-01-move-shared-ui-contracts.md \
  ../../docs/specs/ui/system-design/sidebar-hover-reveal.md \
  ../../docs/specs/ui/system-design/commit-detail-target-types.md)
```

Run `validateCoverage` against the changed-file list and repository contents
for this PR. The unit tests alone do not prove that this work order covers the
changed paths.

## Results

- Removed all four exact grandfathered imports. The baseline count changed
  from four entries to zero.
- `make lint-architecture` passed. `python3 scripts/lint-architecture.test.py`
  passed all 62 tests.
- The focused Vitest run passed 11 files and 174 tests.
- `pnpm run typecheck` and `pnpm run lint` passed from `apps/web`.
- `python3 scripts/list-docs.py validate` validated 309 decisions and 1184
  specifications. `python3 scripts/lint-spec-files.py --all` passed.
- Prettier passed for the plan, work order, and both system designs.
- `node --test .github/scripts/pr-docs.test.cjs` passed all 81 tests.
- The local `validateCoverage` evaluation returned `covered` for all 31 PR
  changed files and this work order.
- `git diff --check` passed.
