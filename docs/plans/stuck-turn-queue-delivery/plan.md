---
created: 2026-11-01
status: partial
requirements:
  - REQ-TASKS-SESSION-TURN-SETTLEMENT-002
  - REQ-TASKS-RESUME-PROMPT-QUEUE-003
  - REQ-UI-MESSAGE-QUEUE-SEND-NOW-001
  - REQ-UI-CANCEL-TURN-PROGRESS-001
  - REQ-AGENTS-BACKGROUND-WORK-010
  - REQ-PLATFORM-ACP-DEBUG-LOG-RETENTION-001
  - REQ-PLATFORM-ACP-DEBUG-LOG-RETENTION-002
system_design:
  - ../../specs/tasks/system-design/session-turn-settlement.md
  - ../../specs/tasks/system-design/resume-prompt-queue.md
  - ../../specs/ui/system-design/message-queue-send-now.md
  - ../../specs/ui/system-design/cancel-turn-availability.md
  - ../../specs/agents/system-design/background-work.md
  - ../../specs/platform/system-design/acp-debug-log-retention.md
legacy_specs: []
---

# Implementation Plan: Stuck turn and queue delivery repair

## Overview

Session `c969d17b` (ACP `b668a3d6`) stayed `RUNNING` for an hour after its agent
finished, which blocked a queued message from another agent. Send Now then failed
and left the message stranded until a task focus resumed the session and delivered it. The
session also showed two finished jobs as running.

The order puts the blocking defect first. Tasks 01 and 02 restore message delivery,
Task 03 pins the rule that a restart starts no agent, and 04 and 05 correct the job count. Tasks 06 and 07 are independent cleanups.

**Status: partial.** Tasks 01 to 04, 06, and 07 are delivered and verified. Task 05
is blocked: the capture is recorded and the bridge frames are confirmed, but the
pinned ACP SDK cannot represent `async_task_state_update`. As a result, job rows
still stay `running` while a long-lived Claude agent remains alive and its detached
shell finishes; they settle only when the owning process ends (Task 04). That
user-visible gap is tracked by the not-yet-delivered
`AC-AGENTS-BACKGROUND-WORK-010.2`.

## Evidence and root causes

| # | Defect | Root cause | Evidence |
| --- | --- | --- | --- |
| 1 | Turn never completes | `trackTool` adds a status-less update to `activeTools`. The tool has already finished, so no terminal update follows, and the idle timer never starts again. | Seq 1173, recovered from DB free pages by type and exact size (386 bytes). A temporary test reproduced `activeTools=map[launch:{}]` with no completion. |
| 2a | Restored entry is stranded | Restoring a Send Now claim triggers no drain, and the readiness drain ran about 100 ms earlier while the entry was claimed. | Backend log, 22:48:44.179 to .284, with no drain afterwards. The cause of the `superseded` prompt failure was not pinned. |
| 3a | Rows outlive their process | Owned teardown and `reclaimIdleSession` do not reach settlement. Startup settlement may also read unknown liveness before recovery completes; Task 04 confirms this. | Rows stayed `running` through the 22:48 replacement, the 17:47 restart, and the 17:53 reclaim. |
| 3b | No live completion evidence | Detached launches stay `running`, `background_complete` has no tool ID, and the bridge sends task lifecycle only to clients advertising `asyncTasks`. | Row timestamps, inbox frames, and bridge source. |
| 4 | Debug log deleted | The count cap ignores open writers, and agent subprocesses inherit the debug flag, so nested tests add files to the real directory. | 172 files last written at 21:47 to 21:48, 145 of them from one test session. Deletion is inferred; no log line records it. |
| 5 | Misleading cancel | Explicit cancel has no idle check, so it creates a turn and a status message. | Turn `ba9b2f02` and the status message at 17:56:06, after `end_turn`. |

## Scope

### In scope

- The five defects above, with regression tests.
- Terminal Claude task lifecycle events for already recognized shells, subject to
  the capture in Task 05.

### Out of scope

- Delivering queued messages at startup. A restart starts no agent, so entries wait
  until a user opens the task. The 17:47 restart and 17:55:46 focus showed this
  behavior, and it is correct.
- The 30-second `agent not ready` wait and the `superseded` failure. Task 02 makes
  any such failure recoverable, and Task 01 removes the stuck turn that led to it.
- Job controls, spawn cards, or progress from the bridge's task lifecycle.
- Repairing history for sessions already affected. Rows settle at the next startup.
- Default log limits, profile defaults, and the Claude prompt-handoff experiment.

## Technical approach

### Turn completion

`apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_async_complete.go`: `trackTool` ignores an empty `ToolStatus`.
`maybeScheduleAsyncTurnComplete` and `isAsyncTurnContentEvent` do not treat a
status-less tool update as content when the tool is not tracked. The origin-bearing
`usage_update` handled through `usageLifecycleEvent` clears `activeTools` and arms
the timer when no prompt turn owns the session.

### Queue delivery

`apps/backend/internal/orchestrator/queue_send_now.go`: a successful restore in the running backend
marks a pending drain. Startup gets no drain, and a regression test pins that.

### Background settlement

`apps/backend/internal/orchestrator/event_handlers_agent.go` settles on the owned-teardown branch before the
successor launches, `apps/backend/internal/orchestrator/reconcile_liveness.go` settles after reclaim, and startup
settlement runs after lifecycle recovery if Task 04 confirms that ordering defect.

### Task lifecycle

Blocked: the capture is recorded and confirms the bridge's `async_task_*` frames,
but the pinned SDK cannot represent `async_task_state_update`. No capability is
advertised; process-end settlement from Task 04 stays authoritative, and
`AC-AGENTS-BACKGROUND-WORK-010.2` is not yet delivered. See Task 05's results.
`acpdbg` gained `--air-capabilities` so the capture can advertise `asyncTasks`.

| Provider | Capability shape | Behavior | Evidence | Fallback |
| --- | --- | --- | --- | --- |
| `claude-acp`, bridge advertises `asyncTasks` | Not adopted; capture recorded | No capability sent; rows settle from process-end evidence | Recorded 0.81.2 capture and SDK inspection in Task 05 | Process-end settlement |
| `claude-acp`, older bridge | No advertisement | Capability not sent | Existing suites | Process-end settlement |
| Other ACP and Codex transports | Not applicable | Unchanged | Existing suites | Their existing evidence |

### Cancel

`apps/backend/internal/orchestrator/task_operations.go`: `runExplicitCancellationOwned` returns success early for
an idle target, under the guard, per the design's idle definition.

### Log retention

`shared/acplog.go`: the count cap excludes files with open writers. A shared
predicate removes debug settings from agent environments in `server/config` and
`server/utility`. `acp` gains a `TestMain`.

## Tests

| Criteria | Evidence |
| --- | --- |
| `AC-TASKS-SESSION-TURN-SETTLEMENT-002.1` to `.4` | `apps/backend/internal/agentctl/server/adapter/transport/acp/async_complete_status_test.go` |
| `AC-TASKS-SESSION-TURN-SETTLEMENT-002.5` | `apps/web/e2e/tests/chat/queue-after-background-followup.spec.ts` |
| `AC-UI-MESSAGE-QUEUE-SEND-NOW-001.12` | `apps/backend/internal/orchestrator/queue_send_now_restore_drain_test.go` |
| `AC-TASKS-RESUME-PROMPT-QUEUE-003.1` to `.3` | `apps/backend/internal/orchestrator/queue_restart_no_autostart_test.go` |
| `AC-AGENTS-BACKGROUND-WORK-010.1` | `apps/backend/internal/orchestrator/background_work_settlement_paths_test.go` (with the existing `background_work_settlement_test.go`) |
| `AC-AGENTS-BACKGROUND-WORK-010.2` to `.4` | Not delivered; see Task 05 and the blocked ADR. `.3` holds through process-end settlement. |
| `AC-UI-CANCEL-TURN-PROGRESS-001.13` | `apps/backend/internal/orchestrator/cancel_idle_target_test.go` |
| `AC-PLATFORM-ACP-DEBUG-LOG-RETENTION-001.*` | `apps/backend/internal/agentctl/server/adapter/transport/shared/acpjanitor_test.go` |
| `AC-PLATFORM-ACP-DEBUG-LOG-RETENTION-002.*` | `apps/backend/internal/agentctl/server/config/config_debugenv_test.go`, `apps/backend/internal/agentctl/server/utility/acp_executor_debugenv_test.go`, `apps/backend/internal/common/launchwrapper/environment_test.go` |

## E2E tests

| Flow | Criteria | Spec and project |
| --- | --- | --- |
| An agent-started follow-up ends, and a message queued during it is delivered | `AC-TASKS-SESSION-TURN-SETTLEMENT-002.5` | `tests/chat/queue-after-background-followup.spec.ts`, `chromium` |
| The job count clears when the agent process is replaced | `AC-AGENTS-BACKGROUND-WORK-010.5` | `tests/task/background-work-process-end.spec.ts` for `chromium` and `tests/task/mobile-background-work-process-end.spec.ts` for `mobile-chrome`, both over `tests/task/background-work-process-end.helpers.ts` |

No rendered UI changes. These flows verify existing controls and counts.

## Work orders

- [x] [Task 01: Complete agent-started turns](task-01-complete-agent-started-turns.md)
- [x] [Task 02: Drain restored Send Now entries](task-02-drain-restored-send-now.md)
- [x] [Task 03: Pin that a restart starts no agent](task-03-restart-starts-no-agent.md)
- [x] [Task 04: Settle workloads when the process ends](task-04-settle-on-process-end.md)
- [ ] [Task 05: Settle shells from task lifecycle](task-05-claude-task-lifecycle.md) (blocked on the SDK variant; capture recorded)
- [x] [Task 06: Ignore cancel on an idle session](task-06-ignore-idle-cancel.md)
- [x] [Task 07: Protect ACP debug logs](task-07-protect-acp-debug-logs.md)

## Verification results

Backend (all from `apps/backend`, with `KANDEV_DEBUG_AGENT_MESSAGES` unset):

- `go build ./...` succeeds.
- `go test ./internal/orchestrator ./internal/orchestrator/messagequeue` passes
  (265s / 5s), covering the new settlement, restart, Send Now restore-drain,
  idle-cancel, deadlock-regression, prompt-attribution, and startup suites.
- `go test ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/adapter/transport/shared ./internal/agentctl/server/config ./internal/agentctl/server/utility ./internal/common/launchwrapper ./internal/agent/acpdbg ./cmd/mock-agent` passes.
- `KANDEV_DEBUG_AGENT_MESSAGES=true go test ./internal/agentctl/server/adapter/transport/acp -run 'AsyncTurn|Statusless|ProviderBoundary'` leaves the host's `~/.kandev/logs/acp` directory byte-identical (the package `TestMain` redirects it).
- `bash scripts/lint-go-changed <changed files>` reports 0 issues.
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all` pass.

Review follow-ups verified separately: the deadlock regression holds the
lifecycle lock across a synchronously published `agent.stopped` and fails with
the blocking settle; the prompt-generation test fails if status-less updates are
excluded from prompt attribution; the startup test uses the real
`RowProcessLiveness` classifier and the post-reconciliation row shape
(execution-less lookup, `failed` status, cleared PID) and fails without the
ended-status stop evidence; the 0.81.2 capture is recorded in Task 05's results.

E2E (managed host runner, `--shards 1`, rerun after the review fixes):

- `queue-after-background-followup.spec.ts` and
  `background-work-process-end.spec.ts` pass together on chromium (2 passed,
  22s). The queued message is delivered after the agent-started follow-up
  completes without user action.
- `mobile-background-work-process-end.spec.ts` passes on mobile-chrome (1
  passed, 6s). The desktop and phone flows stop and resume the owning process
  through the same WS actions the UI uses, then assert the active projection
  reaches zero and the history chip remains. The resume is given a 90s WS budget
  because the default 30s budget is shorter than the agent-readiness window on a
  loaded host.

Task 05 (blocked): the 0.81.2 capture ran through
`acpdbg prompt --mode auto --linger ... --air-capabilities asyncTasks` and
recorded both variants. Without the capability the bridge sends no `async_task_*`
frames; with it, `async_task_spawned`, `async_task_progress`, and terminal
`async_task_state_update` frames appear with the launch `toolCallId`, and the
completion adds an agent-started task-notification turn. The pinned SDK decodes an
unknown `sessionUpdate` as an empty `user_message_chunk`, which is the remaining
blocker. Full frames and differences are in Task 05's results.

## Risks

- A queued message waits indefinitely after a restart if nobody opens its task.
  This is intended, and the queue row stays visible.
- Task 05 is blocked on the missing SDK variant. The capture shows that
  advertising the capability also adds an agent-started task-notification turn
  when a detached shell finishes, so even with SDK support adoption needs a
  product decision on that transcript change.
- Clearing `activeTools` on the provider boundary could complete a turn early if a
  bridge sends that frame mid-turn. Observed frames show one per result; Task 01
  covers this with the existing idle timer and a test.
- The wire form of seq 1173 is inferred. Only the normalized frame was recovered,
  and the PostToolUse hook is its likely source. The fix does not depend on the source.
- Free-page evidence is not durable. Keep
  `data/backups/kandev-v0.98.0-154-g838043071d9-20261031T175021Z.db` if the frames matter.
