---
status: draft
system: executors
created: 2026-10-28
owners:
  - kandev
---

# Remote executor plugins: queued and pending placement requirements

## Overview

Continuation of [shared environments, admission, and placement](remote-executor-plugin-shared-admission-placement.md)
(moved here for the file size limit, IDs unchanged): placement that can wait in a queue beyond the current launch deadline, and
a refusal that releases the record. The terms of that document apply. Both are opt-in [capabilities](remote-executor-plugin-capability-versioning.md).

## Requirements

### REQ-EXECUTORS-PLUGIN-PLACEMENT-001: Queued and pending placement

**Intent:** A launch waiting for capacity is visible, cancellable, and bounded, and capacity refusal differs from placement failure.

#### Acceptance criteria

- **AC-EXECUTORS-PLUGIN-PLACEMENT-001.1:** When a provider requires `pending_placement`, a provision response can report `queued` or `placing` with a closed reason code, an optional queue position, a progress phase, and a poll interval. The host shall persist the pending state and the operation before replying.
- **AC-EXECUTORS-PLUGIN-PLACEMENT-001.2:** The host shall poll the provider within the clamped interval (2 to 60 seconds), shall record each phase change, and shall end polling at the placement deadline. The deadline is the `placement_deadline_seconds` that the provider reports in profile validation (60 to 86400, default 600) and is recorded in the pending state at the first pending response; a provider response cannot extend it.
- **AC-EXECUTORS-PLUGIN-PLACEMENT-001.3:** The task and session shall show a pending-placement status with the reason code and progress phase, and the existing stop action shall cancel a queued launch. While the operation is cancelling, the session shall stay `STARTING` with the label `cancelling` and shall move to its stopped state only when the provider says `cancelled` or a destroy confirms absence. Every new user-facing string shall be localized.
- **AC-EXECUTORS-PLUGIN-PLACEMENT-001.4:** A cancel of a queued launch shall mark the operation cancelling under the claim and ask the provider to cancel. The outcome shall be `cancelled` (nothing allocated) or `already_placed`, and `already_placed` shall continue with the normal destroy. When the cancel call fails or times out, the operation shall stay cancelling and the host shall retry with backoff until the placement deadline, then run the existing recovery destroy by operation; it shall report cancelled only after the provider says `cancelled` or a destroy confirms absence.
- **AC-EXECUTORS-PLUGIN-PLACEMENT-001.5:** When placement completes while a cancel is in flight, exactly one shall commit first. If the cancel commits first, a later completion is treated as `already_placed`, destroy runs to confirmation, the launch outcome is `placement_cancelled`, and the session ends in its existing stopped state with no allocation left. If the completion commits first, the launch is ready, the stop is processed as an ordinary Stop (the environment is preserved, as today), and the task shall not report the launch as cancelled.
- **AC-EXECUTORS-PLUGIN-PLACEMENT-001.6:** The host shall report failures as `capacity_refused` (the provider will not queue this request; this is the refusal reason of REQ-EXECUTORS-PLUGIN-PLACEMENT-002, one outcome), `placement_failed` (the provider tried and failed), or `placement_timeout`, and shall not merge them into a generic error.
- **AC-EXECUTORS-PLUGIN-PLACEMENT-001.7:** After a backend or plugin restart the host shall resume polling the persisted operation and shall neither allocate a second environment nor lose the pending state.
- **AC-EXECUTORS-PLUGIN-PLACEMENT-001.8:** A provider not requiring `pending_placement` shall keep the current synchronous launch and deadline, and a pending response from it shall be rejected as `conflict`.
- **AC-EXECUTORS-PLUGIN-PLACEMENT-001.9:** Every host-originated launch failure code a user can meet (`capability_mismatch`, `missing_capability`, `executor_migrating`, `admission_denied` with its reasons including `scope_full`, `admission_unavailable`, `launch_policy_unsupported`, `launch_policy_unenforced`, `secret_reference_missing`, `secret_in_state`, `bootstrap_consumed`, `repository_inventory_mismatch`, `state_too_large`, `api_unreachable`, the three placement outcomes, and each refusal reason) shall appear in the task and session view as localized text that names the next step, shown with an icon and text and never by colour alone. The codes `environment_closing` and `conflict` on attach are resolved by the host and shall not reach the user.
- **AC-EXECUTORS-PLUGIN-PLACEMENT-001.10:** The reason codes are the closed set `no_capacity`, `quota_wait`, `node_unavailable`, `maintenance`, and `other`; the progress phases are the closed set `queued`, `reserving`, `provisioning`, `starting_worker`, and `preparing_workspace`. A value outside a set shall be recorded as `other` (reason) or shown with the text under `executors:pluginPlacement.unknown` (phase), and a provider-supplied string shall never be displayed. A queue position is 1-based, and an absent or zero position means unknown. An absent or zero poll interval is 5 seconds before clamping. A response carrying more than one of a resource, a refusal, and a pending state shall be rejected as `conflict`.
- **AC-EXECUTORS-PLUGIN-PLACEMENT-001.11:** When a poll reports that placement is complete and no cancel committed first, the host shall continue the launch from the persisted operation through the same post-provision path as a synchronous launch (bootstrap handshake, instance creation, and the agent start with the prompt already stored for the session) without a user action. This continuation is the delayed launch of REQ-EXECUTORS-PLUGIN-ADMISSION-001: it re-runs the capability gate against the negotiated set recorded at admission (a mismatch ends as `capability_mismatch` with a destroy), and it keeps the reservation taken at the original launch without a second admission. Until the environment is ready the session shall be `STARTING` with the pending-placement label, and a failure shall give the session the failure code. The bootstrap nonce shall be valid until the placement deadline plus 600 seconds, and a placement reported after the deadline shall end as `placement_timeout` with a destroy.
- **AC-EXECUTORS-PLUGIN-PLACEMENT-001.12:** One poll loop per operation shall exist in a backend process, held in a registry keyed by operation ID and created again from the persisted state after a restart. A poll that returns a transport error or `unavailable` shall be retried at the next interval until the deadline. `rate_limited` shall retry at the next interval (a longer provider hint is honored up to the clamp). `not_found`, `denied`, `invalid_config`, `unsupported`, `expired`, `conflict`, and `cleanup_pending` end as `placement_failed`, each followed by the recovery destroy by operation, since only destroy confirms absence. A cancel retry shall wait 2 seconds, doubling to a cap of 30 seconds.

### REQ-EXECUTORS-PLUGIN-PLACEMENT-002: Refused provision releases the record

**Intent:** A provision the provider refused, with nothing allocated, does not leave a record that only recovery can release.

#### Acceptance criteria

- **AC-EXECUTORS-PLUGIN-PLACEMENT-002.1:** When a provider requires `provision_refusal`, a provision response can carry `refused` with `nothing_allocated`, and a reason from `capacity_refused`, `admission_refused`, `policy_refused`, or `invalid_request`. A provider requiring `pending_placement` shall also require `provision_refusal`, so a pending-placement provider's refusal always settles through this path.
- **AC-EXECUTORS-PLUGIN-PLACEMENT-002.2:** The host shall accept the refusal only when no checkpoint and no provider state for the operation was recorded. It shall then, in one write, record the refusal, settle the environment absent, and release the admission reservation, without running recovery.
- **AC-EXECUTORS-PLUGIN-PLACEMENT-002.3:** A refusal for an operation that has a recorded checkpoint or state shall be rejected as `conflict`, and recovery shall run as for any failed provision.
- **AC-EXECUTORS-PLUGIN-PLACEMENT-002.4:** A lost reply, a timeout, or a response error shall never be read as a refusal. Only recovery or destroy can say that a resource is absent.
- **AC-EXECUTORS-PLUGIN-PLACEMENT-002.5:** The task shall show the refusal reason as a distinct outcome, and a provider not requiring the capability shall keep today's behavior.

## Out of scope

- Resizing a live environment, preemption, and cross-provider failover.
- Autoscaling or provisioning machines.

## Implementation plans

- [Native worker capabilities for executor plugins](../../../plans/executor-plugin-native-workers/plan.md)
