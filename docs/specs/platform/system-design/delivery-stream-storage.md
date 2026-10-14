---
status: current
system: platform
created: 2026-10-13
requirements:
  - REQ-PLATFORM-BOUNDED-SESSION-HISTORY-002
  - REQ-PLATFORM-DELIVERY-RETENTION-001
  - REQ-PLATFORM-DELIVERY-RETENTION-002
  - REQ-PLATFORM-DELIVERY-RETENTION-003
  - REQ-PLATFORM-DELIVERY-RETENTION-005
  - REQ-PLATFORM-DELIVERY-RETENTION-006
---

# Delivery stream storage

## Ownership and boundaries

Platform owns operational storage bounds and authenticated delivery evidence.
This supplements [bounded storage](pr-watch-and-bounded-storage.md),
[retention](backend-agent-delivery-retention.md) and its
[payload proof](backend-agent-delivery-retention-payloads.md).
The [admission decision](../../../decisions/2026-10-13-superseding-shell-snapshots.md)
defines which transient snapshots can be replaced before journal acceptance.
Wire version 1, journal format, hashes, retention format 1 and existing configuration
remain unchanged. A compatible older backend receives fewer ordinary full snapshots.

| Requirement | Design sections |
| --- | --- |
| REQ-PLATFORM-BOUNDED-SESSION-HISTORY-002 | Snapshot normalization and admission |
| REQ-PLATFORM-DELIVERY-RETENTION-001 | Projected control notifications |
| REQ-PLATFORM-DELIVERY-RETENTION-002 | Replay and checkpoints |
| REQ-PLATFORM-DELIVERY-RETENTION-003 | Transaction and lifecycle boundaries |
| REQ-PLATFORM-DELIVERY-RETENTION-005 | Work and storage bounds |
| REQ-PLATFORM-DELIVERY-RETENTION-006 | Upgrade and operational behavior |

## Snapshot normalization and admission

The ACP normalizer's `shell_output.go:boundShellOutput` retains a UTF-8-safe
256 KiB tail. `NormalizeShellToolUpdate` merges deltas, cumulative replacements
and final results into the active shell payload. `adapter_tools.go` then emits
that accumulated snapshot on every update. Keep normalization of every source
notification; changing the normalizer to ignore deltas would corrupt the final tail.

Reduce only superseding output snapshots before `Adapter.sendUpdate` and before
`process.deliveryEventWriter.persist` assigns journal sequence numbers. Extend the
existing single `runUpdateWorker` owner with one pending snapshot and one timer.
No additional goroutine or lifetime-sized per-tool map is necessary.

Eligible notifications are output-only, nonterminal updates for the same session,
prompt generation and tool-call identity, with a complete normalized `shell_exec`
snapshot. Their command, working directory, parent, status and non-output metadata
must agree. Recognize only established shell-output carriers; unfamiliar metadata,
locations, inputs or content variants are ordered barriers. Background shell work,
non-shell tools and standalone output chunks are not eligible.

Codex ACP 2.2.2 uses `{data, terminal_id}` inside its output carrier, as grounded
in its [versioned renderer](https://github.com/agentclientprotocol/codex-acp/blob/v2.2.2/src/tool-calls/AcpToolCallRenderer.ts).
Accept that optional identifier only when it is a string equal to the exact
tool-call ID. A different identifier or any unknown carrier field is a barrier.

Emit the first snapshot immediately. During a consecutive eligible run, replace the
pending snapshot with an immutable copy of the newest complete normalized state.
Carry the notification's captured prompt generation on emitted snapshots, preserving
an already populated generation. The manager binds delivery submissions using this
field; pending output must not fall back to a successor's active submission.
Use a fixed 250 ms window from the preceding emission, not a debounce that resets
on every chunk. The existing worker's select handles the timer, so the trailing
snapshot is delivered when the producer becomes quiet.

Flush pending output before another tool/session/generation, conversation text,
reasoning, permission, configuration, background observation, state change or terminal
event. A terminal tool result is emitted immediately and can supersede pending output
only if it carries the same complete normalized result; otherwise flush first.
`syncNotifQueueThen` flushes before running its callback and releasing its waiter.
Audit direct and locked emission sites outside `handleACPUpdate`: an ordered control
event must not overtake pending output. Route necessary flushes through the existing
notification owner/barrier rather than holding `Adapter.mu` across a channel send.
Prompt completion, cancellation, reset and load transitions retain their barriers.

The downstream channel remains lossless for emitted events and applies existing
cancellable backpressure. A timer does not bypass the consumer or promise delivery
while the consumer is unavailable. All journaled events keep their original payload,
sequence and hash forever until existing acknowledged retention permits deletion.

## Projected control notifications

The initial supported control set is `agent_capabilities`, `session_status`,
`available_commands`, `session_mode`, `session_models`, `session_info`,
`context_window`, `mcp_attachment` and `turn_started`.
These are typed notifications already handled by lifecycle/orchestrator consumers,
not permission decisions, recovery authorization, prompt dispatch or workflow receipts.
`turn_started` remains subject to its normal consumer's generation checks and native
turn persistence. Retention never reconstructs or triggers that boundary.

Before admitting each type, tests must ground its consumed state in the current
handler/recovery path. In the existing protocol, a completed generic event receipt
already suppresses its callback on replay. Retirement removes redundant transport
evidence; it does not claim that a previously swallowed metadata-write error succeeded.
Current configuration and attachment state continue to come from their existing
session snapshots/provider refresh paths. Add coverage of state preservation after
retirement and restart. Do not replace current state from old payloads.

Use the same type classification for candidate SQL, locked reload, typed payload
validation, compaction, receipt retirement and read-only preview. A control body
qualifies only when all existing identity, projected-cursor, grace, exact completed
generic effect, no-pending/unknown-effect, hash and byte-budget checks hold.
An empty bootstrap submission is permitted as today; a supplied submission must pass
the existing exact retained-submission proof for receipt retirement.

Unknown types, permission events, `response_attempt_reset`, authority-changing
events without independent proof, malformed/user-role payloads and pending or
unknown effects remain protected. Terminals and prompts keep their full settlement
requirements. Do not add a blanket nonterminal allowlist or age-based authority.

## Replay and checkpoints

Once a proved control payload is compacted, retain its format-1 normalized hash
and envelope exactly as for supported output. Identical replay returns the committed
result, changed replay fails, and its callback is not repeated.
Retirement still requires every row from `retired_sequence + 1` to be present and
independently eligible. An unsupported control between two eligible rows is a genuine
barrier; neither MAX(sequence) nor a later eligible row permits crossing it.

Delete expired inbox rows and only their exact completed generic effects in the same
transaction as the monotonic retired cursor. Preserve authoritative terminal/workflow
effects. Receive and projection reject replay at or below the checkpoint using the
existing typed expiry result. State-only ACK recovery uses the authenticated projected
cursor; an expired event itself never grants ACK or dispatch authority.

## Transaction and lifecycle boundaries

Retain SQLite immediate writer admission and PostgreSQL sorted session/stream barriers
from the [barrier decision](../../../decisions/2026-10-10-delivery-event-retention-barriers.md).
Reload control/event/effect proofs after lock admission. Grace and window expiry are
separate: pruning a body does not automatically expire its recent receipt.
Hash verification, deletion, checkpoint and durable scan progress commit together.
Interrupted transactions preserve raw content and checkpoints; committed progress
survives restart without rescanning from zero or recreating retired events.

The pending output snapshot exists only inside the adapter's existing lifecycle.
On graceful prompt/session boundaries flush it before completion. On adapter shutdown
cancel the timer, release cancellable sends and join the existing worker before closing
its channel. Abrupt process loss can discard an unjournaled pending snapshot, as it can
discard an existing in-memory queue item; it cannot discard committed journal records.
Never claim that pending, unaccepted data is durably saved.

## Work and storage bounds

For a continuous eligible run lasting T, the snapshot count is bounded by the initial
and final snapshots plus the 250 ms cadence. Count ordered barriers separately because
they may force earlier flushes. Memory holds one pending full snapshot, independent of
provider chunk count. Preserve 256 KiB stream bounds and UTF-8 truncation semantics.

Measure marshaled emitted/journaled bytes, not only event counts. A synthetic saturated
10,000-update run over ten seconds should emit no more than 42 snapshots including
boundaries and persist under 16 MiB rather than roughly 2.6 GiB. Tests use controlled
time and small state fixtures, not sleeps or a giant historical database.

Keep existing cleanup row/byte/transaction/pass limits, indexed keyset discovery,
maintenance gate, fair progress and productive continuation. No new collector,
full-table scan, total-storage quota or per-session metric labels are introduced.
Existing counters report increased event retirement only after commit.

## Upgrade and operational behavior

Existing raw controls are revisited by the normal bounded worker after upgrade.
No destructive migration, historical transcript rewrite or forced checkpoint backfill
is required. Unknown historical events and unresolved terminals can still pin streams.
Canonical history and compact terminal/workflow evidence retain their own lifetimes.

Logical cleanup releases space for reuse; backups preserve free pages and compaction
remains an explicit operator action. This delivery does not compact or restart the
developer's live installation. Keep rollback readers compatible with format 1.
Public guidance in `docs/public/sessions-and-review.md` and `operations.md` should
describe periodic live shell snapshots, replay preservation and physical reclamation.

## Implementation plan

[Single-PR delivery stream storage repair](../../../plans/delivery-stream-storage/plan.md).
