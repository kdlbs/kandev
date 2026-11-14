---
id: "02-launch-and-probe-reuse"
title: "Reuse repairs in tasks and discovery"
status: done
wave: 2
depends_on:
  - "01-owned-repair-store"
plan: "plan.md"
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-004
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-006
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-007
acceptance_criteria:
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.4
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.5
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.7
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.1
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.2
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.3
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.4
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.6
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.7
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.9
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.11
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.12
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.13
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.14
system_design:
  - ../../specs/agents/system-design/managed-npm-repair-reuse.md
  - ../../specs/agents/system-design/managed-npm-cache-recovery.md
---

# Task 02: Reuse repairs in tasks and discovery

## Summary

Use the owned store at task and utility launch boundaries. Publish a verified
replacement after initialization, and select it before later first attempts.

## In scope

- Preserve trusted installer-environment provenance through executor/profile resolution and authenticated requests.
- Negotiate the optional reuse context and capability; retain the existing private-preparation fallback.
- Task start/configure, initialize-success publication, teardown, and retained-process adoption boundaries.
- Generation-owned publication tickets across HTTP start return, with the original recovery deadline and abandoned-initialize cleanup.
- ACP and native Codex utility paths, including agent-wide/profile/model-option callers.
- Physical-cache evidence and runtime observations for selected generations.
- Existing retry budget, context cancellation, safe logs, and failure controls.

## Out of scope

- Eligibility broadening, requirement 005, shared-tree repair, new UI, and runtime selection changes.

## Acceptance

1. `TestManagedRuntimeInstallContext` and `TestManagedRuntimeRepairReuseCompatibility`
   prove provenance, credential/configuration separation, old peers, and absent-context fallback.
2. `TestManagedRuntimeRepairReuse` and `TestManagedRuntimeRepairReuseProbe` prove
   first-attempt reuse for task resume and all discovery owners after publication.
   Initialization failure, stale generations, and cancelled contexts cannot publish.
   An abandoned initialize cannot retain a lock or unpublished candidate indefinitely.
3. Publication failure keeps the initialized process usable. A selected damaged
   generation uses physical-cache evidence and the original single replacement budget.
   Existing healthy siblings and profile publication ownership remain intact.

## Verification

Run from the repository root. Start with the named failing integration regressions.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/agent/hostutility ./internal/agentctl/server/config ./internal/agentctl/server/instance ./internal/agentctl/server/process ./internal/agentctl/server/utility ./internal/agentctl/server/api -run 'ManagedRuntimeInstallContext|ManagedRuntimeRepairReuse|ManagedRuntimeCache|ManagedRuntimePreparation|ManagedRuntimeStartup|PreparedRuntimeOwner' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle ./internal/agent/hostutility ./internal/agentctl/server/process ./internal/agentctl/server/utility -run 'ManagedRuntimeInstallContext|ManagedRuntimeRepairReuse|ManagedRuntimeCache|ManagedRuntimePreparation' -count=1)
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/environment_resolution.go`, `utility_environment.go`, executor request builders, and focused tests.
- `apps/backend/internal/agent/runtime/environment/` only for preserving resolved provenance.
- `apps/backend/internal/agentctl/types/managed_runtime_cache.go` and authenticated instance/utility DTO owners.
- `apps/backend/internal/agent/runtime/agentctl/client.go`, `utility.go`, and instance/configure serialization.
- `apps/backend/internal/agentctl/server/config/config.go` and `instance/manager.go`.
- `apps/backend/internal/agentctl/server/process/manager.go`, `manager_command.go`, and `managed_runtime_cache_evidence.go`.
- `apps/backend/internal/agentctl/server/api/agent.go`, `server.go`, and `identity.go`.
- `apps/backend/internal/agentctl/server/utility/handler.go`, `types.go`, `managed_runtime_preparation.go`, `acp_executor.go`, `codex_app_server.go`, and `runtime_observation.go`.
- New focused `managed_runtime_repair_reuse_test.go` siblings in the affected packages.

## Dependencies

Task 01. Reuse its generation owner; do not add another installer or retry owner.

## Risks

- Context construction after task/transport injection can prevent all cross-launch reuse.
- A mutable process snapshot can publish the wrong generation after reconfiguration.
- Synchronous initialize publication must not call back into a held manager lock.
- A profile result must not populate the agent-wide capability catalogue.

## Parallelism

`sequential`

## Inputs

- Design: Lookup and recovery flow; Task and utility boundaries; Failure behavior and compatibility.
- Existing `managedRuntimeProbeFingerprint` and strict environment provenance.
- Task/probe cache recovery, initialization-phase, generation, and publication tests.

## Results

Implemented on 2026-11-08.

- Lifecycle fingerprints only authorized installer environment sources
  (profile, executor profile, repository, managed credentials, and agent
  runtime defaults). Generated task, session, instance, and transport values
  are excluded, so compatible launches share a repair without merging user
  contexts. Absent provenance keeps the private path.
- Authenticated instance `/agent/configure` and utility probe contracts accept
  an optional install context. `managed_runtime_repair_reuse_v1` is advertised
  by both the control and instance identity endpoints. Older peers ignore the
  field and keep private recovery.
- `process.Manager` selects a published generation before the first cold start,
  validates it again under the start mutex, rewrites only the trusted npx
  project/cache, and publishes a replacement only after successful
  `agent.initialize` for that exact process generation.
- A remaining-budget field carries the caller's original recovery deadline.
  Agentctl caps publication at 90 seconds, and an abandoned initialization
  releases its context lock at expiry.
- Utility ACP and Codex app-server probes reuse and publish through the same
  store. Agent-wide, profile, and model-option result ownership is unchanged.
- Cache evidence for a selected generation inspects that generation's physical
  cache instead of rediscovering the default user cache.
- Repair identity is bound to the authenticated application owner (task actor
  or workspace owner) and to agent-wide/profile discovery scopes. Generated
  task, session, and transport identifiers remain outside the key.
- The recovery path captures one immutable configuration revision and
  environment snapshot. Preparation uses that snapshot, and a later configure
  rejects the start or publication instead of publishing mixed-context files.
- A normal start that misses or invalidates its lookup detaches the previous
  reusable handle. Published files remain for their consumers; a stale handle
  can no longer rewrite a new configuration.
- Publication uses one admission-time deadline, retires an uninitialized
  candidate at expiry, and revalidates the effective environment and npm
  config before writing selection. Expiry reaps the exact owned process when it
  is still the current uninitialized generation.
- Utility probes require authenticated owner-bound provenance, negotiate the
  optional reuse capability, and publish immediately after ACP or Codex
  protocol initialization so later auth/model/session failures keep the
  validated installation.
- Tests: `TestManagedRuntimeRepairReuseSelection`,
  `TestManagedRuntimeRepairPublicationAfterInitialization`,
  `TestManagedRuntimeRepairRetirementKeepsCurrentProcessUsable`,
  `TestManagedRuntimeRepairReuseWithoutProvenance`,
  `TestManagedRuntimeRepairNormalMissDetachesStaleHandle`,
  `TestManagedRuntimeRepairRejectsConfigurationChangeDuringRecovery`,
  `TestManagedRuntimePublicationDeadlineUsesAdmissionTime`,
  `TestManagedRuntimeRepairExpiryRetiresUninitializedCandidate`,
  `TestManagedRuntimeRepairReuseProbe`, `TestPublishProbeRepairRevalidatesContext`,
  `TestManagedRuntimeInstallContextRequiresProvenance`,
  `TestManagedRuntimePreparationRemainingBudget`, and
  `TestManagedRuntimeInstallerEnvironmentDigest` passed.
