---
id: "05-canvas-host-viewport"
title: "Fill the canvas host viewport"
status: done
wave: 5
depends_on:
  - "04-canvas-action-guidance"
plan: "plan.md"
requirements:
  - REQ-CANVASES-AGENT-WEB-APPS-006
acceptance_criteria:
  - AC-CANVASES-AGENT-WEB-APPS-006.4
  - AC-CANVASES-AGENT-WEB-APPS-006.5
  - AC-CANVASES-AGENT-WEB-APPS-006.6
system_design:
  - ../../specs/canvases/system-design/agent-authored-web-apps.md
---

# Task 05: Fill the canvas host viewport

## Summary

Give the portaled Dockview canvas route a full-height flex boundary. Prove that
all canvas hosts give the iframe their complete application area.

## In scope

- Correct the canvas Dockview renderer wrapper.
- Preserve direct-route and phone viewport calculations.
- Assert normal, maximized, resized, direct, and phone geometry.
- Keep one scroll owner and no phone document overflow.

## Out of scope

- Shared `PageShell` behavior for unrelated routes.

## Acceptance

- The iframe height matches the available application area within two CSS
  pixels.
- Dockview resize and maximize operations preserve full-height rendering.
- The direct and phone routes retain their current full-height behavior.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/task/dockview-panel-content.test.tsx components/plugins/canvas-page.test.tsx components/plugins/web-app-frame.test.tsx
cd apps/web && pnpm run typecheck && pnpm run lint
cd apps/web && pnpm e2e:run tests/canvas/plugin-canvas.spec.ts -- --retries=0
cd apps/web && pnpm e2e:run --project mobile-chrome tests/canvas/mobile-plugin-canvas.spec.ts -- --retries=0
```

## Files likely touched

- `apps/web/components/task/dockview-panel-content.tsx`
- `apps/web/components/settings/canvas-host-components.tsx`
- `apps/web/e2e/tests/canvas/plugin-canvas.spec.ts`
- `apps/web/e2e/tests/canvas/mobile-plugin-canvas.spec.ts`

## Dependencies

- Task 04 finalizes host chrome height and action composition.

## Risks

- A shared layout change can regress direct routes that already work.
- Fixed-pixel assertions can become flaky across browser scale factors.

## Parallelism

`sequential`

## Inputs

- Desktop information architecture and mobile geometry sections.
- `CANVAS-UX-06` measurements in the investigation.

## Results

Added the missing full-height flex boundary around the Dockview canvas route
and kept direct and phone route sizing unchanged. The desktop fixture now
checks iframe width and height after normal, maximized, restored, and resized
states; the phone fixture checks direct and switched canvas routes without
document overflow.

Verification passed: focused web tests, desktop and phone Playwright flows
with retries disabled, web typecheck, and lint.
