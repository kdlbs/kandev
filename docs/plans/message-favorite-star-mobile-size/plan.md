---
spec: docs/specs/ui/requirements/message-favorite-star-mobile-size.md
created: 2026-08-08
status: done
---

# Implementation Plan: Message favorite star mobile sizing

## Overview

`FavoriteButton` in the chat message action row applies a 44×44px touch target
(`min-h-11 min-w-11`) below the `sm` breakpoint while every sibling action
control uses the shared `ACTION_BUTTON_SIZE` (`h-5 w-5 p-1`, 20×20px) on all
viewports. On mobile the star therefore renders ~2.2× larger than its
neighbors. The fix makes the favorite control use the same shared sizing as
its siblings on every viewport, updates the unit and mobile E2E tests that
currently lock in the 44px behavior, and verifies parity with the sibling copy
control on the phone viewport.

## Root cause

`apps/web/components/task/chat/messages/message-actions.tsx`:

- `FavoriteButton` (added in #2008) uses
  `flex min-h-11 min-w-11 items-center justify-center sm:min-h-0 sm:min-w-0 sm:h-5 sm:w-5 sm:p-1`
  with an `h-5 w-5` (20px) `IconStar`.
- Sibling buttons (`CopyButton`, `NavigationButtons`, `MessageDebugDialog`
  trigger) use `ACTION_BUTTON_SIZE = "h-5 w-5 p-1"` with `h-full w-full` icons
  (~12px glyph inside the padded 20px button) at all viewports.

Below 640px the star control is 44×44px with a 20px glyph next to 20×20px
controls with 12px glyphs. The oversized rendering is locked in by
`message-actions.test.tsx` (asserts `min-h-11`/`min-w-11`) and by
`apps/web/e2e/tests/chat/mobile-message-favorite.spec.ts` (asserts a bounding
box ≥ 44px).

## Frontend

### `apps/web/components/task/chat/messages/message-actions.tsx`

Replace `FavoriteButton`'s className with the shared constants so it matches
the sibling buttons exactly on every viewport:

- Button: `ACTION_BUTTON_SIZE`, `ACTION_BUTTON_HOVER`,
  `ACTION_BUTTON_TRANSITION`, `"cursor-pointer"`, and the favorite
  `text-yellow-500` color. Drop the `flex min-h-11 min-w-11 items-center
  justify-center sm:min-h-0 sm:min-w-0 sm:h-5 sm:w-5 sm:p-1` string.
- Icon: `<IconStar className={cn("h-full w-full", isFavorite && "fill-yellow-500")} />`
  so the glyph matches the sibling icons' `h-full w-full` sizing.

No other component renders the favorite star control (`agent-message-content.tsx`
and `chat-message.tsx` only use `useMessageFavorite` for the bubble highlight),
so this file is the only production change.

### Mobile design contract

- **Desktop outcome:** the star control already matches sibling sizing at
  ≥ 640px; no desktop change.
- **Mobile entry point:** the chat transcript message action row, which is
  always visible below `sm` (the `opacity-100` base state).
- **Nearest shipped mobile exemplar:** the sibling action buttons in the same
  row (`ACTION_BUTTON_SIZE`, 20×20px with a ~12px glyph) — the star becomes
  visually identical to them.
- **Presentation choice:** inline, unchanged surface; no overlay, drawer, or
  navigation change.
- **Touch targets:** the star loses its sole 44px touch target and joins the
  row's existing 20px controls. This is the deliberate outcome of the report
  (consistency with sibling icons); growing every action button to 44px on
  mobile is out of scope per the spec.
- **Parity proof:** the mobile E2E asserts the star control's bounding box
  matches the copy control's bounding box (≤ 2px) on the `mobile-chrome`
  project and that tapping still toggles the favorite.

## Tests

- **What:** the favorite star button uses the shared action-button sizing and
  no longer carries the mobile 44px minimum classes.
  **File:** `apps/web/components/task/chat/messages/message-actions.test.tsx`.
  **How:** update the favorite-toggle test's class assertions — the star's
  className matches `h-5 w-5 p-1` and does not match `min-h-11`/`min-w-11`,
  and the star icon element matches `h-full w-full`. This test must fail
  before the production change (the star still carries `min-h-11`/`min-w-11`)
  and pass after.
- **What:** on the phone viewport the favorite star renders at the same size
  as the sibling copy control and still toggles by touch.
  **File:** `apps/web/e2e/tests/chat/mobile-message-favorite.spec.ts`.
  **How:** replace the ≥ 44px bounding-box assertions with a comparison of the
  star button's `boundingBox()` against the row's copy button
  (`Copy message to clipboard`), asserting width and height each differ by at
  most 2px, and keep the tap-toggle flow.

## E2E Tests

- **Scenario:** GIVEN a chat message with its action row visible on a phone
  viewport, WHEN the user compares the star with the copy action, THEN the two
  controls render at the same size (≤ 2px) and tapping the star still toggles
  the favorite.
  **File:** `apps/web/e2e/tests/chat/mobile-message-favorite.spec.ts`, run by
  the `mobile-chrome` project.

## Verification Results

- `cd apps && pnpm install --frozen-lockfile` — bootstrap, done in 6.8s.
- `cd apps && pnpm --filter @kandev/web test -- components/task/chat/messages/message-actions.test.tsx` — 1 file, 6 tests passed (ran red before the fix, green after).
- `cd apps/web && NODE_OPTIONS="--max-old-space-size=6144" pnpm run typecheck` — passed (plain run OOM-crashed on this 14Gi box with saturated swap; bounded heap succeeded).
- `cd apps/web && pnpm exec eslint components/task/chat/messages/message-actions.tsx components/task/chat/messages/message-actions.test.tsx e2e/tests/chat/mobile-message-favorite.spec.ts` — passed, no errors.
- `cd apps/web && pnpm exec prettier --check ...` — all matched files use Prettier code style.
- `cd apps/web && pnpm e2e:run --host --project mobile-chrome -- tests/chat/mobile-message-favorite.spec.ts` — 1 passed (10.6s) with the production build (runner rebuilt the Go backend, `build:e2e` web bundle, and plugin fixture package).
- `git diff --check` — passed.

Generated artifacts: `apps/web/dist/` and `apps/backend/bin/kandev` from the
E2E build; the runner cleaned its temporary backend. No external systems
changed.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [task-01-align-star-button-sizing](task-01-align-star-button-sizing.md)

Single sequential task: the component change, its unit test, and the mobile
E2E are one behavior and share the same files.

## Risks

- The mobile E2E compares two `boundingBox()` values; sub-pixel rounding can
  make them differ by 1px, so the tolerance is 2px. The copy button is always
  rendered for `type: "message"` messages (the seeded fixture type), so the
  sibling is reliably present.
- Losing the 44px mobile touch target on the star is intentional per the
  report and spec; if the row-wide touch target question comes up later, it is
  a separate change.
- Fresh worktrees lack `apps/node_modules`; the task includes the
  `pnpm install --frozen-lockfile` bootstrap before any pnpm command.

## Open Questions

None.
