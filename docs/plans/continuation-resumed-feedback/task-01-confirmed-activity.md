---
id: "01-confirmed-activity"
title: "Persist and render confirmed continuation activity"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-002
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-003
acceptance_criteria:
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.1
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.2
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.1
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.3
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.4
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.5
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.6
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.7
system_design:
  - ../../specs/platform/system-design/provider-interruption-continuation.md
---

# Task 01: Persist and render confirmed continuation activity

## Summary

Change the existing card to neutral feedback after verified accepted-prompt
activity. Persist that presentation without ending the retry episode.
Deliver the backend transition and frontend consumer together.

## In scope

- A bounded activity record for the current accepted execution/generation.
- Reconciliation at acceptance and guarded stream observation.
- Persisted `recovery_activity_confirmed` through the existing status update.
- A neutral renderer with localized heading and existing Cancel action.
- Race, stale-event, retry-budget, cleanup, and compatibility regressions.

## Out of scope

- Provider admission, transport adapters, prompt payload, timing, and retry limits.
- New migrations, flags, scheduler, global activity rules, or browser-local latches.
- Browser scenario changes and public documentation, owned by Task 02.

## Acceptance

1. The named backend regression confirms neutral metadata before completion only
   after accepted-prompt activity. A mixed completed/pending predecessor and stale
   events cannot confirm a new attempt. Acceptance alone retains warning feedback.
2. Publication is serialized with retirement and replacement. A late phase write
   preserves confirmation. Another failure retains the same budget progression.
   No presentation path resets ownership, dispatches work, or completes a turn.
3. The card shows neutral localized feedback, provider context, preserved-history
   information, and Cancel. It omits warning visuals, the old failure heading,
   countdown, and counter. Historical or invalid metadata keeps existing behavior.

## ASCII UI preview

Excerpts from [the combined preview](plan.md#ascii-ui-preview), mapped to
`003.1`, `003.4` through `003.7`.

UI-01: Desktop Task > Chat after confirmed resumed activity.

```text
Before: [!] Agent connection lost. Continuing...  [Cancel]
            Attempt 1 of 5
After:  [i] Agent resumed                         [Cancel]
            Cursor / selected model
            Previous history and tool results are preserved.
```

UI-02: Phone Task > Chat after the same transition.

```text
+--------------------------------------+
| [i] Agent resumed                    |
| Cursor / selected model              |
| Previous history and tool results    |
| are preserved.                       |
| [              Cancel              ] |
+--------------------------------------+
```

UI-03: Before activity, keep the existing warning and waiting/reconnecting label.
The drawing labels are illustrative localized text. Preserve the single Chat
scroll owner and existing safe-area layout. The phone action retains at least
44px hit height. Desktop keeps its compact action. Task 02 provides rendered proof.

## Verification

Use `/tdd`. Add the backend and component regressions before production edits.
The expected RED is unchanged warning presentation after valid current activity,
not a missing selector or an unresolved import. Additive metadata can reach the
existing writer/component in the regression fixture before implementation.

Name the backend cases listed in the plan. In `action-message.test.tsx`, name
the rendered case “renders neutral feedback only after attested resumed activity”.
Cover boolean false, omitted/malformed markers, wrong mode/phase, RUNNING without
activity, existing replay, and terminal states in the pure helper suite.

Run from the repository root. If workspace dependencies are missing, first run
`pnpm install --frozen-lockfile` from `apps/`.

```bash
(cd apps/backend && go test -trimpath ./internal/orchestrator -run 'TestInterruptionContinuation|TestTransientRetry|TestHandleTransientFailure|TestPromptAttemptEvidence|TestRetained.*Capacity|TestCapacity' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/orchestrator -run 'TestInterruptionContinuationActivity' -count=1)
bash scripts/lint-go-changed apps/backend/internal/orchestrator/provider_interruption_activity.go apps/backend/internal/orchestrator/provider_interruption_activity_test.go apps/backend/internal/orchestrator/provider_interruption_cancel.go apps/backend/internal/orchestrator/provider_interruption_notice.go apps/backend/internal/orchestrator/event_handlers_streaming.go apps/backend/internal/orchestrator/event_handlers_transient.go apps/backend/internal/orchestrator/event_handlers_transient_messages.go apps/backend/internal/orchestrator/dynamic_evidence.go
(cd apps/web && pnpm exec vitest run components/task/chat/messages/interruption-recovery-feedback.test.ts components/task/chat/messages/action-message.test.tsx)
(cd apps/web && pnpm exec eslint components/task/chat/messages/interruption-recovery-feedback.ts components/task/chat/messages/action-message-details.tsx components/task/chat/messages/transient-retry-notice.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
git diff --check
```

The Go lint list names the planned files. Before execution, reconcile it with
the actual changed Go paths, including any additional test file. Do not pass
nonexistent files or omit changed files from the recorded command.

## Files likely touched

- New `apps/backend/internal/orchestrator/provider_interruption_activity.go`.
- New `apps/backend/internal/orchestrator/provider_interruption_activity_test.go`.
- `apps/backend/internal/orchestrator/provider_interruption_cancel.go`.
- `apps/backend/internal/orchestrator/provider_interruption_notice.go`.
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`.
- `apps/backend/internal/orchestrator/event_handlers_transient.go`.
- `apps/backend/internal/orchestrator/event_handlers_transient_messages.go`.
- `apps/backend/internal/orchestrator/dynamic_evidence.go`, only if bounded
  presentation evidence belongs in the existing prompt-attempt record.
- `apps/web/components/task/chat/messages/action-message-details.tsx`.
- `apps/web/components/task/chat/messages/interruption-recovery-feedback.ts` and
  its existing `.test.ts` companion.
- `apps/web/components/task/chat/messages/transient-retry-notice.tsx`.
- `apps/web/components/task/chat/messages/action-message.test.tsx`.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/chat.json`.

## Dependencies

None. Read the full requirement and the design's “Confirmed continuation activity”
section before implementation. Keep the status `pending` until implementation starts.

## Risks

- The stream handler already holds a cancellation guard. Preserve existing lock
  ordering and release evidence locks before notice publication.
- A tool update can belong to inherited pending work. Positive current-prompt
  attribution is required before it can change presentation.
- A failed message update must not set the publication-complete latch.
- Do not expand the shared prompt-evidence record beyond bounded presentation needs.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/provider-interruption-continuation.md).
- [System design](../../specs/platform/system-design/provider-interruption-continuation.md).
- Existing `dispatchContinuationFixture`, `continuationFailureFixture`,
  `TestInterruptionContinuationPhaseAccepted`, and budget/cancellation tests.
- The existing status-message writer, notice mutex, action-message tests, and
  `TransientRetryNotice` mobile composition.

## Results

Implemented the persisted activity transition and the neutral renderer.

- RED: the new backend regressions expected `recovery_activity_confirmed: true`
  after accepted-prompt output, reasoning, or a foreground tool call, and
  received no marker. The frontend helper suite failed on the missing predicate,
  and the component suite rendered the warning heading instead of `Agent resumed`.
- GREEN: `(cd apps/backend && go test -trimpath ./internal/orchestrator -run 'TestInterruptionContinuationActivity' -count=1)`
  passed.
- `(cd apps/backend && go test -trimpath -race ./internal/orchestrator -run 'TestInterruptionContinuationActivity' -count=1)`
  passed.
- `(cd apps/backend && go test -trimpath ./internal/orchestrator -run 'TestInterruptionContinuation|TestTransientRetry|TestHandleTransientFailure|TestPromptAttemptEvidence|TestRetained.*Capacity|TestCapacity' -count=1)`
  passed.
- `bash scripts/lint-go-changed <six changed Go files>` reported `0 issues`.
- `(cd apps/web && pnpm exec vitest run components/task/chat/messages/interruption-recovery-feedback.test.ts components/task/chat/messages/action-message.test.tsx)`
  passed 75 tests. The full `components/task/chat/messages` suite passed 657
  tests across 61 files.
- `(cd apps/backend && go test -trimpath ./internal/orchestrator -count=1)`
  passed the full orchestrator package.
- `(cd apps/web && pnpm exec eslint ...)` reported no problems for the changed
  feedback, details, notice, and test files.
- `(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)`
  passed. The default Node heap exhausted before the compiler finished.
- `pnpm run i18n:zh-hant`, `pnpm run i18n:pseudo`, `pnpm run i18n:check`, and
  `pnpm run i18n:ratchet` passed.
- Review fix: neutral publication now copies the episode policy's maximum and
  the capacity window deadline. `TestInterruptionContinuationActivityPreservesCapacityMetadata`
  covers attempt 6 with 5 started attempts, a late continuing-phase rewrite, and
  a later interruption; all persist `max_attempts: 11` and the same window
  instead of the transport policy's `5`.
- Review fix: tool-call confirmation now runs after `handleToolCallEvent`
  records the finalized ownership and requires a foreground result. A terminal
  call with no live ownership no longer counts as resumed activity.
- `git diff --check` passed.
