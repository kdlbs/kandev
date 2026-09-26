---
id: "06-permission-acceptance"
title: "Prove end-to-end permission behavior"
status: done
wave: 3
depends_on:
  - "02-executor-mode-delivery"
  - "03-acp-mode-confirmation"
  - "04-auto-approval-audit"
  - "05-mobile-mode-warning"
plan: "plan.md"
requirements:
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-002
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-003
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-005
acceptance_criteria:
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.10
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.13
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.17
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.11
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-005.1
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-005.2
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-005.3
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-005.4
system_design:
  - ../../specs/agents/system-design/agent-permission-control-integrity.md
---

# Task 06: Prove end-to-end permission behavior

## Summary

Exercise the full launch and permission path against the same workspace and
state-changing Git command. Verify both unattended success and default-mode
prompting, with authentication and executor matrix evidence.

## In scope

- Extend the focused mock-agent E2E for both permission directions and replayed
  automatic-decision metadata.
- Record the Docker, SSH, Kubernetes, and host-auth compatibility matrix in
  test results, including unsupported delivery reasons.
- Reconcile the original plan's linked acceptance evidence and final counts.

## Out of scope

- A broad repository-wide verification audit.
- Treating a displayed mode or agent-authored claim as outcome evidence.

## Acceptance

1. The same command proceeds without a human only under the unattended profile; default mode remains pending until answered.
2. The executor matrix proves final agent-visible settings or records a truthful unavailable reason.
3. Permission history survives reload with its automatic decision fields.

## Verification

```bash
cd apps/web
pnpm e2e:run e2e/tests/agent/agent-permission-control.spec.ts
pnpm e2e:run --project=mobile-chrome e2e/tests/task/mobile-session-mode-mismatch.spec.ts
```

## Files likely touched

- `apps/web/e2e/tests/chat/agent-permission-unattended.spec.ts`
- `apps/web/e2e/tests/task/mobile-session-mode-mismatch.spec.ts`
- `apps/backend/internal/agent/runtime/lifecycle/initial_mode_test.go`
- `docs/plans/agent-permission-control-integrity/plan.md`
- `docs/plans/agent-permission-pr3886-remediation/plan.md`

## Dependencies

Tasks 02 through 05.

## Risks

The mock agent proves Kandev wiring but cannot prove real Claude CLI
classification. A focused provider smoke check remains necessary before
claiming provider-specific first-turn enforcement.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/agents/requirements/agent-permission-control-integrity.md), `002`, `003`, `005`.
- [System design](../../specs/agents/system-design/agent-permission-control-integrity.md), end-to-end evidence.
- [Original plan](../agent-permission-control-integrity/plan.md).

## Results

Extended `apps/web/e2e/tests/chat/agent-permission-unattended.spec.ts`. The
unattended profile must create and resolve the Git commit without a human
answer, and its permission transcript must retain `status=approved`, option
`allow`, kind `allow_once`, and source `auto_approve` after a page reload. The
default profile must keep the same Git command pending without creating a
commit until a person approves it. Both runs use the same seeded workspace,
repository, executor, and workspace mode; the profile is the only changed
input.

The executor compatibility evidence is covered by the completed Task 01 and
Task 02 tests:

| Launch case | Evidence | Result |
| --- | --- | --- |
| Local Docker, no selected bundle | `TestApplyInitialModeDoesNotCopyUnselectedHostConfiguration` verifies host env and hook sentinels are absent. | Safe session overlay |
| Local Docker, selected Claude bundle | `TestDockerSelectedSettingsKeepRequestedStartMode` verifies the selected model and environment survive while the final mode is `bypassPermissions`. | Delivered at the mounted path |
| SSH | `TestSSHInitialModeIsInstalledAtExportedConfigPath` reads the uploaded settings file at the exported `CLAUDE_CONFIG_DIR`. | Delivered |
| Kubernetes | `TestKubernetesInitialModeIsInstalledAtExportedConfigPath` verifies uploaded bytes at the pod home named by its launch environment. | Delivered |
| Standalone host with API-key authentication | `TestApplyInitialModeLeavesHostAuthenticationDirectoryUntouched` preserves authentication and reports mode unavailable with a reason. | Unavailable with reason |
| Explicit `CLAUDE_CONFIG_DIR` | `TestApplyInitialModePreservesExplicitConfigurationDirectory` preserves the selected directory and reports mode unavailable with a reason. | Unavailable with reason |
| Reported path differs from installed path | `TestInitialModePathMismatchDoesNotConfirmDelivery` rejects the false delivery claim. | Unavailable with reason |

The E2E checks use the mock agent. They establish Kandev permission wiring and
durable history, but do not prove first-turn enforcement by a real Claude CLI;
that provider smoke check remains open.

Verification passed:

- `pnpm e2e:run e2e/tests/chat/agent-permission-unattended.spec.ts` (2 tests)
- `go test ./internal/agent/runtime/lifecycle/initialmode ./internal/agent/runtime/lifecycle -run 'Test(ApplyInitialMode|InitialMode|Materialize|DockerSelectedSettingsKeepRequestedStartMode|SSHInitialMode|KubernetesInitialMode)' -count=1`
- The Task 04 agentctl, lifecycle, orchestrator, task-service, and replay checks recorded in [Task 04](task-04-auto-approval-audit.md).
