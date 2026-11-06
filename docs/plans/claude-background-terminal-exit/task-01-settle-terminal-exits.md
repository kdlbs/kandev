---
id: "01-settle-terminal-exits"
title: "Settle qualified Claude terminal exits"
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

# Task 01: Settle qualified Claude terminal exits

## Summary

Consume verified Claude bridge terminal evidence without inventing an exit code.
Retire only the matching background shell through existing observation and
accounting paths. Prove persisted state and desktop/phone counts.

## In scope

- Follow the [terminal evidence design](../../specs/agents/system-design/claude-background-shell-terminal-evidence.md)
  and the [sanitized incident sequence](plan.md#confirmed-defect-and-evidence).
- Add `TestClaudeBackgroundTerminalExit` before production edits. Reproduce the
  null-code failure with a numeric control and a live sibling.
- Add the Claude dialect helper, preserve legacy numeric handling, and attach
  the existing typed `ended` workload observation for unavailable outcomes.
- Add durable projection coverage and a captured-wire mock fixture.
- Add desktop/mobile E2E coverage using existing surfaces and shared selectors.

## Out of scope

No async-task capability, SDK upgrade, schema change, live repair, historical
backfill, background controls, UI markup, translated copy, or new prompt behavior.

## Acceptance

1. A qualified null-exit frame ends only its matching detached shell with no
   exit code. The legacy launch-result, foreign-provider, and invalid-evidence
   cases retain their prior behavior. A live sibling remains running.
2. Persistence and publication retain the terminal state, identity, origin, and
   unavailable outcome. Duplicate exit and late launch frames do not reopen it
   or produce another root completion.
3. Desktop and phone show two jobs, one job, then no chip as exact exits arrive.
   Both records remain in History after reload, and the composer draft survives.

## Verification

Run the focused regression first and record its expected failure. Then run
these final checks from the repository root. Use the workspace install once
when dependencies are absent.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/service -run 'Test.*BackgroundWork' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./cmd/mock-agent -count=1)
bash scripts/lint-go-changed apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_tools.go apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt.go apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_claude_terminal.go apps/backend/internal/agentctl/server/adapter/transport/acp/claude_background_terminal_test.go apps/backend/internal/orchestrator/claude_background_terminal_projection_test.go apps/backend/internal/orchestrator/claude_background_terminal_accounting_test.go apps/backend/internal/orchestrator/event_handlers_streaming.go apps/backend/cmd/mock-agent/main.go apps/backend/cmd/mock-agent/handler.go apps/backend/cmd/mock-agent/claude_background_terminal_fixture.go apps/backend/cmd/mock-agent/claude_background_terminal_fixture_test.go
(cd apps/web && pnpm e2e:run --project chromium tests/task/claude-background-terminal-exit.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-claude-background-terminal-exit.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run desktop and mobile sequentially with managed builds. If implementation
changes the file set, synchronize the lint paths before recording results.
Do not treat an undiscovered test or an old build as validation.

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_tools.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_claude_terminal.go` (new)
- `apps/backend/internal/agentctl/server/adapter/transport/acp/claude_background_terminal_test.go` (new)
- `apps/backend/internal/orchestrator/claude_background_terminal_projection_test.go` (new)
- `apps/backend/internal/orchestrator/claude_background_terminal_accounting_test.go` (new)
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- `apps/backend/cmd/mock-agent/main.go`
- `apps/backend/cmd/mock-agent/handler.go`
- `apps/backend/cmd/mock-agent/claude_background_terminal_fixture.go` (new)
- `apps/backend/cmd/mock-agent/claude_background_terminal_fixture_test.go` (new)
- `apps/web/e2e/tests/task/claude-background-terminal-exit.helpers.ts` (new)
- `apps/web/e2e/tests/task/claude-background-terminal-exit.spec.ts` (new)
- `apps/web/e2e/tests/task/mobile-claude-background-terminal-exit.spec.ts` (new)

## Dependencies

None. Existing shared workload observations, terminal monotonicity, and desktop/
phone History surfaces supply the required consumers.

## Risks

The older bridge's launch exit is not job termination. Null does not mean zero.
Test-owned bridge identity must not weaken the production evidence gate.
Existing stale rows remain subject to the process-end fallback.

## Parallelism

`sequential`

## Inputs

- REQ-AGENTS-BACKGROUND-WORK-002, -005, and -010 in the existing requirement file.
- The linked terminal evidence design and shared background-work design.
- Existing `tool_call_update_test.go`, background projection tests, and
  `background-work-process-end.helpers.ts`.
- Read scoped backend, agentctl, mock-agent, and web guidance before changes.

## Results

Completed. The adapter recognizes the verified Claude 0.97.0 tool-scoped
null-code terminal frame, ends only the matching detached shell, retains the
unavailable outcome, and keeps that verified launch binding across foreground
prompt completion so later terminal frames can settle it. Legacy and
unidentified bridges keep their prior prompt-end cleanup. The exact end frame
also retires the session's execution-scoped background-accounting registration
without touching a live sibling or successor execution. Permanent adapter,
accounting, durable-projection, mock-fixture, and desktop/mobile E2E regressions
pass. Phone coverage is Chromium phone emulation only; physical-device validation remains open. The commands and results are recorded in the
[plan](plan.md#verification-results).
