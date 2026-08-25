---
id: "06-idempotent-message-submission"
title: "Make message submission idempotent"
status: completed
wave: 6
depends_on: ["04-stabilize-session-transport", "05-consume-task-summaries"]
plan: "plan.md"
spec: "../../specs/platform/requirements/bounded-task-status-delivery.md"
---

# Task 06: Make Message Submission Idempotent

## Acceptance

- Every in-repository `message.add` caller supplies a stable message ID for one
  user intent and retains it through timeout, reconnect, retry, response, and
  notification hydration.
- Duplicate/concurrent duplicate IDs in the same authorized task return one
  persisted user message and run session checks, turn-start hooks, workflow
  transitions, and prompt dispatch at most once; cross-scope reuse is rejected.
- The composer reports success when response/notification/list reconciliation
  finds the ID and shows unknown status only after bounded reconciliation
  fails; model selection and draft behavior remain intact.

## TDD sequence

1. RED: add backend duplicate, concurrent duplicate, uniqueness-race,
   cross-task, and post-`on_turn_start` session-switch cases with explicit hook
   and dispatch counters.
2. RED: add frontend tests proving one generated ID survives request timeout,
   reconnect retry, dropped notification, and duplicate response hydration.
3. GREEN: serialize backend acceptance before mutable hooks, use
   `CreateMessageWithID`, reload races, and return accepted duplicates early.
4. GREEN: centralize client submission/reconciliation and migrate every direct
   `message.add` caller to the stable-ID helper.
5. REFACTOR: keep optimistic/response/notification upserts idempotent and make
   terminal error classification deterministic.

## Verification

```bash
cd apps/backend && go test ./internal/task/handlers/... ./internal/task/service/... ./internal/task/repository/...
cd apps && pnpm --filter @kandev/web test -- --run \
  hooks/use-message-handler.test.ts \
  lib/chat/message-send-error.test.ts
```

## Files likely touched

- `apps/backend/internal/task/handlers/message_handlers.go`
- `apps/backend/internal/task/handlers/message_handlers_test.go`
- `apps/backend/internal/task/service/service_messages.go`
- message repository/race tests as needed
- `apps/web/hooks/use-message-handler.ts`
- `apps/web/hooks/use-message-handler.test.ts`
- `apps/web/lib/chat/message-send-error.ts`
- chat input/composer send flow
- direct `message.add` callers in review, comments, plans, Office, and
  passthrough surfaces
- focused caller tests

## Dependencies

Task 04 supplies deterministic control delivery/reconnect behavior. Task 05
removes the background traffic source before end-to-end send verification.

## Parallelism

Sequential. Backend and frontend must agree on the message-ID contract before
Task 07 exercises retries.

## Inputs

- Spec **Retry-safe message submission** and uncertain-send failure mode.
- Existing `CreateMessageWithID` service path and response hydration in
  `use-message-handler.ts`.
- Existing message-added gap E2E behavior.

## Risks

- The duplicate check must run before session-state and turn-start mutation.
- A uniqueness conflict after a concurrent first write must reload and return
  the accepted row rather than dispatching again.
- Generated IDs belong to one intent; creating a new ID on retry defeats the
  contract.

## Verification results

- Backend message handler/service/repository integration packages — passed.
- `cd apps/web && pnpm exec vitest run` — passed (1,011 files; 7,743 tests, 4 skipped).
- Every in-repository direct `message.add` UI caller now supplies a stable
  `client_message_id`; timeout/connection-closed reconciliation retries the
  exact same ID and upserts the accepted message.
- Backend acceptance is serialized per ID, checks an existing authorized row
  before session/turn-start side effects, and reloads after a uniqueness race.
- The remaining limitation is the approved crash window: exactly-once
  external agent execution cannot be guaranteed across a complete backend
  process crash.

## Output contract

Report wire contract, serialization strategy, all migrated callers, hook/
dispatch count evidence, backend/frontend test results, and any crash-window
limitations retained from the spec.
