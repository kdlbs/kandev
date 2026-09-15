---
created: 2026-09-14
status: completed
requirements:
  - REQ-TASKS-QUEUE-ADMISSION-001
system_design:
  - ../../specs/tasks/system-design/queue-admission.md
legacy_specs: []
---

# Implementation Plan: Queue admission reliability

## Overview

Repair ordinary composer queue submissions for [issue #3663](https://github.com/kdlbs/kandev/issues/3663).
Durable server replay protection, client recovery, localized feedback, and desktop and phone verification are complete.

## Evidence and root cause

Inspected commit `ba960f973` on 2026-09-14. The issue has no attachments or comments.
The authenticated user is `carlosflorencio`, now assigned to the issue.

- `use-message-handler.ts`, `submitMessageWithAdmission`, supplies `clientQueueId` only when `planCommentRefs.length > 0`.
- `use-queue-admission.ts` forwards only supplied IDs and tolerates a committed refresh failure only for identified submissions.
- `queue-api.ts` omits the third timeout argument on initial and retry requests. Recovery requires a supplied `client_queue_id`.
- `wsQueueMessage` uses replay-aware admission only when plan-comment references exist. Ordinary `admitQueuedMessage` ignores `ClientQueueID`.
- `chat-input-area.tsx`, `showMessageSendToast`, treats errors outside `MessageSendError` as unknown connection failures, including `QueueFullError`.
- `client.ts` starts the request timer before sending. A disconnected send enters `pendingQueue`; timeout removes its waiter, not its buffered frame.

A temporary Vitest reproduction called real `queueMessage` with an ordinary payload and a mocked timeout.
It confirmed one request, two request arguments, no client ID, and immediate rejection without reconciliation.
The reproduction plus existing plan-comment API and message-handler suites passed: 3 files, 37 tests.
The temporary file was removed. At that investigation stage, no production or permanent test changes had been made.

The issue's claim that a timeout proves the request never reaches the server is not established.
A late buffered request can still arrive. This makes server deduplication a prerequisite for safe retries.
The issue's blanket claim of absent reconciliation is outdated for plan-comment submissions.

## Requirement conformance and assumptions

The task system owns this capability because it owns admission identity and durable prompt storage.
Existing resume requirements cover startup, and plan-comment requirements cover selected comments.
Neither defines recoverable ordinary admission during RUNNING. The new queue-admission requirement defines that missing contract.
The existing merge requirement remains authoritative; routing ordinary prompts through the comment-only path would violate its intended behavior.

Confirmed scope: investigation, assignment, implementation, and verification. No user interview was needed to preserve the existing queue and draft behavior.
The receipt design and alternatives are recorded in the ADR. The implementation follows the sequential work orders.

## Scope

### In scope

- Optional existing `client_queue_id` support for ordinary server admission.
- Atomic receipts, replay conflict handling, lifecycle cleanup, and database parity.
- Explicit 10s/30s budgets, bounded recovery, and one automatic retry.
- Typed rejection feedback, submitted-draft preservation, and desktop/mobile evidence.

### Out of scope

- Provider execution guarantees, offline outboxes, and browser-reload recovery of unacknowledged drafts.
- Global WebSocket buffer redesign, new settings, and merge-policy changes.
- Retrying an uncertain draft after a later deliberate Send with the previous identity.

## Technical approach

Apply the [queue-admission design](../../specs/tasks/system-design/queue-admission.md) and
[receipt ADR](../../decisions/2026-09-14-durable-queue-admission-receipts.md).
The server transaction is the correctness boundary. Receipt lookup precedes mutable admission checks, after authorization and incarnation validation.
Client queue/transcript reads accelerate recovery but are not the deduplication authority.
Keep optional unidentified-client compatibility and plan-comment behavior.

## ASCII UI preview

UI-01: Existing composer after failed queue admission. Shared structure for desktop Task chat and Quick Chat, and their phone composers.

```text
Before (capacity rejection):
[Message send status unknown]
[The connection dropped or timed out]
[Draft and attachments retained] [Send]

After (capacity rejection):
[Message not sent]
[The message queue is full.]
[Draft and attachments retained] [Send]

After (unresolved transport):
[Message send status unknown]
[The connection dropped or timed out]
[Draft and attachments retained] [Send]

After (confirmed acceptance):
[Existing queue chip or transcript]
[Empty submitted draft]         [Send]
```

Copy is illustrative and must use locale catalogs. Hierarchy and outcome distinctions are required by AC .5, .6, and .8.
Phone uses the existing toast and inline composer, wrapping text within the viewport.
Reuse `chat-input-toolbar-mobile.tsx` and `mobile-message-queue-management.spec.ts` as the nearest shipped examples.
No new sheet, navigation, scrolling region, or control is introduced. Preserve the existing safe area and touch target.

## Tests

All suffixes reference `AC-TASKS-QUEUE-ADMISSION-001`.

| Criteria | Required regression evidence |
| --- | --- |
| .1, .4 | `use-message-handler.test.ts`: ordinary queue sends carry a stable ID; `queue-api.admission.test.ts`: 10s/30s initial and retry budgets |
| .2, .3 | `repository_admission_test.go`: `TestQueueAdmissionReplay`, `TestQueueAdmissionConflict`, `TestQueueAdmissionConcurrentReplay`, `TestQueueAdmissionLifecycle` |
| .2, .7 | `TestQueueAdmissionMergeReplay`, `TestQueueAdmissionAttachmentReplay`; existing merge and plan-comment suites |
| .5 | `use-queue.plan-comments.test.ts` and new admission tests: accepted mutation plus failed refresh; preserve unsuccessful drafts |
| .6, .8 | `chat-input-area.test.tsx` and desktop/mobile E2E: rejection versus uncertainty and retained attachments |

New test files and names are implementation targets. TDD must first show failures for the specified missing behavior.
The temporary diagnostic test asserted the then-current behavior; it is not the implementation regression assertion.

## E2E tests

Add `e2e/tests/chat/queue-admission-reliability.spec.ts` for chromium and
`e2e/tests/chat/mobile-queue-admission-reliability.spec.ts` for mobile-chrome.
Use isolated fixture backends and real admission. Intercept only selected WebSocket frames to delay or drop requests/responses deterministically.
Cover a 6-second response, lost accepted response, pre-server lost request, queue-full rejection, and final uncertainty.
Assert content is admitted once, automatic attempts are capped, and draft/attachments match the outcome.
Include ordinary Auto-merge ON and OFF, attachment admission, Task chat and Quick Chat, and a real phone tap.
Unit tests cover the 30-second timer boundary without adding long browser waits.
Record expected injected transport errors with the harness rather than globally disabling strict WS accounting.

## Work orders

- [x] [Task 01: Durable ordinary queue admission](task-01-durable-admission.md) (completed)
- [x] [Task 02: Composer recovery and feedback](task-02-composer-recovery.md) (completed)

Executed sequentially. Task 02 depended on the server replay guarantee from Task 01.

## Verification results

Diagnostic evidence: 3 Vitest files, 37 tests passed. Temporary reproduction removed.
Specification validation passed: 268 decisions and 904 specifications validated. All specification files passed.
`git diff --check` passed. The package inventory contains two completed work orders.
Implementation checks passed: backend build and lint, focused backend and race suites, SQL guard, persistence store conformance, 146 focused web tests, typecheck, web lint, i18n validation, production build, and the desktop and mobile admission E2E suites.
Review remediation checks passed: structured WebSocket conflict preservation, scoped admission error mapping, ordinary transcript provenance, staged-upload rejection, full-fold acceptance timestamps with replay stability, session-incarnation fencing, persistence conformance coverage, accepted comment refresh handling, and pseudo-locale component-tag preservation.
The Postgres admission parity test was discovered and skipped because `KANDEV_TEST_POSTGRES_DSN` was not set; it remains available for configured database CI.

## Risks

- A later deliberate Send after uncertainty can duplicate the original admission; this package does not promise otherwise.
- Deployment must update the server before enabling ordinary client retries. Mixed older backends do not honor ordinary IDs.
- PostgreSQL parity still needs execution in an environment with `KANDEV_TEST_POSTGRES_DSN`.
- Public session guidance now documents the shipped admission and recovery behavior.
