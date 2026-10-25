---
id: "01-align-star-button-sizing"
title: "Align favorite star button sizing with sibling action icons"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/message-favorite-star-mobile-size.md"
---

# Task 01: Align favorite star button sizing with sibling action icons

## Intent

Make the chat message favorite star control the same size as the sibling
action icons (copy, navigation, metadata) on every viewport, including mobile
< 640px, and update the unit and mobile E2E tests that currently lock in the
oversized 44px mobile rendering.

## Root cause

`FavoriteButton` in `apps/web/components/task/chat/messages/message-actions.tsx`
applies `min-h-11 min-w-11` (44×44px) below the `sm` breakpoint with a 20px
`IconStar`, while every sibling button uses `ACTION_BUTTON_SIZE = "h-5 w-5 p-1"`
(20×20px) with `h-full w-full` icons at all viewports. The star is locked in
as oversized by `message-actions.test.tsx` (asserts `min-h-11`/`min-w-11`) and
`apps/web/e2e/tests/chat/mobile-message-favorite.spec.ts` (asserts a bounding
box ≥ 44px).

## Acceptance

- The favorite star button's className carries `ACTION_BUTTON_SIZE`
  (`h-5 w-5 p-1`) and contains neither `min-h-11` nor `min-w-11`; the star
  icon is `h-full w-full`, matching the sibling action buttons.
- On the `mobile-chrome` viewport, the star button's bounding box matches the
  sibling copy button's bounding box within 2px on both dimensions, and
  tapping the star still toggles the favorite state.
- Desktop rendering is unchanged: the star control already matches sibling
  sizing at ≥ 640px and no other component renders a favorite star control.

## Regression tests (write first — must fail before the production change)

- `apps/web/components/task/chat/messages/message-actions.test.tsx`: in the
  favorite-toggle test, replace the `min-h-11`/`min-w-11` className
  expectations with expectations that the star button matches
  `h-5 w-5 p-1` and does not match `min-h-11`/`min-w-11`, and that the icon
  element matches `h-full w-full`. (The current test at lines ~99-100 asserts
  the buggy classes.)
- `apps/web/e2e/tests/chat/mobile-message-favorite.spec.ts`: replace the
  `box!.width/height >= 44` assertions with a comparison of the star button's
  `boundingBox()` against the row's copy button (`Copy message to clipboard`):
  `Math.abs(widthDiff) <= 2` and `Math.abs(heightDiff) <= 2`. Keep the tap
  toggle flow asserting the label switches to "Remove message from favorites".

## Files likely touched

- `apps/web/components/task/chat/messages/message-actions.tsx`
- `apps/web/components/task/chat/messages/message-actions.test.tsx`
- `apps/web/e2e/tests/chat/mobile-message-favorite.spec.ts`

## Dependencies

None.

## Parallelism

Sequential. The component change, unit test, and mobile E2E are one behavior
and share the same files.

## Inputs

- Spec: `What` and all three `Scenarios`.
- Plan: `Root cause`, `Frontend`, `Tests`, and `E2E Tests`.
- Existing patterns: `ACTION_BUTTON_SIZE` / `ACTION_BUTTON_HOVER` /
  `ACTION_BUTTON_TRANSITION` constants and sibling button markup in
  `apps/web/components/task/chat/messages/message-actions.tsx`; the seeded
  message fixture and `SessionPage` / `activeChat()` helpers used by
  `apps/web/e2e/tests/chat/mobile-message-favorite.spec.ts`.

## Verification

Bootstrap once — this worktree is fresh and lacks `apps/node_modules`:

```bash
cd apps && pnpm install --frozen-lockfile
```

Then, from the repo root:

```bash
# 1. Regression unit test (red before the fix, green after)
cd apps && pnpm --filter @kandev/web test -- components/task/chat/messages/message-actions.test.tsx

# 2. Typecheck
cd apps/web && pnpm run typecheck

# 3. Mobile E2E (production build required — rebuild the web bundle first)
cd apps/web && pnpm run build:vite
cd apps/web && pnpm e2e:run --no-build --project mobile-chrome -- tests/chat/mobile-message-favorite.spec.ts

# 4. Lint the touched files
cd apps/web && pnpm exec eslint components/task/chat/messages/message-actions.tsx components/task/chat/messages/message-actions.test.tsx e2e/tests/chat/mobile-message-favorite.spec.ts

# 5. Format and whitespace
cd apps/web && pnpm exec prettier --check components/task/chat/messages/message-actions.tsx components/task/chat/messages/message-actions.test.tsx e2e/tests/chat/mobile-message-favorite.spec.ts
git diff --check
```

## Output contract

Report the exact className change, the updated test assertions, exact command
results (red test output before the fix, green after), any E2E blockers, and
synchronized task/plan status. Do not change any other message action sizing,
the favorite state model, or the accessible labels.

## Results

- `FavoriteButton` now uses the shared `ACTION_BUTTON_SIZE` (`h-5 w-5 p-1`) with
  a `h-full w-full` icon on every viewport; the `flex min-h-11 min-w-11
  items-center justify-center sm:min-h-0 sm:min-w-0 sm:h-5 sm:w-5 sm:p-1`
  mobile-only 44px sizing is removed. Desktop rendering is unchanged.
- Unit regression: `message-actions.test.tsx` now asserts the star carries
  `h-5 w-5 p-1`, no `min-h-11`/`min-w-11`, and an `h-full w-full` icon. Ran
  red before the fix (`expected 'flex min-h-11 min-w-11 items-center j…' to
  contain 'h-5 w-5 p-1'`), green after — 6 tests passed.
- Mobile E2E regression: `mobile-message-favorite.spec.ts` now asserts the
  star's bounding box matches the sibling copy button within 2px on both
  dimensions on the `mobile-chrome` viewport and that tapping still toggles
  the favorite. Passed: 1 test, 10.6s, against the production build.
- `cd apps && pnpm install --frozen-lockfile` — bootstrap, done in 6.8s.
- `cd apps && pnpm --filter @kandev/web test -- components/task/chat/messages/message-actions.test.tsx` — 1 file, 6 tests passed.
- `cd apps/web && NODE_OPTIONS="--max-old-space-size=6144" pnpm run typecheck` — passed (first attempt OOM-crashed on this 14Gi box with a saturated swap; bounded heap succeeded).
- `cd apps/web && pnpm exec eslint <3 touched files>` — passed, no errors.
- `cd apps/web && pnpm exec prettier --check <3 touched files>` — all matched files use Prettier code style.
- `cd apps/web && pnpm e2e:run --host --project mobile-chrome -- tests/chat/mobile-message-favorite.spec.ts` — 1 passed (10.6s); runner rebuilt the Go backend, the `build:e2e` web bundle, and the plugin fixture package.
- `git diff --check` — passed.

Generated artifacts: `apps/web/dist/` (Vite `build:e2e` output) and
`apps/backend/bin/kandev` (E2E backend build). No external systems changed;
E2E used an isolated temporary backend that was cleaned up by the runner.
