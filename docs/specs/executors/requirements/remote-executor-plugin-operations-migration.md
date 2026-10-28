---
status: draft
system: executors
created: 2026-10-28
owners:
  - kandev
---

# Remote executor plugins: operations, launch policy, preparation, and migration requirements

## Overview

What an operator needs to run a worker-pool provider for a long time: a bounded view of its health and a typed way to
act on it, a launch-policy requirement that agentctl enforces, a workspace preparation contract that new strategies
can use without another core change, and a safe path from a built-in executor to a plugin. Each is an opt-in
[capability](remote-executor-plugin-capability-versioning.md).

## Terminology

- **Provider status:** A bounded, non-secret read of a provider's nodes and capacity.
- **Maintenance action:** A typed operator action from a closed set, applied by the provider and recorded by the host.
- **Launch policy:** A generic requirement that agent processes start under a sandbox or wrapper policy.
- **Preparation hints:** Non-secret, uninterpreted key-value intent that the host passes to a provider that prepares the workspace.
- **Authority:** The single owner of an environment's lifecycle, either the built-in executor or one plugin installation.

## Requirements

### REQ-EXECUTORS-PLUGIN-OPS-001: Provider status, inventory, events, logs, and maintenance

**Intent:** Operators see provider health in Kandev and act on it through one audited route.

#### Acceptance criteria

- **AC-EXECUTORS-PLUGIN-OPS-001.1:** When a provider requires `provider_operations`, an administrator read shall return provider status with at most 200 nodes, each with an opaque non-secret ID, a state (`ready`, `draining`, `quarantined`, `down`, `unknown`), capacity by resource class, and a reason code. Nodes shall be ordered by node ID in byte order. The response shall be at most 64 KiB, and an oversize response shall be rejected, never truncated silently.
- **AC-EXECUTORS-PLUGIN-OPS-001.2:** An inventory read shall page provider-known environments, ordered by provider ID in byte order (an opaque provider-issued cursor of at most 256 bytes, default and maximum page size 100; an empty cursor starts the read and an absent next cursor ends it) with each environment's provider ID, host environment reference, and state. The host shall mark entries it does not know as `unrecognized` and shall change no host record from this read.
- **AC-EXECUTORS-PLUGIN-OPS-001.3:** The provider shall publish events through a host callback with a kind from a closed set (`node_quarantined`, `node_recovered`, `capacity_low`, `capacity_restored`, `placement_stalled`, `provider_degraded`, `provider_recovered`, `maintenance_completed`), a severity (`info`, `warning`, or `critical`), an opaque subject ID, a message code matching `[a-z0-9_.]{1,64}` with at most 8 parameters (keys `[a-z0-9._-]{1,32}`, values at most 64 bytes, no secret), and a dedupe key of at most 128 bytes. The host shall render the kind as localized text under `executors:pluginEvent.<kind>`, shall show the code only as a parameter, and shall treat an unknown code as the kind's generic text.
- **AC-EXECUTORS-PLUGIN-OPS-001.4:** The host shall limit events to 30 per minute per installation, drop duplicates of a dedupe key within five minutes, and deliver the rest to the instance administrators through the existing notification service (a plugin installation is instance-wide, so there is no workspace audience). An event shall never carry a secret, a log line, or free text.
- **AC-EXECUTORS-PLUGIN-OPS-001.5:** A log read shall return at most 500 lines and 64 KiB for one environment or node, shall pass the host's secret redaction, and shall be limited to administrators.
- **AC-EXECUTORS-PLUGIN-OPS-001.6:** When a provider requires `provider_maintenance`, a typed action (`quarantine_node`, `readmit_node`, `drain_node`, `undrain_node`) with a subject ID, a reason code, and an idempotency key shall be accepted only on a generic administrator route, only for the named installation, and shall return `applied`, `noop`, or `refused` with a reason.
- **AC-EXECUTORS-PLUGIN-OPS-001.7:** The host shall write a structured audit log line for each maintenance request, before the call (`requested`) and after it (outcome), naming actor, installation, action, subject, and outcome. A request that repeats an idempotency key of the same installation within 24 hours with the same body shall return the recorded outcome without a second provider call; with a different body it shall be rejected as `conflict`. After 24 hours or a backend restart the key is forgotten and the request is a new one, which the provider treats idempotently by the key. A second request with the same installation and key that arrives while the first is in flight shall wait for it and return its outcome with no second provider call. Only `applied`, `noop`, and `refused` outcomes are recorded; a provider timeout (30 seconds), transport error, or `unavailable` is not recorded, is returned to the caller as `unavailable` with the audit outcome `unknown`, and a replay of the same key and body calls the provider again.
- **AC-EXECUTORS-PLUGIN-OPS-001.8:** A provider that is disabled, unavailable, or at a different negotiated version shall return `unavailable` from these reads, and the host shall keep working without them.

### REQ-EXECUTORS-PLUGIN-LAUNCHPOLICY-001: Launch policy requirement

**Intent:** A profile can require that agent processes start under a sandbox policy, and a launch that cannot enforce it fails closed.

#### Acceptance criteria

- **AC-EXECUTORS-PLUGIN-LAUNCHPOLICY-001.1:** When a provider requires `launch_policy`, the profile `config` shall accept the keys `launch_policy` (`none`, `preferred`, or `required`; an absent or empty value is `none`) and `policy_id` (an opaque ID of at most 64 characters matching `[A-Za-z0-9._-]+`, required for `preferred` and `required`, and rejected when the policy is `none`), and shall validate them when the profile is saved and again before allocation.
- **AC-EXECUTORS-PLUGIN-LAUNCHPOLICY-001.2:** Before allocation the host shall reject `required` when the provider does not report support for the policy ID with the code `launch_policy_unsupported` and no allocation, and shall reject any policy other than `none` on a provider that does not require `launch_policy` with the same code. A `preferred` policy the provider does not support is not rejected and is recorded as `unwrapped`.
- **AC-EXECUTORS-PLUGIN-LAUNCHPOLICY-001.3:** The workspace descriptor shall carry `applied_launch_policy` (`none` or `applied`) and the applied policy ID. Policies are ordered `none`, `preferred`, `required`. The host shall reject a descriptor weaker than the profile: for `required`, anything but `applied` with the profile's policy ID. A profile of `none` or `preferred` accepts any descriptor.
- **AC-EXECUTORS-PLUGIN-LAUNCHPOLICY-001.4:** For `required`, agentctl shall refuse to start an agent process without the wrapper and shall report the failure, never falling back to an unwrapped start. For `preferred` it shall try the wrapper on every agent start, start unwrapped when none is available, and report on each start whether it did; the host shall record the latest outcome in the envelope.
- **AC-EXECUTORS-PLUGIN-LAUNCHPOLICY-001.5:** Wrapper discovery, placement, and node-local files shall stay in the provider and its worker; the provider gives agentctl the wrapper specification through the instance environment, and the host and agentctl shall handle only the policy ID and the outcome.

### REQ-EXECUTORS-PLUGIN-PREPARATION-001: Extensible workspace preparation

**Intent:** A new preparation strategy (mirror, sparse checkout, pull-request checkout) is provider-only work.

#### Acceptance criteria

- **AC-EXECUTORS-PLUGIN-PREPARATION-001.1:** When a provider requires `preparation_hints`, each repository intent entry shall carry a `checkout` of kind `branch`, `ref`, or `pull_request` with a value, optional `sparse_paths` (at most 64), and `hints`. The host shall build them from the profile `config` keys `checkout_kind` (default `branch`), `sparse_paths` (a newline-separated list; empty means the whole tree), and `hint.<key>` entries, applied to every repository of the launch, with no per-repository override. For kind `branch` the value is the task's branch for that repository, or its base branch when the task names none; for `ref` the value is the profile key `checkout_value`; for `pull_request` it is the pull request number linked to the task. A profile of kind `ref` or `pull_request` for a task that names a branch, a missing value, a duplicate sparse path or hint key, an absolute or `..` sparse path, or a sparse path over 512 bytes shall be rejected at launch as `invalid_config` with a named reason and no allocation.
- **AC-EXECUTORS-PLUGIN-PREPARATION-001.2:** `hints` shall hold at most 16 entries, keys `[a-z0-9._-]{1,64}`, values at most 256 bytes, no secret. The host shall pass them through uninterpreted and shall include them in the operation input digest.
- **AC-EXECUTORS-PLUGIN-PREPARATION-001.3:** The provider shall return for each repository its ID, path, branch, head commit, applied checkout kind, and an opaque strategy label of at most 64 characters. The host shall verify the set, the path containment in the workspace, and the commit format, and shall record the label without interpreting it.
- **AC-EXECUTORS-PLUGIN-PREPARATION-001.4:** The host shall not change for a new strategy label or a new hint key. A provider that returns a repository the intent did not name shall be rejected under REQ-EXECUTORS-NATIVE-WORKER-003.
- **AC-EXECUTORS-PLUGIN-PREPARATION-001.5:** Credentials shall stay executor-owned, as in REQ-EXECUTORS-NATIVE-WORKER-003.

### REQ-EXECUTORS-PLUGIN-MIGRATION-001: Migration and coexistence

**Intent:** An operator moves from a built-in executor to a plugin without a mixed-authority environment, and can go back.

#### Acceptance criteria

- **AC-EXECUTORS-PLUGIN-MIGRATION-001.1:** An administrator read shall list the environments of one built-in executor type (any of `local_docker`, `remote_docker`, `sprites`, `ssh`, `k8s`; main has no other remote built-in) with a state of `active`, `drainable`, or `orphaned`, and the count by state. It shall change no record.
- **AC-EXECUTORS-PLUGIN-MIGRATION-001.2:** Each environment record shall have exactly one authority, `builtin` or one plugin installation. A record without an authority key reads as `builtin` and is not rewritten; the key is written only on a record that the migration operation touches. A transition shall happen only through the migration operation and under the cleanup claim. No code shall convert a record by renaming its executor type.
- **AC-EXECUTORS-PLUGIN-MIGRATION-001.3:** An administrator shall start the operation with `POST /api/v1/executors/profiles/:profile_id/migration` naming a target plugin profile, and shall read its step with `GET` on the same path. The operation shall freeze new launches on the built-in profile with `executor_migrating`, wait for active sessions to stop with no automatic timeout (the read shows the count still active), drain and destroy built-in environments by the built-in cleanup, and only then switch the profile. A launch admitted before the freeze commits shall finish and be drained.
- **AC-EXECUTORS-PLUGIN-MIGRATION-001.4:** The operation shall be resumable: a backend restart shall continue it from its recorded step and shall not freeze twice or destroy a plugin environment.
- **AC-EXECUTORS-PLUGIN-MIGRATION-001.5:** Rollback (`POST` with `action: rollback`, allowed in the steps `freezing`, `draining`, and `switching`; in `done` or `rolled_back` it returns the recorded outcome, and a start on a profile with an operation in `freezing`, `draining`, or `switching` returns the existing operation) shall switch new launches back to the built-in profile, shall leave plugin environments under the plugin as retained records, and shall not adopt them. A later drain shall destroy them.
- **AC-EXECUTORS-PLUGIN-MIGRATION-001.6:** After rollback, status and list reads shall still show retained plugin environments, and disabling or removing the plugin shall not lose them.
- **AC-EXECUTORS-PLUGIN-MIGRATION-001.8:** The host shall store the operation in a new table `executor_profile_migrations`, one row per source profile, holding the target profile ID, the step, a revision, and the created and updated times; a start shall insert the row and a step change shall update it only when the revision matches, so two concurrent starts yield one operation and the loser reads it, and a replayed start returns it. "Switch the profile" shall mean that the host sets the source profile's `config` key `migrated_to` to the target profile ID and that every later launch for the source profile resolves to the target; rollback removes the key. The enumeration of AC 001.1 shall order environments by `created_at` ascending, then ID in byte order, and an environment without an owner reads as `orphaned`.
- **AC-EXECUTORS-PLUGIN-MIGRATION-001.7:** This document removes no built-in executor and adds no gate in the host on any live scenario; whether and when a built-in is removed is a separate owner decision, and the scenarios that inform it live in the vendor plugin repository.

## Out of scope

- Converting or renaming records between authorities.
- Autoscaling, per-person quotas, fleet-management UI beyond the status read, and hostile-workload isolation.
- A per-node repository mirror itself; only the contract in REQ-EXECUTORS-PLUGIN-PREPARATION-001.

## Implementation plans

- [Native worker capabilities for executor plugins](../../../plans/executor-plugin-native-workers/plan.md)
