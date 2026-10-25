---
id: "06-isolated-application-appearance"
title: "Add isolated application appearance"
status: done
wave: 6
depends_on:
  - "05-canvas-host-viewport"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-ISOLATED-WEB-APPS-011
acceptance_criteria:
  - AC-PLUGINS-ISOLATED-WEB-APPS-011.1
  - AC-PLUGINS-ISOLATED-WEB-APPS-011.2
  - AC-PLUGINS-ISOLATED-WEB-APPS-011.3
  - AC-PLUGINS-ISOLATED-WEB-APPS-011.4
system_design:
  - ../../specs/plugins/system-design/isolated-web-app-contributions.md
---

# Task 06: Add isolated application appearance

## Summary

Send bounded semantic appearance data from the Kandev host to each opaque
iframe. Apply initial and live changes without adding host authority.

## In scope

- Add the typed version 1 appearance schema and fixed token allowlist.
- Resolve semantic colors from the active Kandev theme.
- Send initial values to the exact iframe window before reveal.
- Send updates after live host theme changes.
- Add scaffold-side source, type, version, key, and bound validation.
- Map tokens to documented CSS variables with safe fallbacks.
- Cover direct, Dockview, and phone computed colors.

## Out of scope

- A general postMessage SDK, host data access, or iframe reload on theme change.

## Acceptance

- The first visible canvas frame uses the current Kandev appearance.
- A live light or dark change updates computed application colors.
- The payload contains only the documented mode and color fields.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/plugins/web-app-appearance.test.ts components/plugins/web-app-frame.test.tsx
cd apps/web && pnpm run typecheck && pnpm run lint
cd apps/web && pnpm e2e:run tests/canvas/plugin-canvas.spec.ts -- --retries=0
cd apps/web && pnpm e2e:run --project mobile-chrome tests/canvas/mobile-plugin-canvas.spec.ts -- --retries=0
```

## Files likely touched

- `apps/web/components/plugins/web-app-appearance.ts`
- `apps/web/components/plugins/web-app-frame.tsx`
- `apps/web/components/theme/app-theme.tsx`
- `apps/web/e2e/tests/canvas/canvas-fixture.ts`
- `apps/web/e2e/tests/canvas/plugin-canvas.spec.ts`
- `apps/web/e2e/tests/canvas/mobile-plugin-canvas.spec.ts`
- `apps/backend/internal/mcp/canvasskill/files/references/ui.md`
- `apps/backend/internal/backendapp/canvas_authoring.go`

## Dependencies

- Task 05 stabilizes host reveal and viewport geometry.

## Risks

- An iframe can miss a message that is sent before its listener exists.
- Unvalidated CSS values can escape the intended presentation boundary.
- `targetOrigin: "*"` is safe only while the payload stays public and
  presentation-only.

## Parallelism

`sequential`

## Inputs

- Host appearance protocol and browser security sections.
- `CANVAS-UX-07` browser measurements in the investigation.

## Results

Added the typed version 1 presentation-only appearance envelope with a fixed
semantic token allowlist, bounded color validation, safe fallbacks, and live
theme delivery to the exact iframe window before frame reveal. The scaffold
listener validates source, type, version, keys, and bounds before applying
semantic CSS variables. Added protocol unit tests and light/dark assertions to
both desktop and phone fixture flows.

Verification passed: appearance and frame tests, desktop and phone Playwright
flows with retries disabled, web typecheck, lint, i18n checks, and the
new-code ratchet.
