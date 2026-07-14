---
id: "02-acp-live-updates"
title: "ACP live shell updates"
status: done
wave: 2
depends_on: ["01-backend-normalization"]
plan: "plan.md"
spec: "../../specs/ui/acp-shell-command-output.md"
---

# Task 02: ACP Live Shell Updates

## Acceptance

- Initialization advertises `_meta.terminal_output: true` without claiming support for ACP terminal RPC methods.
- Codex deltas, Claude terminal output/exit, OpenCode cumulative content, and final raw output update the tracked shell payload using Task 01's normalization API.
- Output-only statusless terminal updates persist as `in_progress`; updates with a final exit expose their bounded output/exit before active state is removed.

## Verification

```bash
make -C apps/backend fmt
(cd apps/backend && go test ./internal/agentctl/server/adapter/transport/acp -run 'Test.*Initialize|TestConvertToolCall.*(Output|Terminal|Content|Exit)')
(cd apps/backend && go test ./internal/agentctl/server/adapter/transport/acp)
```

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_tools.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/tool_call_update_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/conversion_test.go` or a focused initialize test file

## Dependencies

Task 01.

## Inputs

- Spec provider mapping and failure-mode scenarios.
- Task 01's normalized shell update API.
- Existing `activeToolCalls` ownership and `convertToolCallResultUpdate` lock/cleanup order.
- Existing orchestrator behavior: only known tool statuses are persisted, so recognized statusless output must synthesize `in_progress`.

## Output contract

Report the capability request, event-to-normalizer mapping, cleanup behavior, tests run, files changed, blockers, and follow-up risks. Set this task to `done` and update `plan.md` only after targeted tests pass.

## Completion Report

- Advertised `_meta.terminal_output: true` and routed delta, cumulative content, final raw output, terminal output, and terminal exit fields through the shell normalizer.
- Output-only statusless updates become `in_progress`; completed, failed, cancelled, and exit-bearing updates emit their final payload before active-call cleanup.
- Changed ACP initialization/update handling and focused update/concurrency tests.
- Targeted and full ACP adapter tests, full backend tests, formatting, and lint passed. No blockers remain; lifecycle behavior still depends on providers emitting a terminal status or exit.
