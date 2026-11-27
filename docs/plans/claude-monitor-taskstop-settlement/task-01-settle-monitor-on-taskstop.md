---
id: "01-settle-monitor-on-taskstop"
title: "Settle tracked Claude Monitors on TaskStop and populate registration titles"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-002
  - REQ-AGENTS-BACKGROUND-WORK-010
acceptance_criteria:
  - AC-AGENTS-BACKGROUND-WORK-002.1
  - AC-AGENTS-BACKGROUND-WORK-002.2
  - AC-AGENTS-BACKGROUND-WORK-010.4
  - AC-AGENTS-BACKGROUND-WORK-010.7
  - AC-AGENTS-BACKGROUND-WORK-010.8
system_design:
  - ../../specs/agents/system-design/claude-monitor-lifecycle.md
---

# Task 01: Settle tracked Claude Monitors on TaskStop and populate registration titles

## Summary

Recognize completed Claude `TaskStop` ACP `tool_call_update` frames as terminal
evidence for tracked Monitors so stopped watches immediately settle to
`interrupted` instead of remaining stuck in the running count. Also populate
initial Monitor observation titles from the Monitor payload so running watches
are identifiable before completion.

## In scope

- Write failing regression tests first:
  - `TestClaudeMonitorTaskStopSettlesMatchingMonitor` (replaying the captured
    L769–L775 Monitor registration + L1044–L1048 `TaskStop` sequence with a
    second live Monitor sibling).
  - `TestClaudeMonitorTaskStopRejectsFailedOrUnmatched` (covering L1029/L1035
    failed `TaskStop`, status-less intermediate L1046, mismatched
    `rawInput.task_id`, unknown `task_id`, and foreign provider).
  - `TestClaudeMonitorTaskStopAfterPromptHandoffPreservesSibling` and
    `TestClaudeMonitorTaskStopDeduplicatesWithNativeCompletion`.
  - `TestClaudeMonitorRegistrationObservationPopulatesTitle` in the orchestrator.
- Implement `isTaskStopMeta`, `recognizeTaskStopCompletedTaskID`, and
  `settleTrackedMonitorFromTaskStopLocked` in the ACP adapter, reusing
  `settleClaudeMonitorLocked` (`State: streams.RunStateInterrupted`) and
  dispatching `settlement.send(a)` outside `a.mu` from
  `convertToolCallResultUpdateForGeneration` in `adapter_tools.go`.
- Update `buildObservationFromNormalized` in
  `apps/backend/internal/orchestrator/event_handlers_background_work.go` so
  Monitor observations with empty `ToolTitle`/`ToolName` derive `Title` from the
  Monitor payload's `description`, then `command`, falling back to `"Monitor"`.

## Out of scope

- Changing `claudeMonitorReaderAdmission` or `claudeNativeVersion` pins (owned
  by Task 02).
- `TaskStop` tracking for detached Bash shells.
- Frontend component or E2E changes.

## Acceptance

1. A completed Claude `TaskStop` update whose confirmed `task_id` matches a
   tracked Monitor settles only that Monitor to `interrupted` (emitting both
   `EventTypeBackgroundWorkUpdated` and the Monitor's terminal `tool_update`),
   preserves any live sibling Monitor, and deduplicates against late or repeated
   completion records.
2. Failed (`status: "failed"`), intermediate status-less, foreign-provider,
   stale-session, or mismatched/unmatched `TaskStop` updates do not settle any
   tracked Monitor.
3. Initial Monitor registration observations carry the Monitor's `description`
   or `command` (fallback `"Monitor"`) in `Title` when the ACP registration
   `tool_call_update` omits `ToolTitle` and `ToolName`.

## Verification

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run 'TestClaudeMonitorTaskStop|TestClaudeMonitor' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator -run 'TestClaudeMonitor' -count=1)
(cd apps/backend && go test -race -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp ./internal/orchestrator -count=1)
bash scripts/lint-go-changed \
  apps/backend/internal/agentctl/server/adapter/transport/acp/monitor.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/claude_monitor_completion.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_tools.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/claude_monitor_completion_test.go \
  apps/backend/internal/orchestrator/event_handlers_background_work.go \
  apps/backend/internal/orchestrator/claude_monitor_lifecycle_projection_test.go
```

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/monitor.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/claude_monitor_completion.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_tools.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/claude_monitor_completion_test.go`
- `apps/backend/internal/orchestrator/event_handlers_background_work.go`
- `apps/backend/internal/orchestrator/claude_monitor_lifecycle_projection_test.go`

## Dependencies

None.

## Risks

- On `claude-agent-acp@0.109.0`, `_meta.claudeCode.toolResponse` arrives on a
  status-less `tool_call_update` (L1046) immediately before the
  `status: "completed"` update carrying JSON `rawOutput` (L1048). Extract
  `task_id` from either source when `status == toolStatusComplete`, and cross-check
  against `rawInput.task_id` when present so a failed or corrupted update never
  settles a live watch.

## Parallelism

`sequential`

## Inputs

- [Claude Monitor lifecycle design](../../specs/agents/system-design/claude-monitor-lifecycle.md)
- `apps/backend/internal/agentctl/server/adapter/transport/acp/claude_monitor_completion.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_tools.go`
- `apps/backend/internal/orchestrator/event_handlers_background_work.go`

## Results

- `go test -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run 'TestClaudeMonitorTaskStop|TestClaudeMonitor' -count=1` passed.
- `go test -trimpath -tags fts5 ./internal/orchestrator -run 'TestClaudeMonitor' -count=1` passed after the title projection change.
- `go test -race -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp ./internal/orchestrator -count=1` passed.
- `bash scripts/lint-go-changed` with the six Task 01 Go files passed with zero issues.
- Review follow-up restored `TestClaudeMonitorWorkloadProjection`, retaining
  sibling preservation, successor-turn ownership, foreground-idle preservation,
  terminal monotonicity, and open-workload settlement coverage; the targeted
  race-enabled orchestrator test passed.
