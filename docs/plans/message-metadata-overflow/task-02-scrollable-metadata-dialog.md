---
id: "02-scrollable-metadata-dialog"
title: "Make the metadata dialog entries area scroll"
status: done
wave: 1
depends_on: ["01-seed-turn-metadata-and-prove-overflow"]
plan: "plan.md"
spec: "../../specs/ui/requirements/message-metadata-overflow.md"
---

# Task 02: Make the Metadata Dialog Entries Area Scroll

## Acceptance

- The Message Metadata dialog entries area scrolls when the combined debug
  fields exceed the dialog's `max-h-[85vh]`; `turn_metadata` (the last field)
  is reachable by scrolling.
- The dialog title (`DialogHeader`) and the close button remain visible while
  the entries area scrolls.
- Short metadata sets render exactly as before (no scrollbar when content
  fits).
- `message-metadata-overflow.spec.ts` and
  `mobile-message-metadata-overflow.spec.ts` (from Task 01) pass.

## Root cause (recap)

`DialogContent className="max-h-[85vh] overflow-hidden sm:max-w-2xl"` keeps
the base `display: grid`; the entries container is a grid item with no height
constraint, so its `overflow-auto` never engages and `overflow-hidden` on the
dialog clips the tail. Browser-verified: entries container
`scrollHeight == clientHeight`, `turn_metadata` `<pre>` entirely below the
visible area.

## TDD sequence

1. RED: Task 01's E2E specs already fail against the current layout.
2. GREEN: in `apps/web/components/task/chat/messages/message-actions.tsx`
   (`MessageDebugDialog`):
   - `DialogContent`: `"max-h-[85vh] overflow-hidden sm:max-w-2xl"` →
     `"flex max-h-[85vh] flex-col overflow-hidden sm:max-w-2xl"`.
     `twMerge` drops the base `grid`, so the dialog becomes a flex column.
   - `DialogHeader`: add `className="shrink-0"`.
   - Entries container: `"grid gap-3 overflow-auto pr-1"` →
     `"grid min-h-0 flex-1 gap-3 overflow-auto pr-1"`.
3. GREEN: run both E2E specs; confirm the scroll assertions pass.
4. REFACTOR: keep `grid` on the entries container (stacking unchanged); do
   not touch `MetadataValue`'s per-field `max-h-[48vh]` cap, entry ordering,
   or any i18n string.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint \
  components/task/chat/messages/message-actions.tsx)
(cd apps && pnpm --filter @kandev/web test -- \
  components/task/chat/messages/message-actions.test.tsx \
  components/task/chat/messages/message-debug-metadata.test.ts)
(cd apps/web && pnpm e2e:run --host -- tests/chat/message-metadata-overflow.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome -- \
  tests/chat/mobile-message-metadata-overflow.spec.ts)
git diff --check
```

## Files likely touched

- `apps/web/components/task/chat/messages/message-actions.tsx`

## Dependencies

- Task 01 (harness seeding + RED E2E specs).

## Parallelism

`sequential`. One class-string change, one RED-GREEN cycle.

## Inputs

- `docs/specs/ui/requirements/message-metadata-overflow.md`
- `apps/web/components/github/github-app-policy-dialog.tsx` (established
  scrollable-dialog pattern: `flex ... flex-col`, `shrink-0` header,
  `min-h-0 flex-1` scroll body)

## Output contract

Record the merged class strings after `twMerge`, the measured
scrollHeight/clientHeight before and after, and both E2E spec results for the
plan's verification results.
