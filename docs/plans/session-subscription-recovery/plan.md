---
spec: docs/specs/platform/requirements/session-subscription-recovery.md
created: 2026-08-05
status: implemented
issue: https://github.com/kdlbs/kandev/issues/2287
---

# Implementation Plan: Session subscription recovery

## Overview

The current server already inserts the session membership before sending the
`session.subscribe` acknowledgement and already provides state snapshots,
initial message fetches, and backfills. The gap is on the web client: it starts
`message.list` immediately after enqueueing the subscribe frame, while backend
WebSocket handlers run concurrently. Add an acknowledgement-aware readiness
handle to the ref-counted WebSocket client, make the transcript hook await it,
and route reconnect/retry registrations through the same tracked request. Keep
the existing recovery layers as defense-in-depth and remove only the E2E
workarounds that are specifically masking this race.

## Confirmed root cause

`WebSocketClient.subscribeSession` currently calls `send` and returns an
unsubscribe function without exposing the server response. `useSessionSubscription`
then calls `fetchAndStoreMessages` in the same effect. Since the backend starts
each received message handler in its own goroutine, `message.list` can run
before `handleSessionSubscribe` reaches `Hub.SubscribeToSession`; an event
published in that interval is not delivered, and the fetch can also complete
before the persisted row is visible. The issue's reload fallback rehydrates the
same durable state on a later page load.

## Frontend

### Acknowledgement-aware WebSocket membership

`apps/web/lib/ws/client.ts`

- Centralize session-subscribe frame creation so initial subscriptions,
  reconnect resubscriptions, and explicit delayed-session retries use
  `request("session.subscribe", ...)` and track the pending acknowledgement.
- Preserve the existing `subscribeSession(sessionId): () => void` API for
  consumers that only need membership, and add a readiness-bearing companion
  used by transcript hydration (for example, a handle with `unsubscribe` and
  `ready`).
- Share an in-flight readiness promise for ref-counted consumers, clear it on
  success/failure/disconnect, and ensure discarded requests cannot produce an
  unhandled rejection.

### Transcript hydration ordering

`apps/web/hooks/domains/session/use-session-messages.ts`

- Use the readiness-bearing subscription handle.
- Start `fetchAndStoreMessages` only after the subscription acknowledgement;
  preserve cleanup guards so an unmounted hook does not apply a late result.
- Keep the existing turn-settle, running-turn, visibility, and initial state
  snapshot reconciliation paths unchanged unless the focused regression test
  proves a duplicate request is introduced.

`apps/web/hooks/domains/session/use-session-subscription-retry.ts`

- Replace the raw duplicate `client.send` retry with the tracked
  acknowledgement-aware session-subscribe operation, retaining the current
  backoff and ref-count ownership.

### E2E workaround cleanup

`apps/web/e2e/pages/session-page.ts` and the smallest existing session/chat
specs that exercise the documented reload fallback should stop treating a page
reload as the normal recovery for this race. Preserve bounded waits for genuine
startup/CI slowness, but do not silently reload solely because a subscription
event was missed. Use the existing auto-start and follow-up-response scenarios
as end-to-end evidence; add a focused regression spec only if the deterministic
test setup can force the registration delay without timing sleeps.

## Tests

- **WebSocket client readiness:** add `apps/web/lib/ws/client.test.ts` covering
  first registration acknowledgement, shared in-flight readiness, rejected
  registration cleanup, and reconnect resubscription tracking.
- **Transcript ordering:** extend
  `apps/web/hooks/domains/session/use-session-messages.test.ts` to prove that
  `message.list` is not requested until the subscribe readiness promise
  resolves, and that a late resolution after cleanup does not mutate the
  store.
- **Retry contract:** update
  `apps/web/hooks/domains/session/use-session-subscription-retry.test.ts` to
  assert that delayed retries use the tracked subscribe operation and retain
  their backoff behavior.
- **Existing backend contract:** run the focused gateway tests covering
  `session.subscribe` acknowledgement and initial snapshot delivery; no
  backend production change is planned unless those tests expose a violation
  of the acknowledgement ordering assumed above.

## E2E Tests

- **Scenario:** GIVEN a freshly auto-started mock-agent task and a follow-up
  prompt, WHEN the session page mounts or resumes, THEN the idle state and
  persisted agent reply render without the page-object reload fallback.
- **Files:** the relevant existing cases under
  `apps/web/e2e/tests/session/auto-start-session.spec.ts` and
  `apps/web/e2e/tests/session/session-recovery.spec.ts`; add a dedicated
  `apps/web/e2e/tests/session/session-subscribe-recovery.spec.ts` only if a
  deterministic registration barrier is available.
- **How:** run the chromium session specs with the existing isolated backend;
  include the mobile project if the affected helper is changed for mobile.

## Verification Results

- `cd apps && pnpm --filter @kandev/web test -- --run lib/ws/client.test.ts hooks/domains/session/use-session-messages.test.ts hooks/domains/session/use-session-subscription-retry.test.ts` — 3 files and 46 tests passed.
- `cd apps/web && pnpm run typecheck` — passed.
- `cd apps/web && pnpm exec eslint --max-warnings 0 lib/ws/client.ts lib/ws/client.test.ts hooks/domains/session/use-session-messages.ts hooks/domains/session/use-session-message-fetch.ts hooks/domains/session/use-session-messages.test.ts hooks/domains/session/use-session-subscription-retry.ts hooks/domains/session/use-session-subscription-retry.test.ts e2e/pages/session-page.ts e2e/tests/chat/mobile-unread-divider.spec.ts e2e/tests/chat/unread-divider.spec.ts` — passed.
- `cd apps/backend && go test -race -run 'TestHandleSessionSubscribe|TestBroadcastToSession|TestHandleSessionDataRefresh' ./internal/gateway/websocket/...` — 6 tests passed.
- `cd apps/web && pnpm e2e:run --project chromium tests/session/auto-start-session.spec.ts tests/session/session-recovery.spec.ts` — 5 tests passed.
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-pause-resume-recovery.spec.ts` — 2 tests passed.
- `git diff --check` — passed.

The backend acknowledgement and membership ordering already satisfied the
contract, so no backend production change was needed. The web client now tracks
the acknowledgement for initial, reconnect, and delayed retry subscriptions;
transcript hydration waits on that readiness and ignores late results after
cleanup. The session page-object reload fallbacks specific to this race were
removed while the unrelated bounded startup reload remains.

## Implementation Waves And Parallel Candidates

Wave 1 (sequential):

- [x] [task-01-ack-gated-session-hydration](task-01-ack-gated-session-hydration.md)

The client, hook, retry path, and E2E helper share the same subscription
contract, so they are intentionally one sequential task.

## Open Questions

- The race-specific reload fallback can be removed; the general bounded
  `waitForLoad` reload remains for unrelated startup/SSR hydration failures.
