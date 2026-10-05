---
id: "01-opencode-arguments"
title: "Restore OpenCode ACP argument compatibility"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-FIRST-RUN-SETUP-002
acceptance_criteria:
  - AC-AGENTS-FIRST-RUN-SETUP-002.2
  - AC-AGENTS-FIRST-RUN-SETUP-002.6
system_design:
  - ../../specs/agents/system-design/first-run-agent-setup.md
---

# Task 01: Restore OpenCode ACP argument compatibility

## Summary

Remove the optional parser-specific log-level argument from the shared OpenCode
ACP command. Preserve stderr printing and all existing native/managed selection,
version, cleanup, and inference behavior.

## In scope

- TDD regression `TestOpenCodeACPCommandsAcceptBothLogLevelDialects` in
  `opencode_acp_test.go`, using strict fake CLI subprocesses to reject the old
  command rather than only comparing a changed constant.
- Preserve the common `acp --print-logs` argument set for native and managed
  runtime commands, normal execution, and host utility discovery.
- Update existing OpenCode command expectations, including the built-in agent
  row of `TestManagedNPMRuntimeContracts`; retain unrelated synthetic test fixtures.
- Record the existing isolated parser reproduction as supplemental evidence.

## Out of scope

- Runtime updates, binary precedence changes, package installation, version
  detection/dispatch, stderr-format parsing changes, and UI implementation.

## Acceptance

- The regression fails with the current uppercase built-in argument and passes
  for both strict log-level dialects after correction; commands retain stderr printing.
- Native/managed execution and host utility share the corrected arguments with
  their existing version and precedence semantics.
- No prompt, task/profile write, default-runtime switch, or raw stderr logging
  is introduced by this correction.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test ./internal/agent/agents -run 'Test(OpenCodeACP|ManagedNPMRuntime)' -count=1)
(cd apps/backend && go test ./internal/agentctl/server/utility -run 'Test(ProfileProbeForwards|ProbeACPSession)' -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/agent/agents/opencode_acp.go`
- `apps/backend/internal/agent/agents/opencode_acp_test.go`
- `apps/backend/internal/agent/agents/managed_npm_runtime_test.go`

## Dependencies

None. Read scoped backend guidance before implementation.

## Risks

Provider-default log verbosity can increase stderr volume. Keep bounded draining
and existing sanitization. A lowercase replacement is incompatible with the
reviewed managed default; do not use it as a universal command default.

## Parallelism

`sequential`

## Inputs

- [Launch compatibility and evidence](../../specs/agents/system-design/first-run-agent-setup.md#opencode-launch-compatibility)
- [Requirements](../../specs/agents/requirements/first-run-agent-setup.md)
- Existing native-command and host-utility tests in `opencode_acp_test.go`.
- [Pinned parser source](https://github.com/anomalyco/opencode/blob/v1.18.32/packages/opencode/src/index.ts)

## Results

Implemented the shared OpenCode ACP arguments without the optional log-level flag.
Native, managed-runtime, and host-utility command regressions pass.
`GOCACHE=/private/tmp/kandev-review-go-cache go test ./internal/agent/agents ./internal/agentctl/server/utility -count=1` passed on 2026-10-05.
