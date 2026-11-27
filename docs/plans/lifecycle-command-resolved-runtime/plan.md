---
created: 2026-10-11
status: done
requirements:
  - REQ-AGENTS-OPENCODE-V2-001
system_design:
  - ../../specs/agents/system-design/opencode-v2-adoption.md
legacy_specs: []
---

# Implementation Plan: Build the agent command for the resolved runtime

## Overview

`buildAgentCommandWithContext` derived the runtime for managed command options
from the requested `ExecutorType`. A launch whose request has no executor type,
or one that resolves to another backend, still runs on the backend the caller
resolved, and both call sites already pass that backend's runtime to
`preferNativeBinary`. With a native OpenCode selection, a request with no
executor type resolved the command options for `unknown` while the native
preference was decided for `standalone`: the native version probe was skipped
and the launch failed with `agent command cannot be empty`. A `local_docker`
request resolved to standalone got the managed `npx` command instead of the
selected native one. AC-AGENTS-OPENCODE-V2-001.11 asks a
native installation to launch with arguments compatible with its observed
version.

## Scope

### In scope

- Pass the resolved runtime into `buildAgentCommandWithContext` from both call
  sites (`buildExecutionFromInstance` and `promoteWorkspaceExecution`).
- Record in the OpenCode v2 design that command resolution follows the
  resolved runtime.

### Out of scope

- How the backend is resolved from the request.
- The version probe's own retry and cache policy; the transient-probe launch
  fallback is [Task 02](task-02-transient-probe-fallback.md).

## Technical approach

`buildAgentCommandWithContext` takes an `agentruntime.Runtime` argument and
uses it for `resolveManagedRuntimeCommandOptions`. Both callers pass
`execution.RuntimeName`, the value they already use for `preferNativeBinary`,
so the managed options and the native preference describe the same runtime.

## Tests

`default_executor_command_test.go` covers
`TestBuildExecutionUsesResolvedStandaloneForNativeOpenCode`:
AC-AGENTS-OPENCODE-V2-001.11. A request with no executor type, `local`, or
`local_docker` resolved to the standalone backend yields the native OpenCode
command. Existing command tests pass the runtime explicitly.

## E2E tests

The lifecycle test builds the execution without starting an agent. No rendered
UI changes require browser coverage.

## Work orders

- [x] [Task 01: Command for the resolved runtime](task-01-command-for-resolved-runtime.md)
- [x] [Task 02: Transient probe fallback for native OpenCode launches](task-02-transient-probe-fallback.md)

## Verification results

On the original command builder, the cases with no executor type and with
`local_docker` failed; `local` already passed. All cases pass with the change.

## Risks

A caller that resolved a different backend than requested now gets that
backend's command; this matches the runtime the process actually starts on.

## PR review remediation

The integrated change retains current main's prepared delivery, run generation,
control ownership, held-resume lease, and prompt materialization boundaries.
Native ACP and interactive resolution preserve caller cancellation and deadline
errors; only a probe attempt timeout under a live caller may fall back.
A transient failure cannot erase an unsupported-major observation or permit
an unusable selected version. Workspace promotion also has explicit coverage
for empty, unknown, local, and local-Docker requests resolved to standalone.

Contained focused proofs reproduced the failures before remediation and passed
afterward using exact source fragments, the real detector and SemVer parser,
and explicit storage, registry, family-spec, and logging fakes. These proofs
do not certify the full packages. Complete Backend, Frontend, E2E, and static
verification of the delivered exact head remains a hosted CI delivery gate.


### Hosted CI process-termination remediation (2026-10-12)

The integrated Backend run exposed `TestRetiredCleanupRequiresOriginalProcessTermination`: cleanup remained blocked after the fixture killed and reaped its original owner. Bounded isolated Linux procfs churn reproduced a disappearing unrelated stat entry returning `ESRCH`, which enumeration had not classified alongside `ENOENT`. The narrow classifier now recognizes those two disappearance errors while retaining all other read failures, birth/group/session identity checks, live-child rejection and the original cleanup assertion. Permanent real opened-stat and read-error regressions accompany exact-source contained RED/GREEN ownership proofs; no broad lifecycle/runtime/orchestrator package was rebuilt locally. A complete corrective exact-head Backend, Frontend and E2E run remains the delivery gate.
