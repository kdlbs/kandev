---
id: "01-command-list-scroll-reset"
title: "Reset CommandList scroll on query change"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/search-filter-scroll-reset.md"
---

# Task 01: Reset CommandList scroll on query change

## Intent

Make the shared `@kandev/ui/command` `CommandList` scroll back to the top whenever
the active `cmdk` search query changes, without altering filtering, sorting,
selection, keyboard navigation, or any consumer component. One central change
fixes the Command Center, model selectors, comboboxes, and the sidebar filter
multi-select.

## Root cause

`CommandList` in `apps/packages/ui/src/command.tsx` renders `cmdk`'s
`CommandPrimitive.List` (overflow-y-auto) but never resets `scrollTop` when the
query changes, so the prior scroll offset persists over freshly filtered results.

## Acceptance

- Changing the `cmdk` query resets the list element's `scrollTop` to 0 (verified
  by the regression test), for both the controlled-input Command Center path
  (`shouldFilter={false}`) and the default `cmdk`-filtered path.
- Clearing the query resets the list to the top.
- Moving the highlight with the arrow keys does NOT force the list back to the
  top; `cmdk`'s keyboard auto-scroll still follows the active item.
- An external `ref` passed to `CommandList` still receives the underlying list
  element, and existing `className`/`onWheel`/other props still apply.
- No consumer component is modified.

## Files likely touched

- `apps/packages/ui/src/command.tsx` (edit `CommandList`)
- `apps/web/components/command-list-scroll-reset.test.tsx` (new regression test,
  importing the real `@kandev/ui/command` primitives — must not mock them)

## Dependencies

None.

## Parallelism

Sequential. Single-file behavior change plus its focused test.

## Inputs

- Spec: `docs/specs/ui/requirements/search-filter-scroll-reset.md` (`What`, all `Scenarios`,
  `Failure modes`).
- Plan: `plan.md` (`Root cause`, `Frontend`, `Tests`).
- `cmdk@1.1.1` API: `useCommandState((state) => state.search)` (exported alias of
  `useCmdk`); `CommandPrimitive.List` accepts a forwarded `ref` to the list div.
- Existing scroll-reset precedent in the repo:
  `apps/web/components/task/chat/clamped-scroll-restore.ts`.

## Implementation notes

- Add `useCommandState` to the `cmdk` import in `command.tsx`.
- In `CommandList`, keep a local `React.useRef` to the list node, compose it with
  any incoming `ref` via a callback ref, subscribe with
  `const search = useCommandState((s) => s.search)`, and in
  `React.useLayoutEffect(() => { if (node) node.scrollTop = 0; }, [search])` reset
  the scroll. Preserve `data-slot`, classes, and `{...props}`.

## Verification

Bootstrap once if this worktree lacks dependencies:

```bash
cd apps && pnpm install --frozen-lockfile
```

TDD: write the regression test first and confirm it fails (scrollTop stays
nonzero) before editing `CommandList`, then confirm it passes after.

```bash
cd apps && pnpm --filter @kandev/web test -- components/command-list-scroll-reset.test.tsx
cd apps/web && pnpm run typecheck
```

Also run lint on the touched files if the pre-commit hook flags them.

## Results

- Added a focused regression test at `apps/web/components/command-list-scroll-reset.test.tsx`
  covering plain cmdk filtering, controlled `shouldFilter={false}` inputs, query
  clearing, forwarded refs, and callback-ref cleanup on unmount.
- Updated `apps/packages/ui/src/command.tsx` so `CommandList` subscribes to
  `useCommandState((state) => state.search)`, resets `scrollTop` in a
  `useLayoutEffect`, and preserves forwarded refs.
- Verification passed:
  - `cd apps && pnpm --filter @kandev/web test -- components/command-list-scroll-reset.test.tsx`
  - `cd apps/web && pnpm run typecheck`
