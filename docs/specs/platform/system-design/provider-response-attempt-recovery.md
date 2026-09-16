---
status: current
system: platform
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
created: 2026-09-09
updated: 2026-09-16
owners:
  - Kandev
---

# Provider Response-Attempt Recovery System Design

## Purpose and boundaries

The platform system owns the recovery boundary between provider-specific ACP
retry evidence, lifecycle streaming identity, durable task messages, and live
session delivery. This design covers a provider retry that remains inside one
active prompt. Terminal provider errors and Kandev-owned prompt replay remain
in [Provider Error Recovery](provider-error-recovery.md).

The current production trigger is Codex's structured response-stream
disconnect metadata. The downstream reset contract is provider neutral.

## Requirement mapping

| Acceptance criterion | Design section |
| --- | --- |
| `AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.26` | [Adapter evidence projection](#adapter-evidence-projection) |
| `AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.27` | [Lifecycle record ownership](#lifecycle-record-ownership), [Durable and live cleanup](#durable-and-live-cleanup) |
| `AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.28` | [Commit boundaries and replay safety](#commit-boundaries-and-replay-safety), [Failure behavior](#failure-behavior) |

## Adapter evidence projection

`internal/agentctl/server/adapter/transport/acp` translates implementation
metadata before it crosses the agentctl stream boundary. `acpDialect` exposes
an optional pure response-attempt reset recognizer. The Codex dialect accepts a
`session_info_update` only when all of these fields are present with the stated
types:

- `codex.error.willRetry` is the boolean `true`;
- `codex.error.codexErrorInfo.responseStreamDisconnected` is an object; and
- the notification belongs to the active, non-zero prompt generation.

The recognizer does not inspect `additionalDetails` or another error string.
`willRetry: false`, a missing nested disconnect object, malformed maps, an
unsupported dialect, and zero or stale prompt generations remain ordinary
session information.

The FIFO ACP notification worker emits
`streams.EventTypeResponseAttemptReset` before it can deliver later replacement
chunks. The event contains the session and prompt generation, but no raw
provider diagnostic or provider-specific data. Any ordinary title, timestamp,
or session metadata in the same ACP update keeps its existing session-info
projection.

The controlled `mock-agent` dialect has a test-only structured marker for the
same normalized event. Production recognizers never accept that marker.

## Lifecycle record ownership

`lifecycle.AgentExecution` owns both streaming IDs and the current response
attempt's retraction eligibility. Whenever lifecycle allocates a new Kandev
assistant or thinking message ID, it appends that ID to an ordered attempt-local
record list. Appends to a record created before the current attempt do not make
that record eligible for deletion.

`Manager.handleResponseAttemptReset` first verifies that the event's
execution and prompt generation still own the active prompt. It then performs
one ordered reset:

1. Flush the stream coalescer so every earlier create or append precedes the
   reset on the session stream subject.
2. Under `messageMu`, detach the eligible Kandev record IDs, remove only their
   protocol-ID correlations, reset legacy current IDs and pending message and
   thinking buffers, and discard the uncommitted assistant-history segment.
3. Publish one provider-neutral `response_attempt_reset` stream event carrying
   the detached IDs in allocation order.

An empty record list still completes the local reset but does not require a
message mutation. A second reset can retract only records allocated after the
first one.

## Commit boundaries and replay safety

A top-level tool call is the effect boundary that commits visible response
records. Lifecycle flushes their streaming content and assistant history before
clearing the attempt-local eligibility list. Protocol-to-Kandev correlation can
continue across the tool call for providers that reuse an ACP message ID, but
that existing Kandev row cannot become deletion-eligible again. Subagent
nesting does not let a provider reset remove the parent tool row or another
turn's records.

Prompt start, completion, foreground handoff, cancellation replacement, and
execution teardown clear attempt-local tracking with the existing streaming
state. A reset never reaches backward into a completed prompt generation.

The orchestrator's prompt-attempt evidence remains conservative. Output or
effects observed before the provider-owned reset remain observed for the whole
Kandev prompt. Removing a transcript projection therefore cannot turn a later
terminal failure into permission for an automatic Kandev replay.

Fallback session history uses the existing assistant-history buffer. Resetting
the pending segment prevents abandoned text that has not crossed a history
boundary from being injected into a later non-native resume. Previously
committed history is not rewritten.

## Durable and live cleanup

`lifecycle.AgentStreamEventData` carries `RetractedMessageIDs` only for a
`response_attempt_reset` event. The orchestrator handles it under the existing
per-session cancellation and stream guard, after the completed-execution and
generation fences used for other agent stream events.

A narrow `StreamingMessageRetractionService` exposes task-service
`DeleteMessage` to the handler. The backend application wires the task service
as the implementation. Each successful deletion removes the row and publishes
`session.message.deleted`. The existing frontend message handler treats that
notification as a semantic barrier: it flushes queued updates and removes the
message from the shared session store. Desktop and mobile task chat use that
same store and renderer, so no responsive layout or new user-facing copy is
needed.

The lifecycle session stream serializes provisional creation, reset handling,
deletion, and replacement persistence. The WebSocket gateway consumes
`message.added`, `message.updated`, and `message.deleted` through one ordered
NATS wildcard subscription, so independently scheduled subject callbacks cannot
overtake one another before the per-session journal and live fan-out. A reload
or a second viewer reads the durable transcript after deletion and therefore
cannot restore the abandoned row.

## Failure behavior

- A stale or zero-generation reset is ignored before it can detach IDs.
- A reset with no eligible output is an idempotent no-op at the task-service
  boundary.
- Failure to delete one row is logged with task, session, execution, and
  message identity. Cleanup continues for the remaining IDs and the provider
  replacement turn remains live.
- A protocol message that predates the latest commit boundary is preserved even
  if later chunks reuse its ID. The system does not guess a truncation offset.
- Missing retraction-service wiring leaves the stream running; production
  composition tests require the task service to be wired.

## Observability

ACP normalized logs retain the provider-neutral reset event with agent type,
session, and prompt generation without copying provider diagnostics. Existing
task-service logs identify successful durable deletions, and orchestration logs
failed deletions with task, session, execution, and message identity. A
diagnostic trace can therefore distinguish an intended replacement from two
legitimate messages.

## Test strategy

- ACP adapter tests use the captured Codex metadata shape and cover malformed,
  false, unsupported, zero-generation, and stale-generation negatives plus FIFO
  ordering before replacement output.
- Lifecycle tests cover assistant and thinking IDs, pending buffers, repeated
  resets, committed rows, protocol-ID reuse, generation fencing, and fallback
  history.
- Orchestrator tests use the real task service and event bus to prove durable
  deletion, `session.message.deleted` publication, continued cleanup after an
  individual failure, and safe behavior without wiring. A gateway transport
  test schedules the replacement subscription ahead of delayed deletion and
  proves that the shared subscription retains deletion-first delivery.
- Desktop and mobile Playwright scenarios arm causal WebSocket observations
  before the controlled mock dialect streams an abandoned response. They
  correlate both created message IDs with their deletion events, require both
  deletions before replacement, then reload and assert that only the
  replacement remains.

## Related decisions

- [Retract Output From Abandoned Provider Attempts](../../../decisions/2026-09-09-retract-abandoned-provider-output.md)
- [Separate Agent Error Evidence From Recovery Policy](../../../decisions/2026-08-08-provider-neutral-agent-error-recovery.md)
