---
status: draft
system: executors
created: 2026-10-28
owners:
  - kandev
---

# Remote executor plugins: admission and placement requirements

## Overview

Two launch-path behaviors that a worker-pool provider needs and the plugin path lacks today: a non-secret admission
context checked atomically before allocation, and placement that can wait in a queue beyond the current launch deadline.
Each is an opt-in [capability](remote-executor-plugin-capability-versioning.md). A provider declaring none keeps today's
behavior, including the rejection of workspace reuse and shared workspaces (AC-EXECUTORS-PLUGIN-002.2). Shared
environments were cut from this cycle; see Out of scope.

## Terminology

- **Admission context:** Non-secret identifiers and labels describing who is launching and under which class.
- **Admission hook:** Host-side code that allows, denies, or reserves capacity before any allocation.
- **Pending placement:** A recorded state where the provider accepted the request but has not placed it.

## Requirements

### REQ-EXECUTORS-PLUGIN-ADMISSION-001: Identity and admission context

**Intent:** Quota and placement decisions see who is launching, and are enforced at every path that starts or resumes work.

#### Acceptance criteria

- **AC-EXECUTORS-PLUGIN-ADMISSION-001.1:** When a provider requires `admission_context`, provision and recovery requests shall carry task ID, session ID, workflow step ID, user ID, workspace (team) ID, project ID, requested resource class, priority (`low`, `normal`, `high`), and at most 16 labels. All identifiers shall be host-generated opaque IDs. No name, email address, or secret shall be sent.
- **AC-EXECUTORS-PLUGIN-ADMISSION-001.2:** Label keys shall match `[a-z0-9._/-]{1,64}` and values shall be at most 128 bytes of printable text. The host shall reject a label set that breaks the rule before allocation.
- **AC-EXECUTORS-PLUGIN-ADMISSION-001.3:** The host shall run its admission hook before any provider allocation call on first launch and on a resume of a stopped environment (the continuation of a pending placement keeps its reservation and is not an admission, AC-EXECUTORS-PLUGIN-PLACEMENT-001.11), through one shared function that every call site of the launch validation uses. The hook shall allow, deny with a stable reason, or allow with a lower resource class, and shall name the admission scope and its limit. The hook shall be called outside any database transaction with a fixed 5-second deadline; a deadline overrun is a failure under AC-EXECUTORS-PLUGIN-ADMISSION-001.6. Recovery of an admitted environment is not an admission.
- **AC-EXECUTORS-PLUGIN-ADMISSION-001.4:** An allow decision with a scope shall record a reservation in the same write that creates the operation, after that write has counted the live reservations of the scope under a lock whose key the host derives from the validated scope. Two launches competing for the last unit of one scope shall not both be admitted, on SQLite and on PostgreSQL, and the loser shall receive `admission_denied` with the reason `scope_full`. An allow decision with an empty scope (the default hook) records no reservation. A decision with a scope and a limit of zero or less is a denial with the reason `scope_full`.
- **AC-EXECUTORS-PLUGIN-ADMISSION-001.5:** A reservation shall be released exactly once, and only when absence is confirmed: on destroy confirmation (including a terminal launch failure, after the recovery destroy confirms), and on a provider `cancelled` or a refusal that allocated nothing. While a cancel is uncertain (AC-EXECUTORS-PLUGIN-PLACEMENT-001.4) or a destroy is unconfirmed, the reservation stays held.
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
- **Workspace reuse and shared environments (plan item M1; formerly REQ-EXECUTORS-PLUGIN-SHARED-001 and the `workspace_sharing` capability).** Several owners attaching to one provider environment, with an owner set, a share epoch, holder and owner rows, per-owner agentctl instances, an owner cap, and destroy only on the last release. Why it was cut: Spec Review round 5 (the cap) found 17 open defects, about two thirds of them in this cluster, and the rules for owner release, `closing` retention, reclaim paths, and reservations contradicted each other; the Nomad plugin does not need cross-task sharing for its first release. The owner knowingly reversed an earlier decision to keep M1 in this cycle (decided 2026-10-28, option c2). **Consequence: adding M1 later is a second contract change** (a new capability, new envelope fields and owner rows, new failure codes, and amendments to ADMISSION-001.5 and the destroy and cleanup paths), not a drop-in. Until then workspace reuse and shared-workspace launches stay rejected exactly as today (AC-EXECUTORS-PLUGIN-002.2). The follow-up is card 99beef6d-530e-4afd-949b-1424fc9245df ("Spec: shared executor environments (M1) for plugins"). **That card must decide K5 first, before any design work: the reuse candidate set versus `WorkspaceReuseRequired` on main** (on main it selects attach-only preparation of the request's own already-ready task environment; the cut text instead pooled any matching ready holder across tasks of one principal). Its starting defect list: K1 owner-row deletion versus `closing` retention and the cleanup sweep; K2 owner and re-keyed holder rows reclaimed by the existing archive, startup, and reaper paths (ADR 0025); K3 deleting a non-holder or `releasing` owner's task releasing its reservation early; K4 reservation written with the operation versus in the owner row, and how many times the hook runs per request; K5 reuse candidate set; K7 `attaching` owner recovery (startup or periodic, `ListInstances` failure, ready-write fencing, replayed attach, duplicate-attach dedup); K9 holder-task delete check versus the multi-step durable delete job; K13 `owner_phase` naming; K15 two callers releasing the same owner; K16 stale-fence attach `conflict` versus PLACEMENT-001.9; K17 `max_owners` absent or non-positive and what proves absence on a failed `ListInstances`. The removed text is in git history (commit 4c6dcdf29c and earlier) of the former `remote-executor-plugin-shared-admission-placement.md` requirements and design.

## Implementation plans

- [Native worker capabilities for executor plugins](../../../plans/executor-plugin-native-workers/plan.md)
