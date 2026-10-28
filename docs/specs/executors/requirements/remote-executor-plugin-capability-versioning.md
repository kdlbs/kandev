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
- **AC-EXECUTORS-PLUGIN-CAPABILITY-001.2:** The registry shall contain, at the time of this document: `workspace_descriptor`, `repository_intent`, `tunnel_transport`, `api_url_forward`, `drain_receipt`, `destroy_checkpoints`, `durable_bootstrap`, `workspace_sharing`, `admission_context`, `pending_placement`, `provider_operations`, `provider_maintenance`, `launch_policy`, and `preparation_hints`, each at version 1.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-001.3:** A manifest shall declare `requires_host_capabilities` as a list of objects with `name` and `version` (integer, at least 1). The validator shall reject an unknown name, a duplicate name, a non-integer or zero version, and a list longer than the registry.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-001.4:** The validator shall enforce cross-capability dependencies from the registry (for example `provider_maintenance` requires `provider_operations`) and shall name the missing dependency.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-001.5:** The registry shall be readable by authorized administrators through the existing plugin status read, with names, supported ranges, and deprecation state, and by a provider in the profile validation request (REQ-EXECUTORS-PLUGIN-CAPABILITY-002).

### REQ-EXECUTORS-PLUGIN-CAPABILITY-002: Negotiation before allocation

**Intent:** An incompatible provider is rejected at the earliest point, never after it has created a resource.

#### Acceptance criteria

- **AC-EXECUTORS-PLUGIN-CAPABILITY-002.1:** Install, upgrade, and enable shall reject a manifest that requires a name outside the registry, or a version outside the supported range, before the plugin is activated, and shall name the capability, the requested version, and the supported range.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-002.2:** The profile validation request shall carry the host's registry, and its response shall echo the capabilities and versions the running provider process implements. When the echo differs from the manifest declaration, the host shall mark the provider unavailable with a named reason and shall not call it again for allocation.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-002.3:** Before every allocation, delayed launch, and resume that may allocate, the host shall confirm that the installed manifest, the registry, and the last accepted echo agree, and shall refuse the launch with a stable error code when they do not. Confirmation shall happen before any provider allocation call.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-002.4:** A provider that returns no echo (an older provider or one that does not implement it) shall be treated as having declared no capability, and a manifest that requires any shall be rejected as incompatible.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-002.5:** A host-capability check shall be a pure function of the registry and the manifest, and shall not touch the network or the database.

### REQ-EXECUTORS-PLUGIN-CAPABILITY-003: Snapshot pinning and state-version migration

**Intent:** An upgrade never changes the rules of an existing environment, and an upgrade or rollback never strands state a release cannot read.

#### Acceptance criteria

- **AC-EXECUTORS-PLUGIN-CAPABILITY-003.1:** The host shall record the negotiated set (names and versions) and the provider state schema version in the environment's plugin inventory when an allocation is admitted. Recovery, attach, and destroy shall use the recorded set, not the installed manifest.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-003.2:** A manifest can declare `state_schema` with `current` and `min_readable` integers, `min_readable` at most `current`. A manifest without it shall be treated as `current: 1`, `min_readable: 1`.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-003.3:** When an upgrade or rollback would install a manifest whose readable range excludes the recorded state schema version of any environment that is not terminal, the host shall reject the change before activation, keep the working installation active, and report the count of affected environments without naming provider state.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-003.4:** The host shall never rewrite provider state itself. A provider shall raise a recorded state version only through an accepted provision, recovery, attach, or checkpoint response, and the version shall not decrease within one environment generation.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-003.5:** When a recorded negotiated version is no longer supported by the running host, the provider shall be unavailable for that environment, the record shall be retained, and no cleanup shall be confirmed.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-003.6:** A record written before this document (no recorded set) shall be read as the capability set that its recorded fields imply, or as the empty set, and shall keep working.

### REQ-EXECUTORS-PLUGIN-CAPABILITY-004: Deprecation and retirement

**Intent:** Capability versions leave the contract in a predictable, auditable way.

#### Acceptance criteria

- **AC-EXECUTORS-PLUGIN-CAPABILITY-004.1:** A deprecated capability version shall remain accepted for at least two stable releases after its deprecation is published, and the plugin status read shall show the deprecation and the planned retirement release.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-004.2:** A manifest requiring a deprecated version shall install with a warning that names the replacement; a manifest requiring a retired version shall be rejected with a stable code that names the retirement release.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-004.3:** A retired capability name and version pair shall be recorded in an append-only list and shall never be reused for a different meaning. A test shall fail when an entry is removed or reused.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-004.4:** Retiring a version that a retained environment still records shall be blocked by the same check as AC-EXECUTORS-PLUGIN-CAPABILITY-003.3 until those environments are destroyed or migrated.
- **AC-EXECUTORS-PLUGIN-CAPABILITY-004.5:** The provider contract version shall remain 1 while every change is an additive field or an additive RPC guarded by a capability. A breaking change shall require a new contract version and a separate document.

## Out of scope

- Optional (best-effort) capabilities: a provider requires a capability or does not use it.
- Publishing the provider SDK as a versioned module (the pinned reference stays).
- A host rewriting or translating provider state.

## Implementation plans

- [Native worker capabilities for executor plugins](../../../plans/executor-plugin-native-workers/plan.md)
