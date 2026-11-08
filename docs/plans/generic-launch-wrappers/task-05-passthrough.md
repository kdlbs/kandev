---
id: "05-passthrough"
title: "Passthrough and capability activation"
status: done
wave: 5
depends_on:
  - "04-session-lifecycle"
plan: "plan.md"
requirements:
  - REQ-AGENTS-WRAPPERS-001
  - REQ-AGENTS-WRAPPERS-002
  - REQ-AGENTS-WRAPPERS-003
  - REQ-AGENTS-WRAPPERS-004
  - REQ-AGENTS-WRAPPERS-005
acceptance_criteria:
  - AC-AGENTS-WRAPPERS-001.1
  - AC-AGENTS-WRAPPERS-001.5
  - AC-AGENTS-WRAPPERS-002.1
  - AC-AGENTS-WRAPPERS-002.2
  - AC-AGENTS-WRAPPERS-002.3
  - AC-AGENTS-WRAPPERS-002.5
  - AC-AGENTS-WRAPPERS-003.4
  - AC-AGENTS-WRAPPERS-004.1
  - AC-AGENTS-WRAPPERS-004.2
  - AC-AGENTS-WRAPPERS-004.3
  - AC-AGENTS-WRAPPERS-004.4
  - AC-AGENTS-WRAPPERS-004.5
  - AC-AGENTS-WRAPPERS-005.1
  - AC-AGENTS-WRAPPERS-005.2
system_design:
  - ../../specs/agents/system-design/generic-launch-wrappers.md
---

# Task 05: Passthrough and capability activation

## Summary

Apply the same wrapper template to native CLI passthrough. Advertise the versioned capability only after utility, session, and interactive receiving paths are complete.

## In scope

- Carry the template through initial, fresh, and resume builders into `InteractiveStartRequest`.
- Prepare native inner argv after MCP and profile flags, then wrap once at PTY startup.
- Preserve terminal input, resize, prompt injection, waiting-for-terminal behavior, process identity, and bounded cancellation.
- Redact wrapper arguments in interactive diagnostics. Clean operation-owned MCP artifacts after rejected startup.
- Advertise `launch-wrapper.v1` in agentctl identity and exercise backend admission against compatible and old peers. Keep all plugin infrastructure disabled in generic fixtures.

## Out of scope

No new terminal UI, provider-specific wrapper handling, plugin hook, or native utility protocol support.

## Acceptance

1. Pi and another existing passthrough command pass initial/fresh/resume tests with a wrapper receipt and exact inner argv.
2. Wrapper failure never launches the native command directly. PTY input, resize, cancellation, and descendant cleanup remain functional.
3. Capability advertisement enables all implemented v1 paths together. Older peers refuse wrapped launches while unwrapped profiles remain compatible.

## Verification

Run from the repository root. Implement new behavior with failing tests first.
New test and harness paths below are delivery targets, not files already available in this design package.

```bash
(cd apps/backend && go test ./internal/agent/agents ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/agentctl/server/process ./internal/agentctl/server/api)
(cd apps/backend && go test -race ./internal/agentctl/server/process -run 'GenericWrapper|Interactive')
git diff --check
```

Run `bash scripts/lint-go-changed` with the exact changed Go paths from this work order when Go files change.
Record the expanded command and result. Do not replace targeted checks with an unrecorded broad suite.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_passthrough.go` and `generic_wrapper_passthrough_test.go`.
- `apps/backend/internal/agentctl/server/process/{interactive_runner,interactive_lifecycle}.go` and `generic_wrapper_interactive_test.go`.
- `apps/backend/internal/agentctl/server/api/identity.go` and identity tests.
- `apps/backend/internal/agent/runtime/agentctl/` interactive request and capability consumers.
- `apps/backend/internal/agent/agents/pi_acp_test.go` and existing passthrough command tests where required.

## Dependencies

[Task 04](task-04-session-lifecycle.md). Read its recorded results before implementation.

## Risks

An apparent Pi reply can come from an unwrapped native process. Require a wrapper receipt. Wrappers that detach from PTY ownership are unsupported.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/generic-launch-wrappers.md).
- [System design](../../specs/agents/system-design/generic-launch-wrappers.md), especially the sections that match this work order.
- [Plan](plan.md), compatibility matrix and test mapping.
- [Historical reproduction](reproduction.md), which is diagnostic evidence only.
- Scoped backend/agentctl guidance and nearby existing tests. For browser work, read web guidance and the e2e and mobile-parity skills.

## Results

Implemented unresolved wrapper propagation in initial, fresh, and resumed native command builders. InteractiveRunner prepares the wrapper after the final environment is composed. PTY input, resize, and cleanup retain their existing owners. Wrapped process metadata and startup errors do not expose wrapper argv.

Pi and Claude native command builders have initial, fresh, and resume tests. Real local PTY tests cover immediate and deferred startup, input, resize, and wrapper receipts. Instance and control identity advertise the same launch-wrapper.v1 capability.

Focused tests and race checks passed. Desktop and phone workflow acceptance belongs to Task 06.

The current-base integration adds explicit refusal for wrapped SSH native terminal startup because that separate transport does not carry wrapper settings.
`TestGenericWrapperRemotePassthroughRefusesBeforeStarting` first reproduced an unwrapped launch, then passed with the refusal.
Its unwrapped control still starts a real remote-controller process.
`go test -trimpath -p=2 -race ./internal/agent/runtime/lifecycle -run 'GenericWrapper|SSHPassthrough|RemotePassthrough' -count=1 -timeout=120s` passed with `GOMAXPROCS=2`.
The operator reference and design distinguish this refusal from compatible remote ACP execution.

### PR review remediation

Keep each passthrough command paired with its wrapper snapshot across initial launch, resume, and fallback.
Verify a later execution-state write cannot change a prepared request.
