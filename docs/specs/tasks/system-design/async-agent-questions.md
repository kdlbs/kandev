---
status: current
system: tasks
requirements:
  - REQ-TASKS-ASYNC-QUESTIONS-001
  - REQ-TASKS-ASYNC-QUESTIONS-002
  - REQ-TASKS-ASYNC-QUESTIONS-003
  - REQ-TASKS-ASYNC-QUESTIONS-004
  - REQ-TASKS-ASYNC-QUESTIONS-005
  - REQ-TASKS-ASYNC-QUESTIONS-006
  - REQ-TASKS-ASYNC-QUESTIONS-007
  - REQ-TASKS-ASYNC-QUESTIONS-008
  - REQ-TASKS-ASYNC-QUESTIONS-009
  - REQ-TASKS-ASYNC-QUESTIONS-010
---

# Async agent questions system design

## Purpose and boundaries

Tasks owns the async request record and its lifecycle. Inbox and transcript components consume its projections.
This design adds an explicit mode. It does not broaden current-turn clarification authority.
The [decision record](../../../decisions/2026-10-30-async-question-lifecycle.md) explains this boundary.

The existing [clarification design](clarification-active-lifecycle.md) remains authoritative for blocking calls.
Sections marked **Remediation** describe the design that work orders 06 to 12 implement.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-ASYNC-QUESTIONS-001` | MCP surface and request creation |
| `REQ-TASKS-ASYNC-QUESTIONS-002` | Durable state and resolution |
| `REQ-TASKS-ASYNC-QUESTIONS-003` | Answer reads and queued delivery |
| `REQ-TASKS-ASYNC-QUESTIONS-004` | Withdrawal and terminal cleanup |
| `REQ-TASKS-ASYNC-QUESTIONS-005` | Parent answers and forwarding |
| `REQ-TASKS-ASYNC-QUESTIONS-006` | Explicit wait |
| `REQ-TASKS-ASYNC-QUESTIONS-007` | Inbox and transcript projection |
| `REQ-TASKS-ASYNC-QUESTIONS-008` | Compatibility and capability selection |
| `REQ-TASKS-ASYNC-QUESTIONS-009` | Task chat question surface |
| `REQ-TASKS-ASYNC-QUESTIONS-010` | Agent mode guidance |

## Existing integration points

- `internal/mcp/server/server.go` registers the two question tools and the blocking output schema.
  `internal/mcp/server/handlers.go` builds their relay payloads and extracts blocking answers.
- `internal/mcp/handlers/handlers_ask.go` creates a blocking waiter and changes session state before waiting.
  `parent_question.go` persists a child message and pauses before `dispatchTaskMessage`.
- `internal/task/repository/sqlite/clarification_bundle_query.go` filters current-turn clarification messages and excludes parent questions.
  These queries must retain their predicates.
- `internal/orchestrator/messagequeue/service.go` exposes `QueueMessageWithMetadataForSessionWithClientQueueID`.
  Its durable admission receipt survives queue-row dispatch. `LookupQueueAdmissionWithClientQueueID` supports retry reconciliation.
- `internal/backendapp/helpers.go` constructs clarification services and MCP routes.
  This composition root supplies the async service and its queue adapter.
- The Inbox uses `internal/clarification/inbox_handlers.go`, its boot summary, and the shared frontend refresh controller.
  Async population must reach all list, count, boot, and refresh producers.

## Components and dependency direction

Add **new** `internal/task/questions/` for async application behavior and narrow ports.
Add **new** question models under `internal/task/models/` and repository methods under `internal/task/repository/sqlite/`.
Task questions must not import the orchestrator or messagequeue packages.
The backendapp adapter translates neutral delivery intents into existing exact-session queue admission.

The service owns create, list, resolve, withdraw, forward, wait registration, and outbox reconciliation.
MCP and HTTP handlers call the same service. All public entry points authorize stored task/session bindings.
Task-actor authority comes from the trusted relay execution, never a caller-supplied sender task ID.
User ownership of two tasks does not let one impersonate the other as a question author or parent.
The implementation must register new task-owned tables with existing persistence conformance coverage.

## Durable state and resolution

Use additive tables. Do not convert existing blocking messages or infer async mode from missing metadata.

| New table | Essential fields and constraint |
| --- | --- |
| `task_async_questions` | `id`, task/session/incarnation IDs, source turn, source workflow context, recipient kind, recorded parent ID, normalized question JSON, context, retry key, payload digest, status, outcome JSON, timestamps, version |
| `task_async_question_forwards` | One row per question, current forwarding parent and session, reason, timestamp. The question ID is unique. |
| `task_async_question_deliveries` | Question ID, outcome version, delivery kind, destination task/session/incarnation, immutable payload, admission ID, state, retry time. Unique destination/kind/version. |
| `task_async_question_waits` | Session/incarnation, captured turn/generation, dependency IDs, gate ID, phase. At most one active gate per session. |

The request ID identifies the entire bundle. Item IDs remain local to that bundle.
Question and outcome JSON reuse the existing neutral `clarification/protocol` value types.
Structured operator answers retain option IDs, labels, and free text. Parent replies can retain a bundle-level free-text answer.
Provenance records whether the responder was an operator or the current direct parent.

Canonical states are `pending`, `answered`, `rejected`, `withdrawn`, and `stale`.
Only `pending` can transition to another state. No terminal state can reopen.
Within a transaction, a conditional version/status update chooses the winner and inserts its delivery intents.
No provider call, event publication, or queue call runs inside that transaction.

Resolution success means the outcome and its delivery intent are durable.
It does not claim that the provider received a new prompt already.
Return the winning canonical snapshot on an exact retry or competing outcome.
Reuse the existing question/option validator. Reject malformed and incomplete answers before the conditional update.

Capture direct-parent identity under the same hierarchy admission used for task writes.
PostgreSQL operations lock the applicable task/hierarchy rows and question identity before reading mutable ownership.
Use the established advisory-lock pattern for per-request read-modify-write invariants.
SQLite uses its reserved writer transaction. Include real PostgreSQL concurrency tests.

Add idempotent table creation through the task schema owner and migrations where existing tables gain fields.
Test empty databases, existing databases, and same-database replay on both supported dialects.
No historical blocking data backfill is needed.

## MCP surface and request creation

Both existing ask tools gain optional boolean `async`, default `false`.
For async creation, accept optional `request_id` as a stable retry identity, capped at 128 characters.
Reject wrong types before persistence. `request_id` is valid only with async mode.

An explicit request ID is scoped to the trusted author session and incarnation.
Otherwise reuse the server's exact transport retry identity.
If no retry identity exists, mint a fresh identity.
Include mode, recipient, normalized questions, and context in the payload digest.
An exact retry reuses the request. A different digest with the same identity returns conflict.
Identical wording in a genuinely separate call creates a separate request.

Async creation returns:

```json
{
  "question_id": "opaque-bundle-id",
  "mode": "async",
  "status": "pending",
  "recipient": "user"
}
```

Parent results also include `parent_task_id` and use recipient `parent`.
The server output schema becomes a discriminated union of the existing blocking answer result and this receipt.
Async results bypass `extractQuestionAnswers`, waiter registration, keepalive pings, and timeout-to-pause fallback.
The backend persists the request and notification intents before returning the receipt.

Add these **new** task-scoped tools:

| Tool | Parameters and result |
| --- | --- |
| `list_question_answers_kandev` | `scope=own\|children` (default own), `status=pending\|answered\|rejected\|withdrawn\|stale\|all` (default all), optional `question_ids` (at most 50), limit/cursor. Returns canonical requests and outcomes. |
| `withdraw_question_kandev` | `question_id`, required non-empty reason. Author-session only. Returns canonical terminal state. |
| `forward_question_kandev` | `question_id`, required reason. Direct-parent and user-question capability required. Returns the same question ID. |
| `wait_for_question_answers_kandev` | Non-empty `question_ids` (at most 50), short dependency reason. Returns ready snapshots or establishes the explicit pause. |

Use default page limit 50 and cap 200. Invalid filters, limits, and cursors fail explicitly.
Lists sort by creation time then ID. A cursor pages this order, not a stream of changes.
Checkpoint reads start without a cursor or request known IDs. Reads never mark answers consumed.
Repeated reads and a later continuation can therefore expose the same answer ID without creating another outcome.

## Answer reads and queued delivery

Each outbox intent has a stable admission ID derived from its question, outcome version, kind, and destination.
The backendapp adapter uses `QueueMessageWithMetadataForSessionWithClientQueueID`, with the recorded exact session incarnation.
Add an explicit question-message metadata marker to prevent auto-merge, editing, coalescing, and Send now steering.
No async answer calls `PromptTask` or the interrupt branch of `dispatchTaskMessage` directly.

Queue content includes the request ID, source task/session/turn, original workflow context, questions, and outcome provenance.
It describes a correlated decision. It does not contain a step-completion instruction.
The normal queue waits for the current turn boundary before dispatch.
An existing blocking clarification still owns its live waiter and holds unrelated queued input.

Outbox processing uses these boundaries:

1. Read a bounded page of due durable intents.
2. Revalidate authorization, relationship, and the exact input-capable destination.
3. Admit with the stored ID and immutable payload. Reconcile an uncertain response using the queue admission receipt.
4. Mark the intent admitted only after that receipt exists.
5. Publish workspace-scoped changes after commit. Queue dispatch retains its existing ambiguity policy.

A crash after queue admission and before step 4 repeats the same admission ID, producing no second queue row.
A full queue or unavailable runtime leaves the intent pending. The answer remains visible to checkpoint reads.
Question notifications are not editable user drafts. A skipped or cancelled delivery remains observable in its durable intent.
Do not recreate it with a new admission ID.

The async service owns one cancellable reconciliation loop, with startup drain and event-driven wakeups.
Bound retry backoff and page sizes. `Stop` cancels and joins the worker.
No in-memory waiter owns async durability, and event-bus publication is not a delivery acknowledgement.

## Parent answers and forwarding

Async parent creation uses the existing Autopilot/direct-parent eligibility checks.
It records a parent-request outbox intent without `setSessionWaitingForInput` or `PauseForClarificationInput`.
The direct parent can discover it through `scope=children` even while its prompt notification remains queued.

Extend `message_task_kandev(reply_to_question_id=...)` to dispatch by canonical request kind.
For an async request, resolve through the question service and return its receipt without dispatching the reply directly.
The existing blocking parent-question path remains unchanged.
Reject an explicit target session that differs from the recorded author session.
Validate the sender against the current and recorded direct parent, plus existing workspace permissions.

Forwarding creates a link to the existing async request, not a copied blocking clarification.
Only a direct parent that currently has the user-question capability can expose that link to the operator.
An Autopilot intermediary retains its existing parent-only capability and cannot use this operation for direct operator access.
Multi-hop correlated forwarding is outside this package.

An operator response through a forward resolves the same canonical question row.
The transaction creates one owner-outcome intent and one parent-outcome intent.
A concurrent direct-parent answer uses the same conditional update and observes the winning outcome.
The parent question list reflects the shared canonical state, without a second manual relay.

### Parent session routing (**Remediation**)

The parent task is the authority. `ParentSessionID` records which session the question first targeted and does not authorize.
`ResolveParent` and `Forward` check the task relationship, so a replacement session of the same parent can answer.
Parent-kind deliveries store the destination task and choose a session when admitted.
The adapter uses `GetActiveTaskSessionByTaskID` and that session's current incarnation.
With no active session, the intent stays pending on the existing bounded backoff.
A parent task that is archived or missing makes the request stale through the authority sweep.
Author-kind deliveries keep their exact session and incarnation, and they are never redirected.

## Explicit wait

The wait tool declares that all remaining work depends on its named request set.
It does not require every open request to belong to that set.
The server verifies ownership, session incarnation, and the current turn/generation.
If any named dependency already has a terminal outcome, return `status=ready` with those snapshots.

Otherwise persist a `requested` gate under question/session serialization.
Pause only the captured generation using the orchestrator's guarded pause mechanism.
After cancellation settles, mark the matching gate `parked`.
Answer admission for a requested gate records a release but cannot prompt until the pause finishes.
This ordering prevents an answer from starting a turn that the old pause then cancels.

Any terminal dependency outcome changes the gate to released.
The existing FIFO resumes through the correlated outcome notification after the pause boundary.
If a withdrawal releases a parked gate, create a wake intent.
Ordinary withdrawal needs no self-notification.
Other request states remain independent. Never treat all open requests as an implicit barrier.

Workflow completion and queue draining consult this explicit gate, not the presence of async questions.
For a parked gate, suppress `on_turn_complete`, withhold `step_complete_kandev`, and hold unrelated queue entries.
Once released, ordinary FIFO resumes. Do not reorder unrelated entries to manufacture priority.
A newer accepted operator prompt cancels the old gate while leaving still-valid questions open.
A stale pause completion or predecessor ready event cannot mutate the newer turn.

On restart, reconcile active gates before prompt admission.
A gate on a terminal destination is cancelled, and a `release_requested` gate is released with a readiness wake.
A `requested` gate is cancelled because the pause that owned it did not survive. The session keeps its open questions, and the agent can declare a new wait.
A `parked` gate stays parked, because its release is atomic with the terminal outcome.
Never infer a pause from pending questions alone.

### Gate settlement (**Remediation**)

`Service.Wait` must not return `ready` while its gate is active.
When the re-check finds a terminal dependency, it releases from either `requested` or `release_requested`. No pause exists for that call, so nothing else would acknowledge the fence.
Staleness uses the same release path as other terminal outcomes.
In one transaction it closes the request, releases dependent gates, and inserts a wake intent for a live author.
The wake states that the dependency closed as stale, so the agent can choose its next step.

## Withdrawal and terminal cleanup

Only the original author session can withdraw its request.
Persist the reason and close the canonical request using the same winner transaction as resolution.
Abandon unsent parent-request intents in the closing transaction, and publish terminal state for parent and operator consumers.
After commit, remove a parent-request queue row that has not been dispatched, using its stored admission identity.
A row that has already been dispatched remains history, and it cannot make a withdrawn request answerable.

Task/session cancellation, terminalization, archive/delete, reparent, and workspace transfer must revalidate affected requests.
Use existing lifecycle integration points plus guarded reads and startup reconciliation.
An archive/delete race cannot enqueue into another session or bypass current ownership.
Task deletion removes question rows through the task service. Workspace cascade deletion removes them in its transaction, because these tables have no foreign keys.
History follows the owning task's retention lifecycle. No independent retention timer is introduced.

### Authority sweep (**Remediation**)

Several paths can change a task's parent or workspace, so one sweep covers them instead of a hook on each path.
The reconciliation worker runs a bounded query for pending requests on each tick.
It selects parent requests whose child no longer names the recorded parent, or whose parent task is archived or missing, plus any request whose task left its recorded workspace.
Each match uses the shared stale transition described in Gate settlement.
Every stale transition, including those from answer attempts and session cleanup, publishes a change event after commit.

## HTTP and event contracts

Add task-authorized **new** routes:

- `GET /api/v1/agent-questions`: bounded request listing, with explicit task scope for conversation clients.
  Task scope returns the task's operator requests across its sessions, its forwarded child requests, and requests it forwarded as parent.
- `GET /api/v1/agent-questions/:id`: one authorized request for a transcript entry outside the loaded page.
- `POST /api/v1/agent-questions/:id/respond`: structured answers or rejection, plus a stable submission ID.
  The winning outcome stores that ID, so an exact retry reports `won=true` and creates no delivery.
- `GET /api/v1/agent-questions/:id/images/:imageID?question_id=...`: saved operator-question images only.

HTTP reads and responses expose parent requests only through an authorized forward.
Task-bound parent discovery uses the separate MCP caller context.
Use 400 for malformed requests, non-disclosing 404 for inaccessible records, and the existing scope contract for 403.
Responses include canonical state and whether this caller won. A concurrent terminal winner is an observable outcome, not a retryable submission failure.
An ambiguous HTTP response keeps its stable submission ID for retry.

Image lookup authorizes the canonical async request and its stored source session.
Reuse `clarification.QuestionImageReader`, saved-reference validation, size/MIME limits, and private no-store headers through a composition adapter.
Do not pass an async ID to the blocking resolver or accept caller-supplied filesystem paths.
Parent questions retain their existing image rejection.

Extend `GET /api/v1/clarification-inbox` additively with a typed async item population and a combined count.
Old `bundles` remain blocking records. The typed population identifies `blocking` versus `async` and carries the correct response endpoint.
The same authorized bounded union supplies rows, counts, boot state, hidden enumeration, and snooze expiry.
Apply filters before the shared page bound. Sort requests by creation time then stable kind/ID, then group the returned page by task.
Do not page each source independently and concatenate two full pages.

Reuse per-user Inbox sidecar semantics through typed identities.
Existing blocking keys remain byte-compatible. Async keys use a namespaced identity and cannot collide with a blocking pending ID.
History queries add async terminal records with `withdrawn` as a distinct outcome.

Backend `task.question.*` subjects reach clients as one workspace-scoped `task.question.changed` action.
The payload carries workspace, task, session, parent task, and request identity, without question or answer text.
Clients invalidate the bounded read, so the event needs no snapshot version. A foreground refresh recovers from missed events.

## Inbox and transcript projection

Extend the existing Needs-you controller and slice, rather than adding a second global poller.
List and count updates apply together under workspace/read-generation guards.
Boot count, the unopened-Inbox sidebar badge, hidden count, and snooze wakeup use the same typed response.
When several requests share a task group, the count remains a request count.

An async operator request or forwarded request uses `AsyncQuestionForm`, on every surface.
Do not manufacture blocking clarification messages to activate the blocking lifecycle hook.
Keep blocking hosts and Quick Chat behavior intact.

A row's waiting state comes from the session's active gate and its named dependencies, loaded once per session.
Terminal views resolve option IDs to labels from the stored questions, and each terminal status has its own label.

### Task chat question surface (**Remediation**)

The chat surface reads through a task-scoped store slice and domain hook. Neither consults the Inbox feature flag.
`task.question.changed` invalidates the slice when its task matches the event's task or parent task.
A strip sits in `ComposerFooterAllocation`, above the composer and outside the transcript scroller, and it renders nothing with no open requests.
It expands to the shared answer form in the footer region without covering the composer. A phone opens the existing full-height answer surface.
Blocking clarifications keep precedence: while one is active, the strip collapses to its count.
A forwarded request appears in the author's chat and the forwarding parent's chat under the same ID. The parent's row names the child task.

A durable `transcript` outbox intent creates an `agent_question_request` message in the author's session, with `requests_input=false`.
The message ID derives from the question ID, so a retry cannot duplicate it.
The message stores only the question ID. Its card reads current state from the slice, or from the single-request route, and repeated reads of one ID are deduplicated.
This message kind stays outside clarification bundle queries and the blocking resume path.

### Operator attention (**Remediation**)

Creating a `user` request, or forwarding a child request, raises `HandleClarificationRequested` with a question-scoped occurrence ID.
The notification service's occurrence handling keeps a retry from notifying twice. An unforwarded parent request does not notify the operator.

`TaskPendingAction` means a session is blocked on input, so a running session's open question does not use it.
`pendingActionsBySessionQuery` returns `clarification` for a session with a `parked` or `release_requested` gate.
Gate transitions publish `session.pending_action_changed` through the existing revisioned projection.
A separate indicator for open questions on running tasks is out of scope. The notification, chat strip, and enabled Inbox cover discovery.

Desktop uses task-group headings and inline expandable answer rows in the existing Inbox page.
Phone uses the existing Inbox navigation entry and a full-height answer surface for one request.
`NeedsYouInboxPageClient` supplies page structure. `SessionTaskSwitcherSheet` supplies fixed-header, internal-scroll, and focus-return precedents.
The phone surface has Back, task/parent context, one scrolling question body, and a safe-area-aware submit region.
Dense question content justifies the full-height surface. Short overflow choices keep the shared phone menu treatment.

Use `useResponsiveBreakpoint`, with phone composition below 768px.
Keep ordinary desktop controls at 28px. Phone and coarse-pointer action targets measure at least 44px.
Use dynamic viewport height, one internal scroll owner, keyboard clearance, semantic controls, and focus restoration.
Shared hooks retain drafts and outcomes across width changes. All new copy uses the existing locale catalogs.
The [plan previews](../../../plans/async-agent-questions/plan.md#ascii-ui-preview) define the structural views.

## Compatibility and capability selection

The existing profile selects either user-question, parent-question, or neither.
An `async-questions` capability marks persistent, non-passthrough Kanban task sessions; ephemeral Quick Chat, Office, configuration, and external profiles do not receive it.
This capability can exist without ask authority, such as on an Autopilot root that only reads child requests.
Add withdrawal/wait tools only for supported persistent task sessions with an applicable question capability.
Supported task-mode parents, including Autopilot roots, can use the bounded answer-list tool for direct-child discovery.
This read tool grants no question creation or forwarding authority.
Expose forwarding only where the session has user-question authority. Revalidate all capabilities in backend handlers.
Do not widen configuration, Office, external, quick-chat, or no-question surfaces.

The caller must choose `async: true`. Omitted/false calls keep the existing implementation path.
The existing blocking user-input barrier and parent last-action instruction remain explicit for omitted/false calls.

### Agent mode guidance (**Remediation**)

The agent chooses the mode, because the backend cannot judge whether other useful work remains.
`KandevContextOptions` gains an async-capability input that follows `asyncQuestionsSupported`.
Only sessions with an ask tool and the async capability receive mode-selection guidance.
That guidance covers checkpoints, explicit waits, withdrawal, and stable retry IDs. Other sessions keep blocking-only wording or no question guidance.
Tool descriptions state blocking as the default and carry a short form of the rule only on surfaces that register `async`.
Tests assert the required guidance per capability. They do not claim that a model follows it.

| Caller and transport | Identity or capability | Supported async behavior | Evidence and fallback |
| --- | --- | --- | --- |
| Persistent normal task through MCP relay | Trusted task/session, user-question | Ask operator, read, withdraw, wait, forward direct-child requests | Server/handler/orchestrator tests. Omitted/false uses blocking path. |
| Autopilot child through MCP relay | Trusted task/session, parent-question | Ask direct parent, read, withdraw, wait, correlated parent reply | Parent and ownership races. No direct operator tool or forward. |
| Autopilot root | No question creation capability | Direct-child reads only. No ask, withdraw, wait, or forward. | Profile tests and backend rejection of creation. |
| Other no-question profile | No applicable capability | No async creation | Profile tests and backend rejection. |
| Operator browser through HTTP | Existing workspace visibility and action scope | List, answer, reject operator or forwarded requests from chat, or from the Inbox when enabled | HTTP auth tests and desktop/mobile E2E. Unforwarded parent records remain inaccessible. |
| External MCP, Office, config, Quick Chat | Outside this task-mode contract | No async creation or new async tools | Profile exclusion tests. Existing blocking behavior remains available where already supported. |

This is provider-neutral MCP behavior. Do not claim provider-specific mid-turn push support.
Existing transport discovery and timeout contracts still apply.
The existing Inbox flag controls only the Inbox. Async tools and the chat surface do not consult it.
This package changes no flag registry or profile default.

## Observability and validation

Structured logs record request ID, captured generation, terminal transition, gate phase, and queue admission ID.
Log no question or answer text. Existing queue diagnostics expose delayed admission.
Any counters use closed operation/outcome labels. IDs never become metric labels.

Repository tests cover outcome races, request retries, migration replay, cleanup, and outbox crash windows on both dialects.
Orchestrator tests cover no mid-turn cancellation, blocking coexistence, pause/answer races, and restart reconciliation.
Browser tests cover mixed blocking/async populations, grouping, count convergence, drafts, withdrawal history, and forwarded answers.
Phone tests also assert containment, scroll ownership, focus return, and measured touch targets.
Exact tests and commands belong to the [work orders](../../../plans/async-agent-questions/plan.md#work-orders).

## Related decisions and records

- [Async question lifecycle](../../../decisions/2026-10-30-async-question-lifecycle.md)
- [Current-turn blocking clarification ownership](../../../decisions/2026-08-14-current-turn-clarification-ownership.md)
- [Late blocking answers as ordinary messages](../../../decisions/2026-09-18-late-clarification-messages.md)
- [Exclusive question capability](../../../decisions/2026-08-08-autopilot-mcp-question-capabilities.md)
- [Requirements](../requirements/async-agent-questions.md)

## Implementation Plans

- [Async agent questions](../../../plans/async-agent-questions/plan.md)
