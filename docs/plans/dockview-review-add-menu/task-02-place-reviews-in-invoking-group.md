---
id: "02-place-reviews-in-invoking-group"
title: "Place reviews in invoking group"
status: done
wave: 2
depends_on: ["01-filter-open-review-menu-entries"]
plan: "plan.md"
spec: "../../specs/ui/requirements/task-layout-profiles.md"
---

# Task 02: Place Reviews in Invoking Group

## Confirmed Root Cause

`AddPanelMenuItems` knows which Dockview group opened the menu, but the review
callbacks call `addPRPanel`, `addMRPanel`, and `addReviewPanel` without that
group. Those actions therefore use the canonical PR Details group or
`centerGroupId`; with no canonical tab, a click from another split lands in the
default center group.

## Acceptance

- A missing GitHub, GitLab, or registered-provider review selected from a
  group's add-panel menu opens in that group.
- A matching open review remains omitted and is never relocated.
- Topbar and host callers that do not request a group retain canonical/center
  fallback behavior.
- Action-level placement and component callback wiring have regression tests.
- The isolated Tailscale test environment demonstrates the non-default split
  flow without changing the main instance on `:9998`.

## TDD Sequence

1. Add an action regression requesting a non-default group and confirm it fails
   with the panel in `group-center`.
2. Add menu callback assertions for the invoking group and provider parity.
3. Extend review actions with optional placement and forward `groupId` from the
   menu callbacks.
4. Run focused unit suites, then the targeted Chromium Dockview scenario.
5. Run frontend static/i18n checks, docs validation, and diff hygiene.

## Verification

```bash
cd apps/web && pnpm exec vitest run components/task/dockview-add-panel-items.test.tsx lib/state/dockview-panel-actions-extra.test.ts
cd apps/web && pnpm exec eslint components/task/dockview-add-panel-items.tsx components/task/dockview-add-panel-items.test.tsx lib/state/dockview-panel-actions.ts lib/state/dockview-panel-actions-extra.test.ts lib/state/dockview-store.ts
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Parallelism

`sequential`. The action contract, menu callbacks, tests, and behavioral specs
are one coupled RED-GREEN repair.

## Results

RED confirmed: the action regression requested
`group-invoking-add-menu` but the new panel was created in `group-center`.

GREEN confirmed:

- Review actions now accept optional placement, and the add-panel menu forwards
  its invoking `groupId` for GitHub, GitLab, and registered providers.
- The focused component and action suites passed 41 tests across 2 files.
- Targeted ESLint, frontend TypeScript, i18n checks, and diff hygiene passed.
- A final production build passed the focused Chromium scenario (1 test in
  12.3 seconds), covering both unchanged topbar fallback and split-menu
  placement.
- Interactive testing in the isolated Tailscale environment opened PR #842 in
  `group-right-bottom` after using that split's `+` menu. The main instance on
  `:9998` remained untouched.
