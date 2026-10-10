---
status: draft
system: platform
created: 2026-09-27
owners:
  - kandev
---

# Explicit Same-Turn Steering

## Purpose and ownership

Allow a user to send additional instructions to the main conversation's active
turn without cancelling it or waiting in Kandev's next-turn queue. Platform owns
this runtime delivery contract alongside [existing opportunistic steering](mid-turn-steering.md).
Codex app-server is the first same-turn adapter. Agent background-work inspection
consumes these capabilities but does not own prompt admission or delivery.

This is an additive explicit delivery mode, not a rewrite of existing automatic
ACP steering or queue ordering. It creates a deliberate exception to queue-first
behavior only when the user chooses Send now for a verified same-turn provider.
The [decision](../../../decisions/2026-09-27-explicit-same-turn-steering.md)
records that boundary. A child-agent detail tab does not gain a composer.

## Requirements

### REQ-PLATFORM-EXPLICIT-STEERING-001: Explicit delivery and capability

- **AC-PLATFORM-EXPLICIT-STEERING-001.1:** When the main conversation has a steerable active turn and the user chooses Send now, the system shall deliver additional input to that same turn without cancelling it, starting a new turn, or inserting the message into Kandev's next-turn queue.
- **AC-PLATFORM-EXPLICIT-STEERING-001.2:** When messages are already queued, an explicit Send now shall still target the current turn; existing queued messages and their relative order shall remain unchanged. Queue for later shall retain existing queue admission and ordering.
- **AC-PLATFORM-EXPLICIT-STEERING-001.3:** When discovering capabilities, the system shall distinguish same-turn targeting, provider-managed concurrent delivery, and unsupported steering. It shall not promise same-turn behavior for a provider that may defer a concurrent prompt.
- **AC-PLATFORM-EXPLICIT-STEERING-001.4:** When the feature is disabled, the runtime is disconnected, or the selected turn/input is unsupported, same-turn delivery shall be unavailable and direct requests shall fail before dispatch. Existing queue and ACP behavior shall continue unchanged.
- **AC-PLATFORM-EXPLICIT-STEERING-001.5:** When one explicit send is awaiting acknowledgement, another shall not be silently queued or dispatched concurrently. The draft shall remain editable, and subsequent Send now operations shall become available after acknowledgement if that turn remains steerable.

### REQ-PLATFORM-EXPLICIT-STEERING-002: Ownership, outcomes, and attribution

- **AC-PLATFORM-EXPLICIT-STEERING-002.1:** When the active turn, session incarnation, or owning execution changes before delivery, the message shall not be redirected to the replacement. A definite rejection shall preserve the draft and allow the user to choose a fresh delivery action.
- **AC-PLATFORM-EXPLICIT-STEERING-002.2:** When a send response is lost or the runtime/backend restarts after possible dispatch, the system shall show uncertain delivery and shall not automatically queue, retry, or start a turn with that message. Repeating the same delivery identifier shall not redispatch it.
- **AC-PLATFORM-EXPLICIT-STEERING-002.3:** When a steer is accepted or echoed by the provider, its user message shall appear once and retain the original active-turn ownership. Provider acknowledgement means accepted input, not proof the model has acted on it. The root turn shall retain one completion and once-only usage attribution.
- **AC-PLATFORM-EXPLICIT-STEERING-002.4:** When a root steer overlaps child work, tools, approvals, or clarification requests, that work and its ownership shall remain intact. A steering message shall not implicitly answer a request or stop a child, and cancellation shall retain existing root/session scope.
- **AC-PLATFORM-EXPLICIT-STEERING-002.5:** When steering through local or remote executors, access checks and exact-turn addressing shall apply at the owning runtime. Unauthorized or cross-session targets shall never reach a provider.

### REQ-PLATFORM-EXPLICIT-STEERING-003: Shared composer and compatibility

- **AC-PLATFORM-EXPLICIT-STEERING-003.1:** When same-turn steering is available, desktop and phone shall expose clearly distinct Send now and Queue for later actions in the main composer, including when the queue is nonempty. The chosen action and pending/accepted/rejected/uncertain outcome shall be visible and accessible.
- **AC-PLATFORM-EXPLICIT-STEERING-003.2:** When only provider-managed steering is available, existing opportunistic delivery behavior shall retain truthful copy; when unavailable, the composer shall retain ordinary send/queue behavior. Background jobs or open detail tabs alone shall not enable steering.
- **AC-PLATFORM-EXPLICIT-STEERING-003.3:** When changing delivery mode or receiving live state changes, the composer shall preserve text and attachments, avoid focus theft, and keep phone actions at least 44px with keyboard/safe-area clearance. All new labels, pluralization, errors, and accessibility names shall be localized.
- **AC-PLATFORM-EXPLICIT-STEERING-003.4:** When a turn ends, is a non-steerable operation, or the draft requests an incompatible turn-level option/input, Send now shall become unavailable with a reason. A pending or rejected send shall not silently change mode; an explicit new action is required to deliver it differently.

## Exclusions

Child-agent prompting/steering, process stdin, automatic steering of queued
messages, queued-message editing, synthetic wakeup steering, new-turn creation
as a fallback, and implementation of a new Claude-native adapter are excluded.
Steering does not guarantee immediate interruption of a running tool, nor permit
changing the active turn's model, sandbox, working directory, or output schema.

## Related contracts

- [System design](../system-design/explicit-turn-steering.md)
- [Legacy automatic/ACP steering](mid-turn-steering.md)
- [Queue admission](../../tasks/requirements/queue-admission.md)
- [Provider-neutral background work](../../agents/requirements/background-work.md)
- [Implementation package](../../../plans/agent-background-work/plan.md)
