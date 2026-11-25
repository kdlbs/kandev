---
id: "01-local-runtime"
title: "Use the CLI for local ACP"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-ANTIGRAVITY-ACP-003
  - REQ-AGENTS-ANTIGRAVITY-ACP-004
  - REQ-AGENTS-ANTIGRAVITY-ACP-010
  - REQ-AGENTS-ANTIGRAVITY-ACP-011
  - REQ-AGENTS-ANTIGRAVITY-ACP-013
acceptance_criteria:
  - AC-AGENTS-ANTIGRAVITY-ACP-003.5
  - AC-AGENTS-ANTIGRAVITY-ACP-003.14
  - AC-AGENTS-ANTIGRAVITY-ACP-003.15
  - AC-AGENTS-ANTIGRAVITY-ACP-004.11
  - AC-AGENTS-ANTIGRAVITY-ACP-010.7
  - AC-AGENTS-ANTIGRAVITY-ACP-011.1
  - AC-AGENTS-ANTIGRAVITY-ACP-011.2
  - AC-AGENTS-ANTIGRAVITY-ACP-013.2
system_design:
  - ../../specs/agents/system-design/antigravity-acp-agent.md
---

# Task 01 Use the CLI for local ACP

## Summary

Make new local Antigravity work use the installed CLI, including capability probes, previews, and task sessions. Persist the runtime association with the native conversation and honor it during resume.

## In scope

- Add executable and bounded version checks to the local resolver. Keep filesystem discovery free of subprocesses. Test absent, old, compatible, failed, and cancelled checks.
- Resolve before adding profile flags and wrappers. Carry the same full argv and variant into instance configuration, probes, inference, and registered `acpdbg`.
- Keep built-in probe admission tied to the validated host executable and `acp` arguments. Test an arbitrary absolute executable and stale helper.
- Require an explicit agentctl CLI-runtime capability for CLI task and utility use. Keep CLI setup independent of the standalone harness and reuse one resolved selection through command construction and instance setup.
- Apply the runtime's wire identity and auth policy. Scope the standalone journal and refusal mapping to standalone. Use the CLI's model and configuration data.
- Publish runtime identity with the capability snapshot. Bind new ACP IDs to their runtime at commit and restore both generation-backed and legacy native IDs with their saved variant.
- Reject direct task ACP authentication for the CLI runtime; the operator login flow owns CLI credential setup.
- Build command previews from the selected host-local argv before profile flags and wrappers.

## Scope boundary

This work order covers host runtime selection and ACP. Task 02 owns setup controls and the operator login action. Remote launch uses only executor-local arguments.

## Acceptance

1. With both executables present, the signed-in CLI supplies models. New local task, worktree, preview, and inference commands use the same CLI argv. Older CLI versions select standalone.
2. CLI auth and startup errors preserve their selected runtime and status. The CLI prompt path uses qualified wire results. Standalone launch, consent, and failure tests still pass.
3. Each saved ACP conversation resumes with its committed runtime across a restart. A missing runtime returns a typed resume failure with the original conversation ID.
4. The real utility executor admits only the exact qualified `agy acp` command; CLI-only task and utility paths work without the standalone bundle and reject retained helpers without CLI support.
5. CLI task authentication requests fail without invoking ACP `authenticate`; standalone sign-in remains in the authorized operator flow.
6. Command preview shows the selected CLI or standalone executable and preserves executable paths containing spaces.

## Verification

Write `TestAntigravityLocalProbePrefersCLIWithStandaloneInstalled` first in
`internal/agent/hostutility/antigravity_cli_probe_test.go`. With both executables
in the fixture, expect the signed-in CLI model list and record the exact
standalone auth failure before the change. Add `TestAntigravityResumeUsesSavedRuntime`
in lifecycle and `TestAntigravityCLIPromptUsesWireOutcome` in the ACP adapter.

```bash
(cd apps/backend && go test -trimpath ./internal/agent/antigravityruntime ./internal/agent/agents ./internal/agent/hostutility ./internal/agent/acpdbg ./internal/agent/settings/controller ./internal/agent/runtime/lifecycle ./internal/agentctl/acpcompat ./internal/agentctl/server/utility ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/process ./internal/task/handlers ./internal/orchestrator ./internal/task/repository/sqlite -run 'Antigravity|TestFreshStartCommitRotationAfterAcceptedToken|TestUpdateResumeTokenWithACPAdapterIdentity' -count=1)
make -C apps/backend build-acpdbg
agy_cli_evidence_dir=$(mktemp -d /tmp/kandev-agy-verification.XXXXXX)
apps/backend/bin/acpdbg probe --timeout 45s --out "$agy_cli_evidence_dir" antigravity-acp
apps/backend/bin/acpdbg prompt --timeout 45s --out "$agy_cli_evidence_dir" --prompt 'Reply with exactly ACP_OK. Use no tools.' antigravity-acp
```

Run the two debugger checks on the signed-in host. Confirm the registered entry launches `agy acp`, returns models, and completes a prompt. Keep raw captures private. In an isolated Kandev instance, stop and reopen a CLI task, then compare its ACP ID before and after resume.

## Files likely touched

- `apps/backend/internal/agent/antigravityruntime/resolve.go` and a CLI detection test beside it
- `apps/backend/internal/agent/agents/antigravity_acp.go` and `apps/backend/internal/agent/acpdbg/registry.go`
- `apps/backend/internal/agent/settings/controller/agent_config.go`
- `apps/backend/internal/agent/hostutility/manager.go` and `profile_probe.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`, `executor_standalone.go`, `session.go`, and `session_restore_coordinator.go`
- `apps/backend/internal/agentctl/acpcompat/antigravity.go`, the instance and inference configuration types, and provider policy in `server/utility` and `server/adapter/transport/acp`

## Dependencies

None.

## Risks

The current task launch replaces only argv index zero. A partial change can send standalone flags to the CLI. The provider's wire behavior can differ between releases. Check typed failure, exact model selection, and resume before claiming support.

## Parallelism

`sequential`

## Inputs

- [Runtime selection and persistence](../../specs/agents/system-design/antigravity-acp-agent.md#local-runtime-resolution)
- [Existing standalone lifecycle tests](../../../apps/backend/internal/agent/runtime/lifecycle/antigravity_command_test.go)
- [Vendor CLI ACP contract](https://antigravity.google/docs/cli/acp)

## Results

The review regressions pass for real utility admission of a qualified `agy acp`, CLI-only task setup and retained-helper capability checks, CLI and standalone resume through both saved-token and required-native paths, legacy standalone association, unavailable saved runtimes, task authentication denial, and both runtime previews with executable paths containing spaces. Focused tests pass in the Antigravity runtime, lifecycle, utility, process, task handler, orchestrator, and SQLite repository packages, including the token/identity failure race. Changed-package `golangci-lint` reports zero issues, and `make -C apps/backend build` succeeds for the backend and runtime binaries. On the signed-in host, `agy` 1.5.15 completed the ACP probe with 27 models and the prompt smoke with the exact `ACP_OK` response. Raw captures remained private.
