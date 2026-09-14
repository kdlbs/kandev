---
id: "02-composer-recovery"
title: "Composer recovery and feedback"
status: completed
wave: 2
depends_on:
  - "01-durable-admission"
plan: "plan.md"
requirements:
  - REQ-TASKS-QUEUE-ADMISSION-001
acceptance_criteria:
  - AC-TASKS-QUEUE-ADMISSION-001.1
  - AC-TASKS-QUEUE-ADMISSION-001.4
  - AC-TASKS-QUEUE-ADMISSION-001.5
  - AC-TASKS-QUEUE-ADMISSION-001.6
  - AC-TASKS-QUEUE-ADMISSION-001.8
system_design:
  - ../../specs/tasks/system-design/queue-admission.md
---

# Task 02: Composer recovery and feedback

## Summary

Give ordinary queue submissions a stable admission ID and explicit timeout budget.
Recover uncertain submissions once, and distinguish rejection from uncertainty without losing drafts.

## In scope

- Pass `clientAdmissionId` unconditionally from the message handler's queue branch.
- Use the same request closure, ID, immutable identity, payload, and numeric 10s/30s timeout for both attempts.
- Bound reconciliation and reconnection as specified in the design. Do not retry explicit first-attempt server rejection.
- Keep committed admission successful when terminal queue refetch fails. Reject stale responses through existing operation tokens.
- Preserve typed queue codes and localize rejection feedback through the existing composer surfaces.
- Add all five locale values and generate Traditional Chinese using the repository script.
- Add unit and desktop/mobile browser regressions, then update `docs/public/sessions-and-review.md` with shipped error/recovery behavior.

## Out of scope

New layout, automatic submission after reload, and durable browser outboxes.

## Acceptance

1. Ordinary and comment-bearing queue submissions retain one ID and payload through at most one automatic retry, with correct numeric timeouts.
2. Confirmed acceptance clears only the submitted draft. Rejection or final uncertainty retains text and attachments with the correct localized feedback.
3. Task chat and Quick Chat pass desktop and phone recovery tests against the real replay-safe backend, including Auto-merge and attachment scenarios.

## ASCII UI preview

UI-01: Shared composer outcome, excerpt from the [full preview](plan.md#ascii-ui-preview).

```text
Capacity rejection:  [Message not sent] [The message queue is full.]
Unresolved delivery: [Message send status unknown] [Connection feedback]
                     [Retained draft + attachments] [Send]
Accepted delivery:   [Existing queue chip or transcript] [Cleared draft]
```

AC .5, .6, .8 require the distinction and retained input. Text is illustrative and localized.
Desktop and phone reuse the existing toast/composer hierarchy. Phone wraps copy and keeps the current safe area and 44-pixel Send target.
No new scroll owner or navigation is introduced. Compare rendered screenshots with this structure during the browser scenarios.

## Verification

Run from the repository root. If workspace dependencies are absent, first run `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/web && pnpm exec vitest run hooks/use-message-handler.test.ts hooks/use-message-handler.plan-comments.test.ts hooks/domains/session/use-queue.test.ts hooks/domains/session/use-queue.plan-comments.test.ts lib/api/domains/queue-api.test.ts lib/api/domains/queue-api.plan-comments.test.ts lib/api/domains/queue-api.admission.test.ts components/task/chat/chat-input-area.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/queue-admission-reliability.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-queue-admission-reliability.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Add every new helper test to the targeted Vitest command before marking done.
Use fake timers for budgets. Browser tests must delay/drop only selected frames and preserve real backend admission.
See the plan for the required E2E scenario matrix. Run projects sequentially through the managed builder.

## Files likely touched

- `apps/web/hooks/use-message-handler.ts` and `.test.ts`
- `apps/web/hooks/domains/session/use-queue-admission.ts`
- `apps/web/hooks/domains/session/use-queue.test.ts` and `use-queue.plan-comments.test.ts`
- `apps/web/lib/api/domains/queue-api.ts`, `queue-api.test.ts`, `queue-api.plan-comments.test.ts`
- `apps/web/lib/api/domains/queue-api.admission.test.ts` (new)
- `apps/web/lib/chat/message-send-error.ts` and its test if shared codes change
- `apps/web/components/task/chat/chat-input-area.tsx` and `chat-input-area.test.tsx`
- `apps/web/components/task/passthrough-chat-composer.tsx` if the shared error audit identifies a consumer
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/task.json`
- `apps/web/e2e/tests/chat/queue-admission-reliability.spec.ts` (new)
- `apps/web/e2e/tests/chat/mobile-queue-admission-reliability.spec.ts` (new)
- `docs/public/sessions-and-review.md`

## Dependencies

Task 01 must pass. Do not enable ordinary replay against a backend that ignores the admission ID.

## Risks

A successful replay response can refer to a removed or merged source. Refresh authoritative state instead of adding a phantom queue row.
A new explicit submission after uncertainty is not an automatic retry and can duplicate earlier accepted content.
Unbounded transcript pagination delays useful feedback. Raw server errors are not localized UI copy.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/tasks/requirements/queue-admission.md)
- [Design](../../specs/tasks/system-design/queue-admission.md), Client recovery and Composer feedback
- Existing queue plan-comment API tests and message-handler tests
- `apps/web/e2e/tests/chat/message-queue.spec.ts` and `mobile-message-queue-management.spec.ts`
- `apps/web/AGENTS.md`, `/mobile-parity`, `/e2e`, and `/tdd`

## Results

Implemented stable ordinary admission IDs, 10-second and 30-second request budgets, bounded reconciliation with one same-identity retry, typed queue rejection errors, draft and attachment preservation, and localized composer feedback. Task chat and Quick Chat share the recovery behavior, with desktop and mobile browser coverage. Added the five locale catalogs and updated the public sessions guide.

Verification passed:

- The targeted Vitest command above: 8 files, 142 tests.
- `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, and `pnpm run build`.
- `pnpm run e2e:sleep-ratchet`.
- Chromium admission E2E: 4 passed.
- Mobile Chrome admission E2E: 2 passed.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check`.

Review remediation passed: unrecognized structured WebSocket conflicts preserve their original error identity, code, and details for plan-comment and primary-session recovery. The pseudo-locale retains the named `count` component tags, and unrelated generated catalog churn was removed.
