---
id: "02-reconcile-monitor-lifecycle"
title: "Keep live Claude Monitors visible and settle finished watches"
status: complete
wave: 2
depends_on:
  - "01-qualify-managed-shell-exits"
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
  - AC-AGENTS-BACKGROUND-WORK-010.4
  - AC-AGENTS-BACKGROUND-WORK-010.5
  - AC-AGENTS-BACKGROUND-WORK-010.7
  - AC-AGENTS-BACKGROUND-WORK-010.8
  - AC-AGENTS-BACKGROUND-WORK-010.9
system_design:
  - ../../specs/agents/system-design/claude-monitor-lifecycle.md
---

# Task 02. Keep live Monitors visible and settle finished watches

## Summary

Keep active watches in the composer chip across foreground turns. Settle the
matching workload when Claude reports its outcome.

## In scope

- Write `TestClaudeMonitorSurvivesPromptCompletion` for a registered watch
  with a live owner and an ordinary prompt response. Expect the current
  generic prompt-end sweep to close it. Also cover foreground idle and actual
  transfer to a successor.
- Preserve the original Monitor binding and route across those boundaries.
  With a matching ACP tool update, require a terminal status and known
  Monitor provenance. Advance only the matched workload.
- Qualify the `0.107.0` and `2.1.323` native evidence against the inspected
  incident transcript, then replay sanitized provider-shaped queued-command
  attachments and qualified user records with one exact owner-produced
  outcome. Map native `stopped` to interrupted. Implement the exact-session
  source from the [Monitor design](../../specs/agents/system-design/claude-monitor-lifecycle.md).
  Use the effective launch root and contained file identity. Read bounded
  newly appended records through one cancellable reader while watches are
  open. Discard oversized records through their newline and continue. Validate
  the provider origin and both registration IDs.
- Honor the resolved Background Work flag and keep an EOF cursor without
  polling record bodies until a Monitor launch needs completion evidence.
  Open contained files nonblocking on Unix, validate the opened descriptor,
  and let adapter shutdown drain the reader.
- Emit a typed observation with the provider outcome through the dedicated
  background-work event path. Preserve the original row and turn even when
  late transcript updates are suppressed. Test terminal replay and verified
  owner exit.
- Prove desktop and phone count, Monitor labels, and History from that
  projection with a live sibling present.

## Out of scope

- AIR capability adoption, historical replay, general transcript access,
  timeout-based termination, controls, and repairs to the incident instance.

## Acceptance

1. A registered live Monitor stays counted after foreground idle, ordinary
   prompt completion, successor transfer, and reload of the attached session.
2. A qualified terminal update settles only its tracked Monitor. History
   keeps the original identity and provider outcome. Replay, wrong IDs,
   incomplete records, and unavailable sources preserve trustworthy state.
3. On desktop and phone, the summary identifies Monitors. Finishing one
   leaves its live sibling counted. Finishing the last clears the chip and
   leaves both History entries accessible with the draft intact.
4. A finished transcript card stops showing a watching state when its session
   and source-call IDs match a terminal workload. A live sibling card remains
   watching, including after reload, and foreground state stays idle.

## Verification

Run the first command before implementation. Use sanitized ACP and native
fixtures. The browser mock should append the qualified native completion
record only after both registrations appear in the UI. Record real provider
source qualification separately from deterministic replay.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run '^TestClaudeMonitorSurvivesPromptCompletion$' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run 'TestClaudeMonitor|TestPromptEndSweepsPreserveHandoffBackgroundLineage|TestHumanPromptHandoffAfterAuthoritativeForegroundIdle|TestSweepMonitorsOnPromptEnd' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/server/process -run '^TestClaudeMonitorEvidenceLaunchIdentity$' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator -run 'TestClaudeMonitorWorkloadProjection|TestForegroundBusySignal_MonitorViaUpdate|TestClaudeMonitorTerminalDispatchPreservesNativeOutcomeDuringSuccessorHandoff' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/service -run 'Test.*BackgroundWork' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./cmd/mock-agent -run '^TestClaudeMonitorLifecycleFixture' -count=1)
(cd apps/backend && go test -race -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp ./internal/orchestrator ./cmd/mock-agent -count=1)
pnpm --dir apps/web exec vitest run components/task/chat/messages/monitor-message.test.tsx
make -C apps/backend build
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
bash scripts/lint-go-changed \
  apps/backend/cmd/mock-agent/claude_background_terminal_fixture.go \
  apps/backend/cmd/mock-agent/claude_background_terminal_fixture_test.go \
  apps/backend/cmd/mock-agent/handler.go \
  apps/backend/cmd/mock-agent/claude_monitor_lifecycle_fixture.go \
  apps/backend/cmd/mock-agent/claude_monitor_lifecycle_fixture_test.go \
  apps/backend/internal/agentctl/server/adapter/adapter.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/adapter.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt_test.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_tools.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/claude_background_terminal_test.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_claude_terminal.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/monitor.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/monitor_test.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/claude_monitor_completion.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/claude_monitor_completion_reader.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/claude_monitor_completion_test.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/claude_monitor_completion_reader_lifecycle_test.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/claude_monitor_completion_linux_test.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/claude_monitor_completion_unix_test.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/claude_monitor_lifecycle_test.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/claude_monitor_open_unix.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/claude_monitor_open_windows.go \
  apps/backend/internal/agentctl/server/adapter/transport/shared/config.go \
  apps/backend/internal/agentctl/server/process/manager.go \
  apps/backend/internal/agentctl/server/process/claude_monitor_evidence_test.go \
  apps/backend/internal/agentctl/server/process/claude_monitor_evidence_windows_test.go \
  apps/backend/internal/orchestrator/event_handlers_background_work.go \
  apps/backend/internal/orchestrator/event_handlers_streaming.go \
  apps/backend/internal/orchestrator/claude_monitor_terminal_dispatch_test.go \
  apps/backend/internal/orchestrator/claude_background_terminal_projection_test.go \
  apps/backend/internal/orchestrator/claude_monitor_lifecycle_projection_test.go
(cd apps/web && pnpm e2e:run --project chromium tests/task/claude-monitor-lifecycle.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-claude-monitor-lifecycle.spec.ts)
```

Run desktop and mobile sequentially after the [workspace install](plan.md#e2e-tests)
if needed. Match the final lint command to the Go files changed. Verify
Playwright discovers both named scenarios.

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/monitor.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_tools.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/claude_monitor_completion.go` and `claude_monitor_completion_test.go` (new)
- `apps/backend/internal/agentctl/server/adapter/transport/acp/claude_monitor_lifecycle_test.go` (new)
- `apps/backend/internal/agentctl/server/process/manager.go` and `claude_monitor_evidence_test.go` (new test)
- `apps/backend/internal/orchestrator/claude_monitor_lifecycle_projection_test.go` (new)
- `apps/backend/cmd/mock-agent/claude_monitor_lifecycle_fixture.go` and its test (new)
- `apps/backend/cmd/mock-agent/handler.go`
- `apps/web/e2e/tests/task/claude-monitor-lifecycle.helpers.ts` (new)
- `apps/web/e2e/tests/task/claude-monitor-lifecycle.spec.ts` (new)
- `apps/web/e2e/tests/task/mobile-claude-monitor-lifecycle.spec.ts` (new)
- `apps/web/components/task/chat/messages/monitor-message.tsx` and
  `monitor-message.test.tsx`

## Dependencies

Task 01 supplies the managed Claude mock identity and browser fixture pattern.

## Risks

A successor may start before the original RPC returns. Verify the launch
binding and runtime identity before publication. Cover a terminal record
that arrives early, repeated and partial records, a file replacement,
and a late update after reset or owner replacement. If the exact source
cannot be qualified on a target executor, retain the active display and
record the managed completion gap for that executor.

## Parallelism

`sequential`

## Inputs

- [Monitor lifecycle design](../../specs/agents/system-design/claude-monitor-lifecycle.md).
- [Handoff decision](../../decisions/0049-fine-grained-foreground-idle-busy-signal.md) and
  [async-task capability decision](../../decisions/2026-11-01-claude-async-task-lifecycle-capability.md).
- [Accepted Monitor evidence decision](../../decisions/2026-11-20-claude-monitor-completion-evidence.md).
- `apps/backend/internal/task/service/agenthistory/directory.go` and
  `claude.go` show the containment and identity checks for native history.
- `monitor_test.go` and `prompt_handoff_test.go` cover the existing adapter paths.

## Results

The read-only incident transcript qualified the native queued-command
attachment schema, origin, session ID, task ID, tool-call ID, terminal status,
and timestamp. The parser and mock lifecycle fixture use sanitized
provider-shaped records. A qualified user-shaped duplicate settles the same
outcome once; `stopped` maps to interrupted. Tests cover completion before ACP
registration, duplicate and partial records, stale generations, same-session
resume, file replacement, and an oversized record split across read windows
with valid outcomes before and after it. No live provider run was performed.

Focused ACP adapter, process, orchestrator, task-service, and mock-agent tests
passed. Coverage includes ordinary prompt completion, foreground-idle handoff,
exact per-Monitor settlement, late progress, reset, verified owner exit, and
an open Monitor sibling. Feature-gate tests prove disabled Background Work
does not admit a reader and enabled prompts without a Monitor do not poll
transcript bodies. Oversized records are discarded across read windows;
regular-file replacement, same-inode truncation, and FIFO replacement recover
without another prompt. A Unix FIFO replacement test proves adapter shutdown
drains the reader, and the opened descriptor retains nonblocking mode. Reader
shutdown cancels its context. Native settlement dispatches
`EventTypeBackgroundWorkUpdated`; production event dispatch during successor
handoff settles the original Monitor once while preserving its provider state,
finish time, origin turn, the successor foreground, and a live sibling.

`make -C apps/backend build`, changed-Go lint, the docs catalog, specification
lint, and `git diff --check` passed. Focused ACP, process, orchestrator,
task-service, and mock-agent commands passed. The race-enabled ACP, orchestrator,
and mock-agent suites passed. The Monitor component regression passed. The
Chromium and mobile-chrome lifecycle scenarios each passed. The desktop
scenario passed three consecutive runs. Both kept two Monitors visible through
reload and a successor prompt, settled one at a time, and preserved the draft.
The matching transcript card stopped showing Watching while its live sibling
kept showing Watching, including after reload; both completed rows remained in
History before and after reload. Production event dispatch preserved the idle
successor foreground. The Windows test binary could not be cross-compiled
locally because no MinGW toolchain is installed; the CGO-disabled attempt
stopped in the unrelated SQLite cgo package.
