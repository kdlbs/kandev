---
id: "01-qualify-managed-shell-exits"
title: "Qualify managed Claude Bash exits"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-002
  - REQ-AGENTS-BACKGROUND-WORK-005
  - REQ-AGENTS-BACKGROUND-WORK-010
acceptance_criteria:
  - AC-AGENTS-BACKGROUND-WORK-002.2
  - AC-AGENTS-BACKGROUND-WORK-005.1
  - AC-AGENTS-BACKGROUND-WORK-005.8
  - AC-AGENTS-BACKGROUND-WORK-005.9
  - AC-AGENTS-BACKGROUND-WORK-010.3
  - AC-AGENTS-BACKGROUND-WORK-010.5
  - AC-AGENTS-BACKGROUND-WORK-010.6
system_design:
  - ../../specs/agents/system-design/claude-background-shell-terminal-evidence.md
---

# Task 01. Qualify managed Claude Bash exits

## Summary

Apply the existing Claude terminal-exit path to the managed bridge. Add a
regression that reads the managed npm pin and exercises its wire fixture.

## In scope

- Verify the official `0.107.0` and `0.109.0` initialize identities and
  document sanitized detached launches and subsequent terminal frames. Use the
  tagged bridge sources and the installed `withRunningTerminal` and
  `SessionBackgroundWork.taskEnded` helpers to attest the fixtures. Include a
  launch that remains live.
- Start with `TestClaudeBackgroundTerminalExitManagedDefault`. Read
  `agents.DefaultManagedNPMRuntimeVersion` in the test and require a matching,
  qualified fixture. Verify the expected red result at the version gate.
- Extend the exact-version check used by both tool conversion and prompt-end
  binding retention to include the current managed release. Keep the strict
  identity and terminal envelope checks.
  Reuse `RunStateEnded` and the existing durable and accounting paths.
- Extend the existing mock identity and desktop/phone scenarios to use the
  current managed release. Keep `0.97.0`, `0.107.0`, and rejected variants in
  adapter tests.

## Out of scope

- Runtime package updates, new provider capabilities, and recovery of already
  consumed incident frames.

## Acceptance

1. All three qualified releases settle the matching detached Bash task. Null
   retains an unavailable outcome. Older launch frames, wrong IDs, and
   malformed evidence leave the running work open.
2. A delayed exit can settle its own job after prompt resolution. A live
   sibling remains running. Replay preserves the terminal state and original
   turn. The test fails if the managed pin lacks a qualified fixture.
3. Desktop and phone counts reach zero after both actual exits. The History
   entries survive reload and the draft survives the workload updates.

## Verification

Run the first command before the fix and record its failure. Run the full set
after the fix. Use the [workspace install](plan.md#e2e-tests) if needed.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run '^TestClaudeBackgroundTerminalExitManagedDefault$' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run '^TestClaudeBackgroundTerminal' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator -run '^TestClaudeBackgroundTerminal' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./cmd/mock-agent -run 'TestClaudeBackgroundTerminalFixtureEmitsCapturedShape|TestMockClaudeBridgeAgentInfoRequiresVerifiedVersion' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_claude_terminal.go apps/backend/internal/agentctl/server/adapter/transport/acp/claude_background_terminal_test.go apps/backend/cmd/mock-agent/claude_background_terminal_fixture.go apps/backend/cmd/mock-agent/claude_background_terminal_fixture_test.go
(cd apps/web && pnpm e2e:run --project chromium tests/task/claude-background-terminal-exit.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-claude-background-terminal-exit.spec.ts)
```

Run the browser projects sequentially. Add any touched Go files to the lint
command when recording results.

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_claude_terminal.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/claude_background_terminal_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/testdata/claude-0.107.0-detached-bash.jsonl` and `claude-0.109.0-detached-bash.jsonl` (new)
- `apps/backend/cmd/mock-agent/claude_background_terminal_fixture.go` and its test
- `apps/web/e2e/tests/task/claude-background-terminal-exit.helpers.ts`
- `apps/web/e2e/tests/task/claude-background-terminal-exit.spec.ts`
- `apps/web/e2e/tests/task/mobile-claude-background-terminal-exit.spec.ts`

## Dependencies

None. The `0.97.0` fixture and shared workload projection are already shipped.

## Risks

Provider launch and terminal metadata can have the same field name. Qualify
the exact release and verify both points in the lifecycle. The existing
process-end fallback remains available for unqualified releases and stale rows.

## Parallelism

`sequential`

## Inputs

- [Incident evidence and provider source](plan.md#incident-and-reproduction).
- [Shell termination design](../../specs/agents/system-design/claude-background-shell-terminal-evidence.md).
- [Prior shell repair](../claude-background-terminal-exit/plan.md).

## Results

The managed-default regression failed before the adapter guard changed because
the exact `0.107.0` terminal frame did not settle the detached shell. After
qualifying the managed version, the adapter cleanup and delayed-terminal cases
passed, including retained bindings after prompt cleanup, duplicate delivery,
and a live sibling. The durable projection and accounting regressions passed.
The managed mock identity rejects unsupported versions and both desktop and
phone scenarios passed after reload with the original draft intact. The
sanitized ACP fixture represents the separate null-code `taskEnded` frame; the
official v0.107.0 bridge source was inspected, while no new live provider run
was performed in this work order.

After `main` advanced the managed pin to `0.109.0`, the managed-default test
failed because that release had no qualified fixture. The tagged v0.109.0
source retains the same launch suppression and separate null-code terminal
frame, so a provider-shaped fixture and exact-version qualification were added.
The managed mock and desktop and phone terminal scenarios now advertise
0.109.0. The Monitor lifecycle remains qualified against 0.107.0. The focused
adapter, mock-agent, and both browser scenarios passed with the updated pin.
