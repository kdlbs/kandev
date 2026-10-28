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

- **AC-EXECUTORS-PLUGIN-SHARED-001.1:** When a provider requires `workspace_sharing`, the host shall accept workspace reuse and shared-workspace launches for it. Without the capability, the host shall keep rejecting both with a stable code that names the missing capability.
- **AC-EXECUTORS-PLUGIN-SHARED-001.2:** The host shall record an owner set in the plugin inventory, with at most 16 owners. An owner entry shall hold only the owner identifier and the epoch at which it attached.
- **AC-EXECUTORS-PLUGIN-SHARED-001.3:** The host shall admit an attach only when the attaching owner has the same owning principal, workspace, provider installation, executor profile, workspace identity, and repository set as the environment, and when the environment is `ready` and not closing. Any other attach shall be refused with a named reason and shall allocate nothing.
- **AC-EXECUTORS-PLUGIN-SHARED-001.4:** Attach and release shall each advance the share epoch in one revision-checked write. Provider calls that follow shall carry the ownership generation and the share epoch, and the provider shall reject a stale pair.
- **AC-EXECUTORS-PLUGIN-SHARED-001.5:** Releasing an owner that is not the last shall not call destroy. Releasing the last owner shall, in the same write, mark the environment closing and empty the owner set, and the host shall then run the existing cleanup.
- **AC-EXECUTORS-PLUGIN-SHARED-001.6:** When two owners release concurrently, exactly one destroy shall run. When an attach races the last release, exactly one shall commit first: an attach that commits first keeps the environment, and an attach that loses receives `environment_closing` and provisions a new environment.
- **AC-EXECUTORS-PLUGIN-SHARED-001.7:** A shared environment shall hold one agentctl instance per owner, each with a distinct recorded port, and releasing an owner shall stop only that owner's instance.
- **AC-EXECUTORS-PLUGIN-SHARED-001.8:** The provider shall declare `sharing_supported` and a `max_owners` in profile validation, the host shall cap owners at the lower of the two values, and an attach beyond the cap shall be refused as `capacity_refused`.
- **AC-EXECUTORS-PLUGIN-SHARED-001.9:** The destroy request for a shared environment shall carry `owner_count` of zero, and the host shall reject a destroy while any owner remains.

### REQ-EXECUTORS-PLUGIN-ADMISSION-001: Identity and admission context

**Intent:** Quota and placement decisions see who is launching, and are enforced at every path that starts or resumes work.

#### Acceptance criteria

- **AC-EXECUTORS-PLUGIN-ADMISSION-001.1:** When a provider requires `admission_context`, provision and recovery requests shall carry task ID, session ID, workflow step ID, user ID, workspace (team) ID, project ID, requested resource class, priority (`low`, `normal`, `high`), and at most 16 labels. All identifiers shall be host-generated opaque IDs. No name, email address, or secret shall be sent.
- **AC-EXECUTORS-PLUGIN-ADMISSION-001.2:** Label keys shall match `[a-z0-9._/-]{1,64}` and values shall be at most 128 bytes of printable text. The host shall reject a label set that breaks the rule before allocation.
- **AC-EXECUTORS-PLUGIN-ADMISSION-001.3:** The host shall run its admission hook before any provider allocation call on first launch, delayed launch, and resume. The hook shall allow, deny with a stable reason, or allow with a lower resource class.
- **AC-EXECUTORS-PLUGIN-ADMISSION-001.4:** An allow decision shall record a reservation keyed by owner scope in the same write that creates the operation. Two launches competing for the last unit of one scope shall not both be admitted, on SQLite and on PostgreSQL.
- **AC-EXECUTORS-PLUGIN-ADMISSION-001.5:** A reservation shall be released on terminal launch failure, on destroy confirmation, and on cancellation, and shall be released exactly once.
- **AC-EXECUTORS-PLUGIN-ADMISSION-001.6:** When the hook is unavailable or fails, a launch for a provider requiring `admission_context` shall be denied with `admission_unavailable`. A provider not requiring it shall not call the hook.
- **AC-EXECUTORS-PLUGIN-ADMISSION-001.7:** The provider response shall return the applied resource class. The host shall reject a class that is not in the profile's allowed set or exceeds the admitted class, shall persist the applied class, and shall show it in the environment read.
- **AC-EXECUTORS-PLUGIN-ADMISSION-001.8:** A resume of a live environment shall keep its applied class. A resume that the hook would admit at a different class shall be refused with `conflict` and shall not resize the environment.
- **AC-EXECUTORS-PLUGIN-ADMISSION-001.9:** When a resume races a new launch for the same scope, both shall be serialized by the admission scope and the loser shall see the winner's reservation.

### REQ-EXECUTORS-PLUGIN-PLACEMENT-001: Queued and pending placement

**Intent:** A launch waiting for capacity is visible, cancellable, and bounded, and capacity refusal differs from placement failure.

#### Acceptance criteria

- **AC-EXECUTORS-PLUGIN-PLACEMENT-001.1:** When a provider requires `pending_placement`, a provision response can report `queued` or `placing` with a closed reason code, an optional queue position, a progress phase, and a poll interval. The host shall persist the pending state and the operation before replying.
- **AC-EXECUTORS-PLUGIN-PLACEMENT-001.2:** The host shall poll the provider within the clamped interval (2 to 60 seconds), shall record each phase change, and shall end polling at the provider's placement deadline (1 minute to 24 hours, default 10 minutes).
- **AC-EXECUTORS-PLUGIN-PLACEMENT-001.3:** The task and session shall show a pending-placement status with the reason code and progress phase, and the existing stop action shall cancel a queued launch. Every new user-facing string shall be localized.
- **AC-EXECUTORS-PLUGIN-PLACEMENT-001.4:** A cancel of a queued launch shall mark the operation cancelling under the claim and ask the provider to cancel. The outcome shall be `cancelled` (nothing allocated) or `already_placed`, and `already_placed` shall continue with the normal destroy.
- **AC-EXECUTORS-PLUGIN-PLACEMENT-001.5:** When placement completes while a cancel is in flight, exactly one shall commit first, and no allocation shall remain after the task reports cancelled.
- **AC-EXECUTORS-PLUGIN-PLACEMENT-001.6:** The host shall report failures as `capacity_refused` (the provider will not queue this request), `placement_failed` (the provider tried and failed), or `placement_timeout`, and shall not merge them into a generic error.
- **AC-EXECUTORS-PLUGIN-PLACEMENT-001.7:** After a backend or plugin restart the host shall resume polling the persisted operation and shall neither allocate a second environment nor lose the pending state.
- **AC-EXECUTORS-PLUGIN-PLACEMENT-001.8:** A provider not requiring `pending_placement` shall keep the current synchronous launch and deadline, and a pending response from it shall be rejected as `conflict`.

### REQ-EXECUTORS-PLUGIN-PLACEMENT-002: Refused provision releases the record

**Intent:** A provision the provider refused, with nothing allocated, does not leave a record that only recovery can release.

#### Acceptance criteria

- **AC-EXECUTORS-PLUGIN-PLACEMENT-002.1:** When a provider requires `provision_refusal`, a provision response can carry `refused` with `nothing_allocated`, and a reason from `capacity_refused`, `admission_refused`, `policy_refused`, or `invalid_request`.
- **AC-EXECUTORS-PLUGIN-PLACEMENT-002.2:** The host shall accept the refusal only when no checkpoint and no provider state for the operation was recorded. It shall then, in one write, record the refusal, settle the environment absent, and release the admission reservation, without running recovery.
- **AC-EXECUTORS-PLUGIN-PLACEMENT-002.3:** A refusal for an operation that has a recorded checkpoint or state shall be rejected as `conflict`, and recovery shall run as for any failed provision.
- **AC-EXECUTORS-PLUGIN-PLACEMENT-002.4:** A lost reply, a timeout, or a response error shall never be read as a refusal. Only recovery or destroy can say that a resource is absent.
- **AC-EXECUTORS-PLUGIN-PLACEMENT-002.5:** The task shall show the refusal reason as a distinct outcome, and a provider not requiring the capability shall keep today's behavior.

## Out of scope

- Resizing a live environment, preemption, and cross-provider failover.
- Autoscaling or provisioning machines.
- Per-person quotas as a feature: the hook is the seam and quota policy lives in whoever implements it.

## Implementation plans

- [Native worker capabilities for executor plugins](../../../plans/executor-plugin-native-workers/plan.md)
