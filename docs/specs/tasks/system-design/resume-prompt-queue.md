---
status: draft
system: tasks
requirements:
  - REQ-TASKS-RESUME-PROMPT-QUEUE-001
---

# Resume Prompt Queue System Design

## Boundary and requirement mapping

The task system owns deferred prompt admission and dispatch. This design uses
the current queue storage, session incarnation, and agent readiness events.

| Criteria | Design sections |
| --- | --- |
| `001.1`, `001.2`, `001.6`, `001.9` | Composer admission, Responsive behavior |
| `001.3`, `001.4`, `001.5` | Server dispatch |
| `001.7`, `001.8` | Failure and identity |

All criterion suffixes refer to `AC-TASKS-RESUME-PROMPT-QUEUE-001`.

## Current behavior

`deriveSessionInputMode` already maps `STARTING` to `queue`.
`useMessageHandler` uses that mode to select `message.queue.add`.
`useChatInputContainer` independently disables regular submission during startup.

`useSessionState.isStarting` also includes environment preparation. That broad
presentation flag does not prove queue eligibility for the selected session.

`QueueHandlers.wsQueueMessage` persists an entry and publishes queue status.
`handleAgentBootReady` settles the session and attempts automatic dispatch.
The queue handler currently has no automatic dispatch check after admission.
Thus, readiness before insertion can leave the prompt without a later trigger.

`useQueueAdmissionAction` currently returns without error when identity or an
operation token is missing. A caller can interpret that return as success.
Startup submission must not clear a draft through this path.

## Composer admission

Use the selected session's input mode and complete queue identity to derive
startup queue eligibility. Thread that eligibility through the shared composer.
Do not infer eligibility from `isPreparingEnvironment` or `isAgentBusy` alone.

The submission gate permits startup only when queue admission is available.
Other gates retain their current precedence. Interactive clarification keeps
its existing exception. Button, keyboard, and plugin composer capability use
the same submission gate.

`useMessageHandler` re-reads selected-session state at submission. A session
that becomes ready before this read uses the existing direct path. A session
that remains in startup uses the existing queue payload and identity.

Preserve model, plan mode, attachments, entity references, and context metadata
through the existing payload builder. Clear the draft only after admission
succeeds. Missing identity and conflicting operation-token acquisition must
reject or return an explicit unsuccessful result that the composer preserves.
Use the established `MessageSendError` and localized error presentation.

Reuse the current queue tooltip and chip for accepted work. Keep startup status
visible. New copy, if necessary, uses locale catalogs and existing i18n checks.

## Server dispatch

After successful `wsQueueMessage` admission, request an automatic dispatch
check for the admitted task, session, and incarnation. Add a narrow internal
queue-handler collaborator backed by the orchestrator for this purpose.
This does not add a public WebSocket action.

Reuse existing guarded automatic reservation and dispatch. Revalidate identity,
session readiness, clarification ownership, cancellation, reset, active dispatch,
steering, and task admission before reservation. Retain the atomic Auto-run check.
The existing identity-aware drain and task-admission helpers supply these parts.
Keep identity validation and reservation within the existing guard boundary.

Do not call the manual `DrainQueuedMessage` operation. That operation explicitly
enables Auto-run and would change a user's paused queue policy.

The existing boot-ready handler remains the trigger when admission wins first.
The admission check covers readiness winning first. Concurrent checks use the
existing reservation and in-flight guards to dispatch one eligible head.
Later turn-ready events continue ordinary FIFO processing.

The admission response reports persistence success. A deferred dispatch or
dispatch-check failure does not report that a persisted prompt was rejected.
Use existing structured logs for dispatch errors and existing queue status events
for accepted, reserved, restored, and removed entries.

## Failure and identity

The queue remains server-owned after navigation, reload, or a resume failure.
Failed resume shows existing recovery feedback and leaves pending entries intact.
Successful recovery resumes eligible dispatch. No new recovery loop is introduced.

Admission uses `task_id`, `session_id`, and `session_incarnation_id` from the
captured queue identity. Existing authorization and attachment-claim validation
remain mandatory. Stale identities cannot target replacement sessions.

Queue-full, unavailable identity, and rejected submissions preserve the draft.
Existing queue reconciliation handles transport errors. This change does not
introduce blind retries or promise provider-level exactly-once execution.
Concurrent readiness and admission must not create duplicate dispatches.

## Responsive behavior

The entry point remains the task or Quick Chat composer. The nearest shipped
examples are `chat-input-toolbar-mobile.tsx` and `mobile-message-queue-management.spec.ts`.
`task-layout.tsx` supplies the dedicated phone composition.

The hierarchy remains startup status, pending queue, composer, then Send.
The inline queue fits this frequent, short interaction. No additional drawer
or navigation step is necessary. The queue keeps its internal scroll owner.
The existing task layout owns dynamic viewport and safe-area behavior.

Desktop and mobile share eligibility, mutation, and queue state. Mobile Send
retains its 44-pixel target. Fine-pointer controls retain compact sizing.
Keyboard submission and the accessible queue description remain available.
Mobile E2E uses a real tap and verifies dispatch after resume and no page overflow.

## Persistence and decisions

No schema, migration, setting, or event format changes are planned.
This design applies [server-owned Auto-run](../../../decisions/2026-08-16-server-owned-queue-auto-run.md)
and the current session-identity contract. No new architecture decision is required.

## Verification strategy

Backend tests control readiness and insertion ordering with barriers. They cover
both orders, concurrent notifications, Auto-run OFF, failed resume, and identity
rejection. Handler coverage proves the actual WebSocket admission calls the check.

Frontend tests cover startup gating, current-state routing, payload retention,
and unsuccessful admission. Desktop and mobile E2E hold an actual resume before
readiness, submit, observe the pending prompt, then verify one resulting turn.
An additional case reloads after admission. Initial-start coverage also changes
where the selected session already has queue capability.
