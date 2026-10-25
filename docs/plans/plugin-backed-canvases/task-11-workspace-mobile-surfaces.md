---
id: "11-workspace-mobile-surfaces"
title: "Responsive workspace canvas management"
status: done
wave: 11
depends_on:
  - "07-canvas-host-task-surfaces"
  - "09-promotion-release-management"
  - "10-quick-chat-canvas-editing"
plan: "plan.md"
requirements:
  - REQ-CANVASES-AGENT-WEB-APPS-003
  - REQ-CANVASES-AGENT-WEB-APPS-005
  - REQ-CANVASES-AGENT-WEB-APPS-006
  - REQ-CANVASES-AGENT-WEB-APPS-007
acceptance_criteria:
  - AC-CANVASES-AGENT-WEB-APPS-003.1
  - AC-CANVASES-AGENT-WEB-APPS-003.3
  - AC-CANVASES-AGENT-WEB-APPS-003.5
  - AC-CANVASES-AGENT-WEB-APPS-005.2
  - AC-CANVASES-AGENT-WEB-APPS-005.3
  - AC-CANVASES-AGENT-WEB-APPS-005.5
  - AC-CANVASES-AGENT-WEB-APPS-005.6
  - AC-CANVASES-AGENT-WEB-APPS-006.1
  - AC-CANVASES-AGENT-WEB-APPS-006.2
  - AC-CANVASES-AGENT-WEB-APPS-006.3
  - AC-CANVASES-AGENT-WEB-APPS-006.4
  - AC-CANVASES-AGENT-WEB-APPS-006.5
  - AC-CANVASES-AGENT-WEB-APPS-006.6
  - AC-CANVASES-AGENT-WEB-APPS-007.1
  - AC-CANVASES-AGENT-WEB-APPS-007.2
  - AC-CANVASES-AGENT-WEB-APPS-007.3
system_design:
  - ../../specs/canvases/system-design/agent-authored-web-apps.md
---

# Task 11: Responsive workspace canvas management

## Summary

Add workspace discovery and management for desktop and phone users. Reuse the
shared host and lifecycle actions with a phone-native composition.

## In scope

- Add the folded workspace Canvases sidebar section.
- Add workspace settings for permissions, releases, editing, archive, restore,
  and removal.
- Add promotion and pending-permission dialogs.
- Add mobile workspace navigation for promoted canvases.
- Add the full-height phone route and inset canvas-action drawer.
- Reuse task layout, mobile picker, and direct-navigation patterns.
- Keep one scroll owner, safe-area clearance, 44-pixel controls, and no page
  overflow.
- Add desktop component coverage and complete desktop and mobile Playwright
  flows.
- Add localized copy in all required catalogs.

## Out of scope

- Application-specific responsive behavior outside the scaffold and fixture.
- General plugin placements outside task and workspace canvases.

## Acceptance

- Only active workspace canvases appear in workspace navigation.
- Phone users can open, edit, promote, and manage the same canvas capability.
- The phone route does not mount Dockview or create document-level overflow.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/canvas components/app-sidebar lib/navigation
cd apps/web && pnpm run typecheck && pnpm run lint
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
cd apps/web && pnpm e2e:run tests/canvas/plugin-canvas.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/canvas/mobile-plugin-canvas.spec.ts
```

## Files likely touched

- `apps/web/components/app-sidebar/sections/canvases-section.tsx`
- `apps/web/components/canvas/**`
- `apps/web/components/task/mobile/**`
- `apps/web/components/navigation/**`
- `apps/web/app/settings/workspace/[id]/canvases/page.tsx`
- `apps/web/src/locales/**/canvases.json`
- `apps/web/e2e/tests/canvas/plugin-canvas.spec.ts`
- `apps/web/e2e/tests/canvas/mobile-plugin-canvas.spec.ts`

## Dependencies

- Task 07 provides the shared host and task data model.
- Task 09 provides promotion and release-management APIs.
- Task 10 provides Quick Chat editing.

## Risks

- A responsive wrapper can mount Dockview on phones.
- Secondary actions can become unreachable without a visible touch target.
- The iframe or drawer can create two scroll owners.

## Parallelism

`sequential`

## Inputs

- Desktop information architecture, mobile contract, host state, and API
  sections.
- Current mobile task layout, picker, drawer, and direct-navigation patterns.

## Results

Implemented workspace sidebar and settings management, promotion and release
dialogs, mobile canvas navigation, focused phone routes, and the inset action
drawer with safe-area and viewport containment rules.

Verification:

- Frontend lint, typecheck, i18n completeness, and new-code ratchet — passed.
- Focused Vitest — canvas host, lifecycle, workspace, and Dockview tests passed.
- Desktop and mobile canvas Playwright flows — passed, including two-canvas
  phone switching and host-action parity.
