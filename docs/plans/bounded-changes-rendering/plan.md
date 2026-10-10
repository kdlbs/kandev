---
created: 2026-09-30
status: completed
requirements:
  - REQ-UI-BOUNDED-CHANGES-001
system_design:
  - ../../specs/ui/system-design/bounded-changes-rendering.md
legacy_specs: []
---

# Fix Plan: Bounded Changes Rendering

## Overview

Bound mounted rows across the Changes timeline while preserving complete data
and existing actions. Deliver working-tree protection first, historical rows
second, then lifecycle and browser stress coverage. Execute sequentially.

The user requested implementation of this reviewed package in the Implement
workflow step. Apply the work orders sequentially with TDD. Do not change the
affected task worktree or its temporary files.

## Confirmed evidence

The supplied trace, `Trace-20260930T001112.json.gz`, covers the affected task
`fb8d0533-221b-4b3f-bbd6-248b36d934f4`. DOM nodes rose from 6,119 to 530,999.
JavaScript heap rose from 88,850,528 to 897,258,440 bytes in approximately eight seconds.
The trace does not record the terminal browser crash reason.

Read-only Git enumeration found 43,611 untracked files under
`.agentctl-reattach-tmp-20260927/`. These comprised 40,703 Go cache files,
2,814 Node cache files, and 94 other temporary files.
Deployed CPU positions mapped to `ChangesTree`, `FileRow`, `StageButton`,
`FileHoverActions`, and their translation hooks.

`ChangesTree` expands all directories and maps all visible logical rows.
The flat file list, repository groups, provider files, and commits also map
complete collections. `CommitRow` retains hidden descendants after collapse.
The root cause is unbounded mounted UI, triggered by a large valid Git inventory.

The smallest regression supplies 50,000 ordinary file metadata entries to an
isolated Changes panel. The current implementation exceeds the mounted-row
budget. No cache cleanup or special filename is needed to reproduce the defect.

## Scope

### In scope

- One bounded timeline viewport for working files, directories, groups, commits,
  inline historical files, provider files, and status rows.
- Full-collection selection and counts, source-safe actions, keyboard access,
  variable-height measurements, lifecycle cleanup, and phone parity.
- Efficient tree construction and scoped, lazy inline detail ownership.
- Deterministic component regressions and isolated production-build E2E.

### Out of scope

- Server pagination, API payload limits, global caches, and diff renderer virtualization.
- Changes to Git exclusions, user files, the affected task, or temporary directories.
- New settings, feature flags, dependencies, providers, or public API contracts.

## Technical approach

The [requirement](../../specs/ui/requirements/bounded-changes-rendering.md)
defines the new collection contract. The
[design](../../specs/ui/system-design/bounded-changes-rendering.md) owns technical details.
This is missing size coverage, rather than a rewrite of existing row requirements.

Add a data-only timeline model and one TanStack virtualizer inside `PanelBody`.
Lift row state and inline detail ownership above the window. Preserve section
order, full target identity, and existing controls. Replace sibling scans in
`buildChangesTree` with maps. Keep every expanded child as its own virtual row.

| Input | Preserved behavior | Evidence and fallback |
| --- | --- | --- |
| Local Git status | Per-repository stage/unstage/discard and change layer | Mixed repository tests; existing unavailable-workspace UI |
| Local commit | Existing session/target request and historical navigation | Local source tests; explicit retry |
| GitHub commit | Workspace/owner/repository/SHA identity, read-only files | Provider fixture; never fall back to local Git |
| Provider file projection | Existing PR key and repository routing | Existing adapters; no new provider support |

## ASCII UI preview

UI-01: Desktop Changes, tree preference, large unstaged collection.

```text
BEFORE: every expanded row mounts, even outside the viewport.
AFTER:
+--------------------------------------+
| Changes              [existing tools]| fixed header
| Unstaged (50000)         [Stage all]  | content scroller
| v cache/                             |
|   file-00000          +0 -0 [actions] |
|   file-00001          +0 -0 [actions] |
|   ... viewport window ...            |
+--------------------------------------+
Scroll to any file; offscreen rows keep metadata, not mounted controls.
```

UI-02: Phone Changes from bottom navigation, list preference.

```text
+--------------------------------+
| Changes       [existing tools] | fixed
| Unstaged (50000)               | one content scroller
| file-00000                 ... | tap identity for diff
| cache/                    +0 -0|
| file-00001                 ... | visible action menu
| cache/                    +0 -0|
+--------------------------------+
| Chat | Files | Changes | ...   | fixed, safe-area inset
+--------------------------------+
```

UI-03: Expanded historical commit in the same desktop or phone scroller.

```text
Commits (50000)
v abc123 message                [Open]
    v src/
      historical-file.ts        +4 -2
> def456 message                [Open]
```

Collapse removes children from the rendered sequence. During initial detail
loading, error, or empty results, the child area shows the existing status row.
Retry remains inline. Historical rows never acquire worktree actions.

Order, one scroller, complete counts, and touch actions are structural requirements.
Spacing and sample labels are illustrative. Phone names wrap and use measured
heights. Fine-pointer desktop density remains unchanged.
UI-01/02 cover AC-UI-BOUNDED-CHANGES-001.1 and .3-.8.
UI-03 also covers .2 and .9. Existing empty/recovery screens do not change.

## Tests

All suffixes below refer to `AC-UI-BOUNDED-CHANGES-001`.
New test names are implementation targets, not results.

| Proposed test file and case | Criteria |
| --- | --- |
| `changes-timeline-model.test.ts`: preserves complete order and source keys | .2, .3, .4 |
| `changes-file-tree-model.test.ts`: handles wide and deep inventories | .1 |
| `changes-timeline-viewport.test.tsx`: bounds 50,000 rows before and after scroll | .1, .2, .9 |
| `changes-timeline-selection.test.ts`: selects offscreen ranges and isolates repositories | .3, .4 |
| `changes-inline-commit-state.test.ts`: lazy reads, cache eviction, stale completions, retry | .7, .9 |
| `changes-timeline-interaction.test.tsx`: reveals keyboard targets and restores focus | .4, .5 |
| `changes-timeline-measurement.test.ts`: ignores hidden zero sizes and remeasures wrapping | .6, .8 |

These files live under `apps/web/components/task/`. Work orders name the
existing compatibility suites and own exact commands. Each implementation task
first records its regression failure against the prior implementation.

## E2E tests

New files under `apps/web/e2e/tests/git/`:

- `large-changes-virtualization.spec.ts`, project `chromium`.
- `mobile-large-changes-virtualization.spec.ts`, project `mobile-chrome`.
- `large-changes-helpers.ts`, shared deterministic fixture and geometry assertions.

Use isolated seeded tasks and authorized HTTP/WS test payloads for 50,000-row
stress cases. Use real Git on a small fixture for stage/unstage and routing.
Do not write 50,000 physical files or access the user's affected worktree.

| Scenario | Criteria |
| --- | --- |
| 50,000 working files, tree and list, first/last access and task switch | .1, .3, .7 |
| 50,000 commits, provider files, and one expanded commit file set | .2, .3, .9 |
| Mixed staged/unstaged/history/provider groups and duplicate paths | .2, .4 |
| Range selection across windows, keyboard traversal, open menu then scroll | .4, .5 |
| Hide/reopen, resize, live insert/remove, collapse while scrolled | .6 |
| Phone long names, 44px actions, menu focus, bottom navigation | .8 |
| Delayed old detail response during A-to-B-to-A navigation | .7, .9 |

Assert at most 120 timeline descriptors for viewports up to 1,000px tall.
Count all mounted descriptors, including hidden descendants and retained owners.
Assert contiguous visible bounds, first/last accessibility, and no horizontal
page overflow. Repeat scrolling to prove rows do not accumulate.
Arm causal waits before actions. Do not infer readiness from arbitrary sleeps.

## Work orders

- [x] [Task 01: Bound working-tree rendering](task-01-working-tree-window.md)
- [x] [Task 02: Bound historical timeline rendering](task-02-historical-window.md)
- [x] [Task 03: Prove lifecycle and mobile behavior](task-03-lifecycle-and-stress.md)

Task 02 depends on Task 01. Task 03 depends on both.

## Companion packages and documentation

The existing task-surface render-isolation package owns Files, not Changes.
The completed commit-file-navigation package owns the existing inline actions.
Their historical validation results remain unchanged. Companion links identify
this follow-up without reopening completed work orders.

[Changes timeline measurement refresh](../changes-timeline-measurement-refresh/plan.md)
owns the subsequent contiguous-row regression after presentation invalidation.
The completed work orders and historical results in this package remain intact.

Use the simple-English guidance for documentation. Implementation must assess
`docs/public/sessions-and-review.md` for any observable keyboard or retry change.
Update `apps/web/AGENTS.md` with the viewport ownership rule after implementation.
No ADR is required because this extends existing UI patterns locally.

## Verification results

Design validation passed on 2026-09-30:

- `python3 scripts/list-docs.py validate`: 331 decisions and 1,250 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed for tracked edits.
- New package files passed explicit relative-link and whitespace checks.
- Both new specifications appear in the UI catalog.

Task 01 implementation passed on 2026-09-30. Its exact commands and counts are
recorded in [Task 01](task-01-working-tree-window.md#results): 49 focused
component tests, web typecheck, 31 desktop E2E tests, and 9 mobile E2E tests.
The managed browser runs also built the production Vite bundle. Task 02 passed
on 2026-09-30; its exact commands and counts are recorded in [Task 02](task-02-historical-window.md#results): 46 focused component tests, web
typecheck, 7 desktop E2E tests, and 1 mobile E2E test. Its managed browser runs
also built the production Vite bundle. Task 03 passed on 2026-09-30. Its
focused component tests passed, typecheck and i18n checks passed, all 40 desktop
and 11 mobile E2E tests passed on the production build, and public/spec
documentation validators passed. The browser stress observations and exact
commands are recorded in [Task 03](task-03-lifecycle-and-stress.md#results).
Changed-file ESLint exited successfully with 31 warnings; the remaining
threshold warnings are recorded there.

## Risks

- A virtual parent containing an entire child list defeats the bound.
- Row-local expansion resets on unmount unless ownership moves first.
- Index-only keys can send actions to another repository after reordering.
- Hidden zero measurements can collapse the scroll geometry.
- Inline detail hooks can fetch repeatedly when headers remount.
- Metadata still scales with available data. This package does not guarantee
  bounded server payloads, full diff memory, or arbitrary collection sizes.
