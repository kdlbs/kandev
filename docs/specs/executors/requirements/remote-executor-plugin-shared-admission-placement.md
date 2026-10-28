---
status: draft
system: executors
created: 2026-10-28
owners:
  - kandev
---

# Remote executor plugins: shared environments, admission, and placement requirements

## Overview

Three launch-path behaviors that a worker-pool provider needs and the plugin path rejects or lacks today: several
owners sharing one environment, a non-secret admission context checked atomically before allocation, and placement
that can wait in a queue beyond the current launch deadline. Each is an opt-in [capability](remote-executor-plugin-capability-versioning.md).
A provider declaring none keeps today's behavior, including the rejection of workspace reuse and shared workspaces.

## Terminology

- **Owner:** One task environment owner (task and session binding) attached to a shared environment.
- **Share epoch:** A counter that increases on every attach or release of an environment's owner set.
- **Admission context:** Non-secret identifiers and labels describing who is launching and under which class.
- **Admission hook:** Host-side code that allows, denies, or reserves capacity before any allocation.
- **Pending placement:** A recorded state where the provider accepted the request but has not placed it.

## Requirements

### REQ-EXECUTORS-PLUGIN-SHARED-001: Workspace reuse and shared environments

**Intent:** Owners that are allowed to share one environment attach to it, and it is destroyed only when the last owner leaves.

#### Acceptance criteria

- **AC-EXECUTORS-PLUGIN-SHARED-001.1:** When a provider requires `workspace_sharing`, the host shall accept workspace reuse and shared-workspace launches for it. Without the capability, the host shall keep rejecting both with the stable code `missing_capability`, which names the missing capability.
- **AC-EXECUTORS-PLUGIN-SHARED-001.2:** The host shall record an owner set, with at most 16 owners, in the envelope of the environment's holder row. The holder row is the `executors_running` row of the owner that created the environment; every other owner has its own row (one per session) that names the holder and records that owner's own instance port. An owner entry shall hold only the owner identifier (the session ID) and the epoch at which it attached.
- **AC-EXECUTORS-PLUGIN-SHARED-001.3:** The host shall admit an attach only when the attaching owner has the same owning principal, workspace, provider installation, executor profile, workspace identity, and repository set as the environment, and when the environment is `ready` and not closing. Any other attach shall be refused with a named reason and shall allocate nothing.
- **AC-EXECUTORS-PLUGIN-SHARED-001.4:** Attach and release shall each advance the share epoch in one revision-checked write. Provider calls that follow shall carry the ownership generation and the share epoch, and the provider shall reject a stale pair.
- **AC-EXECUTORS-PLUGIN-SHARED-001.5:** Releasing an owner that is not the last shall not call destroy, and shall remove that owner's row, except that a holder row whose own owner left shall remain as the environment's record until the set is empty. Releasing the last owner shall, in the same write, mark the environment closing and empty the owner set, and the host shall then run the existing cleanup under a cleanup claim whose operation ID is the releasing operation. The record row of the last owner shall stay, with the envelope marked `closing`, until destroy confirms absence, so a crash or a failed destroy leaves a row for the cleanup sweep to find. Releasing an owner that is not in the set shall succeed as a no-op. On a release that is not the last, the host shall stop that owner's agentctl instance before it deletes the owner row; a failed stop shall keep the row with phase `releasing` and the owner out of the set, and the cleanup sweep shall retry the stop.
- **AC-EXECUTORS-PLUGIN-SHARED-001.6:** When two owners release concurrently, the release that commits first removes its owner, the other re-reads the new revision (at most three attempts, then `conflict` to its caller, which the existing cleanup sweep retries), and exactly one destroy runs, started by the release that found the set holding only itself. When an attach races the last release, exactly one shall commit first: an attach that commits first keeps the environment, and an attach that loses receives `environment_closing`, releases its own reservation once, and provisions a new environment in the same call, where a denied admission surfaces as `admission_denied`. Deleting the holder's task while other owners remain shall release the holder's owner first and shall keep the `task_environments` row and the record row until the last release confirms absence.
- **AC-EXECUTORS-PLUGIN-SHARED-001.7:** A shared environment shall hold one agentctl instance per owner, each with a distinct recorded port, and releasing an owner shall stop only that owner's instance.
- **AC-EXECUTORS-PLUGIN-SHARED-001.8:** The provider shall declare `sharing_supported` and a `max_owners` in profile validation, the host shall cap owners at the lower of the two values, and an attach beyond the cap shall be refused as `capacity_refused`.
- **AC-EXECUTORS-PLUGIN-SHARED-001.9:** The destroy request for a shared environment shall carry `owner_count` of zero, and the host shall reject a destroy while any owner remains.
- **AC-EXECUTORS-PLUGIN-SHARED-001.10:** An owner shall be released only by the events that destroy an unshared environment today (archive, delete, reset, and failed launch). Ordinary Stop, backend shutdown, and plugin disable shall not release an owner, because they preserve the environment.
- **AC-EXECUTORS-PLUGIN-SHARED-001.11:** When the owner of the holder row is released while other owners remain, the host shall, in the same transaction, re-key that row's `session_id` to `holder:` followed by the environment ID, so the released session can launch again without colliding with the unique `session_id`, and shall exclude re-keyed rows from the task and session delete cascades; the row of the last owner is deleted only when destroy confirms absence (AC-EXECUTORS-PLUGIN-SHARED-001.5). Every owner row and every re-keyed holder row shall carry a complete `plugin_executor` envelope (installation, provider key, phase, holder reference), so the retained-inventory reads, the upgrade and uninstall guards, and the recovery and cleanup sweeps treat each as an ordinary plugin row of that installation and count it once.
- **AC-EXECUTORS-PLUGIN-SHARED-001.12:** A provider call rejected for a stale `{ownership_generation, share_epoch}` pair shall surface as `conflict` to its caller, and the host shall not retry it inside the same call with a refreshed pair. A launch or cleanup in flight for one owner when another owner attaches or releases shall succeed or fail on its own revision check and shall never write over a newer epoch; the caller's own path (launch retry or the cleanup sweep) re-reads the environment.
- **AC-EXECUTORS-PLUGIN-SHARED-001.13:** An attach shall run admission under AC-EXECUTORS-PLUGIN-ADMISSION-001.3 like any launch and shall hold its own reservation in its own owner row. Releasing an owner that is not the last shall release that owner's reservation in the release transaction, exactly once. The last owner's reservation shall be released only when destroy confirms absence (AC-EXECUTORS-PLUGIN-ADMISSION-001.5).
- **AC-EXECUTORS-PLUGIN-SHARED-001.14:** An attach shall write the owner row with phase `attaching` in the same transaction that appends the owner. An `attaching` owner shall become `ready` in the transaction that records its verified instance port. A recovery pass shall release an `attaching` owner whose agentctl instance was never created, through the same release as AC-EXECUTORS-PLUGIN-SHARED-001.5, and a replayed attach of an owner already in the set shall return the existing owner without a second row.
- **AC-EXECUTORS-PLUGIN-SHARED-001.15:** When several ready environments match one reuse request, the host shall try them in order of holder row `created_at` ascending, then holder row `id` in byte order, and shall attach to the first one that passes AC-EXECUTORS-PLUGIN-SHARED-001.3 and admission. An environment already at its owner cap shall be skipped as a candidate, not refused; when no candidate passes, the host shall provision a new environment, except that a request that requires reuse (`WorkspaceReuseRequired`) shall fail with `capacity_refused` when every match is at its cap (AC-EXECUTORS-PLUGIN-SHARED-001.8).

### REQ-EXECUTORS-PLUGIN-ADMISSION-001: Identity and admission context

**Intent:** Quota and placement decisions see who is launching, and are enforced at every path that starts or resumes work.

#### Acceptance criteria

- **AC-EXECUTORS-PLUGIN-ADMISSION-001.1:** When a provider requires `admission_context`, provision and recovery requests shall carry task ID, session ID, workflow step ID, user ID, workspace (team) ID, project ID, requested resource class, priority (`low`, `normal`, `high`), and at most 16 labels. All identifiers shall be host-generated opaque IDs. No name, email address, or secret shall be sent.
- **AC-EXECUTORS-PLUGIN-ADMISSION-001.2:** Label keys shall match `[a-z0-9._/-]{1,64}` and values shall be at most 128 bytes of printable text. The host shall reject a label set that breaks the rule before allocation.
- **AC-EXECUTORS-PLUGIN-ADMISSION-001.3:** The host shall run its admission hook before any provider allocation call on first launch and on a resume of a stopped environment (the continuation of a pending placement keeps its reservation and is not an admission, AC-EXECUTORS-PLUGIN-PLACEMENT-001.11), through one shared function that every call site of the launch validation uses. The hook shall allow, deny with a stable reason, or allow with a lower resource class, and shall name the admission scope and its limit. The hook shall be called outside any database transaction with a fixed 5-second deadline; a deadline overrun is a failure under AC-EXECUTORS-PLUGIN-ADMISSION-001.6. Recovery of an admitted environment is not an admission.
- **AC-EXECUTORS-PLUGIN-ADMISSION-001.4:** An allow decision with a scope shall record a reservation in the same write that creates the operation, after that write has counted the live reservations of the scope under a lock whose key the host derives from the validated scope. Two launches competing for the last unit of one scope shall not both be admitted, on SQLite and on PostgreSQL, and the loser shall receive `admission_denied` with the reason `scope_full`. An allow decision with an empty scope (the default hook) records no reservation. A decision with a scope and a limit of zero or less is a denial with the reason `scope_full`.
- **AC-EXECUTORS-PLUGIN-ADMISSION-001.5:** A reservation shall be released exactly once, and only when absence is confirmed: on destroy confirmation (including the last owner's release and a terminal launch failure, after the recovery destroy confirms), on a provider `cancelled` or a refusal that allocated nothing, and on release of a non-last owner. While a cancel is uncertain (AC-EXECUTORS-PLUGIN-PLACEMENT-001.4) or a destroy is unconfirmed, the reservation stays held.
- **AC-EXECUTORS-PLUGIN-ADMISSION-001.6:** When the hook is unavailable or fails, a launch for a provider requiring `admission_context` shall be denied with `admission_unavailable`. A provider not requiring it shall not call the hook, and the host shall run no admission step for it.
- **AC-EXECUTORS-PLUGIN-ADMISSION-001.7:** The provider response shall return the applied resource class. The host shall reject a class that is not in the profile's allowed set or exceeds the admitted class, shall persist the applied class, and shall show it in the environment read.
- **AC-EXECUTORS-PLUGIN-ADMISSION-001.8:** A resume of a live environment shall keep its applied class and its existing reservation, and shall not take a second one. A resume that the hook would admit at a different class shall be refused with `conflict` and shall not resize the environment.
- **AC-EXECUTORS-PLUGIN-ADMISSION-001.9:** When a resume races a new launch for the same scope, both shall be serialized by the scope lock. The resume never loses its reservation, and a new launch that no longer fits shall be refused as `admission_denied` with the reason `scope_full`.
- **AC-EXECUTORS-PLUGIN-ADMISSION-001.10:** The admission context shall be built from these sources only. The requested resource class is the profile's `resource_class` value, or the provider's `default_resource_class` when the profile has none; the provider reports `resource_classes` (an ordered list of at most 16 names matching `[a-z0-9._-]{1,32}`, lowest first) and `default_resource_class` in profile validation, and a provider reporting no list has the single class `default`. The allowed set is that list, and "lower than" and "exceeds" compare list positions. The priority is the task priority mapped `critical` and `high` to `high`, `medium` to `normal`, `low` to `low`, and unset to `normal`. The project ID is the task's project ID, and the workflow step ID is the task's current step; either is the empty string when the task has none. No field of the context is omitted; an absent value is the empty string.

Requirements PLACEMENT-001 and PLACEMENT-002 (queued and pending placement, refused provision) are in [placement](remote-executor-plugin-placement.md); they keep their IDs.

## Out of scope

- Resizing a live environment, preemption, and cross-provider failover.
- Autoscaling or provisioning machines.
- Per-person quotas as a feature: the hook is the seam and quota policy lives in whoever implements it.

## Implementation plans

- [Native worker capabilities for executor plugins](../../../plans/executor-plugin-native-workers/plan.md)
