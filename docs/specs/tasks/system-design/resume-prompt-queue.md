---
status: current
system: tasks
updated: 2026-10-04
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
| `001.10`, `001.11` | Workflow transitions during resume |

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

## Workflow transitions during resume

The task system owns this interaction because workflow transitions and prompt
admission share the task session state. This section also applies to a direct
`message.add` request that races with automatic resume after an idle suspension.
The browser can choose direct submission before it receives the startup event.

`MessageHandlers.wsAddMessage` evaluates `ProcessOnTurnStart` before prompt
composition and dispatch. The resulting workflow step and recipient remain
authoritative. Moving this evaluation after dispatch would select the wrong
workflow instructions and completion-signal policy.

`persistResumeStateWithOptions` claims `STARTING` before credential issuance.
`ResumeSessionWithOptions` then persists its credential snapshot against that
state and startup-attempt identity. These guards remain mandatory. The workflow
transition must not change startup state merely to make the session promptable.

### State preparation

Use one workflow-specific state-preparation helper after recipient selection.
Read the selected recipient's authoritative row instead of a pre-transition
snapshot. Validate its task ownership before any state change.

| Authoritative recipient state | Turn-start preparation |
| --- | --- |
| `STARTING` | Preserve the startup claim and its attempt metadata |
| `RUNNING` | Preserve the admitted turn and its runtime projection |
| `WAITING_FOR_INPUT` | Keep the existing ready state |
| `CREATED` or `IDLE` | Retain existing preparation through a conditional waiting-state transition |
| `FAILED`, `CANCELLED`, or `COMPLETED` | Preserve the terminal outcome and error |
| Missing, foreign, unreadable, or unknown state | Report preparation failure without a state write |

Use `transitionTaskSessionState` and its strict conditional persistence for an
eligible state change. Bind the write to the observed state. If the write loses
to startup, an admitted turn, or terminal settlement, preserve the winning row.
Do not retry by writing `WAITING_FOR_INPUT` over the new state.

Apply this rule to engine-backed `transitionLifecycleOnTurnStart`, the legacy
`executeStepTransition` turn-start branch, and the engine's WIP deferral branch
when its mode is turn-start. Keep actual turn-completion settlement unchanged.
The helper must not invoke the general waiting-state path for a working session.
That path also reconciles the task to Review and releases startup capacity.

Recipient selection still uses `maybySwitchSessionForProfile`. An unchanged
recipient retains its startup claim. A changed recipient uses its own current
state and existing routing, transfer, and retirement rules. No source snapshot
can authorize a waiting-state write to a different destination.

### Delivery and failure

After turn-start processing, ordinary prompt admission determines whether input
runs immediately or waits. Reuse the existing startup queue and
`MetaKeyTurnStartAlreadyProcessed` where direct-message fallback already
processed the transition. Queue delivery must not evaluate that trigger again.

Successful boot readiness remains the authority that ends startup. Preserve
Auto-run OFF and existing clarification, cancellation, reset, WIP, and identity
barriers. A genuine startup failure uses existing recovery feedback and prompt
retention. Do not catch the resume persistence error and treat it as success.

No database migration, wire change, new lock hierarchy, timer, runtime flag,
or provider-specific path is required. Existing guarded startup and queue
ownership contracts supply the boundary, so this correction needs no new ADR.
Existing correlated state and launch logs provide diagnostic evidence without
new prompt logging or metrics.

### Regression evidence

Backend tests place a barrier between the resume's early `STARTING` claim and
credential-snapshot persistence. Execute the real turn-start transition while
that barrier holds, then release persistence. Assert successful resume, retained
conversation identity, one workflow transition, and one eventual prompt dispatch.

Also cover a stale waiting snapshot followed by a concurrent startup claim.
Cover a lost conditional write, terminal settlement, a genuine launch failure,
legacy execution, WIP deferral, and an already-admitted `RUNNING` turn.

Desktop E2E uses the real `message.add` action during a delayed resume to model
the browser's stale direct-submission decision. Mobile E2E submits through the
existing startup composer and proves queue delivery with a turn-start transition.
A mock-agent resume delay occurs after credential-snapshot persistence. It
proves lifecycle preservation during startup, but cannot replace the backend
test of the credential boundary.

See the [resume transition repair plan](../../../plans/session-resume-turn-start-race/plan.md)
for exact tests and the incident trace. The earlier
[resume queue package](../../../plans/resume-prompt-queue/plan.md) retains its
completed delivery record.

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
