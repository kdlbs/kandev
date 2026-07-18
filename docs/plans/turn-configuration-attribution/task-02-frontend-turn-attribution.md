---
id: "02-frontend-turn-attribution"
title: "Render changed configuration from turn metadata"
status: done
wave: 2
depends_on: ["01-backend-turn-snapshot"]
plan: "plan.md"
spec: "../../specs/ui/acp-model-configuration-summary.md"
---

# Task 02: Render Changed Configuration from Turn Metadata

## Acceptance

- Each agent message renders its model plus only options changed from that turn's captured baseline, preserving captured provider order and names.
- Changing session configuration for a later turn does not relabel earlier messages.
- Legacy turns without snapshots show available model attribution only and never current session options.

## Verification

```bash
rtk pnpm --filter @kandev/web test -- --run components/task/chat/messages/message-session-config.test.ts components/task/chat/messages/chat-message.test.tsx
rtk pnpm --filter @kandev/web typecheck
```

Run from `apps/`.

## Files

- `apps/web/components/task/chat/messages/message-actions.tsx`
- `apps/web/components/task/chat/messages/message-session-config.ts`
- `apps/web/components/task/chat/messages/message-session-config.test.ts`
- `apps/web/components/task/chat/messages/chat-message.test.tsx`

## Inputs

- Spec message-attribution and legacy-turn scenarios.
- Task 01 metadata shape.
- Existing `MessageActions`, turn store, and selector baseline comparison patterns.

## Output Contract

Report formatting behavior, legacy fallback behavior, desktop/mobile parity assessment, tests run, files changed, and remaining display risks.
