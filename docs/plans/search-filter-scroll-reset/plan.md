---
spec: docs/specs/ui/requirements/search-filter-scroll-reset.md
created: 2026-08-06
status: completed
---

# Implementation Plan: Search/filter dropdown scroll reset

## Overview

Fix the scroll-position bug centrally in the shared `@kandev/ui/command`
`CommandList` primitive so the list scrolls back to the top whenever the active
search query changes. Because the Command Center, model selectors, comboboxes,
and the sidebar filter multi-select all render their results through this one
primitive, a single change fixes every affected input with no call-site edits.

---

## Root cause (confirmed)

`CommandList` (`apps/packages/ui/src/command.tsx`, lines 79-90) renders
`CommandPrimitive.List` from `cmdk` with `overflow-y-auto`. When the query
changes, `cmdk` re-filters/re-sorts items and re-renders, but it never resets the
list element's `scrollTop`. There is no `useEffect`/`ref` in `CommandList` (or any
consumer) that resets scroll on query change, so the previous scroll offset
persists over the freshly filtered results.

`cmdk@1.1.1` exports `useCommandState` (aliased from `useCmdk`), a selector hook
against `cmdk`'s internal store `State`, which includes `search`. `CommandList` is
always rendered inside a `Command` context in every consumer (verified:
`command-panel-footer.tsx`, `model-config-selector.tsx`,
`settings/model-combobox.tsx`, `settings/mode-combobox.tsx`, `combobox.tsx`,
`task/sidebar-filter/filter-multi-select.tsx`), so it can subscribe to
`state.search`. The Command Center uses `shouldFilter={false}` with a controlled
`CommandInput value={search}`; `cmdk` still syncs the controlled input value into
`state.search`, so the subscription reflects query changes there too.

---

## Frontend

### Shared primitive — `apps/packages/ui/src/command.tsx`

Change `CommandList` so it:

- Imports `useCommandState` from `cmdk` alongside the existing `Command as
  CommandPrimitive` import.
- Keeps an internal `ref` to the list element and merges it with any `ref` a
  consumer passes (so external `ref`s keep working). Add `ref` to the component
  signature via `React.ComponentProps<typeof CommandPrimitive.List>` (which already
  includes `ref`) and compose with a local ref (small `mergeRefs`-style callback,
  or assign inside a callback ref).
- Subscribes to the query with `const search = useCommandState((state) =>
  state.search)`.
- In a `useLayoutEffect` keyed on `[search]`, sets the list element's
  `scrollTop = 0`. `useLayoutEffect` (not `useEffect`) avoids a one-frame flash of
  the old scroll offset before the reset paints.

Keep all existing classes, `data-slot="command-list"`, and prop spreading intact.
Do not touch any consumer component.

### State

No store/hook changes. The behavior derives entirely from `cmdk`'s existing
internal search state.

---

## Tests

The shared `@kandev/ui` package has no test runner; `apps/web` has vitest + jsdom
and already renders these primitives, so the regression test lives in `apps/web`
and imports the REAL primitives (it must NOT mock `@kandev/ui/command`).

- **What:** `CommandList` resets `scrollTop` to 0 when the query changes.
  **File:** `apps/web/components/command-list-scroll-reset.test.tsx` (new).
  **How:** Render `<Command><CommandInput/><CommandList>…many CommandItems…
  </CommandList></Command>`. Grab the `[data-slot="command-list"]` element, set
  its `scrollTop` to a nonzero value, type into the input via
  `fireEvent.change`, and assert `scrollTop === 0`. (`scrollTop` is a writable
  property in jsdom.)
- **What:** query cleared resets scroll to top. **File:** same. **How:** set a
  query, set `scrollTop` nonzero, clear the input, assert `scrollTop === 0`.
- **What:** an external `ref` passed to `CommandList` still receives the list
  element. **File:** same. **How:** pass a `ref`, assert `ref.current` is the
  `[data-slot="command-list"]` node.

Run: `cd apps && pnpm install --frozen-lockfile` (fresh worktree only), then
`cd apps/web && pnpm run typecheck` and
`cd apps && pnpm --filter @kandev/web test -- components/command-list-scroll-reset.test.tsx`.

---

## E2E Tests

No new E2E is required; the behavior is covered by the unit test above and would
be brittle to assert via pixel scroll offset in Playwright. If desired later, a
Command Center E2E could type a query and assert the first result is in view, but
it is out of scope for this fix.

---

## Verification Results

- ✅ `cd apps && pnpm --filter @kandev/web test -- components/command-list-scroll-reset.test.tsx`
  - `1` test file passed
  - `5` tests passed
- ✅ `cd apps/web && pnpm run typecheck`
  - `tsc --noEmit` passed

---

## Implementation Waves And Parallel Candidates

Single task; no parallelism.

```text
Wave 1:
- [x] [task-01-command-list-scroll-reset](task-01-command-list-scroll-reset.md)
```

---

## Open Questions

None.
