---
id: "06-acpdbg-sentinel-probe"
title: "Extend acpdbg sentinel probing"
status: pending
wave: 2
depends_on: ["01-attachment-evidence-contract"]
plan: "plan.md"
spec: "../../specs/platform/requirements/mcp-session-observability.md"
---

# Task 06: Extend acpdbg sentinel probing

## Acceptance

- The runner's resolved temporary workdir is the child cwd and the `cwd` sent
  in ACP new/load requests.
- `acpdbg mcp-probe` injects a temporary sentinel MCP server into a real
  `session/new` request.
- The summary distinguishes advertised transport capability, delivery,
  initialize, `tools/list`, and optional sentinel tool use.
- Sentinel connection events are timestamped in JSONL metadata with opaque
  connection IDs.
- A bounded no-connection result is reported as unobserved, not as a portable
  agent failure.
- Registry agent commands, `--exec`, environment handling, timeout, stderr,
  and cleanup keep their existing behavior.

## Verification

- `cd apps/backend && go test ./internal/agent/acpdbg ./cmd/acpdbg`
- `make -C apps/backend build-acpdbg`

Start with RED fake-agent tests for connect/list/call, ignore, explicit error,
timeout, temp cwd propagation, and sentinel cleanup. Do not require installed
third-party agents in automated tests.

## Files likely touched

- `apps/backend/internal/agent/acpdbg/runner.go`
- `apps/backend/internal/agent/acpdbg/runner_test.go`
- `apps/backend/internal/agent/acpdbg/ops.go`
- `apps/backend/internal/agent/acpdbg/ops_test.go`
- `apps/backend/internal/agent/acpdbg/mcp_sentinel.go`
- `apps/backend/internal/agent/acpdbg/mcp_sentinel_test.go`
- `apps/backend/cmd/acpdbg/main.go`
- `apps/backend/cmd/acpdbg/main_test.go`
- `apps/backend/cmd/acpdbg/README.md`
- `.agents/skills/acp-debug/SKILL.md`

## Dependencies

- Task 01 supplies the evidence terminology and interpretation.

## Parallelism

Parallel-safe with Task 02 after Task 01. This task does not edit live agentctl,
lifecycle passthrough, orchestrator, or frontend files.

## Inputs

- Existing `Probe` and raw JSONL recorder
- ACP `session/new` MCP server shape
- `mcp-go` streamable HTTP client/server support
- The default-workdir bug confirmed in `NewRunner`

## Output contract

Report the RED fake-agent cases, exact CLI syntax and summary, cwd fix,
sentinel lifecycle/cleanup, sample sanitized interpretation, tests, files
changed, blockers, and risks. Mark this task `done` and update its plan
checkbox.
