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
and `Release(ctx, ReservationID)`. A `Decision` is allow, deny with a stable reason, or allow with a lower class, plus an `admission_scope` string and a `limit`; the hook never counts, the host does.
`Release` only informs the hook after the host released (best effort, failures logged). The only implementation in core is `defaultAdmissionHook`, which allows everything with an empty scope (no reservation); a quota policy is host code installed at wiring through **new** `Manager.SetAdmissionHook` and never a plugin, and tests use a scripted hook. A provider that does not require
`admission_context` skips `admitLaunch` entirely and the hook is not called (AC 001.6). The hook runs from **new** `admitLaunch`, which `validateLaunch` calls after the capability gate and before any provider allocation call, from
the first-launch and resume paths (one shared function, so a path cannot skip it; a test enumerates the callers of `validateLaunch`). Recovery of an already admitted environment and the continuation of a pending placement do not call it; the continuation keeps the reservation taken at the original launch.
Inputs: the resource class, priority, project and step IDs come from the sources of AC 001.10; the provider's `resource_classes` and `default_resource_class` are read from profile validation and the profile property `resource_class` is a normal schema property. The order is fixed: capability gate (pure), label validation, `Admit` outside any transaction with a 5 s deadline, then one transaction that takes the scope lock, counts, and writes the reservation with the operation.

**Atomicity.** The reservation is inserted in the same transaction that creates the operation record, keyed by an
`admission_scope` string (workspace, user, or project scope named by the hook and validated by the host as `[a-z0-9._:/-]{1,128}`). SQLite's immediate-mode writer serializes it.
PostgreSQL takes `pg_advisory_xact_lock(hashtextextended('plugin_admission:'||scope, 0))`, a key derived by the host from the validated scope, before reading the count of live reservations. A new launch over the limit, or a limit of zero or less, is `admission_denied` (`scope_full`). A reservation
is released exactly once by a `released` flag in the same write as the transition that confirms absence (destroy confirmation, which also covers terminal failure; provider `cancelled`; a refusal that allocated nothing), never before it. Hook failure fails closed with `admission_unavailable` (AC 001.6).

**Context.** The request fields are built from host records only, by a function that cannot read names, emails, or
credentials: it receives IDs and the validated labels. Label validation runs before the hook. The provision and recovery requests
gain additive `admission` messages. The provider returns `applied_resource_class`; the host checks it against the profile's allowed classes and the admitted
class, persists it, and exposes it in the environment read. A resume keeps its class and its existing reservation (taken at first launch, released only at confirmed absence or provider `cancelled`, never at Stop), so a resume
never takes a second unit; a different admitted class is `conflict` (AC 001.8). The resume and a new launch for one scope serialize on the same lock; the new launch is the only possible loser (AC 001.9).

## Pending placement

A provision response with `placement.state` of `queued` or `placing` makes the host write phase `placement_pending`, the
operation ID, the reason code, and the next poll time before it replies, then returns a pending result to the launch path
instead of blocking. **New** RPCs `PollExecutorPlacement` and `CancelExecutorPlacement` carry the operation ID and are gated by
`pending_placement`. A host poller, one per pending operation, polls within the clamped interval and ends at the placement
deadline with `placement_timeout`. The deadline is `placement_deadline_seconds` from profile validation (60 to 86400, default 600), copied into the pending state at the first pending response and never extended by a response. A restart reloads pending operations from the ledger and resumes polling (AC 001.7): `recoverPluginExecutor` gets a **new** early skip for records in `placement_pending` or `cancelling`, which only the poller registry owns, so there is never a second continuation or a handshake against a worker that does not exist yet. The "operation claim" is the existing `recoveryclaim` claim on the task environment carrying the operation ID; polls hold none and each transition takes it. A post-restart continuation publishes through `PublishRecoveredTaskEnvironment` under that claim (AC 007.3).

**Cancel.** The stop path takes the operation claim, writes `cancelling`, and calls `CancelExecutorPlacement`. The reply is
`cancelled` (settle with nothing allocated) or `already_placed` (continue with destroy under the same claim). A completion seen by
the poller after `cancelling` is committed is treated as `already_placed`: the host never publishes ready, and destroy runs to
confirmation. Whichever of completion and cancel wins the revision CAS decides the path (AC 001.5): cancel first ends in `placement_cancelled` with nothing left;
completion first (the write `placement_pending` to `creating`) hands the launch to the continuation, which re-reads the session before the agent start: a session stopped meanwhile gets a ready environment with no agent started, as an ordinary Stop that preserves it, so the task is stopped and not reported cancelled. Archive, reset, and delete start this cancel path if none is running and record `cleanup_requested`; the poller registry stays the only actor on these records, the cleanup sweep never calls the provider for them, each attempt and each terminal transition takes the operation claim for its own write and no claim is held between retries, and a delete is refused with `placement_cancel_pending` (having started the cancel) until absence is confirmed, because the task delete cascades the record (AC 001.13). Every terminal transition re-reads the record under the claim and applies only the transitions allowed from its current phase, so a late poll failure after `cancelling` or a late completion after settlement is dropped without a write or a second destroy (AC 001.14).
If `CancelExecutorPlacement` errors or times out, the operation stays `cancelling`, the host retries with backoff to the placement deadline, then runs the recovery destroy by operation;
`cancelled` is reported only after the provider says so or a destroy confirms absence (AC 001.4).

**Continuation.** On a `placed` poll result the poller, holding the operation claim, runs the existing `finishPluginExecutorLaunch` path from the persisted operation: handshake with the stored nonce, instance creation, and the agent start with the prompt already stored on the session, so no user action is needed (AC 001.11). The continuation is the delayed launch: it re-runs `checkCapabilityAgreement` against the recorded negotiated set and then does not call `admitLaunch`. The session stays `STARTING` with the pending-placement label until ready; while `cancelling` it stays `STARTING` with the label `executors:pluginPlacement.cancelling` and moves to its stopped state only after `cancelled` or confirmed absence (AC 001.3). The nonce is stored with an expiry of `deadline_at` + 600 s.

**Closed values and polling.** The reason codes, phases, position, and zero-interval rules are AC 001.10; the localized keys are `executors:pluginPlacement.<phase>` and `executors:pluginPlacement.reason.<code>`, with `unknown` for any other phase. The poller registry (`map[operationID]` under a mutex, compare-and-set on creation) gives one loop per operation per process (AC 001.12); poll transport errors, `unavailable`, and `rate_limited` retry at the interval, and `not_found`, `denied`, `invalid_config`, `unsupported`, `expired`, `conflict`, and `cleanup_pending` end as `placement_failed` plus the recovery destroy (AC 001.12), and cancel retries back off 2 s doubling to 30 s.

**Outcomes.** `capacity_refused`, `placement_failed`, and `placement_timeout` are distinct values of the existing failure code,
each with its own user-visible message; none releases the admission reservation twice.

**UI.** The existing session status gains one state label with the reason code and progress phase; the existing stop action is the
cancel. All copy goes through `t()` in the seven catalogs under the keys `executors:pluginFailure.<code>` and `executors:pluginPlacement.<phase>`, with an icon beside the text (AC 001.9).
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

No new table and no DDL. Envelope fields: `admission_scope`, `reservation`, `applied_resource_class`, `placement` (`state`, `reason`, `next_poll_at`, `deadline_at`, `phase`), and `cleanup_requested`. They stay inside the per-field maxima of the [versioning design](remote-executor-plugin-capability-versioning.md#state-budget)
(placement, admission, and drain together 1536; labels are never stored). The reservation lives in the envelope of the same environment row, so its
count query reads the rows by scope; a scope index is not added, and the count is bounded by the active environments of one scope.

## Interleavings and failing-first tests

| Interleaving | Required outcome | Tests (level) |
| --- | --- | --- |
| Two launches compete for the last unit of one scope (ADMISSION 001.4) | one admitted, one denied | sqlite and PostgreSQL race |
| Admission hook racing a resume of the same scope (001.9) | serialized by the scope lock; the resume keeps its reservation; the new launch that no longer fits gets `admission_denied` (`scope_full`); applied class never changes (001.8) | sqlite and PostgreSQL race; lifecycle unit |
| Hook unavailable or over its 5 s deadline (001.6) | `admission_unavailable`, no allocation | lifecycle unit |
| A late poll failure after `cancelling`, or a late completion after settlement (PLACEMENT 001.14) | dropped without a write or a second destroy; reservation released once | lifecycle unit; sqlite and PostgreSQL race |
| Archive, reset, or repeated delete while `cancelling` (PLACEMENT 001.13) | no claim blocks it; the cancel is not restarted; delete refused `placement_cancel_pending` each time | lifecycle unit |
| Reservation release after failure and after destroy (001.5) | released exactly once | lifecycle unit |
| Queued launch cancelled while placement completes (PLACEMENT 001.5) | cancel first: `placement_cancelled`, nothing left, never ready; completion first: ready, ordinary Stop, not reported cancelled | barrier e2e over the fixture (`before_placement_commit`, `after_cancel_mark`); sqlite and PostgreSQL race |
| `CancelExecutorPlacement` errors or times out (001.4) | stays cancelling; retried; cancelled only after provider reply or confirmed absence | lifecycle unit with fault-injecting client |
| Restart while pending (001.7) | startup recovery skips `placement_pending` and `cancelling` rows; polling resumes; no second environment, no handshake against an unplaced worker | barrier e2e (`after_placement_persist`) |
| Completion committed, then Stop before the agent start (PLACEMENT 001.5) | continuation re-reads the session, publishes ready with no agent start; task stopped, not cancelled | lifecycle unit; fixture e2e |
| Poll response with no resource, refusal, or pending (001.10) | `conflict` then `placement_failed` and recovery destroy | lifecycle unit |
| Archive, reset, or delete during `placement_pending` or `cancelling` (001.13) | cancel path runs; `cleanup_requested` when the claim is held; delete refused as `placement_cancel_pending` until absence is confirmed | lifecycle unit; store unit |
| Deadline passes while a poll is in flight (001.2) | `placement_timeout`, destroy of anything late-placed | lifecycle unit with a fake clock (synctest) |
| Refusal racing a cancel or archive (PLACEMENT 002.2) | one transaction wins; reservation released once; record absent or cancelled, never retained | sqlite and PostgreSQL race |
| Refusal after a checkpoint was recorded (002.3) | `conflict`; recovery runs | lifecycle unit |
| Lost reply or timeout on provision (002.4) | treated as failure, never as refusal; recovery decides | lifecycle unit with fault-injecting client |
| Pending response from a provider without the capability (001.8) | `conflict`, cleanup | lifecycle unit |
| Poll returns `not_found`, `unavailable`, zero interval (PLACEMENT 001.12, 001.10) | `placement_failed` plus recovery destroy; retry to the deadline; 5 s default | lifecycle unit with fake clock |
| Placement completes after a restart (001.11) | launch continues from the persisted operation and the agent starts with the stored prompt | barrier e2e (`after_placement_persist`) |

Each test is written first and fails on main. Remaining ACs are table tests (label rules, class checks) and the
Playwright spec.

## Observability

`executor_plugin_admission_total` (`decision`: allow, deny, lower, unavailable), `executor_plugin_placement_total`
(`outcome`: placed, cancelled, already_placed, capacity_refused, placement_failed, placement_timeout), and
`executor_plugin_placement_pending` gauge. Task, user, scope, and label values are never metric labels.

## Security

Admission identifiers are opaque host IDs; labels are validated and non-secret by construction. The hook is host code and
never a plugin. The cancel route is the existing authenticated stop path.

## Related decisions

- [Cleanup inventory](../../../decisions/0025-runtime-cleanup-uses-executors-running.md)
- [Ownership generations](../../../decisions/2026-09-04-generation-fenced-task-environment-ownership.md)
