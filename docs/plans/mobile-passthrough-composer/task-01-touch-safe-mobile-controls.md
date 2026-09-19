---
id: "01-touch-safe-mobile-controls"
title: "Touch-safe mobile controls"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/cli/requirements/mobile-passthrough-composer.md"
design: "../../specs/cli/system-design/mobile-passthrough-composer.md"
---

# Task 01: Touch-safe mobile controls

## Outcome

Phone users can operate every passthrough status and composer action owned by
this change through a minimum 44-by-44 CSS-pixel touch target without changing
desktop geometry.

## Scope

- Add a mobile presentation to the shared context, attachment, plan, cancel,
  and send controls.
- Apply mobile-only touch sizing to the owned passthrough Chat, Comments,
  Proceed, and Send to Agent controls.
- Preserve the inline composer, internal toolbar scrolling, terminal flex
  behavior, and existing translated labels.
- Add focused component coverage for mobile and desktop control geometry.

## Exclusions

- No drawer, terminal keybar, raw PTY routing, or slash-command behavior.
- No backend, WebSocket, storage, or localization changes.

## Traceability

- `REQ-CLI-MOBILE-PASSTHROUGH-COMPOSER-001`
- `AC-CLI-MOBILE-PASSTHROUGH-COMPOSER-001.1`
- `AC-CLI-MOBILE-PASSTHROUGH-COMPOSER-001.2`
- `AC-CLI-MOBILE-PASSTHROUGH-COMPOSER-001.8`
- `docs/specs/cli/system-design/mobile-passthrough-composer.md`

## Implementation acceptance

- Every status-row and composer action owned by this change in the mobile
  passthrough surface has a computed width and height of at least 44 CSS
  pixels. Integration-owned status chips retain their own component contracts.
- Desktop controls retain their current compact dimensions.
- Narrow mobile toolbars scroll actions internally and do not create document
  horizontal overflow.

## Files likely touched

- `apps/web/components/task/passthrough-toolbar.tsx`
- `apps/web/components/task/passthrough-toolbar.test.tsx`
- `apps/web/components/task/chat/chat-input-toolbar-mobile.tsx`
- `apps/web/components/task/chat/chat-input-toolbar-primitives.tsx`
- `apps/web/components/task/chat/chat-input-toolbar.test.tsx`

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/task/passthrough-toolbar.test.tsx components/task/chat/chat-input-toolbar.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
```

## Results

Implemented mobile-only touch presentation for the shared context, attachment,
plan, cancel, and send controls, plus passthrough Chat, Comments, and workflow
controls. Desktop controls retain their compact dimensions.

Verification passed:

- Focused Vitest suite: 48 tests passed.
- Web typecheck passed.
- Web lint passed with zero warnings.
- Managed mobile E2E coverage passed as part of Task 02.
