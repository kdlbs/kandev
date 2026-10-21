---
status: current
system: agents
requirements:
  - REQ-AGENTS-AGENT-STALL-RECOVERY-001
created: 2026-09-02
updated: 2026-10-20
owners:
  - Kandev
---

# Agent stall recovery system design

## Purpose and boundaries

Agents owns prompt inactivity classification, acceptance evidence, and the
user-visible advisory. Platform owns durable cancellation uncertainty and
[interrupted-prompt recovery](../../platform/system-design/interrupted-prompt-recovery.md).
Tasks retains persisted state and [stop reachability](../../tasks/system-design/task-stop-reachability.md).
The [work package](../../../plans/silent-prompt-recovery/plan.md) records implementation
and verification.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-AGENTS-AGENT-STALL-RECOVERY-001, .1-.6, .8-.9 | Progress clock, advisory notice resolution |
| REQ-AGENTS-AGENT-STALL-RECOVERY-001, .12-.14 | Acceptance evidence, watchdog and cancellation |
| REQ-AGENTS-AGENT-STALL-RECOVERY-001, .15 | Diagnostic snapshot |

Criteria .7, .10 and .11 are superseded. No-output inactivity produces an
advisory notice and preserves controlled cancellation.

## Components and responsibilities

- `AgentExecution` retains prompt lifecycle identity and activity under its
  existing mutexes. Acceptance is distinct from output/effect evidence.
- `SessionManager.markPromptDispatched` and the execution store record the
  current accepted generation after `triggerPrompt` succeeds.
- `recordActivity` in `manager_events.go` classifies content and metadata.
- The prompt waiter watchdog publishes at most one stall per generation.
- `handleAgentStalled` in the orchestrator validates current ownership and
  writes the existing neutral notice with the `agent.cancel` action.
- Existing cancellation services own protocol cancellation, bounded escalation,
  state projection, and durable recovery blocks. The watchdog does not bypass
  their owner checks or settle their submission records.

## Acceptance evidence

Use the current `dispatchedPromptGeneration` boundary for ordinary dispatch.
Expose acceptance in a snapshot paired with execution ID and prompt generation.
Take lifecycle ownership before activity state, following the existing lock
order; revalidate the same generation at the consumer. Do not read mutable
fields without their owner or infer acceptance from a request being sent.

Restore may establish acceptance only from the authenticated active durable
submission with matching session, execution, incarnation, stream and generation.
Native session identity, `session_info`, usage, and a prior turn's acceptance
are insufficient. Unsupported or missing evidence is unconfirmed and remains
advisory. A provider-specific native start event is optional supporting evidence,
not a prerequisite or a new public ACP capability.

Do not set `agentEventSincePrompt` for acceptance. The existing
`promptAttemptEvidenceSnapshot` uses it for both `OutputObserved` and
`EffectObserved`. Acceptance alone must not change those fields. Preserve
content that arrives before the dispatch acknowledgment; recording acceptance
must not reset already-observed output, effects, or the newer progress clock.
Completion or a successor generation invalidates the old acceptance snapshot.

## Progress clock

Retain the one-minute watchdog and five-minute threshold. Dispatch arms the
clock. Genuine content, reasoning, tool calls/updates, plans, permission requests,
terminal completion, and delivered same-turn user input retain their existing
clock behavior. Metadata and acceptance acknowledgments do not repeatedly reset
it. The completion-signal watchdog retains this same progress meaning and its
separate explicit-signal recovery policy.

## Watchdog and cancellation

1. Snapshot prompt generation, activity epoch, progress time, acceptance and
   output evidence. Log once and publish a bounded stall payload.
2. Under the existing cancellation guard, reject settled sessions, absent active
   turns, replaced executions, stale generations and changed activity epochs.
3. Persist the existing neutral running-only notice. Use the generic waiting
   copy if no active tool title is available. No inactivity-only launch-failure
   write or forced stop follows, including when acceptance is unconfirmed.
4. On user Cancel turn, use the existing `agent.cancel` route and captured
   execution/turn/generation. Busy state prevents duplicate operations. Do not
   hold projection or cancellation locks while waiting for protocol completion.
5. Confirmed terminal cancellation follows normal reconciliation. A notification
   send, HTTP acknowledgment, released local waiter, or timeout is not itself a
   confirmed durable outcome. Missing outcome evidence retains the matching
   block and prevents prompt/queue dispatch. Existing explicit Stop remains
   available and retains its separate process-termination contract.

`NeverStarted` is an internal historical discriminator, not terminal authority.
Replace its uses in this path with accurate no-output/acceptance fields, or
retain it only as a clearly documented compatibility alias. No public version
negotiation is needed for backend-owned stall events.
Trusted startup rejection and correlated terminal errors keep their current
error and cleanup paths. This change does not weaken them or automatically
replay a quiet prompt.

### Advisory notice resolution

The action-message renderer derives advisory visibility from the existing
session, active turn, and session-message store. A running-only status notice
is hidden once an agent-authored content, reasoning, tool, plan, or permission
row for the same session and turn has a timestamp later than the notice's
creation. Use `updated_at` as well as `created_at`, preserving RFC3339
nanosecond precision: a tool can finish by updating a row created before the
notice. System status rows, queued user input, and other turns do not resolve
it. Terminal error diagnostics retain their settled rendering.

The store selector returns a boolean and subscribes only for running-only
status notices. Live message reducers record monotonic
`metadata.running_notice_resolved` on a loaded notice before discarding an
update whose older tool row is outside the loaded history window. Reconciliation
preserves this evidence through stale snapshots and includes it in message
identity even when the notice timestamp is unchanged.

The task service projects the same metadata on full and paginated transcript
reads. The repository aggregates the latest eligible agent creation/update
timestamp across the entire scoped turn without reading tool payloads. This
read projection does not change stored metadata or require a migration, and
query failures propagate rather than supplying incorrect evidence. Both live
events and reload therefore resolve notices independently of the loaded page.
The advisory message remains persisted and the watchdog still publishes at
most once per prompt generation; resolution adds no automatic cancellation.

## Desktop and phone

Reuse `action-message.tsx`, `action-message-actions.tsx`, and the current
`task-layout.tsx` phone chat composition. The transcript owns scrolling.
The short status and inline content-width Cancel turn control need no overlay
or new navigation. Phone and coarse-pointer targets remain at least 44px;
compact fine-pointer desktop styling remains unchanged. Reuse the shared
localized labels, keyboard behavior, cancellation pending state, and recovery
surface. No new animation or alert decoration is needed.

## Diagnostic snapshot

Extend the existing first-stall structured log. Emit at most one record per
execution/prompt generation before any subsequent user cancellation. Capture
from in-memory or already authenticated state; do not perform provider I/O
while holding the activity or cancellation lock.

Allowlist: task/session/execution identity; prompt and harness generations;
submission and stream identity when known; acceptance state; output/effect
booleans; last-progress time and elapsed milliseconds; agent type; managed
adapter and CLI versions when already known; current delivery state; received
and projected sequences when cached; the latest normalized retry/transport
category only if already observed. Unknown fields stay absent or explicitly
unknown. Do not infer bundled Codex version from the host CLI version.
Output and effect booleans share conservative turn-content evidence, as in
`PromptAttemptEvidence`; they do not independently prove persisted effects.

Use fixed enums, typed numbers, and strings capped at 128 UTF-8 bytes, with a
4 KiB total encoded record limit. Omit malformed UTF-8 input. Truncate valid
strings at the last complete rune boundary within the byte cap. Optional values
that fail validation are omitted. No prompts, environment, URLs, paths, tool
arguments, free-form provider errors, raw ACP frames or private logs enter the
record. Use the existing
rotated backend log retention; no new table, file writer or high-cardinality
metric. Snapshot failure is logged as a fixed category and cannot change the
turn, notice or cancel availability.

## Persistence and compatibility

No schema migration. Acceptance snapshots are execution-scoped and rebuilt
from current authenticated evidence on adoption. Notices and recovery blocks
keep existing persistence. Capture logs are diagnostic, never recovery proof.
All managed providers use the same policy; passthrough sessions retain their
existing exclusions. Office completion and scheduling rules remain separate.

## Verification

Use deterministic clocks to cover accepted silence, metadata-only traffic,
missing acceptance, late acknowledgment/output, completion races, restored
acceptance, and stale generations. Confirm the output/effect snapshot remains
unchanged by acceptance. Handler tests assert no terminal state write or stop.

Use real cancellation and journal boundaries for acknowledged cancellation and
unknown outcomes. Desktop and phone E2E must produce a real notice through the
controlled mock path, activate Cancel, and check reload and successor isolation.
Diagnostic tests inject secret-looking values and assert the allowlist and size
bounds. Include multibyte characters crossing the byte cap and malformed UTF-8;
retained strings must stay valid UTF-8 within the cap. The work orders own
commands and implementation results.

## Related decisions

- [Silent prompt acceptance](../../../decisions/2026-10-20-silent-prompt-acceptance.md)
- [User-controlled recovery](../../../decisions/2026-07-29-agent-stall-user-controlled-recovery.md)
- [Terminal diagnostic correlation](../../../decisions/2026-08-02-agent-terminal-diagnostics-over-stderr.md)
- [Historical terminal teardown](../../../decisions/2026-09-02-terminal-stall-owns-process-teardown.md)
