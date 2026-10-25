---
id: "01-executor-local-preparation"
title: "Verify and privately prepare bundled dependencies"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-005
acceptance_criteria:
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.1
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.3
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.4
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-005.1
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-005.2
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-005.3
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-005.4
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-005.7
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-005.9
system_design:
  - ../../specs/agents/system-design/managed-npm-runtime-recovery.md
---

# Task 01: Verify and privately prepare bundled dependencies

## Summary

Add the executor-local evidence and private prepared-runtime primitive with
opt-in start/probe wire contracts. Ordinary starts remain unchanged until
later work orders enable the directive.

## In scope

- Codex-specific diagnostic recognition plus real bounded inventory
  confirmation for the exact registered package and execution host.
- A typed condition separate from top-level process-exit evidence; support
  initialize RPC errors while the bridge remains alive.
- Private npm project/cache ownership, exact bridge installation with optional
  dependencies, bounded executable validation, sanitized failure reporting,
  cancellation, and lifetime through real subprocess cleanup.
- Optional start/probe directives, authenticated authority, capability
  advertisement, old-peer compatibility, and execution-boundary revalidation.
- Shared use of existing cache-key and manifest-inspection primitives.

## Out of scope

- Automatic backend task/probe retry routing, UI changes, persistent runtime
  promotion, arbitrary package repair, and shared-tree deletion.

## Acceptance

1. `TestManagedDependencyEvidence` fails first because the observed initialize
   error has no verified typed classification, then passes for the real
   missing platform package and rejects misleading text, symlinks, unreadable
   inventory, mismatched identity, prefixes, and `CODEX_PATH`.
2. `TestManagedDependencyPreparation` installs only the trusted selected
   bridge in private locations, validates its provider, and leaves healthy
   sibling files usable; npm arguments and environment are asserted directly.
3. `TestManagedDependencyPreparedLifetime` and wire tests prove generation/
   attempt binding, directive opt-in, bounded cleanup after descendants stop,
   detach safety, and old-peer behavior. No path or raw diagnostic becomes
   install authority or a client-visible error.

## Verification

Run from the repository root. Write new regression tests before production
changes and preserve their meaningful RED output.

```bash
(cd apps/backend && go test ./internal/common/npmresolution ./internal/agent/managedruntime ./internal/agentctl/types ./internal/agentctl/server/process ./internal/agentctl/server/utility ./internal/agentctl/server/api ./internal/agentctl/server/instance ./internal/agent/runtime/agentctl -run 'ManagedDependency|ManagedRuntime|ManagedStartup|RuntimeObservation' -count=1)
(cd apps/backend && go test -race ./internal/agent/managedruntime ./internal/agentctl/server/process ./internal/agentctl/server/utility -run 'ManagedDependency' -count=1)
```

## Files likely touched

- `apps/backend/internal/common/npmresolution/matcher.go` and matching tests.
- New `apps/backend/internal/agent/managedruntime/dependency_recovery.go` and `dependency_recovery_test.go`.
- `apps/backend/internal/agent/agents/codex_acp.go`, only if trusted descriptor metadata must expand.
- `apps/backend/internal/agentctl/types/managed_startup.go`.
- `apps/backend/internal/agentctl/server/process/managed_runtime.go`, `manager_command.go`, and new `managed_dependency_test.go`.
- `apps/backend/internal/agentctl/server/api/server.go` and agent initialize error projection.
- `apps/backend/internal/agentctl/server/utility/acp_executor.go`, `types.go`, `runtime_observation.go`, and new `managed_dependency_test.go`.
- `apps/backend/internal/agentctl/server/instance/manager.go`, only for private-owner cleanup.
- `apps/backend/internal/agent/runtime/agentctl/client.go` and matching wire tests.

## Dependencies

None. Read backend and agentctl scoped guidance before implementation.

## Risks

- The bridge can remain live after the provider fails; do not fabricate an
  ordinary process exit to reuse the empty-stderr branch.
- Ensure installer grandchildren cannot outlive cancellation and cleanup.
- A request-scoped deferred cleanup must not remove a task's prepared files.

## Parallelism

`sequential`

## Inputs

- Requirement 005; existing scoped-cache safety criteria.
- Design: Missing bundled Codex dependency, especially eligibility, private
  preparation, and wire contracts.
- Existing `process/managed_runtime_test.go`, `utility/runtime_observation_test.go`,
  `utility/probe_failure_privacy_test.go`, and `client_managed_runtime_test.go`.
- ADR `2026-10-19-isolated-managed-dependency-recovery`.

## Results

Pending.
