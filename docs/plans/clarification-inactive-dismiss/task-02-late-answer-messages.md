---
id: "02-late-answer-messages"
title: "Send late answers as new messages"
status: pending
wave: 2
depends_on:
  - 01-reconcile-inactive-responses
plan: "plan.md"
requirements:
  - REQ-TASKS-CLARIFICATION-LIFECYCLE-001
acceptance_criteria:
  - AC-TASKS-CLARIFICATION-LIFECYCLE-001.4
  - AC-TASKS-CLARIFICATION-LIFECYCLE-001.5
  - AC-TASKS-CLARIFICATION-LIFECYCLE-001.6
  - AC-TASKS-CLARIFICATION-LIFECYCLE-001.7
  - AC-TASKS-CLARIFICATION-LIFECYCLE-001.8
  - AC-TASKS-CLARIFICATION-LIFECYCLE-001.9
system_design:
  - ../../specs/tasks/system-design/clarification-active-lifecycle.md
  - ../../specs/tasks/system-design/clarification-response-reliability.md
---

# Task 02: Send late answers as new messages

## Summary

Let users answer earlier human clarification questions through ordinary messages.
Preserve answers when tool submission receives a recognized inactive result.
This revises PR #3799's removal-only behavior without reopening tool authority.

## In scope

- Transcript entry point and inline late-answer form on desktop and phone.
- A shared question/answer formatter and adapter to ordinary message admission.
- Recognized inactive-answer fallback, draft retention, stable retry identity,
  correct session targeting, and explicit sent/queued feedback.
- Existing live-waiter and detached-current-turn paths remain operational.
- Localization in all supported catalogs and focused public documentation.

## Out of scope

Backend tool-response authority changes, replacement sessions, forced interruption,
Inbox History mutations, permission approvals, and parent-agent question routing.
Do not make historical questions count as pending workflow input.

## Acceptance

1. A user can answer an earlier unanswered question from its conversation.
   Question prompts, selected labels, and custom text reach ordinary admission
   for the source task/session. Idle, busy, and unavailable cases match chat.
2. Recognized inactive results preserve affirmative answers for the same path.
   Close sends nothing. Unknown conflicts and uncertain tool transport results
   retain the existing response retry path instead of sending a duplicate message.
3. Failures retain drafts; retries reuse admission identity. Navigation and
   newer questions cannot redirect delivery or lose a pending admission.
   Desktop and phone distinguish queued admission from sent messages.

## Technical approach

Extend `useClarificationGroup` with a typed late-message callback/outcome.
Capture the submitted questions and answers before awaiting the response.
Do not emit a destructive host outcome before fallback admission settles.
Keep an admission owner mounted independently of the expiring overlay, so
its draft and stable request ID survive cache updates and close/reopen cycles.

Reuse `useMessageHandler`, `deriveSessionInputMode`, and `useQueue` through
one shared adapter. Do not issue raw prompt or queue requests from components.
Read the source session's latest state; an obsolete question must not force
`hasPendingClarification=true`. A separate current question keeps its barrier.
Retain existing request-generation and per-row version fences.

Add a shared formatter with focused tests. Render source questions and answers
as ordinary user text, not a privileged system marker. Never derive the target
from global active selection after asynchronous work.

`ClarificationRequestMessage` exposes the history action for human questions
that are no longer operational. Reuse the existing question form with explicit
new-message mode and Close. The message renderer supplies needed conversation
context. Read-only surfaces navigate to that conversation; the Inbox History
read path remains free of mutations. Existing authenticated message admission
owns access and availability checks.

## ASCII UI preview

UI-02: Shared desktop and phone inline flow, from transcript history.

```text
[Earlier question                                  ]
[Answer as new message                             ]
       opens:
[Question and answer choices                       ]
[Custom answer                                     ]
[Close]                       [Send as new message  ]
[Failure: draft retained, Retry available           ]
```

See [the combined preview](plan.md#revised-contract-and-delivery-scope).
Reuse inline clarification controls and existing scroll ownership. Keep phone
safe areas and 44px coarse-pointer targets. Return focus to opener on Close.
Localize action and sent/queued feedback. Map to criteria `.4` through `.9`.

## Tests and verification

Add deterministic coverage for both entry points: inactive response after an
answer submission, and direct historical-question answering. Cover the source
session ending its turn without further output; no live waiter is required.
Cover busy admission, another current question, unavailable session, timeout,
retry identity, and navigation while admission is unresolved. Close must make
zero new-message requests. Existing live-answer and Skip tests must stay green.

Add `late answer` cases to desktop/mobile clarification browser files. Assert
actual admitted message or queue content with the original question and answers,
not only disappearance of the overlay. A failed admission preserves the form.

Run these commands from the repository root during implementation, not design:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/domains/session/use-clarification-group.test.ts hooks/domains/session/use-clarification-group.regressions.test.ts hooks/domains/session/use-clarification-group.timeout.test.ts hooks/use-message-handler.test.ts hooks/domains/session/session-input-mode.test.ts components/task/chat/clarification-input-overlay.test.tsx components/task/chat/messages/clarification-request-message.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:check)
make build-web
make build-backend
(cd apps/web && pnpm e2e:run --no-build --project chromium -- tests/chat/clarification.spec.ts --grep 'late answer')
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome -- tests/chat/mobile-clarification.spec.ts --grep 'late answer')
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Include any newly extracted helper test files in the exact Vitest command
before implementation completion. Record real results and rendered UI checks.

## Files likely touched

- `apps/web/hooks/domains/session/use-clarification-group.ts` and its tests.
- `apps/web/hooks/use-message-handler.ts` and its existing tests if adaptation is needed.
- `apps/web/components/task/chat/clarification-input-overlay.tsx` and its tests.
- `apps/web/components/task/chat/messages/clarification-request-message.tsx` and its tests.
- `apps/web/components/task/chat/message-renderer.tsx`.
- Task chat, Quick Chat, and run-transcript host callback wiring as required.
- `apps/web/e2e/tests/chat/clarification.spec.ts`.
- `apps/web/e2e/tests/chat/mobile-clarification.spec.ts`.
- Relevant `apps/web/src/locales/*/task.json` and `chat.json` catalogs.
- `docs/public/tasks-and-workflows.md`.

## Dependencies

Task 01, PR #3799 head `6edc7320dd`. Re-read its current head before implementation.

## Risks

A tool failure can be ambiguous. Fall back only on recognized inactive results.
Cache retirement must not unmount the owner of draft/admission state.
Historical content must never become trusted system instructions.
Current-turn authority and Inbox History isolation remain unchanged.

## Parallelism

`sequential`

## Inputs

- Updated lifecycle criteria `.4` through `.9`.
- Response-reliability design's inactive-response and late-answer section.
- ADR `2026-09-18-late-clarification-messages`.
- Task 01's completed race and Escape regressions.

## Results

Pending. No production changes, builds, or tests performed for this revised scope.
