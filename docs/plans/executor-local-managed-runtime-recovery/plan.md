---
created: 2026-08-24
status: completed
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-001
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002
system_design:
  - ../../specs/agents/system-design/managed-npm-runtime-recovery.md
legacy_specs:
  - ../../specs/agents/runtime-updates.md
---

# Implementation plan: Executor-local managed runtime recovery

## Overview

Move exact npm cache repair from the Kandev host to the colocated `agentctl`
instance. Then enable one transparent retry for local PC, local Docker, and
remote SSH runtimes.

The API contract comes first because lifecycle recovery depends on it. Runtime
integration follows. Container-backed evidence and public documentation finish
the package.

## Scope

### In scope

- Add an authenticated agentctl action for one exact npm execution tree.
- Resolve the npm cache with the configured agent process environment.
- Route standalone, Docker, and SSH recovery through the same action.
- Preserve the exact package, selected version, registry, executor, and session.
- Add focused integration and container-backed evidence.
- Update public recovery documentation after implementation.

### Out of scope

- Automatic rollback or another version selection.
- Registry replacement or global npm cache cleanup.
- Sprites, remote Docker, Kubernetes, native runtimes, and passthrough commands.
- New UI copy or recovery controls.

## Technical approach

### Colocated cache repair

Add an authenticated agentctl endpoint under `/api/v1/agent/`. The request
carries only the trusted exact `package@version` specification.

The agentctl process manager runs `npm config get cache` through its owned
command runner. This command inherits the agent environment. The endpoint then
calls `managedruntime.RemoveNpxExecutionTree` for the derived execution key.

Add a typed method to `runtime/agentctl.Client`. Keep paths and raw npm output
inside agentctl. Return bounded errors to the lifecycle manager.

### Lifecycle recovery

Replace the backend-host `ManagedRuntimeCacheInvalidator` dependency with the
agentctl client method. Stop the failed child before repair. Keep the agentctl
server and its authenticated control connection alive.

Allow recovery for `RuntimeStandalone`, `RuntimeDocker`, and `RuntimeSSH`.
Reject all other runtimes. Preserve the startup generation, cancellation gates,
and one-retry limit.

### Executor evidence

Add focused lifecycle integration tests with a real agentctl API server. Add
container-backed Playwright tests for Docker and SSH.

The container tests use a test-only `npx` wrapper in the executor environment.
The wrapper emits the strict ETARGET signature for `--prefer-offline`. It
executes `/usr/local/bin/mock-agent` for the online retry.

The tests assert that the original session completes. They also assert that
the stale managed-runtime marker is removed, a fresh marker is recreated,
sibling cache state is preserved, and no recovery card appears.

### Documentation

Update the recovery section in `docs/public/agents-and-profiles.md`. State the
three supported executor locations and the executor-local cache boundary.

Update `ACP_BRIDGE_VERSIONS.md` if its host-local recovery text changes. Keep
the no-rollback, no-registry-change, and no-global-cleanup limits visible.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.1` | Lifecycle integration emits ETARGET and observes one online retry. |
| `AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.2` | Table tests cover standalone, Docker, and SSH runtime names. |
| `AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.3` | Recovery integration completes the original execution without terminal failure. |
| `AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.4` | Command identity assertions compare every argument except npm preference. |
| `AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.5` | Existing repeated-failure and recovery-card tests remain green. |
| `AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.1` | Agentctl API test uses an environment-specific `NPM_CONFIG_CACHE`. |
| `AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.2` | Cache tests remove only the exact deterministic tree. |
| `AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.3` | Sibling cache trees and registry environment values remain unchanged. |
| `AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.4` | Unsafe-root and symbolic-link tests remain green. |
| `AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.5` | Lifecycle cancellation and shutdown tests stop before replacement launch. |

The first regression test is
`TestRetryManagedRuntimeStartupSupportsAgentctlBackedExecutors`. It fails on
current code because Docker and SSH return before recovery starts.

## E2E tests

- `apps/web/e2e/tests/docker/managed-runtime-npm-recovery.spec.ts` covers local Docker recovery.
- `apps/web/e2e/tests/ssh/managed-runtime-npm-recovery.spec.ts` covers remote SSH recovery.
- Existing Kanban, Office, and mobile recovery-card tests cover terminal retry presentation.

Run the two new specs in the `containers` project. Run existing presentation
specs in their owning desktop, Office, and mobile projects.

## Work orders

- [completed] [Task 01: Add executor-local cache repair](task-01-agentctl-cache-repair.md)
- [completed] [Task 02: Enable recovery across executors](task-02-lifecycle-executor-recovery.md)
- [completed] [Task 03: Prove container executor recovery](task-03-container-recovery-evidence.md)
- [completed] [Task 04: Document executor-local recovery](task-04-document-recovery.md)

## Verification results

Task 01 passed the focused agentctl, cache, process-manager, and client command
with 1,598 tests across 4 packages. Task 02 passed the lifecycle and
orchestrator command with 4,050 tests across 2 packages. The container-backed
Docker and SSH specs each
passed in the host-backed `containers` Playwright project after rebuilding the
backend, mock agent, and E2E plugin artifacts. The nested `e2e:run` wrapper was
not usable in this environment because its runtime had no Docker daemon.
Public documentation validation passed, including the published-doc tests,
published-doc validator, specification lint, and diff checks.

## Risks

- Cache discovery must use the agent environment, not the backend environment.
- Repair must finish before the startup retry closes and replaces the agent stream.
- A delayed first-child event must not fail the replacement process.
- The container E2E fixture must not call the public npm registry.
- The agentctl maintenance endpoint must not accept caller-controlled paths or commands.
