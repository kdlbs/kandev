---
id: "10-settings-result-motion"
title: "Subtle Settings result motion"
status: completed
created: 2026-08-05
wave: 10
depends_on: ["09-compact-settings-search"]
plan: "plan.md"
spec: "../../specs/ui/requirements/settings-discovery.md"
---

# Task 10: Subtle Settings result motion

## Goal

Make Settings-tree search results feel continuous as filtering changes their positions, without
adding conspicuous motion or weakening reduced-motion behavior.

## Implementation

- Add a small FLIP-style layout transition to result rows and persistent group headings in
  `apps/web/components/app-sidebar/sections/settings/settings-search.tsx`.
- Fade newly matching entries in over only a few pixels; do not animate the initial Settings tree
  or delay interaction.
- Cancel superseded animations so rapid typing remains responsive.
- Skip all result motion when `prefers-reduced-motion: reduce` is active.
- Keep the existing grouping, ranking, navigation, focus, and 44 px phone targets unchanged.

## Validation

- Run focused TypeScript and ESLint checks for the Settings search component.
- Exercise changing queries in desktop Chromium and confirm a surviving row moves continuously.
- Exercise the same flow with reduced motion and confirm results update immediately.
- Re-run the desktop and phone Settings-discovery Playwright coverage.

## Result

- `settings-search.test.tsx`: 2 motion-contract tests passed.
- `settings-tree-render.test.tsx`: 16 existing Settings-tree tests passed.
- Focused ESLint and frontend typecheck passed.
- Fresh production build passed.
- Desktop Settings discovery E2E: 4 tests passed, including live layout and reduced-motion checks.
- Mobile Settings discovery E2E: 1 test passed with the existing 44 px and containment assertions.
