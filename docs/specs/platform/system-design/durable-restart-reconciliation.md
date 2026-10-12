---
status: current
system: platform
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-001
---

# Durable restart reconciliation system design

## Ownership and boundaries

Platform owns journal consistency and recovery evidence. Lifecycle owns authenticated
execution control; orchestrator owns existing admission, turn, and workflow authority.
This supplements [durable delivery](durable-agent-delivery.md),
[reattachment](durable-agent-reattachment.md), and
[explicit native recovery](interrupted-prompt-recovery.md).
It introduces no new recovery UI, prompt authority, storage format, or endpoint.
Existing source-process death, native identity, current owner context, and independent
admission checks remain required before a restore-only operation.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001 | Mapping-safe capacity accounting |
| REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003 | Abandoned dispatch classification; no redispatch |
| REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005 | Compare-and-set settlement and projection |
| REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006 | Existing restore-only checkpoints and explicit Resume |
| REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-001 | Automatic inconsistency repair and coherent recovery state |

## Mapping-safe capacity accounting

The journal lifetime lock excludes Close and compaction; it does not exclude concurrent
bbolt write transactions or their mmap replacement. Runtime capacity reads must not call
`DB.Info()` outside protected initialization. Capture the database page size before publishing
a newly opened handle and retain it as immutable handle metadata. Refresh that metadata
under exclusive journal ownership on every compaction reopen, including rename failure.
Runtime reusable-page accounting combines that value with bbolt's synchronized `DB.Stats()`.
Do not retain `Info.Data`, assume the operating-system page size, or add a nested transaction
while holding disk-reservation locks. Existing pending reservations, overflow guards, file
growth charges, and recovery reserves remain authoritative.

## Abandoned dispatch after owner replacement

The process-manager constructor opens its owner journal before starting a harness or accepting
dispatch. At that boundary, exclusive journal ownership and a newly constructed dispatcher
permit classification of its retained, abandoned `dispatching` records as `interrupted_unknown`.
Validate session, incarnation, stream, and eligible generation across the complete affected
record set before changing it. Preserve payloads, hashes, terminal evidence, retirement flags,
and timestamps that describe historical admission; record only the new classification time.
A failed classification keeps durable admission unavailable. Ordinary journal reads and
read-only diagnostic opens do not perform this operation.

Classification is not retirement or proof that an external harness or tool stopped. Backend
reconciliation still requires exact process ownership and termination evidence before native
restore. A retained live manager is reattached and never classified by a replacement-manager
path. Prepared/accepted records and terminal outcomes retain their separate admission rules.
No existing submission is automatically dispatched because its journal was reopened.
A restore-only successor can read earlier-generation unknown history from the same session
and incarnation. It preserves that history until authenticated native restoration retires it;
an earlier dispatch can be classified only when its retained stream record proves the same
session, incarnation, and historical harness generation. Foreign streams remain blocked.

## Automatic repair of provable restart inconsistencies

Use the same bounded reconciliation worker at backend startup and after confirmed local
shared-runtime replacement. Its inventory must include retained unresolved delivery records
and execution identities even when an older row lacks current recovery metadata. A typed
retired-runtime lease triggers identity reconciliation; it is not independently proof that
the original process terminated. Remove a stale runtime handle only after existing exact
process/epoch evidence and compare-and-set guards establish that it is obsolete. Preserve
the saved native token and workspace. Never contact a successor through a recycled port.

For each candidate, compare all unresolved SQL submissions with the authenticated owner's
retained journal records. Match ID, session, incarnation, harness generation, stream binding,
and payload hash. A freshly constructed journal owner classifies abandoned dispatches as
described in [abandoned dispatch classification](#abandoned-dispatch-after-owner-replacement).
With confirmed original-process death, matching SQL `dispatching` may become uncertain;
SQL uncertainty and matching peer uncertainty can proceed through the existing restore-only
checkpoint. Retained terminal evidence instead uses ordered projection and authoritative
terminal settlement. Missing canonical associations use the existing record-reconstruction
contract. Conflicting hashes, future/foreign generations, incomplete inventories, unavailable
stores, and unknown process ownership permit no destructive normalization or native restore.

SQL repair uses the existing session admission/lifecycle locks and repository transactions.
Journal repair remains owner-scoped and durable. No cross-store atomicity is assumed: pin
identities, use compare-and-set writes, and re-read both stores at each checkpoint. A crash
after journal classification or retirement resumes from retained proof. A single bad candidate
does not abort other sessions. Startup and replacement triggers must share bounded ownership
and cancellation rather than start competing restore workers.

When a recovery snapshot exists, both automatic restoration and successful explicit native Resume must commit a newer
`restored` recovery revision with the matching recovery-block resolution. Keep the original
recovery identity as historical evidence, and preserve its submission's unknown outcome.
Guard this SQL commit against a new execution, generation, recovery revision, or independent
block. Clear only the exact associated uncertainty error. Publish the complete recovery
snapshot, including the cleared block projection, after commit. Repeated recovery reads the
committed proof instead of returning success while leaving `phase=uncertain` or recreating
the historical block. Existing frontend revision fences prevent delayed old snapshots from
reviving the banner. Current desktop and phone composer layouts require no new controls.

Verification seeds SQL uncertainty versus journal dispatching, a dead runtime row, stale
uncertain metadata after successful Resume, and a mixed set containing one good and one
conflicting unresolved record. It exercises actual journal reopen and authenticated adoption,
backend restart and in-process runtime replacement, crash boundaries between store writes,
normal follow-up admission, no interrupted-prompt replay, and an independent live sibling.

Before durable dispatch, the existing executor prompt/turn binding also persists the
original runtime process birth identity and admitted prompt generation. A replacement
execution may retain that historical binding, but its own PID, runtime epoch, and prompt
counter never stand in for the source. Reconstruction rechecks the exact saved binding
around payload inspection. Legacy bindings without process proof remain blocked.

The restart inventory includes matching dispatching SQL rows without recovery metadata.
The worker reconstructs a uniquely identified recovery block and source snapshot, including
an existing generic delivery block with empty identities, then proves the original owner
terminated before restoring or retiring work. If an idle successor already loaded the exact
native conversation at the same generation, lifecycle validates its owner and every unknown
SQL/peer record, retires matching history, and atomically resolves the block with the restored
snapshot. It preserves that execution. An active, foreign, or unverifiable successor is never
stopped or taken over. A later restart reads exact resolved native-resume proof and admits
only a new instruction.

A prepared restore checkpoint may refresh a strictly newer reconnecting/uncertain notice
revision only while it has no allocated candidate and all immutable source, block, native,
and workspace identity remains identical. Once a candidate is allocated, the entire source
snapshot is pinned. Notice delivery cannot change an in-flight candidate's authority.

The delivery record for this correction is
[journal growth and restart reconciliation](../../../plans/journal-growth-restart-reconciliation/plan.md).

Recovery block updates preserve a known stream binding when a later partial failure notice
names the same submission without a stream. Existing incomplete blocks can be repaired only
after authenticated full journal/SQL comparison and original-owner termination proof. The
transaction pins the current notice, native generation, observed block timestamp and bindings,
and the sole open admission fence. Foreign bindings and independent blocks remain unchanged.
The production lifecycle adapter exposes the already-loaded-native recovery seam explicitly.

Native identity and recovery-snapshot verification use the authorized session detail read.
Compact navigation rows omit rich metadata and cannot prove native continuity or restoration.

Explicit recovery shares the session infrastructure slot with workspace readers. If it joins
a reader that rejects unresolved recovery, it reacquires that slot and performs its own
launch admission. It never inherits the reader's recovery rejection as the outcome of a
native restore. Ordinary launches, unrelated failures, cancellation, and failures from the
recovery's own launch remain authoritative. Workspace admission stays blocked until the
matching recovery commits its restored snapshot.

Recovery attempts are scoped to the interrupted submission and original execution as well
as the session, incarnation, and harness generation. Retries of one incident reuse its
checkpoint; a later incident in an already loaded conversation gets its own identity.
Existing generation-scoped checkpoints remain authoritative for their matching source.
Historical SQL unknown outcomes may be excluded from unresolved work only after an
authenticated, owner-scoped journal read proves that exact submission retired with the
same hash, incarnation, and generation. This read omits the prompt payload and never
acknowledges events. Missing-metadata reconstruction carries the verified immutable
canonical snapshots into its write transaction; changed or live historical rows block it.
