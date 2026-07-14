---
id: "02-unified-frontend-sessions"
title: "Unified frontend sessions"
status: done
wave: 2
depends_on: ["01-backend-boot-contract"]
plan: "plan.md"
spec: "../../specs/tasks/quick-chat-expiration.md"
---

# Task 02: Unified Frontend Sessions

## Acceptance

- Settings FAB and Command Palette open config setup/real tabs in the responsive Quick Chat modal;
  config setup preserves its profile/copy/suggestions/placeholder and has no repository controls.
- One typed Quick Chat store owns ordinary/config real and setup tabs; older missing kinds normalize
  to `chat`, workspace changes cannot activate foreign sessions, and passthrough remains supported.
- Config start seeds the task session and delivers its initial prompt once; confirmed real-tab close
  deletes the backing task while modal close, tab switch, and blank close do not.

## Verification

`cd apps && pnpm --filter @kandev/web test -- --run lib/state/hydration/hydrator.test.ts components/quick-chat/use-quick-chat-modal.test.ts components/quick-chat/quick-chat-modal.test.ts`

`cd apps/web && pnpm run typecheck`

## Files Likely Touched

- `apps/web/lib/state/slices/ui/types.ts`
- `apps/web/lib/state/slices/ui/ui-slice.ts`
- `apps/web/lib/state/default-state.ts`
- `apps/web/lib/state/hydration/hydrator.ts`
- `apps/web/lib/state/hydration/hydrator.test.ts`
- `apps/web/components/quick-chat/use-quick-chat-modal.ts`
- `apps/web/components/quick-chat/use-quick-chat-modal.test.ts`
- `apps/web/components/quick-chat/quick-chat-modal.tsx`
- `apps/web/components/quick-chat/quick-chat-tab-item.tsx`
- `apps/web/components/quick-chat/quick-chat-setup.tsx`
- `apps/web/components/config-chat/config-chat-provider.tsx`
- `apps/web/components/config-chat/config-chat-panel.tsx`
- `apps/web/components/config-chat/use-config-chat.ts`
- `apps/web/components/global-commands.tsx`

## Inputs

- Spec: What, state machine, failure modes, workspace and backward-compatibility scenarios.
- Task 01 boot session shape.
- Existing ordinary Quick Chat setup/delete and config start/profile-default patterns.

## Dependencies

Task 01.

## Output Contract

Report behavior changed, files touched, red/green test commands, blockers, residual risks, and mark
this task `done` in this file and `plan.md`.
