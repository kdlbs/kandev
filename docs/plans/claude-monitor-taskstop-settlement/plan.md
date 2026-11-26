---
created: 2026-11-25
status: done
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-002
  - REQ-AGENTS-BACKGROUND-WORK-005
  - REQ-AGENTS-BACKGROUND-WORK-010
system_design:
  - ../../specs/agents/system-design/claude-monitor-lifecycle.md
legacy_specs: []
---

# Implementation Plan: Settle Claude Monitors on TaskStop and Managed 0.109.0 Runtime

## Overview

Fix stale `1 Background Job` (`Monitor | Running`) indicators for Claude ACP
sessions when a Monitor is explicitly stopped via `TaskStop` or completes on the
managed `0.109.0` bridge and Claude Code `2.1.328` runtime. Implement `TaskStop`
wire settlement and initial Monitor title projection first (Work Order 01), then
qualify the managed `0.109.0` / `2.1.328` native completion source and update
mock-agent plus desktop/mobile E2E scenarios to exercise the managed default
bridge and both settlement paths (Work Order 02).

## Incident and confirmed root cause

In live session `6e279895-c23c-4a97-9777-d53efedd2b2e` (task
`ea6c1e7f-8bba-45c3-a1ef-02b2127fa469`, ACP session
`cbabf3c9-f100-4b01-8164-964ce7e03481`, bridge
`@agentclientprotocol/claude-agent-acp@0.109.0`, Claude Code `2.1.328`), the
composer chip remained stuck at `1 Background Job` showing `Monitor | Running`
even after the agent stopped the watch:

1. **Unrecognized `TaskStop` wire evidence (direct cause):** At `19:17:44.322Z`
   (raw ACP L774–L775, normalized L776), `Monitor`
   (`toolu_01AdViqnxXmJvZQStEEixHQc`) registered `taskId: "bdwtg1m6y"` and
   created a `running` row in `task_session_background_work` (with empty
   `title`). At `19:19:10.684Z–10.691Z` (raw ACP L1044–L1048, Claude native
   JSONL L472–L473), the agent called `TaskStop`
   (`toolu_01MhuyQcyTR4eUMwXCuuHC8n`) with `task_id: "bdwtg1m6y"`, receiving
   `toolResponse: {"task_id":"bdwtg1m6y","task_type":"local_bash","message":"Successfully stopped task: bdwtg1m6y (...)"}`
   and `status: "completed"` with the same JSON in `rawOutput`. Claude emitted
   no follow-up `tool_call_update` on `toolu_01AdViqnxXmJvZQStEEixHQc` and no
   `<task-notification>` in the native JSONL (0 hits through EOF L649). Because
   the ACP adapter does not inspect completed `TaskStop` updates, `bdwtg1m6y`
   stayed `running` in `activeMonitors` and `task_session_background_work`.
2. **Stale version pins in the Claude Monitor completion reader (systemic):**
   `claudeMonitorEvidenceBridgeBuild` in `claude_monitor_completion_reader.go`
   remained pinned solely to `0.107.0` (`verifiedClaudeTerminalBridgeManagedVersion`)
   after the managed default moved to `0.109.0` (which `dialect_claude_terminal.go`
   already added as `verifiedClaudeTerminalBridgeCurrentVersion`). Meanwhile
   `claudeNativeVersion` remained pinned solely to `2.1.323` while Claude Code
   emitted `version: "2.1.328"`. Consequently `claudeMonitorReaderAdmission`
   rejected admission on every `0.109.0` session and `isQualifiedClaudeMonitorRecord`
   would reject `2.1.328` records.
3. **Empty initial Monitor title:** On the registration `tool_call_update`
   (normalized L776), both `ToolTitle` and `ToolName` are empty, so
   `buildObservationFromNormalized` stored `title: ""` until settlement.

The frontend counter (`selectRunningWorkloads` in
`apps/web/lib/background-work/workload-views.ts`) accurately reflects persisted
`state === "running"` rows and requires no UI changes.

## Scope

### In scope

- Settle a tracked Claude Monitor as `interrupted` when a completed `TaskStop`
  ACP `tool_call_update` on the same session confirms its `task_id`.
- Reject failed, non-terminal, foreign-provider, stale-session, or unmatched
  `TaskStop` updates without affecting live Monitors.
- Populate `WorkloadRunObservation.Title` on initial Monitor registration from
  the Monitor payload's `description` or `command` (fallback `"Monitor"`).
- Qualify `@agentclientprotocol/claude-agent-acp` `0.109.0` alongside `0.107.0`
  and Claude Code `2.1.328` alongside `2.1.323` for native Monitor completion
  evidence, with a sanitized `2.1.328` fixture and a regression checking
  `agents.DefaultManagedNPMRuntimeVersion`.
- Update mock-agent and desktop/mobile Playwright Monitor lifecycle tests to
  exercise the managed default `0.109.0` bridge and cover both `TaskStop`
  (`Interrupted`) and native completion (`Completed`) with a live sibling.

### Out of scope

- Adopting AIR `asyncTasks`, upgrading the ACP Go SDK, or altering
  `ADR-2026-11-20`'s exact-version fixture qualification model.
- Tracking `TaskStop` for detached Bash shells (the bridge already reports shell
  termination via `terminal_exit` keyed by `toolCallId`).
- Database backfill or live mutation of existing stale rows (existing process-end
  reconciliation handles historical sessions).
- Frontend layout, copy, or localization changes.

## Technical approach

### 1. ACP `TaskStop` settlement and initial Monitor title

- In `apps/backend/internal/agentctl/server/adapter/transport/acp/monitor.go`
  and `claude_monitor_completion.go`:
  - Add `isTaskStopMeta(meta map[string]any) bool` checking
    `_meta.claudeCode.toolName == "TaskStop"`.
  - Add `recognizeTaskStopCompletedTaskID(meta map[string]any, rawOutput any, payload *streams.NormalizedPayload) (string, bool)`
    that extracts the confirmed `task_id` from `_meta.claudeCode.toolResponse.task_id`
    or from JSON-decoded `rawOutput` (`{"task_id": "..."}`). When `rawInput.task_id`
    is present on `payload`, require it to match so malformed or inconsistent
    payloads cannot stop another watch.
  - Add `settleTrackedMonitorFromTaskStopLocked(sessionID, taskID string) *claudeMonitorSettlement`
    that looks up `toolCallID := a.activeMonitors[sessionID][taskID]`, validates
    `payloadMatchesClaudeMonitorCompletion`, and delegates to the existing
    `settleClaudeMonitorLocked` with `State: streams.RunStateInterrupted` and
    `FinishedAt: time.Now().UTC()`. Reusing `settleClaudeMonitorLocked`
    automatically records `completedClaudeMonitorCompletions[sessionID]`,
    marks the Monitor view ended, drops `activeMonitors` / `activeToolCalls` /
    handoff protection, and constructs both `EventTypeBackgroundWorkUpdated` and
    the Monitor's `EventTypeToolUpdate`.
- In `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_tools.go`:
  - In `convertToolCallResultUpdateForGeneration`, when `a.agentID == claudeAgentID || a.agentID == mockAgentID`,
    `status == toolStatusComplete`, and `isTaskStopMeta(tcu.Meta)`, call
    `settleTrackedMonitorFromTaskStopLocked` under `a.mu` and dispatch
    `taskStopMonitorSettlement.send(a)` immediately after `a.mu.Unlock()`,
    while returning the ordinary `TaskStop` `tool_update` event unchanged.
- In `apps/backend/internal/orchestrator/event_handlers_background_work.go`:
  - In `buildObservationFromNormalized`, when `title == ""` and
    `bw.Kind == streams.BackgroundWorkKindMonitor`, derive `title` from the
    Monitor's `raw_input.description`, then `raw_input.command` /
    `output.monitor.command`, falling back to `"Monitor"`.

This is the simplest sufficient solution because it reuses `activeMonitors`,
`settleClaudeMonitorLocked`, and `EventTypeBackgroundWorkUpdated` without
introducing new state tables, background pollers, or protocol extensions.

### 2. Managed `0.109.0` / `2.1.328` native completion qualification

- In `apps/backend/internal/agentctl/server/adapter/transport/acp/claude_monitor_completion.go`
  and `claude_monitor_completion_reader.go`:
  - Replace the single-string bridge check with
    `verifiedClaudeMonitorEvidenceBridge(agentID, agentName, agentVersion)`
    accepting official `@agentclientprotocol/claude-agent-acp` versions
    `0.107.0` (`verifiedClaudeTerminalBridgeManagedVersion`) and `0.109.0`
    (`verifiedClaudeTerminalBridgeCurrentVersion`).
  - Replace the single-string `claudeNativeVersion` equality check in
    `isQualifiedClaudeMonitorRecord` with `isQualifiedClaudeNativeVersion(version)`
    accepting `2.1.323` and `2.1.328`, keeping all other schema, origin,
    producer, session, timestamp, and dual-ID checks unchanged.
  - Add sanitized fixture
    `apps/backend/internal/agentctl/server/adapter/transport/acp/testdata/claude-0.109.0-2.1.328-monitor-completion.jsonl`
    derived from the verified `2.1.328` queued-command attachment record.
- In `apps/backend/cmd/mock-agent/claude_monitor_lifecycle_fixture.go` and
  `apps/web/e2e/tests/task/claude-monitor-lifecycle*.ts`:
  - Switch the Monitor E2E fixture bridge identity from `0.107.0` to the managed
    default `0.109.0` and native version `2.1.328`.
  - Exercise both settlement paths in the same two-Monitor scenario (or a focused
    sibling step): stop one Monitor via `TaskStop` (verifying `Interrupted` in
    History and the live sibling still counted) and finish the second via
    qualified `2.1.328` native completion (verifying `Completed` in History and
    chip dismissal).

### Compatibility matrix

| Provider / transport | Identity / shape | Intended behavior | Verification evidence | Unsupported-shape fallback |
| --- | --- | --- | --- | --- |
| Claude ACP v1 (`0.107.0`, `0.109.0`, mock) | Completed `TaskStop` `tool_call_update` with confirmed `task_id` matching `activeMonitors[sessionID]` | Settle only that Monitor as `interrupted`; emit `background_work_updated` + Monitor `tool_update`; sibling stays `running` | Adapter unit tests + orchestrator projection + Chromium & mobile-chrome E2E | Failed, non-terminal, mismatched, or unknown `task_id` leaves tracked Monitors untouched |
| Claude ACP v1 (`0.107.0`, `0.109.0`) + Claude Code (`2.1.323`, `2.1.328`) | Qualified native JSONL `attachment`/`user` `task-notification` matching `sessionId`, `task_id`, `tool_use_id` | Settle only that Monitor with reported `completed`/`failed`/`interrupted` state | `testdata/claude-0.107.0-2.1.323-monitor-completion.jsonl`, `testdata/claude-0.109.0-2.1.328-monitor-completion.jsonl`, managed-default version test, E2E | Unqualified bridge/native version or missing file leaves state intact; verified owner exit settles to `ended` |
| Foreign ACP providers (Codex, Cursor, OpenCode, Copilot) | Any tool update including a tool named `TaskStop` | Ignored by Claude Monitor `TaskStop` settlement (`agentID` gate) | Adapter negative unit test | Existing provider-specific paths unchanged |

### Lifecycle evidence checklist assessment

- **Pause and resume:** Same-session resume preserves `activeMonitors` / reader
  offset; `TaskStop` in a follow-up turn after foreground idle or handoff uses
  the original Monitor binding (`AC-AGENTS-BACKGROUND-WORK-002.1`, `010.7`).
- **Restart and recovery:** Process replacement continues to settle open
  Monitors via `sweepMonitorsOnReplayEnd` / `SettleOpenBackgroundWorkloads`;
  reload in E2E confirms persisted states survive refresh (`002.3`, `010.1`,
  `010.8`, `010.9`).
- **Duplicate and stale delivery:** `completedClaudeMonitorCompletions` fences
  duplicate `TaskStop` or late native JSONL records after `TaskStop`, and
  `isStaleClaudeNativeToolFrameLocked` rejects frames from an outgoing native
  session (`002.2`, `010.7`).
- **Ownership changes:** `TaskStop` settles only the Monitor in `sessionID`
  without mutating successor-turn ownership or foreground state (`002.1`,
  `010.4`).
- **Concurrent or merged work:** Mixed-state regression with two live Monitors
  confirms stopping one leaves the sibling running and counted (`005.1`,
  `010.5`, `010.7`, `010.9`).
- **Entry paths:** Adapter -> orchestrator durable projection -> task service
  -> desktop/phone composer chip, History, and transcript Monitor card all run
  through the single `EventTypeBackgroundWorkUpdated` contract (`005.1`,
  `005.6`, `005.8`, `005.9`, `010.5`, `010.8`, `010.9`).

## Tests

| Criteria | Regression evidence |
| --- | --- |
| `002.1`, `002.2`, `010.4`, `010.7` | `TestClaudeMonitorTaskStopSettlesMatchingMonitor` (fails before fix), `TestClaudeMonitorTaskStopRejectsFailedOrUnmatched`, `TestClaudeMonitorTaskStopAfterPromptHandoffPreservesSibling`, `TestClaudeMonitorTaskStopDeduplicatesWithNativeCompletion`, and `TestClaudeMonitorTaskStopMultiFrameInputCrossCheck` in `apps/backend/internal/agentctl/server/adapter/transport/acp/claude_monitor_completion_test.go` |
| `002.1`, `010.4`, `010.8` | `TestClaudeMonitorWorkloadProjection` and `TestClaudeMonitorRegistrationObservationPopulatesTitle` in `apps/backend/internal/orchestrator/claude_monitor_lifecycle_projection_test.go` |
| `002.2`, `010.7` | `TestClaudeMonitorCompletionManagedDefaultBridge` (checks `agents.DefaultManagedNPMRuntimeVersion` — fails before fix) and `TestClaudeMonitorCompletionSource` covering both qualified fixtures and the empty-runtime-version reader path in `apps/backend/internal/agentctl/server/adapter/transport/acp/claude_monitor_completion_test.go` |
| `005.1`, `005.6`, `005.8`, `005.9`, `010.5`, `010.7`, `010.8`, `010.9` | Desktop and phone Playwright lifecycle specs exercising `0.109.0` + `2.1.328` + `TaskStop` (`Interrupted`) and native completion (`Completed`) with reload and draft preservation |

## E2E tests

On a fresh worktree, run `(cd apps && pnpm install --frozen-lockfile)` once
before the first browser command.

Update `apps/web/e2e/tests/task/claude-monitor-lifecycle.spec.ts` (`chromium`),
`apps/web/e2e/tests/task/mobile-claude-monitor-lifecycle.spec.ts`
(`mobile-chrome`), and `apps/web/e2e/tests/task/claude-monitor-lifecycle.helpers.ts`:

- Restart the mock backend with `E2E_MOCK_AGENT_CLAUDE_BRIDGE_VERSION: "0.109.0"`
  (the managed default).
- Launch two live Monitors, verify count `2`, reload, verify both Watching cards
  and non-empty Monitor titles in the summary.
- Stop the first Monitor via a `TaskStop` follow-up prompt (`Interrupted`) while
  the second remains Watching (`1 Background Job`), preserve draft, reload, then
  settle the second via `2.1.328` native completion (`Completed`), verify chip
  clears and History shows `Interrupted` and `Completed` before and after reload.

## Work orders

- [x] [Task 01: Settle tracked Claude Monitors on TaskStop and populate registration titles](task-01-settle-monitor-on-taskstop.md) — done
- [x] [Task 02: Qualify managed 0.109.0 / 2.1.328 Monitor completion evidence and E2E coverage](task-02-qualify-managed-monitor-completion.md) — done

## Verification results

- Targeted ACP and mock-agent tests passed.
- Chromium and mobile-chrome Monitor lifecycle E2E tests passed.
- `make -C apps/backend build` passed.
- Race-enabled ACP, orchestrator, and mock-agent tests passed.
- Review follow-up restored `TestClaudeMonitorWorkloadProjection` and added
  `TestClaudeMonitorTaskStopMultiFrameInputCrossCheck` for the captured
  four-frame TaskStop update sequence, including JSON-string `rawOutput` with
  absent final-frame `RawInput` and a mismatched input/output task ID.
- Monitor settlement retains the registration `description` title, the native
  completion reader accepts qualified records when its runtime version is
  empty, and the mock-agent emits the captured multi-frame TaskStop shape.
- The summary helper checks the `Monitor` kind and command title, and the
  lifecycle E2E rechecks draft preservation after the second Monitor settles.
- The requested targeted ACP, orchestrator, and mock-agent race test commands
  passed after the review fixes.
- Specification catalog validation, specification lint, and `git diff --check` passed.
- Changed-Go lint passed on the final staged tree in the normal commit hook
  after the review follow-up.

## Risks

- Future Claude Code patch bumps beyond `2.1.328` or bridge bumps beyond
  `0.109.0` will still require a fixture update for the native JSONL fallback
  under `ADR-2026-11-20`; the new managed-default bridge regression catches
  bridge bumps immediately in CI, while `TaskStop` settlement works on ACP wire
  evidence without a native-version pin.
- A `TaskStop` frame could carry `task_id` only in `_meta.claudeCode.toolResponse`
  on an intermediate status-less update and only in JSON `rawOutput` on the final
  `status: "completed"` update (observed in raw ACP L1046 vs L1048). Parsing
  both ensures settlement fires strictly on terminal `completed` without relying
  on a single field location.
