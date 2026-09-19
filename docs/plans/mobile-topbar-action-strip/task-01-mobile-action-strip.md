---
id: "01-mobile-action-strip"
title: "Build the mobile action strip"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/mobile-quick-chat-topbar.md"
---

# Task 01: Build the mobile action strip

## Intent

Make the phone Home and Tasks action region compact and horizontally scrollable. Keep Kandev and
the menu fixed. Normalize native, metric, and documented plugin icon geometry.

## Acceptance

- Terminal, chat, search, and menu use 32px visible boxes with 16px icons. Topbar metric and
  documented plugin icons also use 16px icons. Terminal and chat retain a 44px coarse-pointer hit
  area without widening their visible boxes.
- Page context, plugin actions, metrics, terminal, chat, and search use one middle strip. Kandev and
  menu stay outside that scroll owner.
- Each directional fade appears only while content remains hidden beyond its edge.
- Desktop and tablet header geometry, state, handlers, and existing public plugin slot fields remain
  unchanged. The mobile presentation is exported from the public plugin SDK.

## Files likely touched

- `apps/web/components/kanban/mobile-topbar-action-strip.tsx`
- `apps/web/components/kanban/mobile-topbar-action-strip.test.tsx`
- `apps/web/components/kanban/kanban-header-mobile.tsx`
- `apps/web/components/kanban/kanban-header-mobile.test.tsx`
- `apps/web/components/kanban/main-top-bar-plugin-actions.tsx`
- `apps/web/components/kanban/main-top-bar-plugin-actions.test.tsx`
- `apps/web/components/system-metrics/topbar-metrics.tsx`
- `apps/web/components/system-metrics/topbar-metrics.test.tsx`
- `apps/web/lib/plugins/types.ts`
- `apps/packages/plugin-sdk/src/index.ts`
- `apps/web/lib/plugins/sdk-contract.test.ts`
- `docs/plans/plugins/PLUGIN-API.md`
- `docs/public/plugins-authoring.md`

## Dependencies

None.

## Parallelism

`sequential`. The component, tests, and public contract describe one shared layout behavior.

## Inputs

- Spec: `What`, `Scenarios`, and `Out of scope` in
  `docs/specs/ui/requirements/mobile-quick-chat-topbar.md`.
- Plan: `Confirmed root cause`, `Frontend`, `Mobile design contract`, and `Tests` in `plan.md`.
- Fade exemplar: `apps/web/components/task/chat/chat-input-toolbar-mobile.tsx`.
- Resize exemplar: `apps/web/components/task/task-sidebar-scroll-area.tsx`.
- Shared button sizes: `apps/packages/ui/src/button.tsx`.

## TDD sequence

1. Add failing tests for shared sizes, fixed boundaries, title ownership, and directional fades.
2. Run the focused tests and record the expected failures.
3. Add the smallest strip, sizing, and wrapper changes that make the tests pass.
4. Update the plugin reference contract and run the public-doc checks.
5. Run the focused tests, lint, and type check.

## Verification

```bash
(cd apps && rtk pnpm install --frozen-lockfile)
(cd apps && rtk pnpm --filter @kandev/web test -- --run components/kanban/mobile-topbar-action-strip.test.tsx components/kanban/kanban-header-mobile.test.tsx components/kanban/main-top-bar-plugin-actions.test.tsx components/system-metrics/topbar-metrics.test.tsx)
(cd apps && rtk pnpm --filter @kandev/web exec eslint components/kanban/mobile-topbar-action-strip.tsx components/kanban/mobile-topbar-action-strip.test.tsx components/kanban/kanban-header-mobile.tsx components/kanban/kanban-header-mobile.test.tsx components/kanban/main-top-bar-plugin-actions.tsx components/kanban/main-top-bar-plugin-actions.test.tsx components/system-metrics/topbar-metrics.tsx components/system-metrics/topbar-metrics.test.tsx lib/plugins/types.ts)
(cd apps/web && rtk pnpm run typecheck)
rtk node --test scripts/validate-public-docs.test.mjs
rtk node scripts/validate-public-docs.mjs
```

## Output contract

Report the changed behavior, files, RED and GREEN results, public-doc result, blockers, and risks.
Update this task and `plan.md` in the same conversation.

## Results

- Added the resize-aware mobile action strip with conditional directional fades.
- Kept Kandev and the menu outside the scroll owner, and placed page context, plugin actions,
  metrics, terminal, chat, and search inside it.
- Normalized native, metric, and documented host plugin icon geometry without changing desktop
  plugin presentation or existing slot data.
- Updated the plugin API reference and public authoring guide with the mobile contract.
- TDD RED: the new focused suite initially failed 9 assertions against the old layout.
- TDD GREEN: focused Vitest passed 4 files and 19 tests.
- Review-fix GREEN: focused Vitest passed 5 files and 20 tests after replacing clipped pseudo-element
  hit areas with real 44px interaction wrappers around the 32px launcher buttons.
- Focused ESLint, web typecheck, and public documentation validation passed.
- No blockers. The E2E task provides the remaining production-build coverage.
- Review fixup: exported `MainTopBarSlotProps` from the public SDK, kept the host type canonical, and
  added a Pixel 5 `elementFromPoint` regression for the 44px terminal/chat hit areas.
- Follow-up alignment fix: fitting action rows right-align against the fixed menu while overflowing
  rows retain their contained horizontal scroll behavior.
