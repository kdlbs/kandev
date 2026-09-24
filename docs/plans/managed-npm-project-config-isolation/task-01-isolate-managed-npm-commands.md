---
id: "01-isolate-managed-npm-commands"
title: "Isolate managed npm commands"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-003
acceptance_criteria:
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-003.1
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-003.2
system_design:
  - ../../specs/agents/system-design/managed-npm-runtime-recovery.md
---

# Task 01: Isolate managed npm commands

## Summary

Give built-in managed runtime npm commands the executor-local
`~/.kandev/managed-npm-runtime` project prefix while keeping the agent process
in its task workspace. Apply that root consistently to preparation, probes,
launches, retries, and cache discovery.

## In scope

- Start with a failing `TestManagedNPMRuntimeLaunchIgnoresWorkspaceNpmrc`
  regression that demonstrates the reported project-config leak.
- Add the trusted prefix to managed launch and update command construction and
  provision its directory with the effective child environment on every host
  execution path. Preserve `cmd.Dir` and ACP session cwd.
- Adjust exact-command recognizers and cache discovery to use the same prefix.
  Confirm the exact package's `_npx` cache key remains unchanged.

## Out of scope

- Release-age error classification or presentation; that belongs to Task 02.
- Native, passthrough, and agent-invoked npm commands.

## Acceptance

- A repository `.npmrc` with `min-release-age` or a different registry does not
  govern a managed runtime's npm resolution on local PC, Docker, or SSH; host
  update/probe commands use the same isolation rule.
- A missing prefix is created before npm starts, and an unavailable prefix
  fails safely without changing the task workspace or npm environment.
- Recovery still accepts the trusted exact package and rejects foreign command
  shapes; cache repair resolves the same cache used by the failed command.

## Verification

```bash
(cd apps/backend && go test ./internal/agent/agents ./internal/agent/runtime/lifecycle ./internal/agent/hostutility ./internal/agentctl/server/process ./internal/agentctl/server/utility ./internal/agent/settings/controller -count=1)
```

## Files likely touched

- `apps/backend/internal/agent/agents/managed_npm_runtime.go`
- `apps/backend/internal/agent/agents/managed_npm_runtime_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/managed_runtime_recovery.go`
- `apps/backend/internal/agent/runtime/lifecycle/managed_runtime_startup_recovery_test.go`
- `apps/backend/internal/agent/hostutility/manager.go`
- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/process/managed_runtime.go`
- `apps/backend/internal/agentctl/server/utility/acp_executor.go`
- `apps/backend/internal/agent/settings/controller/agent_update.go`
- `apps/backend/internal/agent/settings/controller/agent_update_job.go`
- Targeted adjacent tests for process, utility, hostutility, and update jobs.

## Dependencies

None.

## Risks

- The prefix path must be valid for the npm process's user and environment;
  backend-host paths cannot be sent to Docker or SSH as if they were local.
- A prefix flag at the wrong side of npm's `--` separator would become an
  agent argument instead of npm configuration.

## Parallelism

`sequential`

## Inputs

- `REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-003`, the paired system design, and
  ADR-2026-09-24-isolate-managed-npm-project-config.
- Existing command-shape and stale-cache recovery tests.

## Results

Done. Managed `npx` launch and `npm exec` preparation commands use the fixed
executor-local project prefix. Agentctl provisions it from the effective child
home before launches, probes, one-shot prompts, and cache discovery; the host
update runner applies the same rule. The task workspace remains the command
working directory, and the exact package spec still determines the same `_npx`
cache key. Added npm CLI integration coverage for a workspace registry setting,
plus executor-local provisioning and unavailable-prefix tests.

Verification passed:

```bash
(cd apps/backend && go test ./internal/agent/agents ./internal/agent/runtime/lifecycle ./internal/agent/hostutility ./internal/agentctl/server/process ./internal/agentctl/server/utility ./internal/agent/settings/controller -count=1)
```
