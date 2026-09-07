---
created: 2026-09-07
status: completed
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-001
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002
system_design:
  - ../../specs/agents/system-design/managed-npm-runtime-recovery.md
legacy_specs: []
---

# Implementation Plan: Host utility managed runtime recovery

## Overview

Extend the existing trusted managed-runtime recovery contract to host capability
probes. First add a structured probe failure signal and one bounded host-utility
repair attempt. Then prove the visible recovered status and update the existing
operator documentation.

## Scope

### In scope

- Classify only strict top-level npm `ETARGET` evidence from capability probes.
- Repair through the same warm agentctl instance that ran the failed probe.
- Retry the same effective package version once with online-preferred metadata.
- Publish capability failure only after the retry fails.
- Preserve persisted profile model, fallback model, mode, enabled state, and active runtime version.
- Add focused backend tests, one host E2E flow, and public recovery documentation.

### Out of scope

- The separate warning-tooltip defect.
- Automatic version rollback, registry replacement, or global npm cache cleanup.
- Recovery for authentication, provider, transitive dependency, or arbitrary ACP failures.
- Persisting capability catalogues across backend restarts.

## Technical approach

### Structured probe classification

Extend `utility.ProbeResponse` with a stable failure code. In
`ACPInferenceExecutor.Probe`, classify captured stderr only after the trusted
command yields an exact managed package specification. Require npm `ETARGET`
and a missing-package line for that same specification. Keep raw stderr in
diagnostic logs.

### Host utility recovery

Teach `hostutility.Manager.probeWithCommand` to recognize the structured code
for registered `ManagedNPMRuntimeAgent` implementations. Derive the effective
version through the existing selection reader, call
`RepairManagedRuntimeCache` on the warm instance client, and retry with
`ACPCommandWithNpmPreference(..., true)`.

Keep one retry inside one probe operation. Publish `probing` while recovery is
in flight, `ok` on success, and the final failed result otherwise. Do not run or
alter profile reconciliation during recovery.

Run one host utility bootstrap after temporary-artifact ownership is ready.
Profile reconciliation and utility binding migrations follow that bootstrap so
overlapping startup probes cannot race the same managed npm execution tree.

### User-visible evidence and documentation

Use a test-only npm wrapper that fails the offline probe with the strict
signature and succeeds online. Assert that the available-agent API reports the
recovered model catalogue and that task profile selection does not show a
capability warning. Update the stale-runtime recovery section in the public
agent guide and the internal managed-runtime command reference.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.1` | Existing session lifecycle recovery tests remain green. |
| `AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.6` | Host utility manager test observes one repair and one online retry. |
| `AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.7` | Reconciler integration test proves persisted profile fields remain unchanged. |
| `AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.8` | Host E2E receives an `ok` catalogue without launching a task or refreshing manually. |
| `AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.2` | Existing agentctl cache-repair tests plus the host utility integration assert one exact execution tree. |
| `AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.3` | Command and environment assertions preserve registry and sibling cache state. |

The first regression test is
`TestManagerProbeRecoversManagedRuntimeETarget`. It fails on current code because
the first failed probe is published without cache repair or an online retry.

## E2E tests

`apps/web/e2e/tests/settings/host-utility-managed-runtime-recovery.spec.ts`
covers recovery before task launch and the resulting warning-free profile
selection on desktop. No frontend implementation change is required.

## Work orders

- [x] [Task 01: Recover host utility probes](task-01-recover-host-utility-probes.md)
- [x] [Task 02: Prove recovered profile status](task-02-prove-recovered-profile-status.md)

## Verification results

- Backend recovery and startup wiring: `go test ./internal/agentctl/server/utility ./internal/agent/hostutility ./internal/agent/settings/controller ./internal/backendapp -count=1` passed.
- Shared matcher compatibility: `go test ./internal/agent/runtime/routingerr ./internal/common/npmresolution -count=1` passed.
- Task 02: `pnpm e2e:run --host tests/settings/host-utility-managed-runtime-recovery.spec.ts` passed.
- Public documentation validators and the complete specification lint passed.

## Risks

- A broad failure classifier could turn unrelated npm or provider failures into destructive cache repair.
- A retry that resolves a different version would violate the active-version contract.
- Publishing the first failure can race the recovered result and leave stale warning state in clients.
