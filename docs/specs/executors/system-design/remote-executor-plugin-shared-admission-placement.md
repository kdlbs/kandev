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

**Owner set.** Envelope fields `owners` (at most 16 entries `{owner_id, epoch}`), `share_epoch`, and `closing`. Every attach or release is one revision-checked write that bumps `share_epoch`. An attach is admitted only for the same owning principal, workspace, installation, profile, workspace identity, and repository set, and only when the environment is `ready` and not `closing`; the principal rule follows the [per-user ownership](per-user-ownership.md) design and refuses cross-principal sharing (open question for the owner).

**Release.** `releaseOwner` removes the entry and bumps the epoch in one CAS. If the set became empty it sets `closing` in the
same write and starts the existing cleanup under a cleanup claim. Destroy carries `owner_count: 0` and the host refuses to
call destroy while any owner remains (AC 001.9). Per-owner instances: each owner has its own agentctl instance and port,
released with the owner (AC 001.7); the control and instance port set of the tunnel grows to the recorded ports.

**Fence.** Provider calls after an attach or release carry `{ownership_generation, share_epoch}`; the provider rejects a stale
pair (the pair is already part of idempotency inputs, so retries stay safe). A provider declares `sharing_supported` and
`max_owners` in profile validation, and the host uses the lower cap (AC 001.8).

**Provider guarantees** (documented in the authoring guide, asserted by the fixture): stable `workspace_identity` for the
life of the environment; no destroy on anything but the host's destroy call; per-owner isolation is the provider's
responsibility only where it declares it, and core makes no isolation claim between owners of one environment.

## Admission context and hook

**New** `internal/agent/runtime/lifecycle/admission.go` defines `AdmissionHook` with `Admit(ctx, AdmissionRequest) (Decision, error)`
and `Release(ctx, ReservationID)`. The default implementation allows everything and is used for providers that do not require
`admission_context`. The hook runs in `validateLaunch` after the capability gate and before any provider allocation call, from
the first-launch, delayed-launch, and resume paths (one shared function, so a path cannot skip it; a test enumerates callers).

**Atomicity.** The reservation is inserted in the same transaction that creates the operation record, keyed by an
`admission_scope` string (workspace, user, or project scope chosen by the hook). SQLite's immediate-mode writer serializes it.
PostgreSQL takes `pg_advisory_xact_lock(hashtextextended('plugin_admission:'||scope, 0))` before reading the count. A reservation
is released exactly once by a `released` flag in the same write as the transition (terminal failure, destroy confirmation,
cancel). Hook failure fails closed with `admission_unavailable` (AC 001.6).

**Context.** The request fields are built from host records only, by a function that cannot read names, emails, or
credentials: it receives IDs and the validated labels. Label validation runs before the hook. The provision and recovery requests
gain additive `admission` messages. The provider returns `applied_resource_class`; the host checks it against the profile's allowed classes and the admitted
class, persists it, and exposes it in the environment read. A resume keeps its class; a different admitted class is `conflict`
(AC 001.8). The resume and a new launch for one scope serialize on the same lock, so the loser observes the winner's
reservation (AC 001.9).

## Pending placement

A provision response with `placement.state` of `queued` or `placing` makes the host write phase `placement_pending`, the
operation ID, the reason code, and the next poll time before it replies, then returns a pending result to the launch path
instead of blocking. **New** RPCs `PollExecutorPlacement` and `CancelExecutorPlacement` carry the operation ID and are gated by
`pending_placement`. A host poller, one per pending operation, polls within the clamped interval and ends at the placement
deadline with `placement_timeout`. A restart reloads pending operations from the ledger and resumes polling (AC 001.7).

**Cancel.** The stop path takes the operation claim, writes `cancelling`, and calls `CancelExecutorPlacement`. The reply is
`cancelled` (settle with nothing allocated) or `already_placed` (continue with destroy under the same claim). A completion seen by
the poller after `cancelling` is committed is treated as `already_placed`: the host never publishes ready, and destroy runs to
confirmation (AC 001.5). Whichever of completion and cancel wins the revision CAS decides the path, and both paths end with no
allocation.

**Outcomes.** `capacity_refused`, `placement_failed`, and `placement_timeout` are distinct values of the existing failure code,
each with its own user-visible message; none releases the admission reservation twice.

**UI.** The existing session status gains one state label with the reason code and progress phase; the existing stop action is the
cancel. All copy goes through `t()` in the seven catalogs. Playwright covers queued, cancel while queued, and refused, with the
fixture provider.

## Refused provision

With `provision_refusal` in the snapshot, `ProvisionExecutorEnvironmentResponse` may carry additive `refusal {reason, nothing_allocated}`.
`settleRefusedProvision` runs in one transaction: it checks that the operation has no checkpoint and no provider state, writes the
refusal reason, settles the record absent, and releases the reservation once. The write is guarded by the operation's revision, so a
concurrent cancel or archive cannot double-release. Any other error path is unchanged: a failed or lost provision runs recovery, and only
recovery or destroy can confirm absence (AC 002.4). The refusal is a distinct failure code in the task view.

## Data model

No new table and no DDL. Envelope fields: `owners`, `share_epoch`, `closing`, `admission_scope`, `reservation`,
`applied_resource_class`, `placement` (`state`, `reason`, `next_poll_at`, `deadline_at`, `phase`). They stay under the 32 KiB
cap (16 owners at about 100 bytes, labels capped). The reservation lives in the envelope of the same environment row, so its
count query reads the rows by scope; a scope index is not added, and the count is bounded by the active environments of one scope.

## Interleavings and failing-first tests

| Interleaving | Required outcome | Tests (level) |
| --- | --- | --- |
| Two owners release one shared environment concurrently (SHARED 001.6) | exactly one destroy; owner set empty; second release is a no-op or retries | sqlite and PostgreSQL multi-connection `-race`; fixture over the process boundary |
| Attach races the last release (001.6) | attach wins and environment stays, or attach gets `environment_closing` and provisions anew | sqlite and PostgreSQL race |
| Stale `share_epoch` call to the provider (001.4) | provider rejects; host does not retry as new | fixture conformance |
| Destroy attempted with owners remaining (001.9) | host refuses before calling the provider | lifecycle unit |
| Two launches compete for the last unit of one scope (ADMISSION 001.4) | one admitted, one denied | sqlite and PostgreSQL race |
| Admission hook racing a resume of the same scope (001.9) | serialized; loser sees the reservation; applied class never changes (001.8) | sqlite and PostgreSQL race; lifecycle unit |
| Hook unavailable (001.6) | `admission_unavailable`, no allocation | lifecycle unit |
| Reservation release after failure and after destroy (001.5) | released exactly once | lifecycle unit |
| Queued launch cancelled while placement completes (PLACEMENT 001.5) | one commit wins; no allocation remains; never ready | barrier e2e over the fixture; sqlite and PostgreSQL race |
| Restart while pending (001.7) | polling resumes; no second environment | barrier e2e |
| Deadline passes while a poll is in flight (001.2) | `placement_timeout`, destroy of anything late-placed | lifecycle unit with a fake clock (synctest) |
| Refusal racing a cancel or archive (PLACEMENT 002.2) | one transaction wins; reservation released once; record absent or cancelled, never retained | sqlite and PostgreSQL race |
| Refusal after a checkpoint was recorded (002.3) | `conflict`; recovery runs | lifecycle unit |
| Lost reply or timeout on provision (002.4) | treated as failure, never as refusal; recovery decides | lifecycle unit with fault-injecting client |
| Pending response from a provider without the capability (001.8) | `conflict`, cleanup | lifecycle unit |

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
