---
id: "03-unrecoverable-claims"
title: "Mark unrecoverable claims not delivered"
status: complete
wave: 2
depends_on:
  - "02-claim-owns-sources"
plan: "plan.md"
requirements:
  - REQ-UI-MESSAGE-QUEUE-SEND-NOW-001
acceptance_criteria:
  - AC-UI-MESSAGE-QUEUE-SEND-NOW-001.14
system_design:
  - ../../specs/ui/system-design/message-queue-send-now.md
---

# Task 03: Mark unrecoverable claims not delivered

## Summary

When a Send Now claim can be neither delivered nor restored, the backend
releases it and marks the user message as not delivered. The chat shows that
status on desktop and phone, so the message never looks delivered when it was
not.

## In scope

- One release step for `ErrSendNowClaimChanged` after the retry budget, called
  from the restore in `executeSendNowClaimWithContext`, from
  `restorePendingSendNowClaimsForRecovery`, and from startup reconciliation
  without a recovery-block store. It verifies that the exact claim still owns
  its sources; when no successor owns a surviving source, it sets
  `delivery_status: not_delivered` on the recorded user message or records the
  message from the dispatch envelope in an already-completed lifecycle-only
  turn, then releases the claim and every source row it owns. It is idempotent
  per claim and retries transient failures with the bounded claim mutation
  helper.
- Apply it only while the claim's session identity is still current. A session
  identity mismatch keeps its existing handling.
- `models.MessageDeliveryStatusNotDelivered`, and two narrow `MessageCreator`
  methods: one sets the delivery status on an exact session-owned user message;
  the other records an idempotent user message in a completed lifecycle-only
  turn, so release never starts or adopts an active turn.
- `chat-message.tsx` renders the badge for `not_delivered` beside the existing
  `blocked` badge, with `task:deliveryMessageNotDelivered` in every shipped
  locale. Run `pnpm run i18n:zh-hant` for the Traditional Chinese pair. Use no
  em dash.

## Out of scope

- Resending automatically, or a resend button.
- Claims invalidated by deleting or replacing the task or session.
- Rows left on already-affected installs by the old code.

## Acceptance

- Orchestrator regressions for each of the three sites: the claim row is gone,
  the user message carries `not_delivered`, and a second pass changes nothing.
  Write them first and confirm they fail on `main`: the row stays, or the
  message is unmarked.
- A claim whose restore succeeds leaves the message unmarked, and the #4988
  restore drain still delivers it.
- Desktop and phone render the localized badge for a not-delivered message,
  with no horizontal overflow on the phone.

## UI preview

`UI-01: Not-delivered user message`, from the
[plan's preview](plan.md#ascii-ui-preview):

```text
                              [ Not delivered. Send it again. ]
                                       +---------------------------------+
                                       | Fix the button spacing (2 notes) |
                                       +---------------------------------+
```

The badge reuses the `blocked` badge's `role="status"` element and slot. Copy
and color are illustrative.

## Verification

```bash
(cd apps/backend && env -u KANDEV_DEBUG_AGENT_MESSAGES go test -trimpath ./internal/orchestrator -run 'SendNow' -count=1)
(cd apps/backend && go test -trimpath ./internal/task/... -count=1)
bash scripts/lint-go-changed <changed Go files>
(cd apps/web && pnpm exec vitest run components/task/chat/messages/chat-message.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps && pnpm --filter @kandev/web lint)
(cd apps/web && pnpm e2e:run tests/chat/send-now-not-delivered.spec.ts tests/chat/mobile-send-now-not-delivered.spec.ts)
```

The E2E specs seed a pending claim whose source row is missing, then reach the
release step without a live agent where possible. With SQLite, startup parks
the claim behind an `unknown_send_now_outcome` recovery block, and resolving
that block runs `restorePendingSendNowClaimsForRecovery`. Record the chosen
path in Results. The delivery-record recovery fixture in
`apps/web/e2e/helpers/delivery-record-recovery.ts` shows how to seed backend
state.

## Files likely touched

- `apps/backend/internal/orchestrator/queue_send_now.go`
- `apps/backend/internal/orchestrator/queue_send_now_claim.go`
- `apps/backend/internal/orchestrator/service.go` (`MessageCreator`)
- `apps/backend/internal/orchestrator/queue_send_now_unrecoverable_test.go` (new)
- `apps/backend/internal/task/models/models.go`
- The `MessageCreator` implementation and its test fakes
- `apps/web/components/task/chat/messages/chat-message.tsx` and its test
- `apps/web/src/locales/*/task.json`
- `apps/web/e2e/tests/chat/send-now-not-delivered.spec.ts` and
  `mobile-send-now-not-delivered.spec.ts` (new)

## Dependencies

Task 02.

## Risks

- Marking a message that another path did deliver would mislead the user. With
  Task 02, FIFO can no longer take a claimed row, so an unrestorable claim's
  message has not been delivered by FIFO. The release step must not run for an
  accepted claim.

## Parallelism

`sequential`

## Inputs

- [Unrecoverable claims](../../specs/ui/system-design/message-queue-send-now.md#unrecoverable-claims)
- `markDeliveryMessageBlocked` in `apps/backend/internal/task/handlers/message_handlers.go`
- `queue_send_now_recovery_test.go`

## Results

Implemented. All three `ErrSendNowClaimChanged` sites call one release step:
the running restore in `executeSendNowClaimWithContext`, recovery restore, and
startup reconciliation without a recovery-block store. The step verifies the
claim still owns its sources, marks the recorded message or records it from the
dispatch envelope with `delivery_status: not_delivered` while the claim remains
available for retry, then releases the claim and every source row it owns. A
successor-owned source keeps the message unmarked. A message that is absent from
history is recorded in an already-completed lifecycle-only turn, so release
never starts or adopts an active conversation turn. The step retries transient
failures with the existing bounded claim mutation helper. A session identity
mismatch keeps the previous discard behavior, and an accepted claim's
superseded record is discarded without marking the message. The task service
exposes a narrow `SetUserMessageDeliveryStatus` and
`CreateCompletedUserMessageIdempotent` pair through `MessageCreator`, and the
chat bubble renders the localized badge beside the existing `blocked` badge.

Backend coverage: `queue_send_now_unrecoverable_test.go` (all three sites,
source settling, successor ownership, idempotency, transient-release retry,
existing-message and replaced-session cases, plus assertions that release
creates no active turn), `send_now_claim_ownership_test.go` (memory/SQLite/
PostgreSQL release and preserved earlier-claim ownership), `service_message_delivery_status_test.go`
(exact session-owned user message and a completed lifecycle-only turn that
leaves an existing active turn untouched, including concurrent callers), and
the successful-restore ownership regression.
Frontend coverage: `chat-message.test.tsx` renders the localized badge.
E2E coverage: `tests/chat/send-now-not-delivered.spec.ts` and
`tests/chat/mobile-send-now-not-delivered.spec.ts` use the new
`e2e-send-now-fixture`. The fixture seeds a pending claim whose durable source
row is missing and runs `restorePendingSendNowClaimsForRecovery`, the same
release entry point a resolved `unknown_send_now_outcome` recovery block uses,
without a live agent. The managed runner, CI build step, build identity, and
Docker build path all build the fixture. Both specs passed locally with the
host runner (`chromium` and `mobile-chrome`), including the completed
lifecycle-only turn.

Verified:

```bash
(cd apps/backend && env -u KANDEV_DEBUG_AGENT_MESSAGES go test -trimpath ./internal/orchestrator -run 'SendNow' -count=1)
(cd apps/backend && go test -trimpath ./internal/task/... -count=1)
(cd apps/web && pnpm exec vitest run components/task/chat/messages/chat-message.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps && pnpm --filter @kandev/web lint)
bash scripts/lint-go-changed <changed Go files>
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium -- tests/chat/send-now-not-delivered.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome --no-build -- tests/chat/mobile-send-now-not-delivered.spec.ts)
python3 .github/scripts/e2e-tests-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
```
