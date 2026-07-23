---
id: "01-task-only-eager-tool"
title: "Keep the task completion tool discoverable"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/workflow/explicit-completion-signal/spec.md"
---

# Task 01: Keep the task completion tool discoverable

## Acceptance

- `ModeTask` lists `step_complete_kandev` without client-specific eager-load metadata.
- `ModeOffice`, `ModeConfig`, and `ModeExternal` do not register or advertise the tool.
- Tool arguments, handler behavior, and workflow transition semantics remain unchanged.

## Verification

```bash
cd apps/backend && rtk go test ./internal/mcp/server -run 'Test.*(StepComplete|ModeOffice|ModeTask)' -count=1
```

## Files Likely Touched

- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/server_test.go`

## Inputs

- Spec mode-boundary table and MCP metadata contract.
- `registerStepCompleteTool` is called only from the default `ModeTask` branch.

## Output Contract

Report the red test, minimal metadata change, serialized tool evidence, files changed, commands/results, risks, and task status. Use TDD and do not edit `plan.md`.

## Implementation Evidence

- RED: `rtk go test ./internal/mcp/server -run TestServerStepCompleteTool_TaskOnlyAndDiscoverable -count=1` failed because the serialized task-mode tool still contained `_meta.anthropic/alwaysLoad`.
- GREEN: the same focused test passed after removing the client-specific eager-load metadata (4 test cases including restricted-mode subtests).
- Prompt coverage requires signal-gated Kanban context to direct agents to normal tool search/discovery when the canonical tool is not already visible.
- Packet verification: `rtk go test ./internal/mcp/server -run 'Test.*(StepComplete|ModeOffice|ModeTask)' -count=1` passed (10 tests).
- Package verification: `rtk go test ./internal/mcp/server -count=1` passed (126 tests).
- The packet verification emitted a non-fatal `fnm_multishells` read-only filesystem warning from shell initialization; the Go test command itself completed successfully.
- No tool arguments, handler logic, or registration branch changed. Office, config, and external modes remain without `step_complete_kandev`; task-mode clients discover it through their normal catalog/search mechanism.
