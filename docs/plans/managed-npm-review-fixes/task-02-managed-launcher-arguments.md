---
id: "02-managed-launcher-arguments"
title: "Recognize managed task launchers with profile arguments"
status: done
wave: 2
depends_on:
  - "01-preserve-start-ownership"
plan: "plan.md"
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-004
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-006
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-007
acceptance_criteria:
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.1
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.3
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.4
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.3
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.4
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.1
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.4
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.7
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.11
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.12
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.2
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.4
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.10
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.13
system_design:
  - ../../specs/agents/system-design/managed-npm-runtime-recovery.md
  - ../../specs/agents/system-design/managed-npm-cache-recovery.md
  - ../../specs/agents/system-design/managed-npm-repair-reuse.md
---

# Task 02: Recognize managed task launchers with profile arguments

## Summary

Separate trusted npm launcher/package recognition from authorized agent arguments.
Wire the shared recognizer through task evidence, retry selection, repair selection, and final argv rewriting.

## In scope

- Add the structured npm invocation parser under `agent/managedruntime` and a registered-spec matcher under `agent/agents`.
- Validate direct canonical npx, exact package/version, npm options, required protocol arguments, and selected OpenCode family.
- Preserve trailing flags and values exactly. Parse package position instead of searching agent values.
- Update lifecycle online retry and agentctl task evidence, replacement directives, selection, and `RewriteNpxCommand`.
- Use the incident package/version/permission flag in permanent task-start and saved-session regressions.
- Keep physical-cache evidence, policy/auth precedence, native conversation, and exactly-once prompt delivery.

## Out of scope

- Discovery wiring, new npm syntaxes, shell parsing, global binaries, wrappers, and requirement 005.

## Acceptance

1. `TestManagedNPMInvocation` accepts legitimate flags and rejects unsupported launchers, wrapper prefixes, altered npm options, nonexact packages, wrong subcommands, and malformed argv. Agent values resembling npm flags or the package string remain opaque.
2. `TestManagedRuntimeCacheEvidenceWithCLIArgs`, `TestManagedRuntimeCacheStartWithCLIArgs`, and `TestManagedRuntimeCacheResumeWithCLIArgs` cover the actual eligibility pipeline with `@agentclientprotocol/claude-agent-acp@0.100.0` and `--dangerously-skip-permissions`.
3. `TestManagedRuntimeRepairReuseWithCLIArgs` publishes and selects the same compatible installation without another installer. Final argv retains all original flags, the same native token, and at most two provider starts during first recovery.

## Verification

Run sequentially from the repository root after the required failing regression.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/agents ./internal/agent/managedruntime ./internal/agentctl/server/process ./internal/agent/runtime/lifecycle -run 'ManagedNPMInvocation|ManagedNPMRuntime|PreparedRuntime|ManagedRuntime.*CLIArgs|OnlineManagedRuntimeArgs|ManagedRuntimeCacheResume' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agentctl/server/process ./internal/agent/runtime/lifecycle -run 'ManagedRuntime.*CLIArgs|ManagedRuntimeCacheResume' -count=1)
```

## Files likely touched

- apps/backend/internal/agent/managedruntime/managed_npm_invocation.go and matching tests (new)
- apps/backend/internal/agent/agents/managed_npm_runtime.go and matching tests
- apps/backend/internal/agent/managedruntime/prepared_runtime.go and matching tests
- apps/backend/internal/agentctl/server/process/managed_runtime_cache_evidence.go
- apps/backend/internal/agentctl/server/process/managed_runtime_repair.go
- apps/backend/internal/agentctl/server/process/manager.go and task CLI-argument tests
- apps/backend/internal/agent/runtime/lifecycle/managed_runtime_recovery.go
- apps/backend/internal/agent/runtime/lifecycle/managed_runtime_startup.go
- apps/backend/internal/agent/runtime/lifecycle/managed_runtime_cache_recovery_test.go and CLI-argument tests

## Dependencies

Task 01.

## Risks

- A broad prefix match grants installation authority to unsupported commands.
- Duplicate package text or `--cache` in an agent value must not change npm parsing.
- Runtime selection and profile changes still invalidate attempt evidence even when installation files can be shared.

## Parallelism

`sequential`

## Inputs

- Incident task/session and the reported temporary failing controls recorded in plan.md.
- Cache-recovery design: Registered launcher and agent argument boundary.
- Existing `ManagedNPMRuntimeSpec`, `onlineManagedRuntimeArgs`, evidence, and command-rewrite tests.

## Results

Done on 2026-11-09.

- Added the structured managed npm invocation parser and registered-spec matcher.
- Wired task evidence, retry selection, repair selection, and command rewriting through the shared boundary.
- Added CLI-argument coverage for task start/resume and repair reuse.
- Focused Go verification passed, including `-race` for the process and lifecycle CLI-argument cases.
- Remediation: a Preparation-owned ACP retry now chooses its runtime before rewriting, so a concurrently published repair is reused against the original logical argv rather than an already rewritten physical prefix.
