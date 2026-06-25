---
id: "02-ws-accounting"
title: "WS accounting"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/tanstack-query-server-state.md"
---

# Task 02: WS Accounting

## Acceptance

- Backend outbound WebSocket messages carry `connection_id`,
  `connection_seq`, and session-scoped `session_seq` when available.
- E2E-only `/api/v1/_test/ws-sent` endpoint returns bounded sent events and is
  unavailable outside E2E/test harness mode.
- Frontend `WsAccount` records parsed envelopes and E2E teardown fails on
  missing connection/session events when `KANDEV_E2E_WS_ASSERT=1`.

## Verification

- `make -C apps/backend test`
- `cd apps && pnpm --filter @kandev/web test -- apps/web/lib/ws/ws-account.test.ts`
- `cd apps/web && pnpm e2e:docker tests/system/ws-event-accounting.spec.ts`

## Files Likely Touched

- `apps/backend/pkg/websocket/message.go`
- `apps/backend/internal/gateway/websocket/client.go`
- `apps/backend/internal/gateway/websocket/hub.go`
- `apps/backend/internal/gateway/websocket/ws_sent_log.go`
- `apps/backend/cmd/kandev/e2e_reset.go`
- `apps/web/lib/ws/client.ts`
- `apps/web/lib/ws/ws-account.ts`
- `apps/web/e2e/helpers/ws-account.ts`
- `apps/web/e2e/fixtures/test-base.ts`
- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/tests/system/ws-event-accounting.spec.ts`

## Dependencies

None.

## Inputs

- Old PR files: `ws_sent_log.go`, `ws-account.ts`, `helpers/ws-account.ts`,
  `tests/system/ws-event-accounting.spec.ts`.
- Current docs mismatch: runner/README mention strict WS accounting, but no
  current implementation exists.

## Output Contract

Update this task to `done`, include commands run, and document any events that
must be excluded from receipt accounting.

## Output

- Added backend envelope stamping at the final WebSocket send boundary:
  `connection_id`, `connection_seq`, and `session_seq` for
  `BroadcastToSession` fan-out.
- Added a per-connection bounded sent-log and E2E-only
  `/api/v1/_test/ws-sent` route mounted only with the existing
  `KANDEV_E2E_MOCK` test harness.
- Added frontend `WsAccount`, parsing hook, E2E helper diffing, and
  `KANDEV_E2E_WS_ASSERT=1` teardown enforcement in the base Playwright fixture.
- Added expected-drop reconciliation for intentional WebSocket fault-injection
  tests; those tests register the exact dropped `session.message.added` frames
  and still fail on unexpected or missing drops.
- Added the previously missing
  `apps/web/e2e/tests/system/ws-event-accounting.spec.ts` Docker smoke test.

## Commands Run

- `go test ./internal/gateway/websocket -run 'Test(Client_SendMessageStampsConnectionSequenceAndLog|Hub_BroadcastToSessionStampsSessionSequence|WsSentLogEvictsOldestAndFiltersSince)'`
- `go test ./internal/backendapp -run TestRegisterWsSentTestRoute`
- `go test ./internal/gateway/websocket`
- `make -C apps/backend test`
- `cd apps && pnpm --filter @kandev/web test -- lib/ws/ws-account.test.ts`
- `cd apps/web && pnpm test -- lib/ws/ws-account.test.ts lib/ws/ws-account-e2e-helper.test.ts`
- `cd apps/web && pnpm typecheck`
- `cd apps && pnpm exec prettier --check web/lib/ws/ws-account.ts web/lib/ws/ws-account.test.ts web/e2e/helpers/ws-account.ts web/e2e/fixtures/test-base.ts web/e2e/helpers/api-client.ts web/lib/ws/client.ts web/global.d.ts`
- `git diff --check`
- `cd apps/web && pnpm e2e:docker -- tests/system/ws-event-accounting.spec.ts tests/chat/message-add-ws-gap.spec.ts tests/task/task-list.spec.ts tests/kanban/kanban-board.spec.ts` — 7 passed
- `cd apps/web && pnpm e2e:docker --no-build --project mobile-chrome -- tests/chat/mobile-message-add-ws-gap.spec.ts` — 1 passed
