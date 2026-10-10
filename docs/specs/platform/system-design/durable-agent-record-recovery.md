---
status: draft
system: platform
created: 2026-10-09
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-008
---

# Recovery of missing durable delivery records

## Ownership and dependencies

Platform owns delivery reconstruction and its recovery presentation. SQL remains
the canonical conversation store. Agentctl supplies retained transport evidence.
Tasks and Agents retain task ownership, workspace identity, and native history.

This design extends [reattachment](durable-agent-reattachment.md), including the
explicit continuation operation introduced by PR #4380. It does not implement
another continuation engine. The implementation package pins that dependency.

| Requirement | Design boundary |
| --- | --- |
| REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003 | Reconstruction never admits the old instruction |
| REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006 | Normal reconciliation, preserved causes, persistent notice |
| REQ-PLATFORM-DURABLE-AGENT-DELIVERY-008 | Evidence inspection, conditional import, compatibility, visible results |

The [record reconstruction decision](../../../decisions/2026-10-09-retained-delivery-record-reconstruction.md)
records the trust boundary and rejected alternatives.

## Entry points

`RetrySessionDelivery` gains a missing-record branch before returning
`missing_canonical_submission`. It runs only for an authorized task/session pair
with an unresolved delivery cause or authenticated retained-work evidence.

Session inspection and prompt rejection project the existing block into a
persistent notice, even without `AgentDeliveryRecovery`. They do not import
records, start a provider, or scan journals for other sessions.

The retry operation can reconstruct records and reconcile output. It cannot
submit either the old instruction or a previously rejected new instruction.
Normal admission and queue guards remain closed during this operation.

## Authenticated evidence inspection

Add a typed inspection operation through `runtime.Runtime`, lifecycle, and the
agentctl clients. Do not make orchestration import lifecycle implementation types.

For a retained live instance, use its current authenticated runtime lease and
instance binding. For an absent instance, extend the retained-journal control
path from PR #4380. Resolve storage from the server-owned session association.
Do not accept a browser-supplied path, execution identity, or payload.

The inspection returns one bounded snapshot with a descriptor, candidate
summaries, and completeness information. Reuse `RecoveryDescriptor` and its
16-summary bound. A truncated result is blocked, not a partial selection.
After unique selection, fetch only that submission's immutable payload through
a backend-only operation, limited to the existing 1 MiB submission ceiling.
The ordinary descriptor and browser response remain free of payloads.

The absent-instance reader preserves `ExistingOnly`, owner-marker, lost-marker,
regular-file, same-file, and exclusive-lock checks from `ReadRetainedRecovery`.
It must not create a journal or steal a live instance's file lock.
Inspection does not acknowledge events, prune records, retire submissions, or
start an instance. A locked journal returns an actionable unavailable result.

Use the existing 30-second explicit retry deadline. Revalidate the runtime lease
and evidence identity after payload fetch. A changed snapshot requires another
bounded read or a stale-evidence result, never an unconditional import.

## Candidate and evidence rules

Consider every recovery-relevant submission in the owner snapshot, including
unresolved records from earlier generations. Completed or retired history does
not count as active work. Another unresolved candidate blocks this operation,
even when exactly one candidate matches the current generation.

Require exactly one candidate for this repair. Do not choose by timestamp,
array order, recent message, model, or provider name. Preserve the initial-prompt
special case in `durable_adoption.go` until it passes these same proofs.

| Evidence | Required authority and comparison |
| --- | --- |
| User authority | Existing task/session recovery authorization, archive and Office restrictions |
| Session and workspace | Existing SQL task/session and generation workspace binding |
| Incarnation and generation | Existing SQL generation equals journal owner and selected submission |
| Stream | Descriptor and submission agree with the exact SQL cursor, when present |
| Submission association | Exact saved user-message identity or existing inbox/effect binding for this submission and owner |
| Initial submission | Verified deterministic initial ID plus its persisted session/generation association |
| Payload | Original retained bytes with matching `journal.SubmissionHash`, valid bounded envelope, and matching summary hash |
| Turn and prompt generation | Existing message/turn, queue receipt, or correlated inbox evidence, never the newest open turn |
| Original process | Verified historical process identity or authenticated surviving execution evidence, evaluated separately |

A cursor alone proves a stream association, not the selected prompt or process.
An absent cursor is valid only when replay can start at sequence one. An expired
cursor or a database read error remains blocked. A fully acknowledged stream
can still contain an unresolved prompt and must remain eligible for inspection.

Do not infer prompt generation from harness generation. Do not invent an
execution ID from the current resumed instance or treat a missing process as dead.
The incident can include an idle replacement instance in the same generation.
Observe that instance separately from the process that executed the old prompt.

Missing historical process proof does not prevent a proven submission record
from being restored. It does prevent process-sensitive reconciliation or
continuation. Represent incomplete control identity explicitly in the recovery
projection. Do not fill mandatory identity fields with placeholders to satisfy
`completeAgentDeliveryRecoveryIdentity`.

A tracked execution can supply process proof only when its bound client and
current prompt match the selected session, incarnation, harness generation,
stream, and submission. Retry checks this identity again after reading the
payload. The prompt generation must have been confirmed as dispatched through
that execution. An idle execution, a successor prompt, or a recreated generation
supplies no historical proof.

## Atomic reconstruction

Add a dedicated task repository operation, tentatively
`ReconstructAgentDeliverySubmission`. It receives validated evidence and an
observed session/generation/block snapshot. It is not a general import API.

Within one transaction:

1. Lock the session before an absent-submission read. PostgreSQL uses a scoped
   transaction advisory lock and row locking. SQLite uses its writer transaction.
2. Recheck session, incarnation, generation, workspace, observed execution,
   recovery revision, and independent admission guards.
3. Check all existing unresolved backend submissions. Reject competing owners
   and conflicting records, including mixed live and historical rows.
4. Insert the missing submission using the retained ID, original bytes, hash,
   owner generation, and timestamps. Record bounded reconstruction provenance.
5. Persist the recovery projection and bind only the matching delivery block.
   Commit all three records together, or preserve all previous state.

No new table is planned. Use existing submission, recovery metadata, and block
storage. Extend typed models and repository contracts where necessary. Any
required column needs the normal additive migration and store conformance tests.

An identical concurrent import returns the existing identity and revision.
It does not update timestamps or advance recovery merely because Retry repeats.
A conflicting row returns a typed conflict without overwriting any field.
An import racing deletion, archive, generation change, Stop, or successor
admission loses its conditional write.

Retain the observed journal state in provenance. A dispatching or accepted old
record has no confirmed terminal outcome. Its imported backend state must keep
automatic dispatch blocked. Never turn it into a new prepared queue candidate.
Only normal reconciliation can establish completion or cancellation.

The exact provenance stores the source owner tuple, submission hash, observed
state, and reconstruction time. IDs stay in protected storage and structured
diagnostics. Payloads never enter generic logs, errors, or metrics.

## Reconciliation and continuation

After import, reload the committed identity and use the existing reconciler.
Terminal evidence uses the normal projection and settlement barriers. Repeated
retry must not duplicate messages, effects, queue release, or completion.

If control identity remains incomplete, return a persistent blocked result with
a specific cause. Imported records alone do not set `allowed_actions`.
Each explicit Retry inspects incomplete records again. New verified proof can
advance the same association by one revision in a conditional transaction.
The transaction requires the observed incomplete revision and preserves the
immutable payload and block association. Complete identities cannot be replaced
through reconstruction. The reconciler checks termination after this commit.

If the original process is alive, retain attachment/Stop behavior and prohibit
a replacement prompt. If verified termination and native identity permit
continuation, use PR #4380's `ResumeInterruptedSession` unchanged in authority:
explicit acknowledgment, observed recovery revision, one new instruction, and
the existing idempotency key. Preserve the old uncertain outcome.

A saved instruction rejected before provider admission is not the old journal
submission. Keep its message and mark its delivery as blocked. Do not bind it to
the interrupted prompt, use its orphan turn as historical evidence, or resend it
as a side effect of Retry. Only a later explicit user action admits new work.

## Presentation and compatibility

Use the existing recovery card, `session.recover` result, and session projection.
Before a canonical recovery identity exists, identify the notice by the durable
block ID, incarnation, generation, and revision. Keep that identity distinct from
PR #4380's complete continuation identity.

After reconstruction, replace that notice only with the matching committed
recovery result. Discard late responses after task, session, block, or generation
changes. Browser reload derives the notice from persisted state.
Persisting a recovery block also publishes its current projection through the
session state event. Block creation and resolution advance the session timestamp
used to reject stale notifications. An omitted projection preserves the client
state; an explicit empty list clears it.

Use bounded reason codes for incomplete evidence, ambiguous candidates,
identity/hash conflict, unavailable retained storage, and missing process proof.
Reuse existing codes where their meaning matches. Extend the typed client,
public reason mapper, and all seven locale catalogs together.

The mobile exemplar is `components/task/recovery-actions.tsx` with PR #4380's
inline continuation form. Keep the chat as the only scroll owner. Desktop uses
an aligned action row. Phone uses stacked actions with at least 44px targets.
No new overlay or navigation is needed for this infrequent recovery action.
Progress and results use an accessible status region. Controls remain usable
with the phone keyboard and safe-area clearance.

An older peer without inspection support returns a specific blocked result.
Neither missing capability nor malformed evidence permits legacy fallback.
No runtime feature flag is added. Local retained storage is the first supported
absent-instance path. Remote executors require equivalent authenticated storage
and process evidence, otherwise they retain their existing blocked behavior.

## Verification and delivery

Use real journal and SQL fixtures for missing records, atomic failure windows,
concurrent retries, and backend restart. Include an already acknowledged stream,
completed history beside one unresolved candidate, and a conflicting second owner.
Keep fixture payloads synthetic and limited to the selected test session.

Desktop and phone tests must exercise the actual retry/import operation and
count provider prompts. Metadata-only seeded banners do not prove reconstruction.
The [work package](../../../plans/durable-agent-record-recovery/plan.md) defines
the exact commands, dependency snapshot, test matrix, and delivery exclusions.
