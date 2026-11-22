---
id: "03-discovery-arguments"
title: "Keep recovery and reuse for profile discovery arguments"
status: done
wave: 3
depends_on:
  - "02-managed-launcher-arguments"
plan: "plan.md"
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-001
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-004
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-006
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-007
acceptance_criteria:
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.6
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.7
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.8
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.3
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.2
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.4
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.7
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.9
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.11
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.12
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.3
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.4
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.7
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.14
system_design:
  - ../../specs/agents/system-design/managed-npm-runtime-recovery.md
  - ../../specs/agents/system-design/managed-npm-cache-recovery.md
  - ../../specs/agents/system-design/managed-npm-repair-reuse.md
---

# Task 03: Keep recovery and reuse for profile discovery arguments

## Summary

Use the shared managed-launcher boundary for startup, profile, and model-option discovery.
Compose and preserve authorized CLI arguments once for ACP and managed native Codex.

## In scope

- Replace full-command equality in `managedRuntimeProbePackageForAgent` and host retry with the registered matcher.
- Remove blanket CLI-flag exclusion for validated managed commands from utility eligibility and the native Codex managed path.
- Use the effective composed argv for evidence and observations while retaining the original logical command for rewriting.
- Preserve required `app-server`/ACP arguments, profile supersession, request fingerprints, owner scopes, and result publication guards.
- Trace agent-wide startup, interactive refresh, profile baseline, and profile model-option entry points individually.
- Keep native direct-binary policy and operator-defined/wrapped command exclusions unchanged.
- Keep the existing profile-argument rejection for Codex one-shot prompts. Flag support here applies to discovery probes.
- Keep old helpers on their supported private path. If they reject flagged evidence, retain bounded visible failure without destructive fallback or restarting a healthy retained peer.

## Out of scope

- New provider capabilities, direct-native repair, wrapper support, or permission-policy changes.

## Acceptance

1. `TestManagedRuntimeProbeRepairWithCLIArgs` covers verified failure, one recovery, initialization publication, and next first-attempt reuse through ACP and managed native Codex fixtures. Flags appear exactly once.
2. Hostutility cases cover startup, profile baseline, and model-option requests with their real DTO composition and recovery guards. The incident Claude flag/version has permanent coverage.
3. Negative tests retain wrapper/custom/native exclusions, reject changed required protocol arguments, and prevent old profile evidence or results from affecting a successor or agent-wide catalogue.
4. `TestCodexAppServerExecuteRejectsProfileArguments` rejects prompt flags before starting a provider, with an unflagged launch as its control. The flagged discovery probe still succeeds.

## Verification

Run sequentially from the repository root after the required failing regression.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/hostutility ./internal/agentctl/server/utility ./internal/agent/runtime/agentctl ./internal/agentctl/server/api -run 'ManagedRuntime.*CLIArgs|ManagedRuntimeProbe|ManagedRuntimePreparation|ManagedRuntimeCodex|ProfileProbe|ProfileModel|ModelConfig|Profile.*Context|CodexAppServerExecuteRejectsProfileArguments|CodexAppServerManagedProbePreservesProfileArguments' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/hostutility ./internal/agentctl/server/utility -run 'ManagedRuntime.*CLIArgs|ManagedRuntimeRepairReuse|Profile.*Context|CodexAppServerExecuteRejectsProfileArguments|CodexAppServerManagedProbePreservesProfileArguments' -count=1)
```

## Files likely touched

- apps/backend/internal/agent/hostutility/manager.go
- apps/backend/internal/agent/hostutility/profile_probe.go
- apps/backend/internal/agent/hostutility/managed_runtime_recovery_test.go and CLI-argument tests
- apps/backend/internal/agentctl/server/utility/managed_runtime_preparation.go
- apps/backend/internal/agentctl/server/utility/acp_executor.go
- apps/backend/internal/agentctl/server/utility/codex_app_server.go
- apps/backend/internal/agentctl/server/utility/launch_wrapper.go and related tests
- apps/backend/internal/agentctl/server/utility/managed_runtime_cli_args_test.go (new)

## Dependencies

Task 02.

## Risks

- Appending flags in both request composition and final launch silently duplicates them.
- Installation reuse does not permit stale profile result publication.
- A generic error flag is not proof that a provider accepts that argument. Use valid provider or fixture arguments.

## Parallelism

`sequential`

## Inputs

- Cache-recovery design: Startup discovery and profile recovery.
- Existing native initialize-before-discovery and profile supersession tests.
- Task 02's shared recognizer.

## Results

Done on 2026-11-09.

- Utility and host-utility recovery now accept validated profile CLI arguments for managed commands.
- Managed native Codex app-server probes preserve valid trailing arguments exactly once.
- Added flagged profile probe, Codex utility, and probe repair regressions.
- Desktop and mobile flagged discovery E2E cases passed in Chromium and mobile Chrome.

### Review follow-up

- Restored rejection of profile flags in Codex one-shot `Execute`; managed discovery probes retain flag support.
- `TestCodexAppServerExecuteRejectsProfileArguments` failed before the fix because the provider started. It passes after the guard, including its unflagged launch control.
- The full utility suite and focused race tests passed. The existing flagged native Codex probe regression passed.
- Changed-Go lint passed with zero issues after the follow-up.
