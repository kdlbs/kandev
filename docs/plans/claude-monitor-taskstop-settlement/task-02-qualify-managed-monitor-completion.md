---
id: "02-qualify-managed-monitor-completion"
title: "Qualify managed 0.109.0 / 2.1.328 Monitor completion evidence and E2E coverage"
status: done
wave: 2
depends_on:
  - "01-settle-monitor-on-taskstop"
plan: "plan.md"
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-002
  - REQ-AGENTS-BACKGROUND-WORK-005
  - REQ-AGENTS-BACKGROUND-WORK-010
acceptance_criteria:
  - AC-AGENTS-BACKGROUND-WORK-002.1
  - AC-AGENTS-BACKGROUND-WORK-002.2
  - AC-AGENTS-BACKGROUND-WORK-002.3
  - AC-AGENTS-BACKGROUND-WORK-005.1
  - AC-AGENTS-BACKGROUND-WORK-005.6
  - AC-AGENTS-BACKGROUND-WORK-005.8
  - AC-AGENTS-BACKGROUND-WORK-005.9
  - AC-AGENTS-BACKGROUND-WORK-010.1
  - AC-AGENTS-BACKGROUND-WORK-010.5
  - AC-AGENTS-BACKGROUND-WORK-010.7
  - AC-AGENTS-BACKGROUND-WORK-010.8
  - AC-AGENTS-BACKGROUND-WORK-010.9
system_design:
  - ../../specs/agents/system-design/claude-monitor-lifecycle.md
---

# Task 02: Qualify managed 0.109.0 / 2.1.328 Monitor completion evidence and E2E coverage

## Summary

Qualify the managed default `@agentclientprotocol/claude-agent-acp` `0.109.0`
bridge and Claude Code `2.1.328` native task-notification records for Monitor
completion evidence alongside `0.107.0` / `2.1.323`. Update the mock-agent and
desktop/mobile Playwright lifecycle tests to run against `0.109.0` and verify
both `TaskStop` (`Interrupted`) and native completion (`Completed`) with a live
sibling preserved.

## In scope

- Add sanitized captured fixture
  `apps/backend/internal/agentctl/server/adapter/transport/acp/testdata/claude-0.109.0-2.1.328-monitor-completion.jsonl`.
- Write failing regression tests first:
  - `TestClaudeMonitorCompletionManagedDefaultBridge` verifying that
    `agents.DefaultManagedNPMRuntimeVersion` (`0.109.0`) is admitted by
    `claudeMonitorReaderAdmission` and `verifiedClaudeMonitorEvidenceBridge`.
  - Extend `TestClaudeMonitorCompletionSource` to verify both `2.1.323` and
    `2.1.328` fixtures while still rejecting unqualified versions (e.g., `2.1.322`).
- Update `verifiedClaudeMonitorEvidenceBridge` and `claudeMonitorReaderAdmission`
  in `claude_monitor_completion.go` to accept both
  `verifiedClaudeTerminalBridgeManagedVersion` (`0.107.0`) and
  `verifiedClaudeTerminalBridgeCurrentVersion` (`0.109.0`), and update
  `isQualifiedClaudeMonitorRecord` in `claude_monitor_completion_reader.go` to
  accept qualified native versions `2.1.323` and `2.1.328`.
- Update `apps/backend/cmd/mock-agent/claude_monitor_lifecycle_fixture.go`
  (and its unit test) to emit `2.1.328` native completions and support a
  deterministic `TaskStop` prompt command that stops the first Monitor via the
  captured `TaskStop` ACP frame shape.
- Update `apps/web/e2e/tests/task/claude-monitor-lifecycle.spec.ts`,
  `mobile-claude-monitor-lifecycle.spec.ts`, and
  `claude-monitor-lifecycle.helpers.ts` to run against
  `E2E_MOCK_AGENT_CLAUDE_BRIDGE_VERSION: "0.109.0"`, assert non-empty Monitor
  titles in the running summary, stop the first Monitor via `TaskStop`
  (`Interrupted` in History, sibling still `Watching` and counted as
  `1 Background Job`, draft intact across reload), then finish the second via
  `2.1.328` native completion (`Completed` in History, chip dismissed).

## Out of scope

- Removing `0.107.0` or `2.1.323` backwards compatibility.
- Relaxing exact-version qualification beyond `ADR-2026-11-20`.

## Acceptance

1. `agents.DefaultManagedNPMRuntimeVersion` (`0.109.0`) and `0.107.0` are both
   admitted for Claude Monitor completion reading, and both `2.1.323` and
   `2.1.328` native records parse while unqualified versions remain rejected.
2. Desktop (`chromium`) and phone (`mobile-chrome`) Playwright lifecycle tests
   pass against `0.109.0`, proving mixed live/stopped state after `TaskStop`
   (`Interrupted`), native completion (`Completed`), non-empty summary titles,
   card status transitions, reload persistence, History states, and draft
   preservation.
3. Specification, catalog, and Go lint gates pass with zero errors.

## Verification

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run 'TestClaudeMonitorCompletionManagedDefaultBridge|TestClaudeMonitorCompletionSource|TestClaudeMonitor' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./cmd/mock-agent -run '^TestClaudeMonitorLifecycleFixture' -count=1)
(cd apps/backend && go test -race -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run 'TestClaudeMonitor' -count=1)
(cd apps/backend && go test -race -trimpath -tags fts5 ./internal/orchestrator -run 'TestClaudeMonitor' -count=1)
(cd apps/backend && go test -race -trimpath -tags fts5 ./cmd/mock-agent -run '^TestClaudeMonitorLifecycleFixture' -count=1)
make -C apps/backend build
bash scripts/lint-go-changed \
  apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_tools.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/claude_monitor_completion.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/claude_monitor_completion_reader.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/claude_monitor_completion_test.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/monitor.go \
  apps/backend/cmd/mock-agent/claude_monitor_lifecycle_fixture.go \
  apps/backend/cmd/mock-agent/claude_monitor_lifecycle_fixture_test.go \
  apps/backend/cmd/mock-agent/emitter.go \
  apps/backend/cmd/mock-agent/handler.go \
  apps/backend/internal/orchestrator/claude_monitor_lifecycle_projection_test.go \
  apps/backend/internal/orchestrator/event_handlers_background_work.go
(cd apps/web && pnpm e2e:run --project chromium tests/task/claude-monitor-lifecycle.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-claude-monitor-lifecycle.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/claude_monitor_completion.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/claude_monitor_completion_reader.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/claude_monitor_completion_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/testdata/claude-0.109.0-2.1.328-monitor-completion.jsonl` (new)
- `apps/backend/cmd/mock-agent/claude_monitor_lifecycle_fixture.go`
- `apps/backend/cmd/mock-agent/claude_monitor_lifecycle_fixture_test.go`
- `apps/backend/cmd/mock-agent/handler.go`
- `apps/web/e2e/tests/task/claude-monitor-lifecycle.helpers.ts`
- `apps/web/e2e/tests/task/claude-monitor-lifecycle.spec.ts`
- `apps/web/e2e/tests/task/mobile-claude-monitor-lifecycle.spec.ts`

## Dependencies

- [Task 01](task-01-settle-monitor-on-taskstop.md)

## Risks

- Changing `mockClaudeNativeVersion` to `2.1.328` and the E2E bridge version to
  `0.109.0` verifies the live managed default while unit fixtures retain full
  `0.107.0` / `2.1.323` regression coverage.

## Parallelism

`sequential`

## Inputs

- [Claude Monitor lifecycle design](../../specs/agents/system-design/claude-monitor-lifecycle.md)
- [ADR-2026-11-20 Claude Monitor completion evidence](../../decisions/2026-11-20-claude-monitor-completion-evidence.md)
- `apps/backend/internal/agentctl/server/adapter/transport/acp/claude_background_terminal_test.go` (managed-default version test pattern)

## Results

- The targeted ACP completion/monitor tests passed.
- `TestClaudeMonitorLifecycleFixture` passed with the managed `0.109.0` bridge identity and `2.1.328` native completion.
- `make -C apps/backend build` passed.
- Chromium and mobile-chrome Monitor lifecycle E2E tests passed.
- `go test -race -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp ./internal/orchestrator ./cmd/mock-agent -count=1` passed for all three packages.
- After the code-review fixes, the three focused race commands listed above
  passed. The TaskStop regression replays the four observed update frames and
  verifies both matching and mismatched task IDs when the final frame has no
  `RawInput`.
- The orchestrator projection test was restored, the mock-agent now emits the
  captured statusless metadata and JSON-string completion updates, and Monitor
  settlement preserves a payload `description` title.
- The native completion source test now covers the production reader path with
  an empty runtime version, both accepting `2.1.328` and rejecting `2.1.322`.
- Chromium and mobile-chrome lifecycle E2E runs passed with kind/title summary
  checks and draft preservation asserted after second-Monitor completion.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check` passed.
- Changed-Go lint passed on the final staged tree in the normal commit hook
  after the review follow-up.
