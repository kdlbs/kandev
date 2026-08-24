---
id: "01-capture-opencode-error-diagnostics"
title: "Capture OpenCode error diagnostics"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/agent-stall-recovery.md"
---

# Task 01: Capture OpenCode error diagnostics

## Acceptance

- Every managed OpenCode ACP command enables `--print-logs --log-level ERROR`,
  while other managed agent commands remain unchanged.
- Agentctl delivers cleaned live stderr lines to an optional consumer without
  blocking child-process drainage or changing the bounded recent-stderr API.
- For OpenCode, generic logs, recent-stderr, and nonzero-exit events contain
  only the parser's safe provider-message projection; unrecognized records are
  excluded so workspace URLs and identifiers cannot leak through the fallback
  error path.
- The OpenCode parser accepts only foreground `stream error` records and
  returns allowlisted, URL-free provider/model/message/timestamp/reset fields;
  background, malformed, and unrelated records are rejected.

## Verification

- `cd apps/backend && go test ./internal/agent/agents ./internal/agentctl/server/process ./internal/agentctl/server/adapter/transport/acp -run 'Test(OpenCode|ManagedNPMRuntime|Manager.*Stderr|ManagerProcessExit|ParseOpenCode)'`

Use TDD: command-contract, stderr-delivery, and parser tests must fail before
their production changes are added.

## Files likely touched

- `apps/backend/internal/agent/agents/opencode_acp.go`
- `apps/backend/internal/agent/agents/opencode_acp_test.go`
- `apps/backend/internal/agent/agents/managed_npm_runtime_test.go`
- `apps/backend/internal/agent/agents/ACP_BRIDGE_VERSIONS.md`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch_test.go`
- `apps/backend/internal/agentctl/server/adapter/adapter.go`
- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/process/manager_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/opencode_stderr.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/opencode_stderr_test.go`
- `apps/backend/internal/agentctl/types/streams/provider_error.go`
- `apps/backend/internal/agentctl/types/streams/agent.go`

## Dependencies

None.

## Parallelism

Sequential. Task 02 consumes the parser, consumer, and safe provider-error
contract introduced here.

## Inputs

- Spec sections `Correlated terminal diagnostics`, `Diagnostic contract`, and
  diagnostic failure-mode scenarios
- ADR-2026-08-02 stderr ownership, correlation, and sanitization rules
- Existing `Manager.readStderr`, `StderrProvider`, `ManagedNPMRuntimeSpec`, and
  ACP adapter construction patterns

## Risks

- `readStderr` is a process-liveness boundary; consumer delivery must remain
  non-blocking, including when the bounded provider-diagnostic channel is full.
- The parser must not preserve the OpenCode workspace URL or identifiers found
  in the observed `error.error` suffix.
- The generic process-exit path must consume only the safe OpenCode projection;
  raw stderr must never appear in logs, the recent-stderr ring, or exit events.

## Output contract

Report RED assertions, exact argv, normalized field allowlist, rejected record
cases, test results, files changed, blockers, and risks. Mark this task `done`
and update its plan checkbox in the same conversation.

## Results

The managed command contract, optional stderr consumer, parser privacy,
bounded-channel, and nonzero-exit projection cases all have focused
regressions. OpenCode now uses `acp --print-logs --log-level ERROR`; the parser
keeps only the validated foreground
session/provider/model/message/timestamp/reset fields and rejects title,
`small=true`, malformed, unrelated, or incomplete records. The focused
verification passed with 26 tests across three packages; the full backend
suite also passed.
