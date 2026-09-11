---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-002
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-004
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
---

# Durable agent delivery system design

## Purpose and boundaries

The backend owns canonical messages, turns, and workflow authority.
Agentctl owns a bounded delivery journal close to the harness process.
The journal is not another product transcript or a native harness checkpoint.

This contract covers authenticated backend-to-agentctl traffic.
[Browser subscription recovery](../requirements/session-subscription-recovery.md) remains separate.
[Harness continuity](../../agents/system-design/harness-session-continuity.md) consumes submission outcomes.
The existing task queue and environment-generation guards retain admission authority.
The design baseline is main commit `65d64f6fa9f85b4977b8cdd62cf162380732effc`.
Its durable queue dispatch claims are part of this design, not a parallel recovery mechanism.

The guarantee starts at a successful journal commit.
A harness can perform a tool action before agentctl observes its event.
This design does not provide exactly-once external execution or restore arbitrary in-flight model state.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001 | Agentctl journal |
| REQ-PLATFORM-DURABLE-AGENT-DELIVERY-002 | Executor lifetime |
| REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003 | Submission protocol |
| REQ-PLATFORM-DURABLE-AGENT-DELIVERY-004 | Event protocol |
| REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005 | Backend inbox and projection |
| REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006 | Reconciliation |
| REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007 | Compatibility and rollout |

## Components and responsibilities

| Component | Responsibility |
| --- | --- |
| agentctl journal package | Transactions, sequence allocation, submissions, retention, exclusive ownership |
| agentctl process manager | Commit normalized events before publication and fence harness dispatch |
| agentctl HTTP API | Capability negotiation, submissions, outcomes, replay, acknowledgment |
| Runtime agentctl client | Bounded stream reader, reconnect cursor, outcome queries |
| Backend task repository | Durable inbox, projection checkpoints, canonical effects |
| Lifecycle | Reconciliation state and Stop |
| Orchestrator | Existing turn authority, idempotent workflow intents, queue admission |
| Executor providers | Stable journal location and environment ownership |
| Existing recovery UI | Reconnecting, uncertain, and legacy-delivery status |

## Data and contracts

Each durable event has the following identity:

```text
session_id + incarnation_id + harness_generation + stream_id + sequence
```

The stream belongs to a harness generation.
Sequence starts at one and increases within that stream.
A new connection does not create a stream.
A replacement native conversation creates a new stream.

Each request also carries the current environment owner generation.
The journal rejects stale owners before dispatch or acknowledgment.
The durable stream identity survives a valid transfer to a new backend process.
A connection ID or process PID is never a durable identity.

The authenticated initialization response advertises `durable_delivery.version = 1`.
It includes retained-storage support and the current stream identity.
This capability is distinct from ACP capabilities.

The proposed additive HTTP contract uses the existing authenticated instance scope:

| Method and route | Contract |
| --- | --- |
| POST /api/v1/agent/submissions | Admit an immutable submission with ID, identity, owner generation, hash, and payload |
| GET /api/v1/agent/submissions/{id} | Return durable submission state without dispatch |
| GET /api/v1/agent/stream?stream_id=…&after=… | Replay events after a sequence, then deliver live events |
| POST /api/v1/agent/stream/ack | Acknowledge the highest contiguous backend inbox commit |
| GET /api/v1/agent/delivery | Return stream bounds, submission summary, journal health, and protocol version |

The implementation reuses current authentication and request-limit middleware.
It does not expose these endpoints directly to the browser.
Unknown versions, invalid ownership, hash conflicts, expired cursors, and lost journals return typed errors.

## Agentctl journal

A new package under `internal/agentctl/journal` owns one bbolt database per agentctl owner.
bbolt provides pure-Go transactions without breaking CGO-disabled remote agentctl builds.
The implementation pins a compatible maintained version in go.mod and go.sum.

Buckets contain schema metadata, stream records, normalized events, submissions, acknowledgment cursors, and retired incarnations.
Each write transaction allocates sequence numbers and persists its records atomically.
Default durable synchronization remains enabled.
No debug-only JSONL file serves as the journal.

The adapter first produces a normalized event.
The journal writer commits that event before the API can publish it.
A bounded batch can contain at most 64 KiB or wait at most 20 ms.
Terminal state and its terminal event commit together.

The producer queue has a 4 MiB byte bound.
An event has a 1 MiB serialized limit.
Larger text output uses ordered chunks with stable message identity.
Oversized unsupported payloads stop durable admission with a typed error.
They cannot disappear silently.

The writer does not hold the ACP receive lock during storage work.
Cancellation responses and permission responses cannot wait for the event-consumer lock.
The implementation preserves the existing notification FIFO barrier before prompt completion.
It replaces the unbounded client queue with bounded intake into the SQL inbox.

The default logical limits are 256 MiB per stream and 2 GiB per journal.
Each journal reserves 1 MiB for health and terminal metadata.
Acknowledged event pruning precedes new admission.
At the limit, agentctl refuses new prompts and requests cancellation of active owned work.
An out-of-band health error reports a full journal if even the reserved write fails.
The active submission then remains uncertain.

Files use mode 0600 inside a 0700 directory.
The exclusive file lock prevents concurrent agentctl writers.
Corrupt or newer-format journals fail closed.
They remain intact for diagnostics, not automatic recreation.

Logical limits do not alone cap the bbolt file size.
Idle compaction uses a validated sibling temporary file, durable replacement, and a bounded disk-space preflight.
No active writer participates in compaction.
Quota tests cover reuse, compaction interruption, and inadequate free space.

## Executor lifetime

Executor providers supply a stable journal root through their existing launch configuration.
The root must not derive from a process ID, task title, or temporary binary directory.

| Executor environment | Persistence requirement |
| --- | --- |
| Native host | Owner-scoped Kandev data directory outside the worktree |
| Docker | Retained environment volume outside the disposable container layer |
| SSH | Owner-scoped remote data directory outside the deployed binary path |
| Kubernetes | Retained environment storage, not an empty disposable pod filesystem |

The journal remains separate from .codex, .claude, and OpenCode native storage.
Retaining either store does not imply retention of the other.

An executor that does not implement retained storage reports legacy delivery or `storage_not_durable` before stream creation.
It cannot advertise the v1 durability guarantee.
A supported executor with missing, full, locked, or corrupt journal storage instead returns a journal error.
A runtime failure cannot disguise itself as an unsupported capability.
Environment deletion remains an explicit destructive lifecycle operation.
A recreated empty environment reports `journal_lost` for the previous stream.

The storage-maintenance owner exposes size, retention status, and active ownership.
Cleanup preserves live owners and unacknowledged events.
Explicit authorized session/environment deletion can remove journal data.
The deletion result states that delivery recovery is no longer possible.

Acknowledged events can be pruned immediately after a durable acknowledgment update.
Submission identifiers remain until the incarnation retires.
A stream accepts at most 10,000 submissions before it requires an idle stream rollover.
Rollover seals the old stream and retains a compact tombstone until incarnation deletion.
Sealed or retired identities reject all new dispatch requests, even after payload pruning.
No time-based expiration can make an old identifier executable again.

## Submission protocol

The backend allocates and persists the submission ID before the HTTP request.
Queued submissions retain the existing queue entry and dispatch-attempt identity.
The record contains the current turn/queue identity and final composed payload hash.
The hash covers context, prompt text, attachment references, effective configuration, and target identity.
The payload cannot change under the same ID.

The durable states are:

```text
accepted -> dispatching -> completed | failed | cancelled
                       -> interrupted_unknown
```

The backend also has a local `prepared` state before acceptance.
Agentctl commits `accepted` before it returns an admission response.
It commits `dispatching` before it calls the harness.
Only one owner can perform that transition.

A duplicate ID with the same hash returns the existing state.
A duplicate ID with a different hash returns `submission_conflict`.
Neither case sends another prompt.

A crash before the dispatching commit permits controlled dispatch from accepted state.
That dispatch requires reconciliation with the current owner and unchanged authorization.
A crash after the dispatching commit leaves the outcome uncertain unless a terminal record exists.
The system never automatically redispatches that submission.
This conservative rule includes a crash before the actual harness call.

Stop uses the current owned process and submission identity.
The first valid terminal transition wins.
A late response cannot overwrite cancellation or a newer generation.
Cancellation acknowledgment alone does not prove that an external tool made no changes.

The continuation snapshot and current user prompt form one immutable submission.
An uncertain result leaves the continuation checkpoint uncertain.
A later user-authorized recovery creates a new submission, not an automatic resend of the old ID.

## Existing queue admission and startup recovery

The queue service remains the owner of `queue_dispatch_claims` and its dispatch-attempt ID.
Its `WithSessionAdmission` boundary serializes claims, edits, Send Now, transfers, cancellation, and automatic drain.
The durable-delivery implementation extends these paths instead of introducing another queue lock or reservation model.

The queue claim stores its protocol mode and durable submission reference.
The immutable payload hash covers the final post-composition content and attachments.
A transport retry preserves the original dispatch attempt, submission ID, and hash.
A new logical user instruction receives a new attempt.
Queue edit operation IDs and lease generations are not submission IDs.

The task repository owns delivery outcomes and the agentctl inbox.
The queue repository owns the claim-to-submission association.
Cross-repository preparation uses the existing durable claim as an intent.
No HTTP dispatch occurs before both the association and prepared submission exist.
Restart repairs incomplete preparation without replacing an already dispatched identity.

Current `reconcilePendingQueueDispatchesOnStartup` restores claims with unknown outcomes.
For v1 claims, it must query the original submission before restoring dispatch eligibility.
The record can become visible in the queue while a recovery block keeps it ineligible.

| Durable result | Queue recovery |
| --- | --- |
| Prepared with proof of no dispatch | Repair preparation and use normal guarded admission |
| Accepted and undispatched | Reconcile current ownership, then dispatch the same submission once |
| Dispatching with a reachable live owner | Reconnect and hold subsequent work |
| Durable terminal outcome | Apply the outcome and settle the original claim once |
| Unknown, missing journal, or incompatible identity | Keep the claim visible and blocked for operator recovery |

Legacy claims need explicit migration behavior.
An accepted legacy claim remains acknowledged without another dispatch.
An unaccepted pre-upgrade claim with an unknown outcome receives `legacy_delivery_uncertain`.
It cannot enter automatic drain merely because no v1 journal exists.
New legacy submissions retain their documented transport limits and never receive a false v1 guarantee.

The queue transfer contract remains authoritative.
A transfer before dispatch can create a replacement submission only after the old attempt is durably sealed as undispatched.
After dispatch begins, transport identity and original owner stay fixed.
Moving the visible queue row does not authorize the destination to resend it.
Ownership ambiguity requires recovery.

The existing `acceptedPromptDispatchError` path cannot restore an already accepted prompt for another send.
Admission after edit-save, agent-ready, backend restart, and lifecycle automation all check the same recovery block.
Completed-task follow-ups retain conversational-only completion.
Replay cannot repeat final-step actions, reopen the task, or forge a workflow completion signal.

These rules extend [queued message editing](../../ui/system-design/message-queue-edit.md)
and preserve [task completion](../../tasks/system-design/task-completion.md).
They require tests for the current startup-recovery path, not only tests for a new transport client.

## Event protocol

The journal allocates the sequence and records the normalized payload before publication.
Every durable event carries submission identity where applicable.
Tool IDs, message IDs, permission-request IDs, and terminal outcomes retain their stable association.

The backend reconnects using its highest contiguous committed inbox sequence.
Agentctl replays later records, then joins the same stream at a captured high-water mark.
A writer cannot create a gap between replay and live delivery.
Duplicate delivery is allowed. Sequence gaps are not.

Acknowledgment cannot exceed the stream high-water mark.
It cannot acknowledge gaps or another owner.
A cursor before retained history returns `cursor_expired`.
An unknown stream returns `stream_unknown`.
The backend reconciles these outcomes rather than treating them as an empty successful replay.

Native ACP load can replay historical notifications.
The adapter's native-history suppression remains active.
Only current-generation semantic events enter the transport journal.
Transport replay never calls ACP tools or reissues historical permission decisions.

Permission requests remain tied to the original process and request ID.
On process replacement, unresolved requests close as interrupted.
A new harness request requires a new current permission decision.
The journal does not replay an approval into a replacement process.

## Backend inbox and projection

The task repository owns additive inbox, checkpoint, submission, and effect-intent tables.
SQLite and PostgreSQL use the same typed repository contract.
Queue claim changes remain under the message-queue schema owner.
Task, message-queue, and Office changes extend their existing required-store adapters and boot/upgrade evidence.
The existing task schema owner includes these tables in required-store conformance and upgrade tests.

Inbox uniqueness uses stream identity plus sequence.
An inbox transaction inserts the event and advances the contiguous received cursor.
Only after commit can the backend acknowledge agentctl.
A crash after commit but before acknowledgment produces a harmless duplicate.

A separate projector processes events in sequence.
One SQL transaction applies message changes, records deterministic effect keys, and advances the projected cursor.
Message effects use stable stream/event/message keys rather than random IDs on replay.
Partial assistant text and tool updates preserve current rendering semantics.

Workflow transitions cannot rely on replaying the existing event bus without deduplication.
The projection transaction records a durable effect intent.
Consumers claim that intent through the existing turn and environment ownership guards.
They record a unique effect key at the authoritative state transition.
Delivery to a consumer can repeat. The state transition cannot.

External actions require their existing idempotency contract.
Without such a contract, an uncertain external action requires reconciliation.
The projector never repeats MCP calls, Git mutations, or provider requests from recorded output.

Historical events from a replaced owner can enter the inbox for audit.
Only events for the matching current incarnation, generation, and turn can change current runtime state.
A terminal event for an old turn cannot complete the new turn.

Inbox retention follows canonical projection.
Payloads can be pruned after projection and acknowledgment recovery no longer needs them.
Compact received/projected cursors and effect keys remain until incarnation deletion.
An acknowledged event must never depend on an agentctl record that cleanup can remove before projection.

## Reconciliation

A disconnect first enters `reconnecting`, not prompt failure.
Lifecycle queries the same agentctl owner, submission state, and stream identity.
It makes at most three attempts within a ten-second connection-recovery window.
The existing request cancellation and shutdown contexts can end this window early.

A reachable live owner resumes event intake from the inbox cursor.
A durable terminal outcome is projected before lifecycle publishes completion.
An accepted but undispatched submission follows the controlled dispatch rule.
A dispatching submission without a recoverable live process becomes `interrupted_unknown`.

If the journal is lost or the owner cannot be reached, the outcome remains uncertain.
Queue auto-run, provider retry, and context injection cannot send another prompt.
The existing completion-signal policy remains authoritative.
Office and automation consumers use the persistent block from the continuity contract.
Provider recovery, budget retries, and scheduler ticks cannot release that block.
Transport loss does not synthesize task completion or workflow advancement.

The user sees reconnecting, recovered, or uncertain status in the existing recovery surface.
Stop remains available during reconciliation.
Retry reconnect queries state only.
It does not resend the prompt.

A user can inspect the workspace and submit an explicit next instruction.
That action retains the prior uncertainty notice and creates a new submission.
A replacement conversation additionally requires the explicit context-continuation action.

This package does not change executor parent-death behavior.
If the executor terminates the harness, recovery retains journal evidence and uses native restore or explicit context continuation.

## Compatibility and release

This functionality has no runtime feature flag, environment toggle, or hidden rollout switch.
The user's 2026-09-10 decision replaces the earlier proposed flag rollout.
The draft flag identity never shipped, so no runtime registry or retired-identity change is needed.

Continuity and transport remain separate release milestones.
Continuity ships only after native recovery, explicit continuation, and autonomous parking work end to end.
Durable transport ships only after journal storage, submissions, replay, projection, reconciliation, and upgrade tests pass.
Incomplete transport work stays on the implementation branch, not in a published default path.

Once the durable milestone ships, compatible peers in supported retained environments use v1 automatically.
Capability negotiation describes implemented protocol and storage support.
It is not an operator-controlled feature gate.

| Connection state | Admission |
| --- | --- |
| Compatible peers and healthy supported storage | Use v1 automatically |
| Older peer or explicitly unsupported executor storage | Use identified legacy delivery for eligible new streams |
| Authentication failure or unreachable peer | Fail connection setup, never infer legacy support |
| Supported durable storage fails | Block admission with a typed journal error |
| Existing v1 stream loses capability or journal identity | Reconcile or require recovery, never downgrade |
| In-flight legacy stream during upgrade | Retain its recorded mode until completion or explicit recovery |

A missing optional capability from a valid older handshake can establish legacy support.
A timeout, failed journal open, or malformed response cannot establish it.
An idle legacy session can start a new v1 stream after its prior submission is settled.
Protocol mode is immutable during an active submission.

Required SQL schema and historical reads remain available regardless of negotiated mode.
Startup reconciles queue claims before any automatic drain.
No compatible installation needs configuration edits to obtain the new behavior.

There is no instant configuration rollback.
A regression requires a corrected release or a supported binary rollback after work stops.
The release must document the oldest recovery-capable version that can read its journal and queue records.
Arbitrary rollback to a pre-v1 binary is not supported while durable or uncertain submissions remain.
Old binaries cannot be assumed to enforce new admission guards.

A supported rollback retains journal data, stops active admission, and reconciles all retained submissions before new work.
It never treats a newer journal format as an empty store.
A crash or forced replacement can still leave work uncertain.
Operators must not delete the journal to bypass that state.

Release acceptance includes default-path, legacy-peer, storage-failure, upgrade, rollback, crash, replay, and desktop/mobile evidence.
All supported executors need their retained-storage evidence.
Missing PostgreSQL or container fixtures are release blockers, not waived checks.
Public docs state the no-toggle behavior, recovery limits, and supported rollback procedure.

## Security

The API checks session, incarnation, environment owner, and existing agentctl authentication on every request.
Journal paths cannot come from untrusted request fields.
Logs exclude prompt bodies, raw event payloads, authentication data, and full context snapshots.

Journal and inbox payloads receive the same access restrictions as canonical conversations.
Storage is not encrypted by this package.
Deployments that require encryption use encrypted executor and backend storage.

## Observability

All delivery metrics use `agent_delivery_*`.
One shared metrics contract defines the names, units, and bounded enums.
Each producer owns its measurements rather than declaring a parallel family.

| Metric | Unit | Producer and labels |
| --- | --- | --- |
| agent_delivery_submissions_total | Submissions | agentctl admission, outcome |
| agent_delivery_duplicate_submissions_total | Requests | agentctl admission, result |
| agent_delivery_replayed_events_total | Events | agentctl replay writer |
| agent_delivery_sequence_errors_total | Errors | Stream validator, reason |
| agent_delivery_uncertain_submissions_total | Submissions | Backend reconciliation, cause |
| agent_delivery_journal_errors_total | Errors | Journal owner, reason |
| agent_delivery_journal_bytes | Bytes | Journal owner, aggregate local file size |
| agent_delivery_unacknowledged_bytes | Bytes | Journal owner, aggregate retained unacknowledged payloads |
| agent_delivery_inbox_lag_events | Events | Backend intake, sum of known high-water minus received cursors |
| agent_delivery_projection_lag_events | Events | Backend projector, sum of received minus projected cursors |

Journal gauges belong to the agentctl health/metrics surface.
The backend can expose their aggregate health without incrementing the same counters again.
Backend counters and lag gauges use the existing expvar and structured-metric conventions.
Unknown remote high-water marks are unavailable measurements, not zero lag.
Repeated observations of the same uncertain submission do not increment a new transition counter.

Structured diagnostics include stream identity and submission ID.
Metrics never use those identities, paths, payloads, or arbitrary provider text as labels.
Root `AGENTS.md` documents the family with its producer boundaries when implementation ships.
`CLAUDE.md` remains the symlink, not a second documentation source.

The existing recovery notice persists uncertainty and continuity outcomes.
Routine successful reconnects do not append duplicate conversation messages.

## Related decisions

- [Durable sessions across harness generations](../../../decisions/2026-09-10-durable-harness-session-boundaries.md)
- [Generation-fenced environment ownership](../../../decisions/2026-09-04-generation-fenced-task-environment-ownership.md)
- [Required internal persistence](../../../decisions/2026-09-05-required-internal-persistence.md)
