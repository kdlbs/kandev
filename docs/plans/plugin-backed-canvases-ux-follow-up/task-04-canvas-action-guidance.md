---
id: "04-canvas-action-guidance"
title: "Explain canvas lifecycle actions"
status: done
wave: 4
depends_on:
  - "03-first-release-host-activation"
plan: "plan.md"
requirements:
  - REQ-CANVASES-AGENT-WEB-APPS-006
acceptance_criteria:
  - AC-CANVASES-AGENT-WEB-APPS-006.3
  - AC-CANVASES-AGENT-WEB-APPS-006.4
  - AC-CANVASES-AGENT-WEB-APPS-006.7
system_design:
  - ../../specs/canvases/system-design/agent-authored-web-apps.md
---

# Task 04: Explain canvas lifecycle actions

## Summary

Explain release review, permission approval, promotion, and disabled states in
the host chrome. Keep the same information available to touch users.

## In scope

- Add pointer and keyboard tooltips to desktop lifecycle controls.
- Use focusable wrappers for disabled controls.
- Show visible descriptions in the mobile action drawer.
- Keep visible button labels and 44-pixel touch targets.
- Add translated copy in every required locale.

## Out of scope

- Lifecycle API behavior and dialog content changes.

## Acceptance

- Desktop users can read each explanation with pointer or keyboard focus.
- Disabled controls state the reason for the disabled state.
- Phone users can read the same meaning without hover.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/settings/canvas-host-components.test.tsx components/settings/canvas-host-route.test.tsx
cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check && pnpm run i18n:ratchet
cd apps/web && pnpm e2e:run --project mobile-chrome tests/canvas/mobile-plugin-canvas.spec.ts -- --retries=0
```

## Files likely touched

- `apps/web/components/settings/canvas-host-components.tsx`
- `apps/web/components/settings/canvas-host-route.tsx`
- `apps/web/src/locales/**/canvases.json`
- `apps/web/e2e/tests/canvas/mobile-plugin-canvas.spec.ts`

## Dependencies

- Task 03 finalizes the host state that controls action availability.

## Risks

- Disabled buttons do not emit tooltip pointer or focus events directly.
- Dense descriptions can make the phone drawer exceed its viewport.

## Parallelism

`sequential`

## Inputs

- Desktop information architecture and mobile design sections.
- The disabled-tooltip pattern in `apps/web/AGENTS.md`.
- `CANVAS-UX-05` in the investigation.

## Results

Added desktop tooltip guidance with focusable wrappers for disabled lifecycle
actions and equivalent visible descriptions in the mobile action drawer.
Archived and disabled canvas mutations are disabled in both host surfaces, and
the action controls retain touch-sized targets. Added localized action guidance
in every required catalog.

Verification passed: focused host tests, phone Playwright coverage with
retries disabled, web typecheck, lint, i18n checks, and the new-code ratchet.
