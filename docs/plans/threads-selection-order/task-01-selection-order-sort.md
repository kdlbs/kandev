---
id: "01-selection-order-sort"
title: "Add the Selection order sort to Threads views"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-THREADS-SAVED-VIEWS-003
acceptance_criteria:
  - AC-UI-THREADS-SAVED-VIEWS-003.14
  - AC-UI-THREADS-SAVED-VIEWS-003.15
  - AC-UI-THREADS-SAVED-VIEWS-003.16
  - AC-UI-THREADS-SAVED-VIEWS-003.17
  - AC-UI-THREADS-SAVED-VIEWS-003.18
system_design:
  - ../../specs/ui/system-design/threads-saved-views.md
---

# Task 01: Add the Selection Order Sort to Threads Views

## Summary

Add a `selection` sort key that orders a `selected`-scope view by the existing
`taskScope.taskIds` order. Expose it in View settings only for that scope and
show selection positions in the task picker. The deck order then survives a
reload.

## In scope

- `ThreadSortKey`, `THREAD_SORT_KEYS`, and `THREAD_SORT_OPTIONS` entries.
- Effective-sort resolution and the index comparator in `queryThreadView`.
- Scope-dependent option visibility and the one-update scope/sort reset.
- Picker ordinals while Selection order is active.
- `threads:sortSelection`, `threads:sortSelectionDescription`, and
  `threads:selectionPosition` in all seven catalogs.
- A Go round-trip test that proves the submitted `task_ids` order is kept.
- Desktop and phone E2E evidence, plus the public Threads usage text.

## Out of scope

Drag-and-drop reordering, persisted on-screen order for other sorts, new
settings fields, backend sort-key validation, stable-order arrival changes,
and sidebar views.

## Acceptance

1. With the `selected` scope and Selection order, `queryThreadView` returns
   candidates in `taskIds` order (reversed for descending). The order is the
   same after a reload and ignores attention or activity changes. A stored
   `selection` sort with the `all` scope orders as ascending attention.
2. View settings lists Selection order only for the `selected` scope. Switching
   to `all` while it is active writes `{ key: "attention", direction: "asc" }`
   in the same draft update. Checked picker rows show one-based positions; a
   cleared and reselected task moves to the end.
3. Targeted unit, Go, i18n, desktop E2E, and phone E2E checks pass. The
   rendered editor and picker match UI-01 and UI-02.

## ASCII UI preview

UI-01 and UI-02, excerpt from the [combined preview](plan.md#ascii-ui-preview).

```text
Sort:         [Selection order  v] [^]  Reapply sort   (Selected tasks only)
Choose tasks: [x] 2  Task B
              [ ]    Task D
              [x] 1  Task A
              [x] 3  Task C
```

The ordinal sits between the checkbox and the task-state icon. Picker row
order does not change. Phone uses the same drawer page with 44-pixel rows.

## Verification

Run from the repository root. Install once in this fresh worktree. Use `/tdd`;
start with failing `thread-view-query` cases for both directions and the `all`
fallback.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/threads/thread-view-query.test.ts lib/threads/stable-order.test.ts lib/state/slices/ui/thread-view-wire.test.ts lib/state/slices/ui/thread-view-actions.test.ts components/threads/threads-view-editor-utils.test.ts components/threads/threads-view-filter-registry.test.ts components/threads/threads-view-controls.test.tsx)
(cd apps/backend && go test ./internal/user/service/ ./internal/user/store/ -run 'ThreadView')
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm exec eslint lib/threads/thread-view-query.ts lib/state/slices/ui/thread-view-types.ts lib/state/slices/ui/thread-view-wire.ts components/threads/threads-view-filter-registry.ts components/threads/threads-view-editor-sections.tsx components/threads/threads-view-editor-utils.ts components/threads/threads-view-task-picker.tsx e2e/tests/task/threads-view.spec.ts e2e/tests/task/mobile-threads-view.spec.ts --max-warnings 0)
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/task/threads-view.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/task/mobile-threads-view.spec.ts
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

The desktop E2E starts three agent tasks, selects them in a non-default order,
chooses Selection order, saves, reloads, and asserts `data-thread-column-id`
order. The phone E2E asserts the option and ordinals inside the native drawer.

## Likely files

- `apps/web/lib/state/slices/ui/thread-view-types.ts`
- `apps/web/lib/state/slices/ui/thread-view-wire.ts` and its test
- `apps/web/lib/threads/thread-view-query.ts` and its test
- `apps/web/components/threads/threads-view-filter-registry.ts` and its test
- `apps/web/components/threads/threads-view-editor-sections.tsx`
- `apps/web/components/threads/threads-view-editor-utils.ts` and its test
- `apps/web/components/threads/threads-view-task-picker.tsx`
- `apps/web/components/threads/threads-view-controls.test.tsx`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/threads.json`
- `apps/backend/internal/user/service/thread_views_test.go`
- `apps/web/e2e/tests/task/threads-view.spec.ts`
- `apps/web/e2e/tests/task/mobile-threads-view.spec.ts`
- `apps/web/components/threads/AGENTS.md` (stable-order note)
- `docs/public/sessions-and-review.md`

## Dependencies and risks

- `threads-view-editor-sections.tsx` is near the file-length limit. Keep the
  scope/sort rule in `threads-view-editor-utils.ts`.
- `compareSortKey` already carries a complexity suppression. Resolve the
  effective sort and the `selection` comparison outside that switch.
- E2E tasks need real agent turns, so keep the new desktop case to three tasks
  and reuse `startAgentTask`.

## Results

Implemented on 2026-10-28 with TDD. Unit RED came first for the comparator,
the wire round trip, option visibility, and the scope/sort helper. The E2E
cases were added after the production change and run against a fresh managed
build, so they are regression evidence rather than the RED gate. The Go test is
contract coverage and passed on the existing store, which already keeps order.

- `queryThreadView` resolves the effective sort; `selection` ranks by
  `taskScope.taskIds` and falls back to ascending attention for `all`.
- `threadSortOptionsForScope` hides the option for `all`;
  `threadTaskScopeModeUpdate` writes the scope and the attention reset in one
  draft update.
- The task picker shows a muted ordinal and adds the position to the checkbox
  accessible name while Selection order is active.
- Copy added in seven locales plus pseudo; Traditional Chinese generated with
  `pnpm run i18n:zh-hant` (its unrelated `task.json` rewrites were discarded).
- Public usage text: `docs/public/sessions-and-review.md`; scoped note:
  `apps/web/components/threads/AGENTS.md`.

Verification:

- Targeted Vitest (7 files): 102 passed. Wider `components/threads`,
  `app/threads`, `lib/threads`, `lib/state/slices/ui`: 47 files, 496 passed.
- `go test ./internal/user/service/ ./internal/user/store/ -run ThreadView`: ok.
  `scripts/lint-go-changed`: 0 issues.
- `pnpm run typecheck` (needed `NODE_OPTIONS=--max-old-space-size=8192`
  locally), `pnpm run i18n:check`, `pnpm run i18n:ratchet`: pass.
- ESLint `--max-warnings 0` and Prettier on changed TS files: pass.
- Desktop `threads-view.spec.ts`: 10 passed. Phone
  `mobile-threads-view.spec.ts`: 8 passed.
- `list-docs.py validate` and `lint-spec-files.py --all`: pass.

PR review remediation (PR #4916):

- The sort picker now displays `effectiveThreadSort`, so a stored `selection`
  sort with the `all` scope shows ascending attention instead of an unlabelled
  key and a stale direction. `ATTENTION_SORT` has one owner in
  `thread-view-query.ts`.
- `AC-UI-THREADS-SAVED-VIEWS-003.15` and the public guide state the live
  arrival exception; behavior is unchanged.
- Vitest on `components/threads`, `app/threads`, `lib/threads`,
  `lib/state/slices/ui`: 47 files, 498 passed.
