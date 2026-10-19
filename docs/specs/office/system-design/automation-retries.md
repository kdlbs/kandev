---
status: draft
system: office
requirements:
  - REQ-OFFICE-AUTOMATION-RETRIES-001
created: 2026-09-29
owners:
  - kandev
---

# Automation Retry System Design

## Purpose and boundaries

The Office automation service owns retry policy, durable retry-group identity, attempt scheduling, operation recovery, and retry-history projection. The existing task/session system owns task execution and session lifecycle; this design records the exact task and turn bindings needed to reconcile automation runs but does not replace those contracts.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-AUTOMATION-RETRIES-001` | [Data and contracts](#data-and-contracts), [Frontend editor behavior](#frontend-editor-behavior), [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery), [Persistence](#persistence), [Security](#security) |

## Components and responsibilities

- `internal/automation` validates and persists `RetryPolicy`, admits retry groups and attempts, claims due work, projects history, and coordinates cancellation.
- The automation lifecycle worker periodically discovers due automations, obtains a durable run claim, and dispatches the attempt through the existing automation service.
- Retry admission and task operations persist the intent before creating a task. The operation ledger makes task creation recoverable across process interruption.
- The WebSocket/API handlers expose retry policy, group controls, and bounded history to the web client.
- `retry-policy-section.tsx` edits the policy and history mode. The automation editor owns target-aware visibility and resetting; the retry-mode selector shares card-state styles with the other automation radio-card sections.

## Frontend editor behavior

When the editor selects or loads an automation with `managed_conversation`,
the draft retry policy is normalized to the canonical disabled values and the
entire retry section is hidden. The create/update payload builder also emits
that canonical disabled policy for managed targets, including when stale
enabled values came from an older saved automation. Switching back to
`automation_run` or `normal_task` reveals the disabled policy; discarded
settings are not restored.

For an existing managed automation with a non-disabled persisted policy, the
editor presents the disabled draft but retains the persisted policy in its
dirty baseline. The reset therefore remains visibly unsaved until the operator
saves; loading the editor does not write to the API.

Retry-mode radio cards share the selected primary border/background and
unselected neutral-border/hover treatment used by Context between runs and Run
destination. Their RadioGroup semantics remain unchanged. On phones, the cards
stay full width and at least 44 pixels high; selected styling remains the touch
feedback, while hover is desktop-only.

This is an editor and payload contract. It adds no API shape or backend
dispatch change and leaves managed conversation delivery retries unchanged.
Existing managed records keep their saved non-disabled policy until Save, so
current generic retry admission may create retry groups from that policy before
then. Loading does not auto-write configuration or block this backend path.

## Data and contracts

- `RetryPolicy` stores mode (`disabled`, `finite`, `infinite`), canonical decimal `max_retries` and `delay_seconds`, backoff (`fixed`, `exponential`), and history mode (`attempts`, `timeline`). Disabled mode normalizes retry-only fields to fixed, zero-delay, attempts mode. Finite mode requires a retry count of at least one. Missing policy defaults to disabled.
- A `RetryGroup` identifies the original firing and carries the current generation and state. Each `AutomationRun` attempt carries its group ID and generation, attempt number, policy snapshot, trigger snapshot, launch snapshot, and exact `retry_scheduled_at` value where scheduled.
- Retry task intents and operation rows persist the expected task-creation action before side effects. Retry claims, operation leases, and outbox leases use database state and expiration rather than process-local ownership.
- Task creation and provider launch are separate ownership boundaries. A committed task ID is not an accepted launch identity. Launch ownership must cover immutable session/turn preparation, provider acceptance, queued starts, and recovery; an expired owner cannot authorize a second launch when the first acceptance is uncertain. Task 02 implements and verifies this protocol.
- Exact binding transitions an unbound run once. A replay with the same task/session/turn is idempotent and preserves the original history disposition; conflicting identities are rejected at the SQL update boundary.
- Retry-history responses are bounded and use an opaque continuation cursor plus a high-water mark. The service supplies enough group and attempt data for either view without making the client infer ownership from adjacent rows.
- Generic webhook deliveries require `X-Webhook-Secret` and `X-Kandev-Delivery-ID`. The delivery ID is unique per automation receipt. Retry snapshots retain the legacy payload when no safe pointers are configured; interpolation, dedup-key, and repository-selector paths resolve against that payload. When pointers are configured, only selected, bounded values are retained. The whole webhook body is limited to 1 MiB.

## Control flow

1. Policy updates are normalized and persisted with the automation. Switching from disabled to finite supplies a valid retry count before saving; disabled mode hides or locks retry-only controls. Selecting or loading a managed-conversation target resets the policy to canonical disabled values and hides the retry section; editor create/update payloads preserve that reset. Changing back to a retry-capable target does not restore the discarded values.
2. A failed eligible attempt advances its retry group generation transactionally, records the next attempt and its exact due timestamp, and writes any required publication intent to the outbox.
3. The lifecycle worker scans due work at a bounded cadence. A database claim serializes instances. Capacity deferral releases the claim with a later due time instead of repeatedly retrying at the scheduler tick rate.
4. Admission persists a task intent and operation before invoking task creation. Completion binds the resulting task/session/turn to the exact run and acknowledges the matching outbox lease.
   Event consumers resolve run-only events through the persisted run and launch snapshot before selecting a destination. Supplied event routing fields do not replace the snapshot's owner or task mode.
5. Recovery reads pending or expired work, acquires a time-bounded lease, validates the current group generation, resumes only the durable operation still owned by that lease, and releases or acknowledges only with the matching token.
6. The web client requests bounded history pages. Timeline rows are projected from complete retry groups before status filtering and counts; deleting a visible group expands to all underlying attempt IDs. Standalone runs remain in the ordinary run history.
7. Refresh follows the next scheduled retry due time and stops when no visible retry is pending. Cursor traversal tracks every seen cursor and has a finite page bound.
   Mutation reconciliation carries the selected history mode. A mode change registers a new request even while the previous mode is loading; the existing latest-request token rejects the older response. Stop uses an immediate per-store submission guard, exposes busy state, and catches failures for localized reporting.

## Failure and recovery

- A transient provider failure creates a retry only while the group generation and policy permit one. Replayed failure signals return the already-created next attempt rather than duplicating it.
- A live retry-operation lease is not reclaimed during startup. Expired operation and outbox leases can be reclaimed by another instance; token and generation checks reject stale workers.
- If task creation succeeded before a crash, recovery resolves the existing task through the durable intent/idempotency identity instead of creating another one.
- A failed stop leaves the run and its ownership records available for retry or reconciliation. Disabling an automation fences new admission before it attempts to stop currently open runs.
- Capacity exhaustion schedules a later retry without consuming the attempt or making a tight claim/deferral loop.
- A history request failure is surfaced to the user. It does not erase prior rows or loop forever on a repeated cursor.
- Permission requests that cannot be answered because the original turn no longer exists terminalize the run through its task binding rather than leaving an admitted run open.

## Persistence

Retry policy and lifecycle state live in the automation tables, runs, retry groups, task intents, operation ledger, and retry-event outbox. Migrations establish indexes for due retries and group/attempt identity on both supported SQL dialects. Indexed queries preserve PostgreSQL placeholder rebinding and context cancellation. Recovery is restart-safe and works across multiple backend instances; no process-local scheduler state is authoritative.

## Security

Webhook secrets are compared before accepting a receipt and are never copied into retry snapshots or history. Delivery identifiers and webhook bodies are bounded. Configured JSON pointers constrain which payload values enter retained retry input. Retry APIs authorize the automation in its owning workspace, and the client cannot supply task, generation, lease, or retry-group ownership identities.

### Launch admission and compatibility enforcement

The launch protocol has separate admission-version and immutable launch records. New admissions mark protocol version 1 in the admission transaction; scheduled children inherit that version. Rows created by older code have no such evidence. A version -1 receipt denotes a verified stop of the legacy exact binding; it authorizes cancellation, never launch or accepted identity adoption. They remain uncertain without automatic launch, because an older task-only commitment may hide an accepted prompt. The version records have explicit SELECT/mapping boundaries and join the required-store catalogue.

Reservation projects one task/session/turn tuple before external launch, retaining the original action/reason, and records a stable submission ID. A non-expiring preparing claim transfers only into dispatching, accepted, queued (definitively deferred before provider admission), uncertain, or stopped. Recovery never converts preparing/dispatching/uncertain to replay permission. Queue release claims the same tuple with a fresh owner token. Acceptance can reconcile the original tuple after cancellation, but cannot change it or reopen a stopped launch.

Persistence guards on session and turn insertion constrain older entrypoints as well as the current orchestrator. All active retry rows are constrained: version-1 work permits only its reserved IDs, and unresolved legacy work rejects new session or turn IDs. Persisted retry task bindings and fully bound session/turn/action/reason fields are immutable at the database boundary, including writes from older code. PostgreSQL guards serialize with the automation row lock; SQLite uses its writer serialization. Store-only fixtures without core task tables cannot establish that compatibility capability and must fail closed at launch reservation. Existing session/turn evidence must be checked before reserving a fresh identity, rather than ignored after a crash.

The alternative of checking version/state only in the new Go dispatcher leaves an older dispatcher free to launch after an outbox lease expires. Extending lease duration has the same failure window and cannot resolve a crash after acceptance. Provider submission IDs are useful correlation, not a universal exactly-once promise. The selected constraint keeps uncertain work visible and retains its capacity until exact accepted/terminal evidence or a definitive stop resolves ownership.

Generic provider retry, created-session start and resume paths refuse an active automation-owned attempt unless they carry its claimed launch context. Continuation of an already prompt-ready execution carries the reserved submission and turn through guarded admission. A continuation requiring cold runtime recovery remains pending and cannot automatically start a new provider process while launch acceptance is unresolved.

A stop request fences admission first. The ordinary coordinator stop acknowledges logical cancellation and schedules teardown; that acknowledgment alone does not release launch ownership. The automation-specific stop waits for termination of the observed execution ID. A network error, absent local execution, or changed session/turn remains uncertain. Exact authenticated turn-complete evidence can settle the matching launch without interpreting session-state transitions as provider termination. These guards do not retroactively prove what an older instance already submitted before persistence evidence was installed.

Task metadata records the admitted run owner at task creation. Session and turn INSERT guards associate both the run binding and that persisted metadata in SQLite and PostgreSQL, closing the interval before task binding commits. Generic starts also fail closed in that interval. A protocol version of 0 records that a current admission encountered an existing session without sufficient launch evidence; it cannot be automatically relaunched.

Legacy attempts with unresolved acceptance also retain a canonical session recovery block. This closes the older warm-continuation path that reuses an existing turn instead of inserting one. The block uses the persisted session incarnation and current harness generation, carries no invented delivery or acceptance identity, and is installed/backfilled atomically with the compatibility guards. Only definitive terminal/stop evidence or an explicit authorized recovery action can release ownership; an automatic recovery does not resolve the block. Historical effects before guard installation cannot be retroactively certified.

### Retired admission compatibility

Run-history deletion and a known-execution stop do not revoke a separate delayed worker by themselves. Persist retry admission owner tombstones independently of history rows, including the task owner acquired during binding and the run owner carried in late task metadata. Session and turn INSERT guards must consult this retained evidence after cancellation, supersession, terminal settlement, and history or automation deletion. Only a matching live reserved tuple may admit an automatic creation. Preserve explicit manual creation through a server-owned request-origin context authorizing the exact creation ID inside the repository transaction; neither task metadata nor inherited session metadata grants that exception. The transaction inserts a temporary exact-kind/task/creation permit, performs the guarded INSERT, and removes the permit before commit; failure rolls both back. Older dispatchers cannot inherit a permit from product metadata. Tests must cover a delayed older PrepareSession, late task creation with automation_run_id after stop/delete, unrelated ordinary creation, explicit manual session/turn creation, and a new independently reserved retry on a reused task. Retained tombstones carry ownership and revocation only, never fabricated provider acceptance or termination.


Control revocation and ownership settlement are separate transactions. Stop, disable, and deletion first revoke the affected live group's generation, future outbox publication, and any prepared/queued admission. Scheduled or proven unstarted work may settle immediately; preparing, dispatching, accepted, or unresolved legacy work retains its run and exact ownership evidence until definitive stop/terminal evidence. A failed control therefore cannot revive an earlier retry chain when enablement changes later. Final history/automation deletion still requires resolved ownership.


Unbound recovery records unresolved acceptance visibly instead of interpreting a missing local execution as termination. Revoked task-operation state retains its observed tuple; a definitive legacy stop receipt must match that tuple and generation and cannot overwrite a conflicting retained owner. Explicit conversation forking observes pending retry ownership before provider effects and uses the same exact transactional manual-creation authority after settlement.


Persisted task-operation tuples also associate an unprojected retry with its task. Operation writes and schema backfill capture that task in the retained admission tombstone and quarantine existing legacy sessions, even when task metadata and the run's binding projection are absent. Stop lookup may hydrate the observed operation tuple without persisting accepted launch IDs; conflicting partial run bindings fail closed. Definitively acknowledged synchronous teardown completes only the matching execution-cleanup claim.


The final main integration includes the normally merged profile-ordering fixture repairs and durable delivery reconstruction/retention. Retry initial launch binds its pre-reserved canonical `prompt:retry:` submission identity and consumes its single-use launch claim before harness start. Lifecycle can then prepare context and materialize attachments. Immediately before prompt dispatch, its typed admission callback revalidates the exact retry owner and persists those final text and descriptors under the same immutable submission identity. Previously admitted payloads are never rewritten; a changed payload or revoked owner rejects dispatch while ownership remains retained. Commandless workspace promotion consumes admission only after command configuration. The shared resume entry point fences both ordinary and reserved-capacity recovery, while explicit Resume, Fresh Start and Continue from History reject unresolved retry ownership before settlement or continuation checkpoint mutation. State-only delivery reconciliation retains main's authenticated identity checks and does not authorize a new retry launch.


Silent restart recovery also treats a retry continuation on an ordinary-origin task as automation-owned. Initial owner eligibility and each owner revalidation fence that session before source cleanup, queue pause, candidate allocation or checkpoint advancement. Candidate allocation revalidates ownership independently. The ordinary non-retry path remains eligible. Exact owner-method proofs with dependency fakes fail without these guards and pass with them; the permanent real-repository regression asserts source execution, queue and checkpoint preservation. Full package and race execution remain hosted-CI work.
