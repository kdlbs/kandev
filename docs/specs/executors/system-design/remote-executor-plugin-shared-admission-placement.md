---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-PLUGIN-SHARED-001
  - REQ-EXECUTORS-PLUGIN-ADMISSION-001
  - REQ-EXECUTORS-PLUGIN-PLACEMENT-001
  - REQ-EXECUTORS-PLUGIN-PLACEMENT-002
---

# Remote executor plugins: shared environments, admission, and placement system design

## Purpose and boundaries

Three launch-path mechanisms, all on the one ledger: the `plugin_executor` envelope in `executors_running.metadata`, its
revision CAS, and the existing recovery claims. Capability gating comes from the [versioning design](remote-executor-plugin-capability-versioning.md).
Names marked **new** do not describe existing APIs. Core keeps owner sets, admission decisions, reservations, and cancel
admission; the provider keeps placement, queueing, and worker choice.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| REQ-EXECUTORS-PLUGIN-SHARED-001 | Shared environments |
| REQ-EXECUTORS-PLUGIN-ADMISSION-001 | Admission context and hook |
| REQ-EXECUTORS-PLUGIN-PLACEMENT-001 | Pending placement |
| REQ-EXECUTORS-PLUGIN-PLACEMENT-002 | Refused provision |

## Shared environments

`validateLaunch` today rejects `req.WorkspaceReuseRequired || isPluginExecutorSharedWorkspace`. With the `workspace_sharing`
capability in the snapshot, it routes to **new** `attachSharedOwner`; otherwise it keeps the rejection, now with the
`missing_capability` code (AC 001.1).

**Owner set.** Envelope fields `owners` (at most 16 entries `{owner_id, epoch}`), `share_epoch`, and `closing`. Every attach or release is one revision-checked write that bumps `share_epoch`. An attach is admitted only for the same owning principal, workspace, installation, profile, workspace identity, and repository set, and only when the environment is `ready` and not `closing`; the principal rule follows the [per-user ownership](per-user-ownership.md) design and refuses cross-principal sharing (answered by the owner on 2026-10-28: same principal only).

**Rows.** `executors_running.session_id` is unique, so an environment with several owners spans several rows. The holder row (created with the environment) carries the owner set and `share_epoch`; each other owner's row carries `holder_row` and its own `instance_port`. An attach is one transaction: CAS the holder envelope (append owner, bump epoch) and insert the owner's row. A release is one transaction: CAS the holder (remove owner, bump epoch; if the set is empty also set `closing`), release that owner's reservation once, and delete the owner's row. When the released owner is the holder's own, the holder row stays as the record and its `session_id` is re-keyed to `holder:`+environment ID in the same transaction (AC 001.11), so the session can launch again; the delete cascades skip rows with that prefix and the last release deletes the row. Owner rows and re-keyed holder rows carry a complete `plugin_executor` envelope, so `retainedExecutorProviderInventory` and the cleanup and recovery sweeps count each as a plugin row of the installation. An attach writes its owner row with phase `attaching` and the recovery pass releases an `attaching` owner that has no instance (AC 001.14). Candidates for a reuse request are ordered by holder `created_at` ascending, then `id` in byte order (AC 001.15). Each owner holds its own admission reservation (AC 001.13). Releases come only from archive, delete, reset, and failed launch (AC 001.10), never from Stop.

**Release.** `releaseOwner` removes the entry and bumps the epoch in one CAS. If the set became empty it sets `closing` in the
same write and starts the existing cleanup under a cleanup claim (`recoveryclaim`, keyed by the holder's task environment, with the releasing operation's ID). A release that loses the revision CAS re-reads and retries up to three times, then returns `conflict`; the cleanup sweep re-issues it. A release of an absent owner is a no-op, so replays are safe. Destroy carries `owner_count: 0` and the host refuses to
call destroy while any owner remains (AC 001.9). Per-owner instances: each owner has its own agentctl instance and port,
released with the owner (AC 001.7); the control and instance port set of the tunnel grows to the recorded ports.

**Fence.** Provider calls after an attach or release carry `{ownership_generation, share_epoch}`; the provider rejects a stale
pair, which surfaces as `conflict` with no same-call retry (AC 001.12). A provider declares `sharing_supported` and
`max_owners` in profile validation, and the host uses the lower cap (AC 001.8).

**Provider guarantees** (documented in the authoring guide, asserted by the fixture): stable `workspace_identity` for the
life of the environment; no destroy on anything but the host's destroy call; per-owner isolation is the provider's
responsibility only where it declares it, and core makes no isolation claim between owners of one environment.

## Admission context and hook

**New** `internal/agent/runtime/lifecycle/admission.go` defines `AdmissionHook` with `Admit(ctx, AdmissionRequest) (Decision, error)`
and `Release(ctx, ReservationID)`. A `Decision` is allow, deny with a stable reason, or allow with a lower class, plus an `admission_scope` string and a `limit`; the hook never counts, the host does.
`Release` only informs the hook after the host released (best effort, failures logged). The default implementation allows everything with an empty scope (no reservation) and is used for providers that do not require
`admission_context`. The hook runs from **new** `admitLaunch`, which `validateLaunch` calls after the capability gate and before any provider allocation call, from
the first-launch, delayed-launch, and resume paths (one shared function, so a path cannot skip it; a test enumerates the callers of `validateLaunch`). Recovery of an already admitted environment does not call it.
Inputs: the resource class, priority, project and step IDs come from the sources of AC 001.10; the provider's `resource_classes` and `default_resource_class` are read from profile validation and the profile property `resource_class` is a normal schema property. The order is fixed: capability gate (pure), label validation, `Admit` outside any transaction with a 5 s deadline, then one transaction that takes the scope lock, counts, and writes the reservation with the operation.

**Atomicity.** The reservation is inserted in the same transaction that creates the operation record, keyed by an
`admission_scope` string (workspace, user, or project scope named by the hook and validated by the host as `[a-z0-9._:/-]{1,128}`). SQLite's immediate-mode writer serializes it.
PostgreSQL takes `pg_advisory_xact_lock(hashtextextended('plugin_admission:'||scope, 0))`, a key derived by the host from the validated scope, before reading the count of live reservations. A new launch over the limit, or a limit of zero or less, is `admission_denied` (`scope_full`). A reservation
is released exactly once by a `released` flag in the same write as the transition (terminal failure, destroy confirmation,
cancel). Hook failure fails closed with `admission_unavailable` (AC 001.6).

**Context.** The request fields are built from host records only, by a function that cannot read names, emails, or
credentials: it receives IDs and the validated labels. Label validation runs before the hook. The provision and recovery requests
gain additive `admission` messages. The provider returns `applied_resource_class`; the host checks it against the profile's allowed classes and the admitted
class, persists it, and exposes it in the environment read. A resume keeps its class and its existing reservation (taken at first launch, released only at terminal failure, destroy, or cancel, never at Stop), so a resume
never takes a second unit; a different admitted class is `conflict` (AC 001.8). The resume and a new launch for one scope serialize on the same lock; the new launch is the only possible loser (AC 001.9).

## Pending placement

A provision response with `placement.state` of `queued` or `placing` makes the host write phase `placement_pending`, the
operation ID, the reason code, and the next poll time before it replies, then returns a pending result to the launch path
instead of blocking. **New** RPCs `PollExecutorPlacement` and `CancelExecutorPlacement` carry the operation ID and are gated by
`pending_placement`. A host poller, one per pending operation, polls within the clamped interval and ends at the placement
deadline with `placement_timeout`. The deadline is `placement_deadline_seconds` from profile validation (60 to 86400, default 600), copied into the pending state at the first pending response and never extended by a response. A restart reloads pending operations from the ledger and resumes polling (AC 001.7).

**Cancel.** The stop path takes the operation claim, writes `cancelling`, and calls `CancelExecutorPlacement`. The reply is
`cancelled` (settle with nothing allocated) or `already_placed` (continue with destroy under the same claim). A completion seen by
the poller after `cancelling` is committed is treated as `already_placed`: the host never publishes ready, and destroy runs to
confirmation. Whichever of completion and cancel wins the revision CAS decides the path (AC 001.5): cancel first ends in `placement_cancelled` with nothing left;
completion first leaves a ready environment and the stop is an ordinary Stop that preserves it, so the task is stopped and not reported cancelled.
If `CancelExecutorPlacement` errors or times out, the operation stays `cancelling`, the host retries with backoff to the placement deadline, then runs the recovery destroy by operation;
`cancelled` is reported only after the provider says so or a destroy confirms absence (AC 001.4).

**Continuation.** On a `placed` poll result the poller, holding the operation claim, runs the existing `finishPluginExecutorLaunch` path from the persisted operation: handshake with the stored nonce, instance creation, and the agent start with the prompt already stored on the session, so no user action is needed (AC 001.11). The session stays `STARTING` with the pending-placement label until ready. The nonce is stored with an expiry of `deadline_at` + 600 s.

**Closed values and polling.** The reason codes, phases, position, and zero-interval rules are AC 001.10; the localized keys are `executors:pluginPlacement.<phase>` and `executors:pluginPlacement.reason.<code>`, with `unknown` for any other phase. The poller registry (`map[operationID]` under a mutex, compare-and-set on creation) gives one loop per operation per process (AC 001.12); poll errors retry at the interval, `not_found` and `denied` end as `placement_failed` plus the recovery destroy, and cancel retries back off 2 s doubling to 30 s.

**Outcomes.** `capacity_refused`, `placement_failed`, and `placement_timeout` are distinct values of the existing failure code,
each with its own user-visible message; none releases the admission reservation twice.

**UI.** The existing session status gains one state label with the reason code and progress phase; the existing stop action is the
cancel. All copy goes through `t()` in the seven catalogs under the keys `executors:pluginFailure.<code>` and `executors:pluginPlacement.<phase>`, with an icon beside the text (AC 001.9).
Playwright covers queued, cancel while queued, and refused, with the fixture provider, and one parametrized spec in the fixture-proof work order triggers each remaining
host-originated failure code and asserts the localized text with the pseudo-locale (no raw code). `environment_closing` is resolved inside the attach path and never shown.

## Refused provision

With `provision_refusal` in the snapshot, `ProvisionExecutorEnvironmentResponse` may carry additive `refusal {reason, nothing_allocated}`.
`settleRefusedProvision` runs in one transaction: it checks that the operation has no checkpoint and no provider state, writes the
refusal reason, settles the record absent, and releases the reservation once. The write is guarded by the operation's revision, so a
concurrent cancel or archive cannot double-release. `pending_placement` requires `provision_refusal` (registry dependency), so a provider that will not queue
reports `capacity_refused` through this path and the record is settled absent, never left for recovery. Any other error path is unchanged: a failed or lost provision runs recovery, and only
recovery or destroy can confirm absence (AC 002.4). The refusal is a distinct failure code in the task view.

## Data model

No new table and no DDL. A `holder:` `session_id` value is data in the existing column. Envelope fields: `owners`, `share_epoch`, `closing`, `admission_scope`, `reservation`,
`applied_resource_class`, `holder_row`, `role` (`holder` or `owner`), `phase` `attaching`, `placement` (`state`, `reason`, `next_poll_at`, `deadline_at`, `phase`). They stay inside the per-field maxima of the [versioning design](remote-executor-plugin-capability-versioning.md#state-budget)
(owners 2048 bytes; placement, admission, and drain together 1536; labels are never stored). The reservation lives in the envelope of the same environment row, so its
count query reads the rows by scope; a scope index is not added, and the count is bounded by the active environments of one scope.

## Interleavings and failing-first tests

| Interleaving | Required outcome | Tests (level) |
| --- | --- | --- |
| Two owners release one shared environment concurrently (SHARED 001.6) | the first CAS removes its owner; the second re-reads and, finding only itself, empties the set, sets closing, and holds the cleanup claim; exactly one destroy; replays are no-ops | sqlite and PostgreSQL multi-connection `-race`; fixture over the process boundary |
| Attach races the last release (001.6) | attach wins and environment stays, or attach gets `environment_closing` and provisions anew | sqlite and PostgreSQL race |
| Stale `share_epoch` call to the provider (001.4) | provider rejects; host does not retry as new | fixture conformance |
| Destroy attempted with owners remaining (001.9) | host refuses before calling the provider | lifecycle unit |
| Two launches compete for the last unit of one scope (ADMISSION 001.4) | one admitted, one denied | sqlite and PostgreSQL race |
| Admission hook racing a resume of the same scope (001.9) | serialized by the scope lock; the resume keeps its reservation; the new launch that no longer fits gets `admission_denied` (`scope_full`); applied class never changes (001.8) | sqlite and PostgreSQL race; lifecycle unit |
| Hook unavailable or over its 5 s deadline (001.6) | `admission_unavailable`, no allocation | lifecycle unit |
| Owner attaches when its row is the 17th (001.8) | `capacity_refused`, nothing allocated | lifecycle unit |
| Stop on a shared owner (001.10) | no release; owner set unchanged | lifecycle unit |
| Reservation release after failure and after destroy (001.5) | released exactly once | lifecycle unit |
| Queued launch cancelled while placement completes (PLACEMENT 001.5) | cancel first: `placement_cancelled`, nothing left, never ready; completion first: ready, ordinary Stop, not reported cancelled | barrier e2e over the fixture (`before_placement_commit`, `after_cancel_mark`); sqlite and PostgreSQL race |
| `CancelExecutorPlacement` errors or times out (001.4) | stays cancelling; retried; cancelled only after provider reply or confirmed absence | lifecycle unit with fault-injecting client |
| Restart while pending (001.7) | polling resumes; no second environment | barrier e2e (`after_placement_persist`) |
| Deadline passes while a poll is in flight (001.2) | `placement_timeout`, destroy of anything late-placed | lifecycle unit with a fake clock (synctest) |
| Refusal racing a cancel or archive (PLACEMENT 002.2) | one transaction wins; reservation released once; record absent or cancelled, never retained | sqlite and PostgreSQL race |
| Refusal after a checkpoint was recorded (002.3) | `conflict`; recovery runs | lifecycle unit |
| Lost reply or timeout on provision (002.4) | treated as failure, never as refusal; recovery decides | lifecycle unit with fault-injecting client |
| Pending response from a provider without the capability (001.8) | `conflict`, cleanup | lifecycle unit |
| Holder owner released while others remain, then its session launches again (SHARED 001.11) | holder row re-keyed; the new launch inserts its own owner row with no `session_id` collision; the last release deletes the record row | sqlite and PostgreSQL |
| Task delete of the holder's task while owners remain (001.11) | owner released first; record row survives the cascade until the last release | store unit |
| Uninstall or upgrade with owner and re-keyed rows present (001.11) | rows counted once as retained inventory of the installation | guard unit |
| Attach commits, backend dies before the instance exists (001.14) | recovery releases the `attaching` owner and its reservation once | barrier e2e; sqlite and PostgreSQL |
| In-flight provider call carries an epoch that another attach advanced (001.12) | `conflict`; no write over the newer epoch | fixture conformance |
| Attach for a non-last owner release, then destroy (001.13) | each owner's reservation released once at its own release | lifecycle unit |
| Poll returns `not_found`, `unavailable`, zero interval (PLACEMENT 001.12, 001.10) | `placement_failed` plus recovery destroy; retry to the deadline; 5 s default | lifecycle unit with fake clock |
| Placement completes after a restart (001.11) | launch continues from the persisted operation and the agent starts with the stored prompt | barrier e2e (`after_placement_persist`) |

Each test is written first and fails on main. Remaining ACs are table tests (label rules, class checks, owner caps) and the
Playwright spec.

## Observability

`executor_plugin_shared_owners` gauge (no owner label), `executor_plugin_attach_total` (`outcome`),
`executor_plugin_admission_total` (`decision`: allow, deny, lower, unavailable), `executor_plugin_placement_total`
(`outcome`: placed, cancelled, already_placed, capacity_refused, placement_failed, placement_timeout), and
`executor_plugin_placement_pending` gauge. Task, user, scope, and label values are never metric labels.

## Security

Admission identifiers are opaque host IDs; labels are validated and non-secret by construction. The hook is host code and
never a plugin. Sharing is limited to one principal and one workspace, and the host makes no isolation claim between owners of a
shared environment. The cancel route is the existing authenticated stop path.

## Related decisions

- [Cleanup inventory](../../../decisions/0025-runtime-cleanup-uses-executors-running.md)
- [Ownership generations](../../../decisions/2026-09-04-generation-fenced-task-environment-ownership.md)
