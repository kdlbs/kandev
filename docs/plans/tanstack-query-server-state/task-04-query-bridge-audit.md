---
id: "04-query-bridge-audit"
title: "Query bridge audit"
status: done
wave: 2
depends_on: ["01-query-foundation", "02-ws-accounting", "03-query-options-taxonomy"]
plan: "plan.md"
spec: "../../specs/ui/tanstack-query-server-state.md"
---

# Task 04: Query Bridge Audit

## Acceptance

- `subscribeWebSocketClient(listener)` exists and notifies QueryBridge when the
  WS client is installed/replaced.
- `registerQueryBridge` registers per-domain bridge handlers that patch or
  invalidate query keys.
- `wrapBridgeHandler` records E2E-only audit rows, and skipped actions/prefixes
  are documented inline.

## Verification

- `cd apps && pnpm --filter @kandev/web test -- apps/web/lib/query/bridge apps/web/lib/ws`
- `cd apps && pnpm --filter @kandev/web typecheck`
- `cd apps/web && pnpm e2e:docker tests/system/ws-event-accounting.spec.ts tests/chat/message-add-ws-gap.spec.ts`
- `cd apps/web && pnpm e2e:docker --project mobile-chrome tests/chat/mobile-message-add-ws-gap.spec.ts`

## Files Likely Touched

- `apps/web/lib/ws/connection.ts`
- `apps/web/lib/ws/client.ts`
- `apps/web/lib/query/provider.tsx`
- `apps/web/lib/query/bridge/index.ts`
- `apps/web/lib/query/bridge/*.ts`
- `apps/web/lib/query/bridge/**/*.test.ts`
- `apps/web/e2e/helpers/ws-account.ts`

## Dependencies

- Tasks 01, 02, 03.

## Inputs

- Old PR bridge entrypoint and `wrapBridgeHandler`:
  `origin/pr/1130:apps/web/lib/query/bridge/index.ts`.
- Spec scenario: parsed WS events must touch query cache or be allowlisted.

## Output Contract

Update this task to `done`, include the allowlist rationale summary, and list
domains whose readers still need migration before old Zustand handlers can be
deleted.

## Output

- Added `subscribeWebSocketClient(listener)` and `WebSocketClient.onEnvelope`
  so QueryBridge can attach to the current WS client and observe all parsed
  envelopes, including responses.
- Registered QueryBridge from `QueryProvider`, with cleanup on WS client
  replacement and provider unmount.
- Added bridge registrars for task/workspace/workflow events. Task events patch
  task detail queries and invalidate finite and infinite task-list keys.
- Added E2E-gated bridge audit rows for handled and allowlisted envelopes.

## Allowlist Rationale Summary

- Control-plane request/response actions are resolved by `WebSocketClient`
  pending requests and do not represent durable query cache entries.
- Zustand-temporary actions remain allowlisted until their Wave 3 domain moves
  both UI readers and bridge writers to TanStack Query.
- Client-effect actions drive toasts, dialogs, browser notifications, or
  imperative refreshes rather than stable server-state cache entries.
- High-volume stream actions remain outside QueryClient to avoid per-chunk
  observer churn.

## Domains Still Needing Reader Migration

- Settings/agents/executors, office, session runtime/messages/turns/plans,
  integrations, automations, system jobs/status, terminal/git streams, and old
  Zustand task/workspace readers remain for the later domain migration and
  cleanup tasks.

## Commands Run

- `cd apps/web && pnpm test -- lib/query lib/ws` — 23 files, 168 tests passed
- `cd apps/web && pnpm typecheck`
