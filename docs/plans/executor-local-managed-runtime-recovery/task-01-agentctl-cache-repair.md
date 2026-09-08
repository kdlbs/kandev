---
id: "01-agentctl-cache-repair"
title: "Add executor-local cache repair"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002
acceptance_criteria:
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.1
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.2
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.3
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.4
system_design:
  - ../../specs/agents/system-design/managed-npm-runtime-recovery.md
---

# Task 01: Add executor-local cache repair

## Summary

Add an authenticated agentctl action that removes one exact npm execution tree.
Resolve the npm cache with the configured agent environment.

## In scope

- Add the typed agentctl request and response.
- Add the authenticated agentctl API route and handler.
- Resolve npm cache configuration through the process manager.
- Add the runtime agentctl client method.
- Preserve the existing exact-tree and symbolic-link protections.

## Out of scope

- Lifecycle retry policy.
- Executor eligibility.
- Version rollback or registry changes.

## Acceptance

- The request accepts one exact stable package specification and no path or command.
- Cache discovery uses the agent environment and the local agentctl process.
- The handler removes only the deterministic target tree and preserves siblings.

## Verification

```bash
go test ./internal/agent/managedruntime ./internal/agentctl/server/api ./internal/agentctl/server/process ./internal/agent/runtime/agentctl -count=1
```

## Files likely touched

- `apps/backend/internal/agent/managedruntime/cache.go`
- `apps/backend/internal/agent/managedruntime/cache_validation_test.go`
- `apps/backend/internal/agentctl/server/process/managed_runtime.go`
- `apps/backend/internal/agentctl/server/process/managed_runtime_test.go`
- `apps/backend/internal/agentctl/server/process/manager_command.go`
- `apps/backend/internal/agentctl/server/process/manager_stderr_exit_test.go`
- `apps/backend/internal/agentctl/server/api/server.go`
- `apps/backend/internal/agentctl/server/api/managed_runtime.go`
- `apps/backend/internal/agentctl/server/api/managed_runtime_test.go`
- `apps/backend/internal/agent/runtime/agentctl/client_managed_runtime.go`
- `apps/backend/internal/agent/runtime/agentctl/client_managed_runtime_test.go`

## Dependencies

None.

## Risks

- The endpoint can become too broad if it accepts a cache path or shell input.
- Cache discovery can target the wrong tree if it drops the agent environment.

## Parallelism

`sequential`

## Inputs

- `REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002`
- Executor-local cache contract in the system design.
- Existing `hostRuntimeUpdater` and `RemoveNpxExecutionTree` tests.

## Results

- Added exact stable `package@version` validation and retained the existing
  deterministic `_npx` tree and symlink protections.
- Added the authenticated agentctl cache-repair route and typed runtime client
  method. The route resolves npm cache through the configured agent environment.
- Added API, client, managedruntime, and process-manager tests for exact-tree
  repair, sibling preservation, unsafe selectors, and bounded cache discovery.
- Verification passed:
  `go test ./internal/agent/managedruntime ./internal/agentctl/server/api ./internal/agentctl/server/process ./internal/agent/runtime/agentctl -count=1`
