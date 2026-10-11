---
status: current
system: platform
requirements:
  - REQ-PLATFORM-PRUNED-DELIVERY-RECOVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-001
---

# Pruned delivery recovery system design

## Purpose and boundaries

Platform owns cross-store acknowledgment and native continuity. Lifecycle owns
authenticated runtime control, orchestrator owns recovery admission, and the
task repository owns transactional recovery evidence. This adds a narrow
historical-loss exception to [restart reconciliation](durable-restart-reconciliation.md).
It does not weaken live adoption, normal stream rollover, missing-payload
guards, or ordinary [ordered projection](durable-agent-stream-processing.md).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| REQ-PLATFORM-PRUNED-DELIVERY-RECOVERY-001 | Loss evidence; guarded native restoration; presentation |
| REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005 | Acknowledgment durability |
| REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007 | Stream identity, typed diagnosis, and compatibility |
| REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-001 | Inventory, source proof, and crash convergence |

## Acknowledgment durability

`internal/db.OpenSQLite` opens the single writer with WAL synchronous FULL.
Every reopened writer connection inherits FULL from its DSN. Reader connections
remain read-only; their synchronous setting is connection-local and does not
establish writer durability. Do not issue a global PRAGMA on an unrelated pool
and assume the actual delivery writer changed.

Inbox receipt and canonical projection/cursor transactions commit before the
existing cumulative ACK scheduler can release journal records. Preserve the
current projection-before-ACK rule and bounded batches. A failed commit cannot
advance pending ACK. Journal `NoSync: false` remains required. PostgreSQL
delivery writers must likewise retain synchronous commit; deployments with
disabled persistence cannot claim the power-loss guarantee.

## Stream identity, typed diagnosis, and compatibility

`Executor.applyDeliveryIdentity` already selects `incarnation:generation` and
the lifecycle execution constructor forwards the requested stream identity.
Operational recovery of the five missing events restored ACP initialization
without a cursor alias or bootstrap patch. The earlier zero-cursor hypothesis
is unconfirmed; the session fallback in `connectUpdatesStream` is not evidence
that the affected production launch used it. Preserve and test this constructor
path. Do not require an identity rewrite without an independently failing
production-path regression.

For actual coverage-gap diagnosis, lifecycle can use the existing authenticated
HTTP `Client.GetDeliveryStatus`, available without ACP initialization. Validate
session, incarnation, harness generation, stream, runtime lease, and SQL
evidence against the already selected execution identity. The updates socket
transports control responses needed for ACP; gap diagnosis must not depend on
ACP negotiation through a socket that replay failure has disconnected.

A verified fresh owner retains existing launch bootstrap authority, and its
event processor waits for committed generation before canonical projection.
A gap successor uses only its explicit pending restore checkpoint authority.
Control responses must remain serviceable while projection is pending. A
nonempty stream without its matching SQL checkpoint remains blocked. Preserve
positive legacy compatibility and reject unresolved durable downgrade;
transport failure and unknown capability are not positive legacy evidence.

Carry a typed startup result through the existing stream readiness boundary.
Closing `ready` must not translate descriptor or replay failure into readiness.
Lifecycle publishes `delivery_history_gap` with structured range evidence and
the actual source cause; orchestrator persists a matching block/notice before
returning failure. The client/server stream path must preserve typed replay
expiry rather than replacing it with an ACP timeout.

## Loss evidence

`AgentDeliveryRecovery.HistoryGap` pins the missing bounds and the unchanged
SQL received/projected snapshot. Existing reconstruction provenance pins the
payload hash, peer high-water/acknowledged/first-retained snapshot, process
proof, and observation time. The existing `session_restore_attempts` checkpoint
retains this recovery plus source native/generation/block, original runtime,
workspace owner, and allocated candidate. Reuse these admission and revision
fences instead of adding a parallel gap table or allocator.

Successful restoration atomically archives an immutable
`agent_delivery_history_gaps` map in session metadata. Each stream record owns
its incarnation/generation, exact submission/hash, native identity, and missing
interval. Later prompt recovery snapshots cannot erase this historical record.
It is neither an inbox event nor evidence of terminal completion.

First verify cursor inequalities and contiguous backend receipt through
`ReceivedSequence`, accounting for legitimate earlier SQL retention. If receipt
covers the expired peer prefix, finish ordered projection from the actual inbox
instead of declaring loss. Otherwise a peer prefix absent from receipt has
bounds `ReceivedSequence + 1` through `FirstRetained - 1`, limited by validated
ACK/high-water. A missing projected prefix alone does not prove lost receipt.
The initial observed example was 5495 through 5499, with receipt/projection
5494, ACK/high-water 5499, first-retained 5500, and no indexed retained events.
All five original nonterminal events were subsequently salvaged from unused
bbolt pages by an offline operational repair and restored to validated replay.
Normal projection recovered them and resumed the same native conversation on
the original stream. That repair does not establish dependable free-page
retention or a production salvage mechanism.

Prefer complete validated replay whenever original events are available.
Only actual receipt/projection can advance the old cursor. When original events are available before fallback, use ordinary replay and
projection without allocating a loss successor or archiving a loss notice.
Recovered nonterminal events do not prove completion. Changed receipt or
retention evidence invalidates a pinned pending gap; never commit a stale interval. Raw unused-page scanning is outside this implementation.

Persist the gap and matching recovery fence under session admission locks and
SQL compare-and-set checks, including cases without prior recovery metadata or
blocks. A changed cursor, descriptor, native identity, notice, or execution
invalidates the snapshot. Corrupt, truncated, or conflicting evidence remains
specifically blocked. Cursor values are never rewritten to cover the gap.

## Guarded native restoration

Extend the existing `session_restore_attempts` checkpoint with the pinned gap
identity and one proposed successor harness generation/stream. The old SQL
generation remains current until verified restoration commits. The backend-only retained recovery request adds `pruned_through` and
`expected_hash`. Its response must positively return `history_gap_verified`
after exclusive journal ownership and original-session death proof. Older
peers cannot provide this proof and remain blocked. No new public recovery
action is introduced: the restart worker and `session.recover(action=resume)`
call the same recovery path.

Eligibility requires the existing source-process death proof, full authenticated
submission inventory and payload-hash comparison, saved native identity and
workspace, unarchived ordinary chat ownership, and no independent admission
block. The historical stream must have `Acknowledged == HighWater`, no retained
event suffix, and no independently live, prepared, or accepted work. A peer
`interrupted_unknown` may reconcile exact SQL `dispatching` only after source
death is proved. Known terminal states are preserved, not rewritten unknown.

Under exclusive journal ownership, guarded retained inspection validates the
entire bounded inventory, exact payload hash, fully acknowledged empty stream,
and original OS process session. It classifies an abandoned dispatch as unknown
without changing ACK, replaying events, or retiring it. The existing SQL restore
checkpoint allocates the sole candidate and successor generation. A failed
replacement execution is separately proven dead before identity-scoped cleanup;
its stopped status or the old owner's death alone never authorizes cleanup.

After verified native load, existing authenticated submission retirement seals
an empty, fully acknowledged predecessor in the same journal transaction when
all its work is settled and the recovery generation is newer. Same-generation
Resume keeps its stream writable; retained suffixes remain intact. Ordinary
`Journal.RolloverStream` guards remain unchanged. The old ACK, cursor, payload,
and unknown outcome remain readable and unchanged.

The restore-only candidate connects to the authenticated empty successor stream
using its pending checkpoint authority and loads the same saved native
conversation with no initial prompt. Pending authority permits only native
restoration, not normal prompt or queue admission. After native identity and
current owner verification, retire only the exact historical unknown records
using the existing authenticated native-resume mechanism. Retain their unknown
outcome and payload evidence. Commit the successor generation, restored
metadata, gap successor linkage, and resolution of only the matching block
atomically. Publish the complete session snapshot before admitting a new user
instruction. Never derive missing tool or terminal effects from native load.

## Crash convergence, inventory, and rollback

The restart inventory includes unresolved canonical submissions and cursor coverage gaps
even when submission/block metadata is absent. Existing bounded reconciliation
and ownership locks serialize automatic and explicit attempts. The existing SQL checkpoint records attempt and successor identity; its
transitions compare the complete pinned source, including gap evidence. Journal
retirement retains exact submission identity and unknown outcome. Every commit
rechecks the source SQL cursor under the session write lock.

| Crash boundary | Recovery behavior |
| --- | --- |
| SQL gap observed, no journal transition | Revalidate source and peer before allocating the same candidate |
| Candidate allocated, source unknown classified | Reuse the existing pinned SQL candidate; no source retirement before native load |
| Native loaded, historical unknown not retired | Reuse only the verified idle candidate and finish exact retirement |
| Journal retirement done, SQL restore pending | Validate retained retirement/checkpoint proof and commit the same recovery |
| SQL restore committed, publication interrupted | Republish the newer restored snapshot; retain the historical notice |
| Later backend restart | Adopt current successor; old cursors and gap remain historical |

A peer lacks authority to clear a SQL block merely because its stream is empty.
A failed native load leaves the gap and admission fence intact. Retry reuses the
checkpoint. Changed owners, independent blocks, live work, unavailable payloads,
or incomplete proof cannot normalize the mismatch. Old peers expose a typed
unsupported result. Rollback preserves new records and must refuse pending
gap recovery; it must not open the sealed historical stream as fresh legacy.

## Presentation

Use the existing task-chat `session-recovery-card`, bootstrap card, service error
mapping, and revision-fenced store projections. Add a localized missing-output
reason and structured details. Resume remains the existing action; automatic
restoration displays progress without a new form. After success, remove the
current admission card and preserve a historical notice in the conversation:
some earlier output is unavailable and the prior prompt outcome is uncertain.

Desktop and phone share reason and state logic. Reuse the inline phone recovery
card, its bounded internal scrolling, wrapping details, and existing stacked
44px touch actions. Do not introduce a table of sessions or a desktop dialog on
phone. Normal composer and focus behavior follow verified restored state.
All new copy uses localization across the supported seven languages. Missing
sequence bounds belong in expanded diagnostic details, not the primary flow.

## Observability and validation

Structured logs identify the actual stream/generation, missing range, restore
attempt, original execution, and failure category without payloads or tokens.
Reuse bounded existing recovery outcome metrics; do not add IDs as labels or
count unavailable events as projected. Public operations guidance explains the
historical notice, eligibility, unsupported peers, and preserved uncertainty.

Regression coverage uses the observed missing-tail shape (including synthetic
1/6 cursors), actual journal reopen, source and replacement liveness controls,
real repository transactions, existing constructor identity, typed startup
diagnosis, and actual inbox projection. Existing checkpoint regressions cover
allocation, native verification, and publication boundaries. CI desktop/phone
coverage checks another restart followed by a distinct instruction. Browser
desktop/phone validation runs only in CI per the user's host constraint.

## Related decision and delivery

- [Honest delivery loss recovery ADR](../../../decisions/2026-10-11-honest-delivery-loss-recovery.md).
- [Work package](../../../plans/pruned-delivery-recovery/plan.md).

## No-gap historical admission fences

When real SQL/journal coverage agrees, native Resume uses the current generation.
An authenticated journal `interrupted_unknown` may classify an exact SQL
`dispatching` submission only after native initialization and matching immutable
owner/hash evidence. Validate the entire unresolved inventory before changing
any row. Preserve the original payload and unknown outcome; retire only through
the authenticated runtime API before resolving the SQL fence.

A sole empty `agent_delivery` / `unresolved_durable_work` block may resolve after
that acknowledgment even if recovery metadata is absent. A restored snapshot for
an older instruction is historical only when its exact generation, submission,
stream, timestamp, and authorized Resume resolution exist in SQL. Pending or
conflicting metadata, active submissions, independent blocks, unprojected receipt,
or a pending restore attempt keep the fence open. Resolution never creates a
prompt, advances a delivery cursor, or manufactures terminal evidence. Report
an evidence refusal distinctly from an already-closed block.

Restart reconstruction may replace that verified historical snapshot with current
interrupted-work evidence under the existing source-death, journal ownership,
revision, payload, and sole-block transaction guards. Its revision advances;
resolved blocks retain historical proof. This does not invoke the pruned-gap
fallback or allocate a successor when current native continuity is usable.

Legacy older restored notices can postdate their recorded resolution and retain
a stream ID while the resolved historical block lacks one. This narrower legacy
proof requires the exact older session/incarnation/generation/submission, a Resume
resolution before the notice, SQL unknown state with no terminal, and the same
saved native conversation. It applies only to historical replacement or generic
fence settlement after full authenticated retirement; it does not relax current
restored-admission verification. Pending metadata and foreign work remain blocked.
