---
id: "01-preserve-measurements"
title: "Preserve valid file-tree measurements"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-FILE-TREE-CHAT-CONTEXT-001
acceptance_criteria:
  - AC-UI-FILE-TREE-CHAT-CONTEXT-001.7
  - AC-UI-FILE-TREE-CHAT-CONTEXT-001.9
  - AC-UI-FILE-TREE-CHAT-CONTEXT-001.10
system_design:
  - ../../specs/ui/system-design/task-surface-render-isolation.md
---

# Task 01: Preserve Valid File Tree Measurements

## Summary

Reject zero-height measurements from hidden file-tree rows.
Retain valid cached geometry until the browser can measure each row again.
The [plan](plan.md#confirmed-cause-and-reproduction) records the matching browser reproduction.

## In scope

- A typed file-tree measurement callback and focused regression tests.
- Wiring the callback into `VirtualizedFileTreeView`.
- Desktop panel-restoration and phone Files-navigation E2E coverage.

## Out of scope

Dependency upgrades, global observer changes, panel remount policies, backend
changes, public copy, and persisted state.

## Acceptance

1. Unavailable measurements use a positive cache entry for the current row key,
   or its current estimate. Positive measurements remain authoritative.
2. Files restoration shows every expected viewport row without gaps or overlaps,
   including compact and touch-sized rows (`.9`, `.10`).
3. Large trees retain bounded mounting, last-file access, create reveal, and
   reachable 44px touch actions (`.7`).

## ASCII UI preview

UI-01: Files restored at the top. Full comparison: [plan preview](plan.md#ascii-ui-preview).

```text
Desktop Files             Phone Files
+-------------------+     +----------------------+
| Files toolbar     |     | Files toolbar        |
| > folder-00       |     | > folder-00     [...]|
| > folder-01       |     | > folder-01     [...]|
| > folder-02       |     | > folder-02     [...]|
| ...               |     | ...                  |
+-------------------+     +----------------------+
                          | Existing bottom nav  |
                          +----------------------+
```

The existing viewport owns scrolling. Rows remain contiguous after restoration.
Phone touch actions retain their current geometry. This implements `.7`, `.9`, and `.10`.

## Implementation sequence

1. Mark this work order `in_progress`.
2. Add `file-tree-measurement.test.ts` with the zero-height regression.
3. Run it against a callback that delegates to the current default measurement.
4. Record the behavioral failure: expected cached height or estimate, received zero.
5. Implement the fallback and wire the callback into the real virtualizer.
6. Extend both existing virtualization specs with the plan's transition scenarios.
7. Add shared viewport geometry assertions and focused cases for blank top, overflowing
   blank bottom, legitimate short-tree trailing space, and end padding.
8. Run every verification command and record the results.
9. Compare the rendered desktop and phone geometry with UI-01.
10. Mark this work order `done` and synchronize the plan.

## Verification

Run from the repository root. If dependencies are absent, first run
`(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/web && pnpm exec vitest run components/task/file-tree-measurement.test.ts components/task/file-browser-render-identity.test.tsx components/task/file-browser-responsive.test.tsx components/task/file-tree-geometry.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/file-tree-measurement.ts components/task/file-tree-measurement.test.ts components/task/file-tree-geometry.test.ts components/task/file-browser-parts.tsx e2e/tests/task/file-tree-geometry.ts e2e/tests/task/large-file-tree-virtualization-helpers.ts e2e/tests/task/large-file-tree-virtualization.spec.ts e2e/tests/task/mobile-large-file-tree-virtualization.spec.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/task/large-file-tree-virtualization.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-large-file-tree-virtualization.spec.ts tests/task/mobile-file-tree-chat-context.spec.ts)
(cd apps/web && pnpm run lint:e2e-sleeps -- e2e/tests/task/large-file-tree-virtualization.spec.ts e2e/tests/task/mobile-large-file-tree-virtualization.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The managed E2E commands build the current production sources.
Run desktop and mobile commands sequentially.

## Files likely touched

- `apps/web/components/task/file-tree-measurement.ts` (new).
- `apps/web/components/task/file-tree-measurement.test.ts` (new).
- `apps/web/components/task/file-browser-parts.tsx`.
- `apps/web/e2e/tests/task/file-tree-geometry.ts` (new).
- `apps/web/components/task/file-tree-geometry.test.ts` (new).
- `apps/web/e2e/tests/task/large-file-tree-virtualization.spec.ts`.
- `apps/web/e2e/tests/task/mobile-large-file-tree-virtualization.spec.ts`.
- `apps/web/e2e/tests/task/large-file-tree-virtualization-helpers.ts`.

## Dependencies

None.

## Risks

Cache identity must follow the row key. Accept later positive measurements so
responsive sizes and inline controls can change height.
Actual mobile Files navigation unmounts the tree. It does not reproduce desktop
Dockview lifetime, so retain the controlled hidden-measurement regression too.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/file-tree-chat-context.md), criteria `.7`, `.9`, and `.10`.
- [System design](../../specs/ui/system-design/task-surface-render-isolation.md#file-tree-virtualization).
- Existing desktop and mobile virtualization specs and their shared fixture.
- `file-browser-render-identity.test.tsx` for nearby component-test conventions.

## Results

- RED: the focused measurement test against TanStack's default callback returned
  `0` for a hidden row where the positive cached height was `32`.
- GREEN: `file-tree-measurement.ts` delegates positive measurements to TanStack
  and otherwise retains the current row-keyed positive cache entry or estimate.
  `VirtualizedFileTreeView` uses the callback without changing its scroll owner,
  row keys, estimates, overscan, or reveal effects.
- Review RED: the original intersection-filtered viewport-edge checks accepted
  synthetic blank top and bottom gaps because every retained row satisfied the
  later edge comparisons.
- Review GREEN: the shared pure viewport validator checks the first row's top,
  the last row's bottom for overflowing trees, and adjacent row bounds. It skips
  the bottom fill requirement for short trees and allows the container's end
  padding. Four focused geometry cases passed.
- Focused Vitest passed 4 files and 14 tests. It covers cached zero measurements,
  first-measurement estimates, later positive measurements, and the shared
  blank-edge/short-tree geometry contract.
- Frontend typecheck and targeted ESLint passed.
- Desktop production-served E2E passed 3 tests. The new scenario restores the
  collapsed tree, expanded tree at the top, and expanded tree after scrolling
  through actual Dockview tab transitions. It compares ordered visible path
  windows before and after restoration and checks visible row bounds.
- Mobile production-served E2E passed 3 tests. The large-tree scenario checks
  Files to Chat to Files navigation, the shared contiguous-row validator, the
  ordered collapsed path window, 44px actions, last-file access, and document
  overflow, alongside the existing chat-context tests.
- E2E sleep lint, specification validation, specification lint, and `git diff --check`
  all passed.
