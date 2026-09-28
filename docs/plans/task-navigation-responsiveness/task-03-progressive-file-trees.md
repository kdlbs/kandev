---
id: "03-progressive-file-trees"
title: "Restore file trees progressively"
status: done
wave: 3
depends_on:
  - "02-shared-session-reads"
plan: "plan.md"
requirements:
  - REQ-UI-TASK-NAVIGATION-RESPONSIVENESS-001
acceptance_criteria:
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.3
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.4
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.6
system_design:
  - ../../specs/ui/system-design/task-navigation-responsiveness.md
---

# Task 03: Restore File Trees Progressively

## Summary

Publish available file-tree content before all expanded folders finish loading.
Retain a bounded set of recent valid trees and restore independent folders
concurrently, preserving owner and parent dependencies.

## In scope

- Store-owned metadata snapshots with four inactive-entry and 20,000-node total
  retention limits, scope checks, and least-recently-used eviction.
- Immediate retained-tree/root publication and at most four active-owner folder
  reads, with per-path sharing between restoration and manual expansion.
- Correct concurrent merging, cancellation-by-owner, transient branch retry,
  authoritative removal, and usable partial-tree presentation.

## Out of scope

File content caching, new filesystem APIs, prefetching collapsed descendants,
virtualizer replacement, changed file actions, or persisted cache/settings.

## Acceptance

1. A valid retained tree is usable before refresh completes; an uncached root
   appears before held children finish. Independent siblings overlap within the
   limit; descendants wait for their parents. New merges preserve earlier
   sibling results and unchanged subtree identities (`.3`, `.4`).
2. A-to-B-to-A and same-environment/new-session races cannot publish old work.
   Budget eviction and auth/workspace/connection/recovery changes cannot reuse
   invalid data. Authoritative unavailability overrides cached rows (`.5`).
3. Transient branch failure retains available rows and expansion intent, with
   retry; authoritative missing paths remove only affected expansions. Desktop
   and phone preserve file actions, scroll/selection, and loading/error hierarchy
   while partial data is available (`.4`, `.6`).

## ASCII UI preview

UI-02: Files during restoration or branch retry. Full [preview](plan.md#ascii-ui-preview).

```text
Desktop task                         Phone task / Files
+---------------+----------------+   +--------------------------+
| Current task  | Files toolbar  |   | Task header              |
| chat/editor   | v src          |   | Files toolbar            |
|               |   app.ts       |   | v src               [...]|
|               | > pending-dir  |   |   app.ts            [...]|
|               | loading/retry  |   | > pending-dir       [...]|
+---------------+----------------+   | loading/retry            |
                                     +--------------------------+
                                     | Existing bottom nav      |
                                     +--------------------------+
```

Names and state labels are illustrative. Reuse localized loading/retry strings.
Only uncached initial loading or authoritative unavailability replaces the
usable tree. The existing Files viewport scrolls; phone navigation clears safe
areas and touch actions retain 44px targets. See criteria `.3`-`.6`.

## Implementation sequence

1. Mark in progress. Extend the actual loader-hook tests to hold two child
   responses and assert root publication and sibling request overlap. These must
   fail against the serial implementation for the expected behavioral reason.
2. Add bounded tree-cache tests for entry/node eviction, oversized-tree fallback,
   current-context reuse, and owner retirement; then implement the cache.
3. Extend the loader's existing `TreeLoadOwner` generation checks to progressive
   publication and every scheduled request. Add an owner/path read registry;
   manual expansion joins a matching outstanding restoration read.
4. Schedule parent-ready folders with the concurrency limit. Merge each response
   against the current tree rather than a captured starting tree. Distinguish
   transient failure from authoritative absence when pruning expansions.
5. Make `useFileBrowserTree` restore eligible cached data without clearing it,
   while preserving the workspace readiness/empty-root retry contract.
6. Audit `treeLoaded`/load-state callers so available nodes can open and expand
   before full restoration finishes. Show loading/retry feedback alongside them;
   preserve authoritative recovery precedence, selection, and scroll behavior.
7. Add late-completion, collapse/expand, failed-branch retry, missing-parent,
   root-empty, and same-environment/new-session cases. Run targeted checks.
   Task 04 verifies UI-02 through actual desktop and phone navigation.

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/task/file-browser-tree-cache.test.ts components/task/file-browser-restore-loader.test.tsx components/task/file-browser-restore-expanded.test.tsx components/task/file-browser-load-state.test.tsx components/task/file-browser-load-children.test.ts components/task/file-browser-toggle-expand.test.ts components/task/file-browser-apply-changes.test.ts components/task/file-browser-render-identity.test.tsx components/task/file-browser-reset-key.test.ts)
(cd apps/web && pnpm exec eslint components/task/file-browser-tree-cache.ts components/task/file-browser-tree-cache.test.ts components/task/file-browser-restore.ts components/task/file-browser-tree-loader.ts components/task/file-browser-hooks.ts components/task/file-browser.tsx components/task/file-browser-load-state.tsx components/task/file-browser-actions.ts components/task/file-browser-restore-loader.test.tsx components/task/file-browser-restore-expanded.test.tsx components/task/file-browser-load-state.test.tsx components/task/file-browser-load-children.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/task/file-tree-lazy-load.spec.ts tests/task/large-file-tree-virtualization.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-large-file-tree-virtualization.spec.ts tests/task/mobile-file-tree-chat-context.spec.ts)
git diff --check
```

The existing E2E cases protect geometry/actions; new causal navigation coverage
belongs to Task 04. If new visible strings are unavoidable, add all supported
locales and also run `(cd apps/web && pnpm run i18n:check)`.

## Files likely touched

- `apps/web/components/task/file-browser-tree-cache.ts` and `.test.ts` (new)
- `apps/web/components/task/file-browser-tree-state.ts` and `.test.tsx` (new)
- `apps/web/components/task/file-browser-tree-reader.ts` (new)
- `apps/web/components/task/file-browser-data.ts`
- `apps/web/components/task/file-browser-restore.ts`
- `apps/web/components/task/file-browser-tree-loader.ts`
- `apps/web/components/task/file-browser-hooks.ts`
- `apps/web/components/task/file-browser-actions.ts` (`loadNodeChildren`)
- `apps/web/components/task/file-browser.tsx`
- `apps/web/components/task/file-browser-load-state.tsx`
- Corresponding loader, expansion, load-state, and child-loading tests listed above

## Dependencies

Task 02 establishes the scope identity conventions. The tree scheduler owns its
per-path requests; it must not add a second generic session query abstraction.

## Risks

Cached rows can outlive a workspace if keys omit recovery generations. Parallel
merges can lose siblings or trigger full-tree rerenders. Retry feedback must
not hide valid rows. Mobile unmounts Files whereas desktop Dockview may retain
it, so cover both lifetimes. Uncancelable obsolete WS requests may still settle;
stop scheduling their descendants and reject publication.

## Parallelism

`sequential`

## Inputs

- [Requirements `.3`-`.6`](../../specs/ui/requirements/task-navigation-responsiveness.md)
- [File-tree retention and restoration design](../../specs/ui/system-design/task-navigation-responsiveness.md#file-tree-retention)
- [Existing render isolation](../../specs/ui/system-design/task-surface-render-isolation.md)
- Existing loader tests, `restoredExpandedPaths`, and `completeRestoredTree`
- `mobile/session-mobile-layout.tsx` and `mobile/session-task-switcher-sheet.tsx`

## Results

- RED: root publication, sibling overlap, transient branch availability, warm
  navigation, cache budgets, stale file-watch completion, and phone recovery
  identity each failed before their corresponding corrections.
- GREEN: 57 loader/cache/state tests plus four responsive component tests passed.
  The 95-test combined regression run also passed after the final owner fix.
- Desktop lazy loading and large-tree virtualization: five browser cases passed.
  Phone virtualization, visible actions, coarse-pointer geometry, workspace
  isolation, and the new task-chooser/Files/retry case passed. Final navigation
  evidence is recorded in Task 04.
- `file-browser-tree-state.ts` binds retained metadata to canonical environment
  recovery, including phone callers using session-based UI preference keys.
  `file-browser-tree-reader.ts` shares pending paths with manual expansion.
  `file-browser-data.ts` supplies the binding. These small helpers are part of
  this work order's implementation alongside the originally listed files.
- Targeted ESLint and frontend typecheck passed. Existing responsive component
  fixtures now mock the new cache-binding boundary alongside their mocked tree
  loader; real-store recovery is covered separately.

PR review added a direct real-provider tree-state regression for two batched
folder completions. Both merges remain visible, reach the shared cache, and
survive A-to-B-to-A rebinding. This test passed without production tree changes.
