---
id: "02-client-verification"
title: "Verify queue steering across clients"
status: complete
wave: 2
depends_on:
  - "01-live-fifo"
plan: "plan.md"
requirements:
  - REQ-UI-MESSAGE-QUEUE-SEND-NOW-001
acceptance_criteria:
  - AC-UI-MESSAGE-QUEUE-SEND-NOW-001.1
  - AC-UI-MESSAGE-QUEUE-SEND-NOW-001.2
  - AC-UI-MESSAGE-QUEUE-SEND-NOW-001.9
  - AC-UI-MESSAGE-QUEUE-SEND-NOW-001.11
system_design:
  - ../../specs/ui/system-design/message-queue-send-now.md
---

# Task 02: Verify queue steering across clients

## Summary

Prove the reported FIFO sequence through desktop and phone queue controls.
Verify the existing transport error mapping and document the refined behavior.

## In scope

- Add a shared scenario in `message-queue-workflow-helpers.ts` with distinct
  markers for FIFO A, selected B, and remainder C. Disable auto-merge for the
  disposable session so the entries remain distinct.
- Start A through ordinary Auto-run/FIFO, not through Send Now or a direct
  composer prompt. Wait for its output and authoritative running state, then
  enqueue B/C and invoke B's row Send Now. Hold B active while asserting C
  remains pending; after B finishes, assert C executes once and A does not
  restart. Assert the workflow step does not advance from silent cancellation.
- Add desktop `Send Now interrupts a running FIFO turn` and phone
  `mobile Send Now interrupts a running FIFO turn` tests in the existing suites.
  Use click/hover on desktop and tap on phone. Scope selectors to active chat.
  Arm `watchWs` before navigation and causal response waits before mutations.
  Use existing mock scripts to keep a turn active; never infer readiness from
  elapsed sleep. Clean up disposable tasks and modified settings on failure.
- Add a `sendQueuedNow` API regression whose rejection is an actual
  `WebSocketRequestError` with `send_now_conflict`, asserting `QueueSendNowError`.
  Current production mapping is expected to pass. Do not change production
  mapping unless this focused regression exposes a separate failure.
- Clarify in the existing public queue how-to sections that a running
  FIFO-delivered turn can be redirected after handoff. Retain brief genuine
  conflict/retry guidance and existing explicit Cancel distinctions.

## Out of scope

New UI controls, localization keys, real provider credentials, generic transport
refactors, and changes to global defaults or dynamic routing policy.

## Acceptance

1. Desktop and phone each cancel running FIFO A, deliver B once, and retain C
   for later Auto-run without a workflow transition or page reload.
2. Real transport conflict errors reach the existing localized conflict path.
   Existing genuine conflict behavior and queue reconciliation remain intact.
3. Phone controls are visible without hover, touch-sized, and contained; public
   how-to guidance matches the new behavior and all required checks pass.

## ASCII UI preview

UI-01, same existing inline composition as the [plan](plan.md#ascii-ui-preview):

```text
A running -> [B: Send Now] -> Cancel pending -> B running
Queue: B, C                                Queue: C
Composer remains visible                  Auto-run remains ON
```

Desktop uses hover/focus row actions; phone exposes a visible 44px touch target.
One queue scroll region, no new navigation or overlay. Verify the current
mobile queue exemplar rather than adding responsive CSS. Maps to .1/.2/.9/.11.

## Verification

From the repository root; bootstrap dependencies once if this worktree has not
been installed:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/api/domains/queue-api.test.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/message-queue.spec.ts -- --grep 'Send Now')
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-message-queue-management.spec.ts -- --grep 'Send Now')
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Managed E2E commands rebuild artifacts. Run them sequentially with the runner's
worker limits. Backend RED is owned by Task 01; these E2E tests verify the
integrated change after that fix, not a claimed pre-fix browser reproduction.

## Files likely touched

- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/tests/chat/message-queue-workflow-helpers.ts`
- `apps/web/e2e/tests/chat/message-queue.spec.ts`
- `apps/web/e2e/tests/chat/mobile-message-queue-management.spec.ts`
- `apps/web/lib/api/domains/queue-api.test.ts`
- `docs/public/sessions-and-review.md`
- `docs/public/coordination.md`

## Dependencies

Task 01.

## Risks

A directly prompted A would miss the FIFO-only defect. An API-level accepted
request alone does not prove B ran: assert agent output and ordering. Restore
any shared settings; prefer per-session overrides on disposable records.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/ui/requirements/message-queue-send-now.md), .1/.2/.9/.11.
- [Design](../../specs/ui/system-design/message-queue-send-now.md), presentation
  and compatibility sections.
- `expectSendNowWorkflowRunning` and the existing mobile targeted-order test.
- `queue-api.ts` current `asWSError`, `knownQueueError`, and existing API tests.

## Results

- `pnpm exec vitest run lib/api/domains/queue-api.test.ts`: 30 tests passed,
  including an actual `WebSocketRequestError` with `send_now_conflict`.
- Web typecheck, Prettier, and targeted ESLint passed.
- Chromium Send Now subset: 3 tests passed.
- Mobile Chrome Send Now subset: 3 tests passed, including the touch-sized
  visible action and overflow assertion.
- `node --test scripts/validate-public-docs.test.mjs`: 62 tests passed.
- `node scripts/validate-public-docs.mjs`: 46 published docs pages validated.
- `python3 scripts/list-docs.py validate`: 272 decisions and 935
  specifications validated.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
