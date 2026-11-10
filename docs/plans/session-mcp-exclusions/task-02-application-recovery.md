---
id: "02-application-recovery"
title: "Preserve application and recovery truth"
status: done
wave: 2
depends_on:
  - "01-persist-and-resolve"
plan: "plan.md"
requirements:
  - REQ-AGENTS-SESSION-MCP-EXCLUSIONS-001
  - REQ-AGENTS-SESSION-MCP-EXCLUSIONS-002
acceptance_criteria:
  - AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.3
  - AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.7
  - AC-AGENTS-SESSION-MCP-EXCLUSIONS-002.1
  - AC-AGENTS-SESSION-MCP-EXCLUSIONS-002.2
  - AC-AGENTS-SESSION-MCP-EXCLUSIONS-002.3
  - AC-AGENTS-SESSION-MCP-EXCLUSIONS-002.4
  - AC-AGENTS-SESSION-MCP-EXCLUSIONS-002.6
system_design:
  - ../../specs/agents/system-design/session-mcp-exclusions.md
---

# Task 02: Preserve application and recovery truth

## Summary

Prove that exclusions use the existing lifecycle boundaries and exact acceptance
snapshot. Add only missing wiring discovered by focused tests.

## In scope

- Busy-turn immutability, idle resume/load, next-start paths, reset/rebind, and adoption.
- Failure/retry, overlapping revisions, replacement execution, and retained baseline.
- Existing safe evidence transport for `session_excluded`, including reason sanitizers.
- Built-in injection preservation and no preparation for suppressed definitions.

## Out of scope

New reconnect capabilities, new workers, endpoint probes, tool pruning, and health redesign.

## Acceptance

1. The captured outbound set and accepted snapshot match for the same operation,
   and active turns never receive an exclusion or restoration mutation.
2. Unsupported paths defer. Failure preserves historical acceptance without
   claiming rollback, and an older result cannot clear newer saved state.
3. Actual restart uses saved exclusions. Live adoption preserves its attachment.
   Safe evidence names session exclusion and built-in injection still occurs.

## Verification

```bash
(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/lifecycle -run 'MCP|Mcp' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agentctl/types/... ./internal/agentctl/server/instance ./internal/agentctl/server/api -run 'MCP|Mcp' -count=1)
git diff --check
```

Use new `mcp_session_exclusions_test.go` for the lifecycle cases. Reuse
`mcp_accepted_setup_test.go`, `mcp_project_reconnect_test.go`,
`mcp_unsupported_reconnect_test.go`, `mcp_reconfiguration_lock_order_test.go`,
and `mcp_passthrough_operation_test.go`. Verify that the requested test names
were discovered. If no evidence consumer changes, its existing tests still
prove the new reason survives the transport. Run changed-Go lint if code changes.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/mcp_session_exclusions_test.go` (new).
- `apps/backend/internal/agent/runtime/lifecycle/{manager_mcp_reconfiguration.go,mcp_accepted_setup.go,mcp_passthrough_operation.go,manager_profile.go}` only where a test exposes missing wiring.
- Existing lifecycle MCP tests named above and relevant `internal/agentctl/types`, `server/instance`, or `server/api` evidence tests.

## Dependencies

Task 01. Read scoped agentctl guidance before changing its files.

## Risks

Resume success followed by runtime-setting restoration failure is uncertain
application, not proven rollback. Preserve the current lock order and bounded
timeouts. Do not alter provider admission to make a test pass.

## Parallelism

`sequential`

## Inputs

- [Design](../../specs/agents/system-design/session-mcp-exclusions.md): lifecycle, recovery, evidence, built-in authority.
- Parent [resolution delivery design](../../specs/agents/system-design/workspace-mcp-resolution-delivery.md).
- `/tdd` and backend test guidance at implementation time.

## Results

The existing lifecycle resolver paths consume exclusions without a new worker.
A real settings-store fixture proves busy application sends no provider operation;
idle resume omits the excluded server and accepts the exact empty catalog setup.
Failed resume preserves the prior accepted setup and revision. The safe evidence
reason required a sanitizer extension, proved RED before the new reason was allowed.

`go test -trimpath -race ./internal/agent/runtime/lifecycle -run 'MCP|Mcp' -count=1`
and focused agentctl types/instance/API suites passed. Existing acceptance,
project restart, unsupported reconnect, lock-order, passthrough, and adoption
regressions passed in that run. Tests use mock provider acceptance; no real-provider
connectivity or revocation claim is made.
