---
id: "02-dismiss-and-continue"
title: "Dismiss and continue the current turn error"
status: done
wave: 2
depends_on:
  - 01-known-command-capacity
plan: "plan.md"
requirements:
  - REQ-PLATFORM-TURN-CONTINUITY-005
acceptance_criteria:
  - AC-PLATFORM-TURN-CONTINUITY-005.3
  - AC-PLATFORM-TURN-CONTINUITY-005.6
  - AC-PLATFORM-TURN-CONTINUITY-005.7
  - AC-PLATFORM-TURN-CONTINUITY-005.8
  - AC-PLATFORM-TURN-CONTINUITY-005.9
  - AC-PLATFORM-TURN-CONTINUITY-005.10
  - AC-PLATFORM-TURN-CONTINUITY-005.11
  - AC-PLATFORM-TURN-CONTINUITY-005.12
system_design:
  - ../../specs/platform/system-design/acp-prompt-error-continuity.md
---

# Task 02: Dismiss and continue the current turn error

## Summary

Add a primary inline action that submits one normal `continue` message and acknowledges the displayed error after acceptance.
Use the existing admission and reconciliation path. Preserve drafts and provide the same outcome on desktop and phone.

## In scope

- New focused continuation hook and shared current-error action group beside the existing dismissal control.
- Exact source task/session/stamp targeting and one stable client message ID per observed failure.
- Existing message acceptance, lost-response reconciliation, applied configuration, store publication, and acknowledgement feedback.
- Busy, rejected, accepted, queued, acknowledgement-failed, newer-error, and historical states.
- Suppression during automatic retry, foreground activity, cancellation, permission barriers, or genuine runtime recovery.
- Primary-first desktop and stacked phone controls, stable accessible names, localized status, and measured hit areas.
- English and six real locale catalogs, generated Traditional Chinese/pseudo catalogs, and accurate unsafe-work guidance.
- Component/hook, message admission, desktop, phone, preview Chat, and task-backed Quick Chat proof.
- Public guidance for both actions, including their effect on history and task attention.

## Out of scope

- A new endpoint, prompt type, dismissal reason, schema, scheduler, or recovery setting.
- Sending draft content, attachments, context references, pending model changes, or review comments with `continue`.
- Automatic acknowledgement repair through another prompt, original-prompt replay, runtime restart, or Start fresh.
- A continuation action in task context menus or historical/sibling error rows.

## Acceptance

1. One click/tap sends exact manual `continue` to the displayed error's session and acknowledges only its observed stamp after acceptance.
   Preserve current applied configuration, runtime continuity, draft text, attachments, and durable error history.
2. Rapid clicks, multiple viewers, remounts, and lost responses create one logical continuation for that stamp.
   Rejection preserves attention. Accepted acknowledgement failure retains attention with Dismiss only and never resends the prompt.
3. Desktop, phone, preview Chat, and task-backed Quick Chat share readiness and failure behavior.
   Phone controls stack, remain contained, and measure at least 44px. Fine-pointer desktop controls measure 28px within 1px.

## UI preview

UI-01 through UI-03, excerpt from the [combined preview](plan.md#ui-preview):

| View/state | Primary action | Secondary action and feedback |
| --- | --- | --- |
| UI-01 desktop, current manual error | Dismiss and continue, first in compact row | Dismiss turn error follows in the same row. Details follow the group |
| UI-01 phone, current manual error | Dismiss and continue, full-width first row | Dismiss turn error occupies the next full-width row. Guidance/details wrap |
| UI-02, submission pending | Stable label, disabled during admission | Competing Dismiss is disabled. One status region says Continuing |
| UI-03, accepted | Visible user continue message and later attributed response | History remains. Successful acknowledgement retires active controls |
| UI-03, acknowledgement failed after acceptance | No additional continuation action for this stamp | Keep attention, Dismiss only, and localized failure feedback |

The normal composer remains independent and keeps its draft.
No extra confirmation, overlay, navigation, or scroll region is introduced.
Use existing shared sizing primitives. Stack phone rows below the canonical 768px boundary, independently of pointer precision.
AC references: 005.9 through 005.12, with stamp behavior inherited from 005.3 and 005.6 through 005.8.
Compare the rendered desktop and phone structure with this preview during browser verification.

## Verification

Run from the repository root. Read `/tdd`, `/mobile-parity`, `/e2e`, and web guidance before implementation.
Use the hook's accepted-send/draft-preservation assertion as behavioral Red before adding production behavior.
Read existing message admission tests before extending their duplicate and acknowledgement-failure cases.

Bootstrap once if Task 01 did not already install dependencies:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

```bash
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/task/handlers -run 'Test(AcceptedManualPrompt|AddMessageFingerprintIncludesObservedTurnErrorStamp|DismissAndContinue)' -count=1)
(cd apps/web && pnpm test hooks/use-continue-turn-error.test.tsx hooks/use-task-turn-error.test.tsx components/task/chat/messages/turn-error-actions.test.tsx components/task/chat/messages/action-message-turn-error-continue.test.tsx components/task/chat/messages/action-message.test.tsx components/task/chat/messages/action-message-recovery.test.tsx lib/task-turn-error-feedback.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings hooks/use-continue-turn-error.ts components/task/chat/messages/turn-error-actions.tsx components/task/chat/messages/turn-error-dismiss-button.tsx components/task/chat/messages/action-message.tsx e2e/tests/session/acp-prompt-error-continuity.spec.ts e2e/tests/session/mobile-acp-prompt-error-continuity.spec.ts)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/session/acp-prompt-error-continuity.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-acp-prompt-error-continuity.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

If additional helpers or suites change, add their exact lint/test paths before marking Results complete.
Run changed-Go lint with every modified/new Go file, using Task 01's collection command.
Run managed browser commands sequentially after the locale generators finish.
Record narrow fine-pointer phone geometry, long translated labels, and a rendered comparison with UI-01 through UI-03.
The acceptance failure case must observe persisted message identity as well as UI feedback.

## Files likely touched

- `apps/web/hooks/use-continue-turn-error.ts` and `.test.tsx` (new).
- `apps/web/components/task/chat/messages/turn-error-actions.tsx` and `.test.tsx` (new).
- `apps/web/components/task/chat/messages/turn-error-dismiss-button.tsx`.
- `apps/web/components/task/chat/messages/action-message.tsx`, existing regression tests, and new `action-message-turn-error-continue.test.tsx`.
- `apps/web/hooks/message-request.ts` only if a small reusable seam is necessary. Preserve existing ID reconciliation.
- `apps/web/lib/task-turn-error-feedback.ts` only if shared acknowledgement feedback requires adjustment.
- `apps/web/src/locales/*/task.json` and `chat.json`, including generated catalogs.
- `apps/backend/internal/task/handlers/message_handlers_turn_error_test.go`.
- `apps/web/e2e/helpers/acp-prompt-error-continuity.ts` and its desktop/mobile session suites.
- `docs/public/tasks-and-workflows.md` and `docs/public/sessions-and-review.md`.

## Dependencies

Task 01. Both tasks update the same recovery guidance and final browser scenario expectations.
Read the new requirements location after the mechanical split. Stable REQ/AC IDs remain unchanged.

## Risks

- Sampling the current stamp during submission can acknowledge an error the user never saw.
- Calling Dismiss before admission can hide the error even when the send fails.
- Recreating IDs after response loss or acknowledgement failure can repeat accepted work.
- Sending through the composer handler without preserving draft ownership can clear unsent content.
- A historical retained flag cannot establish current readiness after runtime loss.
- Ordinary capacity queueing is acceptance without immediate agent activity. Feedback must distinguish those outcomes.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/acp-prompt-error-continuity.md), requirement 005.
- [Design](../../specs/platform/system-design/acp-prompt-error-continuity.md), Manual Dismiss and continue and existing acknowledgement sections.
- Existing `sendMessageRequest`, `resolveTurnErrorDisplay`, `useTaskTurnError`, and acknowledgement feedback.
- Existing task and Quick Chat error fixtures and mobile control-sizing guidance.

## Results

Done. `TurnErrorActions` renders **Dismiss and continue** before **Dismiss turn error** for the current same-session error only. The action sends one visible `continue` message with the observed stamp, reuses `sendMessageRequest` reconciliation, preserves the composer draft, and keeps attention when the send is rejected or its acknowledgement fails. Accepted continuations are detected by their stable message ID, so remounts and other viewers do not submit another prompt. The hook captures the target session's applied `metadata.plan_mode` on the first activation and keeps that value immutable across retries for the same error; it never reads an unsent composer toggle.

- `(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/task/handlers -run 'Test(AcceptedManualPrompt|AddMessageFingerprintIncludesObservedTurnErrorStamp|DismissAndContinue)' -count=1)`: passed.
- `(cd apps/web && pnpm test hooks/use-continue-turn-error.test.tsx hooks/use-task-turn-error.test.tsx components/task/chat/messages/turn-error-actions.test.tsx components/task/chat/messages/action-message-turn-error-continue.test.tsx components/task/chat/messages/action-message.test.tsx components/task/chat/messages/action-message-recovery.test.tsx lib/task-turn-error-feedback.test.ts)`: 123 passed.
- `(cd apps/web && pnpm run typecheck)`: passed.
- `(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings <changed web files>)`: passed.
- `(cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:pseudo && pnpm run i18n:check && pnpm run i18n:ratchet)`: passed.
- `(cd apps/web && pnpm e2e:run --host --no-build --project chromium tests/session/acp-prompt-error-continuity.spec.ts)`: 9 passed, including task Chat, two-viewer concurrent activation, preview Chat, task-backed Quick Chat, reload, and a narrow fine-pointer pseudo-locale layout. Provider rejection and lost-response reconciliation remain covered by the hook selection.
- `(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/session/mobile-acp-prompt-error-continuity.spec.ts)`: 3 passed.
- `node --test scripts/validate-public-docs.test.mjs`, `node scripts/validate-public-docs.mjs`, `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check`: passed.

The rendered desktop row matches UI-01, both controls disable with one Continuing status in UI-02, and UI-03 keeps the historical row and user message after acknowledgement. Phone and narrow fine-pointer rows stack at the 768px boundary; the controls render at 48px and measured targets were at least 44px in both suites, and pseudo-localized labels produced no document overflow. Dismissal and continuation now share one in-flight owner, and a two-viewer activation test asserts one logical continuation.
