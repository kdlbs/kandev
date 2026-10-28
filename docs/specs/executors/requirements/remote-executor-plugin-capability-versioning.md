---
status: draft
system: executors
created: 2026-10-28
owners:
  - kandev
---

# Remote executor plugins: capability negotiation and versioning requirements

## Overview

Every opt-in host behavior for executor plugins, present and future, is a named, versioned capability in one
registry. A provider declares the capabilities and versions it speaks; the host admits or rejects it before it
allocates anything, pins the negotiated set to each environment, and refuses an upgrade or rollback that would strand
recorded state. This extends [native worker capabilities](remote-executor-plugin-native-workers.md), whose
REQ-EXECUTORS-NATIVE-WORKER-001 defers to this document for the registry and the manifest syntax.

## Terminology

- **Capability registry:** The host's closed list of capability names, each with a supported version range and a deprecation state.
- **Negotiated set:** The capabilities and versions a provider declared that the host accepted.
- **State schema version:** An integer a provider writes with its resource state so a later release can tell whether it can read it.
- **Retired capability version:** A version the host no longer accepts; its identity is never reused.

## Requirements

### REQ-EXECUTORS-PLUGIN-CAPABILITY-001: One capability registry

**Intent:** Provider authors and operators see one authoritative list of what the host offers.

#### Acceptance criteria

- **AC-EXECUTORS-PLUGIN-CAPABILITY-001.1:** The host shall define a closed registry of capability names. Each entry shall carry the lowest and highest supported version, a deprecation state, and the release that introduced it.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-001.2:** The registry shall contain, at the time of this document: `workspace_descriptor`, `repository_intent`, `tunnel_transport`, `api_url_forward`, `drain_receipt`, `destroy_checkpoints`, `durable_bootstrap`, `workspace_sharing`, `admission_context`, `pending_placement`, `provider_operations`, `provider_maintenance`, `launch_policy`, `preparation_hints`, `typed_profile_values`, `secret_references`, and `provision_refusal`, each at version 1.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-001.3:** A manifest shall declare `requires_host_capabilities` as a list of objects with `name` and `version` (integer, at least 1). The validator shall reject an unknown name, a duplicate name, a non-integer or zero version, and a list longer than the registry.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-001.4:** The validator shall enforce cross-capability dependencies from the registry (for example `provider_maintenance` requires `provider_operations`) and shall name the missing dependency. `pending_placement` requires `destroy_checkpoints`, `provision_refusal`, and `durable_bootstrap`.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-001.5:** The registry shall be readable by authorized administrators through the existing plugin status read, with names, supported ranges, and deprecation state, and by a provider in the profile validation request (REQ-EXECUTORS-PLUGIN-CAPABILITY-002).

### REQ-EXECUTORS-PLUGIN-CAPABILITY-002: Negotiation before allocation

**Intent:** An incompatible provider is rejected at the earliest point, never after it has created a resource.

#### Acceptance criteria

- **AC-EXECUTORS-PLUGIN-CAPABILITY-002.1:** Install, upgrade, and enable shall reject a manifest that requires a name outside the registry, or a version outside the supported range, before the plugin is activated, and shall name the capability, the requested version, and the supported range.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-002.2:** The profile validation request shall carry the host's registry, and its response shall echo the capabilities and versions the running provider process implements. The host shall keep the last accepted echo per installation and dispatch generation. When an echo differs from the manifest declaration, or differs from an earlier accepted echo of the same generation, the host shall mark the provider unavailable with the reason `capability_mismatch` and shall not call it again for allocation until the dispatch generation changes.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-002.3:** Before every allocation, delayed launch (the continuation of a pending placement, checked against the recorded negotiated set), and resume that may allocate, the host shall confirm that the installed manifest, the registry, and the accepted echo of the current dispatch generation agree, and shall refuse the launch with the stable code `capability_mismatch` when they do not. When the current generation has no accepted echo yet (after a plugin restart), the host shall first run one profile validation with the launch's profile to obtain it. Confirmation shall happen before any provider allocation call. A manifest that declares no capability skips this confirmation: no echo is required and no validation runs for it.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-002.4:** A profile validation response with the echo field absent or empty (an older provider or one that does not implement it) shall be treated as having declared no capability, and a manifest that requires any shall be rejected as incompatible. A provider that declares no capability and returns no echo shall keep working unchanged, including after a restart.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-002.7:** When the first profile validation of a dispatch generation fails, times out after 10 seconds, or returns `unavailable`, the host shall refuse the launch as `unavailable`, shall cache no echo and treat it as no empty echo, and shall retry at the next launch. Concurrent launches of one generation and provider entry shall share one validation. The echo shall be kept and compared per provider entry of the manifest.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-002.6:** An executor provider manifest that requires any capability shall declare `min_kandev_version` at or above the highest `introduced_in` release of its capabilities, and install shall reject a lower value naming the capability and the release. A manifest that declares no capability is unaffected.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-002.5:** The manifest-and-registry check (names, versions, dependencies, minimum host version) shall be a pure function of the registry and the manifest and shall not touch the network or the database. The echo comparison of AC-EXECUTORS-PLUGIN-CAPABILITY-002.3 shall read only the in-memory echo and shall call no provider except for the one validation named there.

### REQ-EXECUTORS-PLUGIN-CAPABILITY-003: Snapshot pinning and state-version migration

**Intent:** An upgrade never changes the rules of an existing environment, and an upgrade or rollback never strands state a release cannot read.

#### Acceptance criteria

- **AC-EXECUTORS-PLUGIN-CAPABILITY-003.1:** The host shall record the negotiated set (names and versions) and the provider state schema version in the environment's plugin inventory when an allocation is admitted. Recovery, attach, and destroy shall use the recorded set, not the installed manifest.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-003.2:** The manifest already declares readable state versions as `supported_state_versions` (a list of integers) on an executor provider; that field is kept and stays authoritative. A manifest can instead declare `state_schema` with `current` and `min_readable` integers (`min_readable` at most `current`), which the validator shall expand into the contiguous `supported_state_versions` list `[min_readable..current]`. A manifest shall not declare both. A manifest declaring neither shall keep the value that the validator applies today.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-003.3:** The existing upgrade check on recorded state versions shall also run on rollback and on any change of the readable range. When the change would install a manifest whose readable range excludes the recorded state version of any environment that is not terminal, the host shall reject it before activation, keep the working installation active, and report the count of affected environments without naming provider state. The check and a launch admission that records a state version shall exclude each other per installation (see the design), so a launch cannot record a version the activated manifest cannot read.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-003.4:** The host shall never rewrite provider state itself. The recorded state version is set from an accepted provision, recovery, or checkpoint response and shall not decrease within one environment generation. An attach response shall keep the recorded version, and one that differs shall be rejected as today. A version outside the manifest's readable list shall be rejected with `conflict` before it is persisted.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-003.5:** When a recorded negotiated version is no longer supported by the running host, the provider shall be unavailable for that environment, the record shall be retained, and no cleanup shall be confirmed.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-003.7:** A manifest that declares any capability can declare `state_budget_bytes` from 1024 to 12288 (default 8192), measured on the JSON-escaped provider state as it appears in the marshaled envelope. The host shall reject an accepted-looking response whose state exceeds the budget, or whose marshaled envelope exceeds 32768 bytes for any reason, with the code `state_too_large` before persisting, and shall run the existing cleanup for any resource the response created. The host's own fields shall have the per-field maxima of the design, whose sum shall not exceed 20480 bytes, so a state within its budget always fits. A provider that declares no capability keeps today's 32 KiB rule and error.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-003.9:** The maxima of the design shall be counted on the marshaled, JSON-escaped form of each field, and the readability count of AC 003.3 shall include every `executors_running` row of the plugin runtime that is not confirmed destroyed, taking the state version from `state_version`, else the highest `supported_state_versions` entry, else 1.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-003.8:** The host shall expose the largest recorded state size per installation in the plugin status read and shall count states above 75 percent of the budget, so a provider can learn that its state does not fit before it fails. A host-side write that would exceed the envelope cap despite the per-field maxima shall be refused whole with `state_too_large` and shall leave the stored envelope unchanged. Raising the 32 KiB cap shall require an owner decision.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-003.6:** A record written before this document has no `negotiated_capabilities` and shall be read as the empty negotiated set and keep working. Its existing `capabilities`, `supported_state_versions`, and `state_version` keys keep their meaning, and `state_version` is the provider state version. When a record holds both, `negotiated_capabilities` shall take precedence and `capabilities` shall never be read as a negotiation result.

### REQ-EXECUTORS-PLUGIN-CAPABILITY-004: Deprecation and retirement

**Intent:** Capability versions leave the contract in a predictable, auditable way.

#### Acceptance criteria

- **AC-EXECUTORS-PLUGIN-CAPABILITY-004.1:** A deprecated capability version shall remain accepted for at least two stable releases after its deprecation is published, and the plugin status read shall show the deprecation and the planned retirement release.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-004.2:** A manifest requiring a deprecated version shall install with a warning that names the replacement; a manifest requiring a retired version shall be rejected with a stable code that names the retirement release.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-004.3:** A retired capability name and version pair shall be recorded in an append-only list and shall never be reused for a different meaning. A test shall fail when an entry is removed or reused.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-004.4:** Retiring a state schema version that a retained environment still records shall be blocked by the same check as AC-EXECUTORS-PLUGIN-CAPABILITY-003.3 until those environments are destroyed or migrated. Retiring a capability version shall not be blocked: an environment that records it becomes unavailable under AC-EXECUTORS-PLUGIN-CAPABILITY-003.5, stays retained, and is counted in the status read.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-004.5:** The provider contract version shall remain 1 while every change is an additive field or an additive RPC guarded by a capability. A breaking change shall require a new contract version and a separate document.

## Out of scope

- Optional (best-effort) capabilities: a provider requires a capability or does not use it.
- Publishing the provider SDK as a versioned module (the pinned reference stays).
- A host rewriting or translating provider state.

## Implementation plans

- [Native worker capabilities for executor plugins](../../../plans/executor-plugin-native-workers/plan.md)
