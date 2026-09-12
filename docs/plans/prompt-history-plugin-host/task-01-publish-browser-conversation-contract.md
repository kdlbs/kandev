---
id: "01-publish-browser-conversation-contract"
title: "Publish browser conversation contract"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/plugins/requirements/prompt-history-extraction-host.md"
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-001
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-002
acceptance_criteria:
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.1
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.2
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.3
  - AC-PLUGINS-PROMPT-HISTORY-HOST-005.1
system_design:
  - "../../specs/plugins/system-design/prompt-history-extraction-host.md"
---

# Task 01: Publish Browser Conversation Contract

## Scope

Publish the additive, runtime-free TypeScript contract before implementing either side. Define conversation DTOs, the `PluginConversationError` shape with stable codes and `retryable`, hook state types, task-panel context, visibility/title/navigation/favorite/mention/lifecycle contracts, and canonical documentation. This task publishes shapes only; runtime behavior belongs to later work orders.

## Acceptance

- `apps/packages/plugin-sdk/src/index.ts` contains the runtime-free public contract from the system design with no Kandev, Zustand, or React runtime dependency. The single declaration uses `PluginConversationMessage` (nullable `taskId`, `turnId?`, `type`, `content`, optional `promptIndex`), `PluginConversationTurn` (nullable `taskId`, optional `completedAt`), `PluginSessionMessagesQuery` (nullable session, tri-state `taskId`, `authorTypes`, `pageSize`), separate message/turn state types, `loadMore(): Promise<number>`, and the stable `PluginConversationError` code union with required `retryable`; the first three codes are `false`, authorized upstream 5xx is `true`, and binding-only `generation_superseded` never reaches plugin code.
- `docs/plans/plugins/PLUGIN-API.md` updates both existing task-panel listings and adds that exact conversation DTO/hook/error/lifecycle contract, `HostReact.useLayoutEffect`, and `host.ui.PromptMentionText`, including the exact error envelope and retryability mapping.
- `apps/web/lib/plugins/types.ts` aliases/refines the SDK types rather than redeclaring a second shape; `PluginUIShape` and `HostReact` include the same UI/component additions.
- SDK/internal-alias assignability tests cover the published shapes, UI additions, and existing registration compatibility; concrete Host implementations and runtime behavior are owned by Tasks 02-04.
- Message and turn state types include the observable terminal `removed` flag; the contract does not expose Host-only cursors, snapshot tokens, sidecars, or event payloads.
- The contract records that the loader serializes per-plugin stages, routes global registration through a private `(pluginId,generation,stageId)` context, rejects late callbacks after staging closes, and atomically swaps registry contributions only at successor commit.

## TDD sequence

1. RED: add type/assignability fixtures for the new contract and existing-registration compatibility.
2. GREEN: add only the SDK, internal aliases, and canonical contract documentation.
3. REFACTOR: remove duplicated type declarations and keep the runtime-free package dependency-free.

## Likely files

- `apps/packages/plugin-sdk/src/index.ts`
- `apps/web/lib/plugins/types.ts`
- `apps/web/lib/plugins/*.test.ts`
- `docs/plans/plugins/PLUGIN-API.md`
- `apps/web/lib/plugins/host-api.test.ts` or the focused contract-consistency test

## Verification

```bash
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web test -- --run lib/plugins
```

Do not implement routes, hooks, panel plumbing, or fixture UI in this task.

## Result

Published the runtime-free conversation, panel-context, navigation, favorite,
mention-rendering, and lifecycle types in `@kandev/plugin-sdk`; internal web
types alias that declaration. Assignability coverage preserves existing
registrations and verifies the new Host surface.

Verified:

- `cd apps/web && pnpm run typecheck`
- `cd apps && pnpm --filter @kandev/web test -- --run lib/plugins` (28 files,
  210 tests)
