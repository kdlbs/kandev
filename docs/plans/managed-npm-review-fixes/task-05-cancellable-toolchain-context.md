---
id: "05-cancellable-toolchain-context"
title: "Resolve toolchain context outside manager state locks"
status: done
wave: 5
depends_on:
  - "04-canonical-owner-scope"
plan: "plan.md"
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-006
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-007
acceptance_criteria:
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.7
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.6
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.9
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.12
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.13
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.19
system_design:
  - ../../specs/agents/system-design/managed-npm-cache-recovery.md
  - ../../specs/agents/system-design/managed-npm-repair-reuse.md
---

# Task 05: Resolve toolchain context outside manager state locks

## Summary

Keep npm version lookup cancellable and outside manager state mutexes.
Reuse stable version facts without caching mutable authorization, npm configuration, or publication authority.

## In scope

- Require an operation context as the first argument to repair context completion and propagate it through task, probe, and initialization call sites.
- Propagate cancellation as cancellation, not as a normal cache miss that starts a provider.
- Capture immutable input/ticket snapshots under `startMu`, resolve metadata outside it, then revalidate identity/revision/generation before applying results.
- Stop retires in-flight start authority even before a process exists. A late metadata result cannot launch after Stop merely because the configuration revision stayed unchanged.
- Memoize successful npm version facts by executable/node identity and the relevant execution context. Bound the process-local cache.
- Limit the version fact's environment key to Node bootstrap options and module paths. Task configuration and npm registry changes still receive fresh installation-context checks.
- Never hold the memo mutex during subprocess or filesystem work. Do not cache failed or cancelled lookups.
- Keep fresh npmrc/environment checks at selection/publication. Version memoization does not bypass context validation.
- Do not add a package-wide selection index or shared-flight cancellation owner for this correction.

## Out of scope

- A global launch mutex, persistent metadata cache, a new installer, or broad startup scanning.

## Acceptance

1. `TestRepairNPMVersionMemoization` proves repeated compatible reads invoke the version command once. Executable/node identity or relevant context changes invalidate the entry. Failures and cancellation never poison it.
2. `TestRepairContextCancellation` and `TestManagedRuntimeRepairContextDoesNotBlockStop` stall a version subprocess with barriers. Caller cancellation and Stop complete without waiting for its ten-second fallback timeout.
3. `TestManagedRuntimePublicationContextRace` changes configuration, stops the owner, or installs a successor during unlocked validation. Late results cannot publish, detach, or stop that successor. An initialized agent survives publication failure.

## Verification

Run sequentially from the repository root after the required failing regression.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/managedruntime ./internal/agentctl/server/process ./internal/agentctl/server/utility ./internal/agentctl/server/api -run 'RepairNPMVersion|RepairContext|ManagedRuntime.*Context|ManagedRuntimePublication|ManagedRuntimeRepairRejectedStart' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/managedruntime ./internal/agentctl/server/process ./internal/agentctl/server/utility -run 'RepairNPMVersion|RepairContext|ManagedRuntime.*Context|ManagedRuntimePublication|ManagedRuntimeRepairRejectedStart' -count=1)
```

## Files likely touched

- apps/backend/internal/agent/managedruntime/repair_context.go and repair_store.go
- apps/backend/internal/agent/managedruntime/repair_toolchain_cache.go and matching tests (new)
- apps/backend/internal/agentctl/server/process/managed_runtime_repair.go and manager.go
- apps/backend/internal/agentctl/server/api/agent.go and initialize tests
- apps/backend/internal/agentctl/server/utility/managed_runtime_preparation.go
- Existing store/context test call sites affected by the internal context signature
- apps/backend/internal/agent/managedruntime/repair_store_test.go
- apps/backend/internal/agent/managedruntime/repair_context_test.go
- apps/backend/internal/agent/managedruntime/repair_reuse_npm_test.go

## Dependencies

Task 04.

## Risks

- Moving work outside the state mutex requires immutable tickets and a second ownership check.
- Caching only the npm pathname can merge different Node/npm execution contexts.
- Request completion must not cancel a process that already initialized successfully.

## Parallelism

`sequential`

## Inputs

- Reuse design: Cancellable context completion and version facts.
- Review finding 3 and existing context-change, expiry, and publication-failure regressions.

## Results

Done on 2026-11-09.

- Repair context completion now accepts caller cancellation and propagates it as cancellation.
- Successful npm version facts are memoized by executable/node identity and execution context.
- Manager state locks are released before toolchain lookup; publication revalidates the ticket and successor under the lock.
- Stop retires in-flight start authority before a process exists.
- Publication revalidation now receives the initialize caller context and derives its timeout from it; StartWithGeneration admits/tracks the metadata lookup for teardown; retrieval cancellation, recovery deadline, CloseAdmission, Stop, and successful/stale publication races are covered with deterministic barriers.
- Cancellation, memoization, Stop availability, and publication-race regressions passed, including `-race`.

### Review follow-up

- `RepairStore.Context(ctx, input)` and `probeRepairContext(ctx, req, cfg)` now require an explicit context. All production and test callers use the new signatures.
- Version facts share an entry across task/provider configuration and registry values, while npm/node identity, `NODE_OPTIONS`, and `NODE_PATH` retain separate entries.
- The cross-task memoization regression failed before the key change because it ran npm twice. It passes after narrowing the key.
- The executable-change fixture sets its timestamp before reading the next key with `os.Chtimes`; it no longer sleeps.
- Caller cancellation remains part of publication validation. An initialized agent keeps running when publication is retired; metadata work remains bounded by the caller and recovery deadline.
- Full managedruntime tests, focused process/API/hostutility/agentctl tests, focused race tests, and the real-npm incident regression passed after the changes.
- Changed-Go lint passed with zero issues for the follow-up's three owning packages.
