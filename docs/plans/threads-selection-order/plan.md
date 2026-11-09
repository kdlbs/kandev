---
created: 2026-10-28
status: implemented
requirements:
  - REQ-UI-THREADS-SAVED-VIEWS-003
system_design:
  - ../../specs/ui/system-design/threads-saved-views.md
legacy_specs: []
---

# Implementation Plan: Threads Selection Order Sort

## Overview

A user who picks exact tasks for a Threads view expects the columns to stay in
the order in which they picked them. Today the deck orders those columns by
the view's sort, which defaults to attention. The stable column order lives
only in memory, so a reload ranks the tasks again by their current attention
and activity. Columns then appear shuffled.

The [Threads saved views requirement](../../specs/ui/requirements/threads-saved-views.md)
owns sort and stable order. `AC-UI-THREADS-SAVED-VIEWS-003.14` through
`003.18` add a Selection order sort for the `selected` scope. The
[paired design](../../specs/ui/system-design/threads-saved-views.md#selection-order)
derives the order from the existing ordered `taskScope.taskIds` array. No
persistence field, endpoint, or backend change is needed. No ADR is needed,
because the change reuses the existing sort contract and settings storage.

The user chose this approach over persisting the live on-screen order of any
view. That alternative remains out of scope.

## Evidence

Read-only trace at `b4e9cf5ed`:

1. `lib/threads/thread-view-query.ts:queryThreadView` filters by scope, then
   sorts with `compareCandidates`. No sort key uses `taskScope.taskIds` order.
2. `lib/threads/stable-order.ts:useStableThreadOrder` keeps column slots in a
   React ref. A reload starts with an empty ref and uses the sorted order.
3. `components/threads/threads-view-editor-utils.ts:updateThreadTaskSelection`
   appends a newly checked ID. `updateVisibleThreadTaskSelection` appends in
   visible picker order. The array therefore already records selection order.
4. `apps/backend/internal/user/service/thread_views.go` validates
   `task_ids` count and content without reordering. The store persists the
   JSON array as submitted.
5. `lib/state/slices/ui/thread-view-wire.ts:normalizeSort` maps an unknown key
   to `attention`. An older client therefore reads `selection` as attention.

## Scope

In scope: the `selection` sort key, comparator, wire key list, sort option and
copy in seven locales, scope-dependent option visibility, the scope-change
sort reset, picker ordinals, a backend order round-trip test, desktop and
phone E2E evidence, and the public Threads usage text.

Out of scope: drag-and-drop reordering, persisting on-screen order for other
sorts, a new settings field, backend validation of sort keys, sidebar views,
and changes to stable-order arrival rules.

## Technical approach

- Add `"selection"` to `ThreadSortKey` and `THREAD_SORT_KEYS`.
- In `thread-view-query.ts`, resolve the effective sort before comparing. With
  `selection` and the `selected` scope, compare candidate indexes in
  `taskScope.taskIds`. With the `all` scope, use ascending attention.
- Add a Selection order entry to `THREAD_SORT_OPTIONS`. Add
  `threads:sortSelection`, `threads:sortSelectionDescription`, and
  `threads:selectionPosition` in English and the six complete locales. Run
  `pnpm run i18n:zh-hant` for the Traditional Chinese pair.
- Pass the scope mode to `ThreadSortSection` and omit Selection order for
  `all`. In `TaskScopeSection`, emit one `onUpdate({ taskScope, sort })` when
  `all` replaces a `selection` sort. Keep that rule in a pure helper in
  `threads-view-editor-utils.ts`.
- Pass the active sort key to `ThreadsViewTaskPicker`. Show a muted one-based
  ordinal on checked rows while the key is `selection`.
- Update the Threads paragraph in `docs/public/sessions-and-review.md` through
  `/docs-maintainer`.

Source paths are relative to `apps/web/` unless they start with `apps/` or
`docs/`.

## ASCII UI preview

UI-01: View settings, Sort section, scope `Selected tasks`. Entry point: the
view settings control in the Threads top bar.

```text
Desktop popover (Sort section)
+---------------------------------------------+
| SORT                                        |
| [Selection order  v] [^]  Reapply sort      |
+---------------------------------------------+
  Sort options when scope = Selected tasks:
    Selection order  Order in which you selected the tasks.
    Attention        Puts tasks that need you first, ...
    Last activity    ...
  Scope = All eligible tasks: Selection order is not listed.
```

UI-02: Choose tasks page with Selection order active.

```text
Desktop popover / phone drawer page (same composition)
+---------------------------------------------+
| <-  Choose tasks                            |
|     3 tasks selected                        |
| [Search tasks                            ]  |
| [-] Select all                 Clear all    |
|---------------------------------------------|
| [x] 2  Remover lista de ecras morta W102    |
| [ ]    Some unselected task                 |
| [x] 1  Spec 2: implementar SibsRequestLog   |
| [x] 3  AtmProducts: migrar para M3DF 7.11   |
+---------------------------------------------+
```

The ordinal sits between the checkbox and the existing task-state icon. Rows
keep their current picker order. The ordinal shows selection position only.
Phone rows keep 44-pixel touch targets and the drawer keeps one vertical
scroll owner. Spacing and sample titles are illustrative.

UI-03: Deck after a reload with Selection order.

```text
Before (attention, after F5)      After (Selection order, after F5)
+------+------+------+            +------+------+------+
| C    | A    | B    |            | A    | B    | C    |
+------+------+------+            +------+------+------+
                                   Selected A, then B, then C.
```

Coverage: UI-01 maps to `003.14` and `003.17`. UI-02 maps to `003.16` and
`003.18`. UI-03 maps to `003.14` and `003.15`.

## Work orders

| Order | Work order                                              | Wave | Depends on |
| ----- | ------------------------------------------------------- | ---- | ---------- |
| 01    | [Selection order sort](task-01-selection-order-sort.md) | 1    | none       |

## Verification strategy

Unit tests prove the comparator, both directions, the `all` fallback, wire
round-trip, the scope-change helper, option visibility, and picker ordinals. A
Go test proves `task_ids` order survives a settings round trip. Desktop E2E
selects three tasks in a chosen order, saves, reloads, and asserts the column
order. Mobile E2E proves the drawer exposes the same option and ordinals.

## Risks

- An older client that saves a Selection order view writes attention. All
  clients normally update together, so this risk is accepted and documented.
- A selected task that becomes eligible while the page is open appends at the
  end until a reload or Reapply sort. This keeps the existing rule that live
  changes never move columns.
- Select all appends visible tasks in picker order. Users who need another
  order must select tasks one by one.
