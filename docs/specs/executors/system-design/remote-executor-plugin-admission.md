---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-PLUGIN-ADMISSION-001
  - REQ-EXECUTORS-PLUGIN-PLACEMENT-001
  - REQ-EXECUTORS-PLUGIN-PLACEMENT-002
---

# Remote executor plugins: admission and placement system design

## Purpose and boundaries

Two launch-path mechanisms plus the refusal path, all on the one ledger: the `plugin_executor` envelope in `executors_running.metadata`, its
revision CAS, and the existing recovery claims. Capability gating comes from the [versioning design](remote-executor-plugin-capability-versioning.md).
Names marked **new** do not describe existing APIs. Core keeps admission decisions, reservations, and cancel
admission; the provider keeps placement, queueing, and worker choice.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| REQ-EXECUTORS-PLUGIN-ADMISSION-001 | Admission context and hook |
| REQ-EXECUTORS-PLUGIN-PLACEMENT-001 | Pending placement |
| REQ-EXECUTORS-PLUGIN-PLACEMENT-002 | Refused provision |

## Shared environments (cut)

Not designed in this cycle. `validateLaunch` keeps rejecting `req.WorkspaceReuseRequired || isPluginExecutorSharedWorkspace` exactly as today (AC-EXECUTORS-PLUGIN-002.2), and no `workspace_sharing` capability exists. The cut scope, the reason, the starting defect list, and the K5-first rule are in the Out of scope entry of the [requirements](../requirements/remote-executor-plugin-admission.md); the follow-up is card 99beef6d-530e-4afd-949b-1424fc9245df. Adding M1 later changes the contract again: a new capability, new envelope fields and owner rows, and amendments to the reservation-release rule and to destroy and cleanup.

## Admission context and hook

**New** `internal/agent/runtime/lifecycle/admission.go` defines `AdmissionHook` with `Admit(ctx, AdmissionRequest) (Decision, error)`
and `Release(ctx, ReservationID)`. A `Decision` is allow (with an optional `admission_scope` string and a `limit`) or deny with a stable reason; the hook never counts, the host does. A decision is valid only as AC 001.6 states (exactly one of allow and deny, reason `[a-z0-9_.]{1,64}`, scope `[a-z0-9._:/-]{1,128}` with an integer limit, no accompanying error); anything else is handled as "unavailable".
`Release` only informs the hook after the host released (best effort, failures logged); it is never sent when the reservation write failed. The only implementation in core is `defaultAdmissionHook`, which allows everything with an empty scope (no reservation); a quota policy is host code installed at wiring through **new** `Manager.SetAdmissionHook` and never a plugin, and tests use a scripted hook. A provider that does not require `admission_context` skips `admitLaunch` entirely and the hook is not called (AC 001.6).

**The shared gate.** **New** `admitLaunch` is the one gate function (capability gate, `profile_too_large` measure, hook) and has two callers. `validateLaunch` calls it on the first-launch path, after the capability gate and before any provider allocation call; it keeps its rejection of `req.WorkspaceReuseRequired || isPluginExecutorSharedWorkspace`, which is exactly the condition under which `CreateInstance` routes to the resume entry, so `validateLaunch` is never reached for a resume. `attachRetainedPluginExecutor` (executor_plugin_retained.go) is the resume entry; it runs before `validateLaunch` on main and calls `admitLaunch` directly (AC 001.3). `admitLaunch` takes the negotiated set as an argument: first launch passes the set computed from the current manifest, resume passes the set recorded in the envelope. So at a resume: the capability gate checks the recorded set, the `profile_too_large` measure runs only when the recorded set is non-empty (PROFILE 001.8), and the hook runs only when the recorded set contains `admission_context`. An environment from before this capability has an empty recorded set and no identity snapshot, so it never reaches the hook and resumes exactly as today. A test enumerates the callers of both entries. Recovery of an already admitted environment and the continuation of a pending placement do not call the gate; the continuation keeps the reservation taken at the original launch.
Inputs: the priority, project and step IDs come from the sources of AC 001.10. The order is fixed: capability gate (pure), `Admit` outside any transaction with a 5 s deadline, then one transaction that takes the scope lock, counts, and writes the reservation with the operation. If that transaction fails for a store reason the launch fails with `admission_unavailable`, nothing is allocated, and no `Release` is sent (AC 001.4).

**Atomicity.** The reservation is inserted in the same transaction that creates the operation record, keyed by an
`admission_scope` string (named by the hook and validated by the host). SQLite's immediate-mode writer serializes it.
PostgreSQL takes `pg_advisory_xact_lock(hashtextextended('plugin_admission:'||scope, 0))`, a key derived by the host from the validated scope, before reading the count of live reservations. A new launch over the limit, or a limit of zero or less, is `admission_denied` (`scope_full`). A reservation
is released exactly once by a `released` flag in the same write as the transition that confirms absence (destroy confirmation, which also covers terminal failure; provider `cancelled`; a refusal that allocated nothing), never before it. Hook failure on a first launch fails closed with `admission_unavailable` (AC 001.6).

**Context.** The request fields are built from host records only, by a function that cannot read names, emails, or
credentials: it receives opaque IDs and the priority. The provision and recovery requests gain additive `admission` messages. There is no label, resource class, or applied class (cut, see the requirements' Out of scope and card 11c92feb-2b4c-4d05-8318-9aff24e2fa10). The identity snapshot (task, session, step, user, workspace, project, priority) is written into the envelope at admission and carried unchanged on recovery and resume (AC 001.12).

**Resume.** A resume keeps the reservation it already holds (taken at first launch, released only at confirmed absence or provider `cancelled`, never at Stop). It never takes, counts against, or locks the scope (AC 001.9, 001.11), so a resume racing a new launch has the same outcome in either order: the resume goes ahead, and only the new launch can be refused `scope_full`, because it counts the live reservations including the resumed environment's. Outcomes (AC 001.11):

| Resume case | Outcome |
| --- | --- |
| Recorded negotiated set lacks `admission_context` (including every environment from before this capability) | no hook; resume as today |
| Hook allows | resume goes ahead; recorded scope kept |
| Hook denies | refused `admission_denied` with the hook reason; reservation and environment unchanged |
| Hook unavailable, over 5 s, error, or invalid decision | resume goes ahead (first launch denies instead) |
| No reservation held (empty scope at admission) | same rules; an allow with a scope records none |
| Environment reset, destroyed, or re-keyed while the hook ran | the fenced attach fails with the existing stale-generation error (ADR 2026-09-04); nothing reaches the provider |

## Pending placement

A provision response with `placement.state` of `queued` or `placing` makes the host write phase `placement_pending`, the
operation ID, the reason code, `deadline_at`, and the next poll time before it replies, then returns a pending result to the launch path
instead of blocking. **New** RPCs `PollExecutorPlacement` and `CancelExecutorPlacement` carry the operation ID and are gated by
`pending_placement`. A host poller, one per pending operation, polls within the clamped interval and ends when the record leaves `placement_pending` or at the deadline. The deadline is the fixed host value of 600 seconds (a provider-reported deadline is cut, see the admission requirements' Out of scope) added to the host clock time of the first persisted pending response, stored as UTC `deadline_at` and never extended by a response. The deadline is evaluated at commit: each guarded write compares the host clock with `deadline_at` inside the write, so a completion written at or before it wins and one attempted after it is `placement_timeout` (AC 001.2). The "operation claim" is the existing `recoveryclaim` claim on the task environment carrying the operation ID; polls hold none and each transition takes it, and a transition that cannot take it retries at the next poll interval or backoff step without blocking (AC 001.12). A post-restart continuation publishes through `PublishRecoveredTaskEnvironment` under that claim (AC 007.3).

**Restart and ownership.** On main, `recoverPluginExecutor` (executor_plugin_recovery.go) handles a record whose resource handle is unset by calling `RecoverExecutorOperation`: `found` is checkpointed as `provisioned` and then attached as a live environment through `attachRecoveredPluginExecutor`; `absent` settles the record; `unknown` logs and retains with no retry; it destroys only a `cleanup_pending` record that holds a resource handle and an `allocating` record of a terminal task; and startup recovery runs once. Left alone, a `placement_failed` or `placement_timeout` record without a handle would therefore be attached live after a restart. This design adds an early skip in `recoverPluginExecutor` for the four phases `placement_pending`, `cancelling`, `placement_failed`, and `placement_timeout`: none of them is attached or recovered by the ordinary path (AC 001.7). The poller registry (`map[operationID]` under a mutex, compare-and-set on creation, one loop per operation per process, AC 001.12) owns all four, recreated from the persisted record after a restart. For `placement_pending` and `cancelling` it runs poll and cancel; for `placement_failed` and `placement_timeout`, and for `cancelling` after the deadline, it runs the **recovery destroy by operation** (AC 001.17): `RecoverExecutorOperation`, then destroy on `found`, settle absent on `absent`, retain and retry with the backoff on `unknown` or an error, and never attach or publish ready. Backoff is 2 s doubling to 30 s and restarts at 2 s after a restart; a `next_poll_at` in the past means one immediate poll.

**Cancel.** One request-cancel function serves Stop, archive, reset, delete, and the sweep (AC 001.13). For a `placement_pending` record it takes the operation claim (waits at most 5 s), writes `cancelling` and, for archive, reset, and delete, `cleanup_requested` in one guarded write, and calls `CancelExecutorPlacement`. For a record already `cancelling` it writes nothing and returns, since the registry entry holds the in-flight marker (a second RPC is never issued). When the claim is busy past 5 s, Stop, archive, and reset set a cancel-requested flag on the registry entry (in memory, cleared by a restart) that the loop consumes at its next iteration, and delete is refused `placement_cancel_pending`. The reply is `cancelled` (settle with nothing allocated) or `already_placed` (set `destroy_started` and destroy under the same claim). A completion seen after `cancelling` is committed is treated as `already_placed`: the host never publishes ready, and `destroy_started` makes exactly one writer the destroy owner. Whichever of completion and cancel wins the revision CAS decides the path (AC 001.5): cancel first ends in `placement_cancelled` with nothing left; completion first (the write `placement_pending` to `creating`) hands the launch to the continuation. The continuation's agent start is a compare-and-set of the session from `STARTING` to `RUNNING`; a Stop that commits first makes it a no-op and the environment is published ready with no agent started, as an ordinary Stop that preserves it, so the task is stopped and not reported cancelled.
Every terminal transition re-reads the record under the claim and applies only the transitions allowed from its current phase (the table in AC 001.14). The rows are: from `placement_pending` completion, failure, timeout, cancel, and refusal (no state: AC 002.2; with state: `conflict`, `placement_failed`); from `cancelling` `cancelled`, completion or `already_placed` (guarded by `destroy_started`), refusal with no state (the `cancelled` outcome), and nothing else (every other poll result is dropped; polling stops when the record leaves `placement_pending`); from `placement_failed` and `placement_timeout` only destroy confirmation. The deadline in `cancelling` switches retries to the recovery destroy. A late poll failure after `cancelling` and a late completion after settlement are dropped without a write or a second destroy.
If `CancelExecutorPlacement` errors or times out, the operation stays `cancelling`, the host retries with backoff to the deadline, then runs the recovery destroy by operation; `cancelled` is reported only after the provider says so or a destroy confirms absence (AC 001.4). A `cancelling` destroy still unconfirmed after the deadline keeps retrying; the session ends `FAILED` with `placement_cancel_pending` so it is never left `STARTING`, and when the destroy confirms the record settles absent and the session code becomes `placement_cancelled` with the session stopped (AC 001.16). A `placement_failed` or `placement_timeout` session is `FAILED` with that code from the transition and keeps it. Start and delete are refused with `placement_cancel_pending` while the record is `cancelling`, `placement_failed`, or `placement_timeout`. A reset starts the cancel and is complete when absence is confirmed. Archive and reset return success after starting the cancel and show the `cancelling` label. A provider `conflict` is resolved by AC 001.15 and never shown.

**Continuation.** On a `placed` poll result the poller, holding the operation claim and inside the deadline check of AC 001.2, runs the existing `finishPluginExecutorLaunch` path from the persisted operation: handshake with the stored nonce, instance creation, and the agent start with the prompt already stored on the session, so no user action is needed (AC 001.11). The continuation is the delayed launch: it re-runs `checkCapabilityAgreement` against the recorded negotiated set and then does not call `admitLaunch`. The session stays `STARTING` with the pending-placement label until ready (a session stopped meanwhile keeps its stopped state); while `cancelling` it stays `STARTING` with the label `executors:pluginPlacement.cancelling` and moves to its stopped state only after `cancelled` or confirmed absence (AC 001.3). The nonce is stored with an expiry of `deadline_at` + 600 s.

**Closed values and polling.** The reason codes, phases, position, and zero-interval rules are AC 001.10; the localized keys are `executors:pluginPlacement.<phase>` and `executors:pluginPlacement.reason.<code>`, with `unknown` for any other phase. Poll transport errors, `unavailable`, and `rate_limited` retry at the interval, and `not_found`, `denied`, `invalid_config`, `unsupported`, `expired`, `conflict`, and `cleanup_pending` end as `placement_failed` plus the recovery destroy (AC 001.12); cancel retries back off 2 s doubling to 30 s.

**Outcomes.** `capacity_refused`, `placement_failed`, and `placement_timeout` are distinct values of the existing failure code,
each with its own user-visible message; none releases the admission reservation twice.

**UI.** The existing session status gains one state label with the reason code and progress phase; the existing stop action is the
cancel. All copy goes through `t()` in the seven catalogs under the keys `executors:pluginFailure.<code>`, `executors:pluginFailure.<code>.<reason>` for `scope_full` and the named `invalid_config` reasons (any other reason shows the generic `<code>` text, never the raw string), and `executors:pluginPlacement.<phase>`, with an icon beside the text (AC 001.9).
Playwright covers queued, cancel while queued, and refused, with the fixture provider, and one parametrized spec in the fixture-proof work order triggers each remaining
host-originated failure code and asserts the localized text with the pseudo-locale (no raw code). `placement_cancel_pending` has its own `executors:pluginFailure.<code>` text.

## Refused provision

With `provision_refusal` in the snapshot, `ProvisionExecutorEnvironmentResponse` may carry additive `refusal {reason, nothing_allocated}`.
`settleRefusedProvision` runs in one transaction: it checks that the operation has no checkpoint and no provider state, writes the
refusal reason, settles the record absent, and releases the reservation once. The write is guarded by the operation's revision, so a
concurrent cancel or archive cannot double-release. `pending_placement` requires `provision_refusal` (registry dependency), so a provider that will not queue
reports `capacity_refused` through this path and the record is settled absent, never left for recovery. Any other error path is unchanged: a failed or lost provision runs recovery, and only
recovery or destroy can confirm absence (AC 002.4). The refusal is a distinct failure code in the task view.

## Data model

No new table and no DDL. Envelope fields: `admission_scope`, `reservation`, `placement` (`state`, `reason`, `next_poll_at`, `deadline_at`, `phase`, `destroy_started`), and `cleanup_requested`. They stay inside the per-field maxima of the [versioning design](remote-executor-plugin-capability-versioning.md#state-budget)
(placement, admission, and drain together 1536; the admission snapshot of seven IDs and values stays inside it and a worst-case test asserts that). The reservation lives in the envelope of the same environment row, so its
count query reads the rows by scope; a scope index is not added, and the count is bounded by the active environments of one scope.

## Interleavings and failing-first tests

| Interleaving | Required outcome | Tests (level) |
| --- | --- | --- |
| Two launches compete for the last unit of one scope (ADMISSION 001.4) | one admitted, one denied | sqlite and PostgreSQL race |
| Resume racing a new launch of the same scope (001.9) | the resume never takes the scope lock and keeps its reservation; only the new launch can be refused `scope_full`; same outcome in either order | sqlite and PostgreSQL race; lifecycle unit |
| Hook unavailable or over its 5 s deadline (001.6) | first launch: `admission_unavailable`, no allocation; resume: goes ahead (001.11) | lifecycle unit |
| Resume via `attachRetainedPluginExecutor` (001.3, 001.11) | same capability gate, measure and hook as first launch, driven by the recorded negotiated set; deny refuses; a pre-capability environment (empty set, no snapshot) resumes as today; stale generation fails the attach (001.11) | lifecycle unit over both entries; caller-enumeration test |
| A late poll failure after `cancelling`, or a late completion after settlement (PLACEMENT 001.14) | dropped without a write or a second destroy; reservation released once | lifecycle unit; sqlite and PostgreSQL race |
| Archive, reset, or repeated delete while `cancelling` (PLACEMENT 001.13) | no claim blocks it; the cancel is not restarted; delete refused `placement_cancel_pending` each time | lifecycle unit |
| Reservation release after failure and after destroy (001.5) | released exactly once | lifecycle unit |
| Queued launch cancelled while placement completes (PLACEMENT 001.5) | cancel first: `placement_cancelled`, nothing left, never ready; completion first: ready, ordinary Stop, not reported cancelled | barrier e2e over the fixture (`before_placement_commit`, `after_cancel_mark`); sqlite and PostgreSQL race |
| `CancelExecutorPlacement` errors or times out (001.4) | stays cancelling; retried; cancelled only after provider reply or confirmed absence | lifecycle unit with fault-injecting client |
| Restart while pending (001.7) | startup recovery skips the four placement phases; registry polling resumes; no second environment, no handshake against an unplaced worker | barrier e2e (`after_placement_persist`) |
| Completion committed, then Stop before the agent start (PLACEMENT 001.5) | continuation re-reads the session, publishes ready with no agent start; task stopped, not cancelled | lifecycle unit; fixture e2e |
| Poll response with no resource, refusal, or pending (001.10) | `conflict` then `placement_failed` and recovery destroy | lifecycle unit |
| Poll refusal in `placement_pending`, in `cancelling`, or with a checkpoint (001.14, 002.2) | settle absent and release once; `cancelled` outcome; `conflict` and `placement_failed` | lifecycle unit; sqlite and PostgreSQL race |
| Stop, archive, and sweep request a cancel together (001.13) | one cancel RPC; the others find it in flight | lifecycle unit with fault-injecting client |
| Destroy unconfirmed after the deadline (001.16) | retries; session `FAILED` with `placement_cancel_pending`; start and delete refused; settles on confirmation | lifecycle unit with fake clock |
| Restart with `placement_failed`, `placement_timeout`, or `cancelling` records (001.7, 001.17) | skipped by `recoverPluginExecutor`; registry runs the recovery destroy by operation (`found` destroys, `absent` settles, `unknown` retries with backoff); never attached live | barrier e2e; lifecycle unit over `recoverPluginExecutor` |
| Provider `conflict` on cancel or provision (001.15) | cancel stays `cancelling`; provision runs recovery destroy; never shown | lifecycle unit |
| Archive, reset, or delete during `placement_pending` or `cancelling` (001.13) | cancel path runs; `cleanup_requested` written in the same guarded write as `cancelling`; claim busy past 5 s sets the in-memory cancel-requested flag (delete refused); delete refused as `placement_cancel_pending` until absence is confirmed | lifecycle unit; store unit |
| Deadline passes while a poll is in flight (001.2) | `placement_timeout`, destroy of anything late-placed | lifecycle unit with a fake clock (synctest) |
| Refusal racing a cancel or archive (PLACEMENT 002.2) | one transaction wins; reservation released once; record absent or cancelled, never retained | sqlite and PostgreSQL race |
| Refusal after a checkpoint was recorded (002.3) | `conflict`; recovery runs | lifecycle unit |
| Lost reply or timeout on provision (002.4) | treated as failure, never as refusal; recovery decides | lifecycle unit with fault-injecting client |
| Pending response from a provider without the capability (001.8) | `conflict`, cleanup | lifecycle unit |
| Poll returns `not_found`, `unavailable`, zero interval (PLACEMENT 001.12, 001.10) | `placement_failed` plus recovery destroy; retry to the deadline; 5 s default | lifecycle unit with fake clock |
| Completion written after `deadline_at` (001.2) | `placement_timeout`; the late placement is destroyed; a completion at or before it wins | lifecycle unit with fake clock; sqlite and PostgreSQL race |
| Cancel and completion both see `already_placed` (001.5) | `destroy_started` makes one writer the destroy owner; one destroy | lifecycle unit; PostgreSQL race |
| Backoff after a restart (001.7) | restarts at 2 s; past `next_poll_at` polls once at once | lifecycle unit with fake clock |
| Admission reservation write fails (001.4) | `admission_unavailable`, nothing allocated, no `Release` | lifecycle unit with fault-injecting store |
| Start or delete while `cancelling`, `placement_failed`, or `placement_timeout` (001.16) | refused `placement_cancel_pending` | lifecycle unit |
| Placement completes after a restart (001.11) | launch continues from the persisted operation and the agent starts with the stored prompt | barrier e2e (`after_placement_persist`) |

Each test is written first and fails on main. Remaining ACs are table tests (decision validity, scope and reason syntax) and the
Playwright spec.

## Observability

`executor_plugin_admission_total` (`decision`: allow, deny, unavailable), `executor_plugin_placement_total`
(`outcome`: placed, cancelled, already_placed, capacity_refused, placement_failed, placement_timeout), and
`executor_plugin_placement_pending` gauge. Task, user, and scope values are never metric labels.

## Security

Admission identifiers are opaque host IDs; the identity snapshot holds only IDs and the priority. The hook is host code and
never a plugin. The cancel route is the existing authenticated stop path.

## Related decisions

- [Cleanup inventory](../../../decisions/0025-runtime-cleanup-uses-executors-running.md)
- [Ownership generations](../../../decisions/2026-09-04-generation-fenced-task-environment-ownership.md)
