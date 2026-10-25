---
id: "04-native-blocks-mobile"
title: "Native blocks and mobile parity"
status: done
wave: 4
depends_on: ["03-frontend-contract-dispatch"]
plan: "plan.md"
spec: "../../specs/agents/requirements/agent-rich-output.md"
---

# Task 04: Native blocks and mobile parity

## Acceptance

1. Successful calls render one restrained native presentation containing
   accessible file, line/bar chart, and metrics blocks; pending/error calls do
   not render unvalidated blocks.
2. Files load only on explicit expansion and open through existing desktop and
   mobile file viewers with optional repository identity.
3. Phone layout contains charts, reflows metrics, provides touch-safe actions,
   and adds no document horizontal overflow or hover-only capability.

## Verification

```sh
cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check && pnpm run i18n:ratchet
```

## Files likely touched

- `apps/web/components/task/chat/messages/kandev/rich-output/*.tsx`
- `apps/web/components/task/chat/messages/kandev/types.ts`
- `apps/web/components/task/chat/messages/kandev-tool-message.tsx`
- `apps/web/components/task/chat/message-renderer.tsx`
- `apps/web/components/task/chat/message-list-shared.tsx`
- `apps/web/components/task/chat/message-list-native.tsx`
- `apps/web/components/task/chat/messages/turn-group-message.tsx`
- `apps/web/components/task/chat/use-chat-panel-state.ts`
- `apps/web/components/task/task-chat-panel.tsx`
- `apps/web/app/office/tasks/[id]/advanced-panels/chat-panel.tsx`
- `apps/web/src/locales/en/task.json`
- `apps/web/src/locales/pseudo/task.json`

## Dependencies

Task 03.

## Parallelism

Sequential. Shared renderer context and file-open callbacks span both layouts.

## Inputs

- Spec mobile design contract and file failure modes.
- `KandevRow`/`KandevBody`, `@kandev/ui` chart primitives,
  `requestFileContent`, and `MobileFileViewerPanel` patterns.

## Output contract

Report UI behavior, interface-polish choices, exact checks, actual files,
remaining rendered-risk notes, and task/plan status update.

## Results

- Added a quiet standalone presentation plus responsive metric, accessible
  line/bar chart, and lazy file-preview blocks under
  `components/task/chat/messages/kandev/rich-output/`.
- File content is requested only after **Preview**. Desktop, mobile, Office,
  and task-center callback paths now preserve optional repository identity for
  preview and native file opening.
- Host tokens own chart colors; numeric metrics use tabular figures; images
  receive a neutral outline; phone actions are at least 44px and charts remain
  width-contained.
- RED: the first live desktop E2E reached the localized unavailable state
  because ACP stores tool input inside `normalized.generic.input.raw_input`.
  A focused unit test reproduced that transport shape. GREEN: shared argument
  extraction unwraps it, and 95 focused tests pass.
- `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, and
  `pnpm run i18n:ratchet` pass. Pseudo and five real locale files contain the
  new host copy; existing catalog-parity notices remain advisory.
