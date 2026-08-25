---
id: "09-compact-settings-search"
title: "Compact Settings search field"
status: completed
wave: 9
depends_on: ["08-command-palette-settings-group"]
plan: "plan.md"
spec: "../../specs/ui/requirements/settings-discovery.md"
---

# Task 09: Compact Settings search field

## UX diagnosis

The search input is 44 px tall at every viewport while desktop Settings rows are roughly 32 px.
That mismatch makes the utility control dominate the compact sidebar hierarchy.

## Acceptance

- The desktop search input aligns vertically with neighboring Settings navigation rows.
- Desktop icon, padding, clear control, and trailing spacing scale down together.
- The phone Settings Sheet retains at least 44 px search and clear touch targets.
- Search behavior, focus styling, sticky positioning, and result layout remain unchanged.

## Verification

- `cd apps/web && pnpm e2e:run --host tests/settings/settings-discovery.spec.ts --grep "filters the tree"`
- `cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/settings/mobile-settings-discovery.spec.ts`

## Files

- `apps/web/components/app-sidebar/sections/settings/settings-search.tsx`
- `apps/web/e2e/tests/settings/settings-discovery.spec.ts`

## Mobile parity

The existing full-height Settings Sheet remains the phone entry point and navigation remains its
single scroll owner. Responsive classes compact only the desktop presentation; the phone input and
clear action remain 44 px, with shared search state and behavior.

## Parallelism

Sequential; the geometry regression defines the responsive implementation.

## Results

- RED: the production-browser geometry assertion measured a 12.5 px height mismatch between the
  44 px desktop search input and a neighboring Settings row.
- GREEN: the desktop input is 32 px, with a 14 px search icon, 32 px clear control, tighter
  horizontal padding, and 6 px trailing space; the surrounding row-height difference is at most
  2 px.
- Phone values remain unchanged at 44 px for both the search input and clear action.
- Fresh-build desktop E2E passed (1/1); the unchanged production build then passed the Pixel 5
  mobile search, containment, touch-target, navigation, and focus scenario (1/1).
- Desktop and phone Chromium screenshots were inspected against the isolated dev instance.
- Typecheck, focused ESLint, and `git diff --check` passed.
- The Tailscale test endpoint returned HTTP 200 and served the updated module; the main Kandev
  process on `:9998` remained untouched.
