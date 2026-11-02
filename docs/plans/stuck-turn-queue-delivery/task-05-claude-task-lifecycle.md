---
id: "05-claude-task-lifecycle"
title: "Settle shells from task lifecycle"
status: blocked
wave: 2
depends_on: ["04-settle-on-process-end"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-010
acceptance_criteria:
  - AC-AGENTS-BACKGROUND-WORK-010.2
  - AC-AGENTS-BACKGROUND-WORK-010.3
  - AC-AGENTS-BACKGROUND-WORK-010.4
system_design:
  - ../../specs/agents/system-design/background-work.md
---

# Task 05: Settle shells from task lifecycle

## Summary

When the bridge advertises it, Kandev negotiates task lifecycle and settles a detached shell from the terminal event naming its launch tool, so the count is right while the agent stays alive.

## In scope

- First, capture a background Bash job through bridge 0.81.2 with `/acp-debug`, with and without the capability, and record frames and differences under Results.
- Confirm where the bridge advertises `asyncTasks` and how the Go SDK fork treats an unknown update variant.
- Add negotiated advertisement and terminal-state mapping.

## Out of scope

- Stop controls, spawn cards, progress, and transcript changes.
- Attribution from `background_complete`.

## Acceptance

- **Stop point:** if the capture shows changed transcript, turn, or admission behavior, or the frames cannot reach the adapter safely, set this task to `blocked`, mark the ADR rejected, and report it. Tasks 01 to 04 stand alone.
- A terminal event settles only the named workload, with its reported outcome.
- Without the advertisement, the capability is not sent and behavior is unchanged.

## Verification

```bash
(cd apps/backend && env -u KANDEV_DEBUG_AGENT_MESSAGES go test -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run 'AsyncTask|ClientCapabilities' -count=1)
(cd apps/backend && env -u KANDEV_DEBUG_AGENT_MESSAGES go test -trimpath -tags fts5 ./internal/orchestrator -run 'BackgroundWork' -count=1)
```

## Files likely touched

- `apps/backend/cmd/acpdbg/main.go`
- `apps/backend/cmd/acpdbg/main_test.go`
- `docs/decisions/2026-11-01-claude-async-task-lifecycle-capability.md`

## Dependencies

Task 04, for the fallback behavior.

## Risks

- The capture uses a real provider and spends a small amount of usage.
- The extension can change between bridge versions, so negotiation and tests must fail safe to Task 04 behavior.

## Parallelism

`sequential`

## Inputs

- [ADR](../../decisions/2026-11-01-claude-async-task-lifecycle-capability.md)
- Bridge `dist/async-tasks.js` and `dist/air-extension.js`
- `dialect_claude*.go` patterns

## Results

Blocked on the pinned SDK and a product decision, with the capture completed.
The task stays blocked; no capability was advertised and no terminal-state
mapping was added. The proposal ADR is rejected for this captured behavior.

**Capture** (bridge `@agentclientprotocol/claude-agent-acp` 0.81.2 at
`/root/.npm/_npx/698eb38f5d7f5a61/node_modules/.bin/claude-agent-acp`, run
through `apps/backend/bin/acpdbg prompt --mode auto --linger ... --exec ...`):

- The bridge advertises its own support in the initialize response:
  `_meta.jetbrains.air = {version: 1, capabilities: [sessionFailure,
  agentFileChangeReport, nativeSubagentSessions, asyncTasks,
  recommendedValue]}`.
- **Without the client capability** (default `acpdbg` payload: no AIR
  `_meta`): no `async_task_*` frames at all. The Bash `sleep 90` launch
  (`run_in_background: true`) emits `tool_call`, four status-less
  `tool_call_update` frames, then one `completed`; the prompt returns and the
  task finishes silently.
- **With `clientCapabilities._meta.jetbrains.air = {version: 1,
  capabilities: ["asyncTasks"]}`** (new `acpdbg --air-capabilities` flag), the
  same launch also emits:
  - `async_task_spawned` (`asyncTaskId`, `taskType: shell`, `canStop: true`,
    `showInTranscript: false`),
  - two `async_task_progress` frames (the second carries `toolCallId` and
    `outputFilePath`),
  - `async_task_state_update` with `state: "stopped"`, then `state:
    "completed"`, carrying `asyncTaskId` and the launch tool's `toolCallId`.
- The launch tool's own `completed` frame arrives before the terminal task
  state, so the launch tool completing does not settle the job.
- **Behavior difference:** with the capability, the task's completion produces
  an extra agent message ("Background task ... completed successfully (exit
  code 0).") and a `_claude/origin: task-notification` usage boundary, i.e. an
  agent-started follow-up turn. Without it, the foreground ends and the task is
  never reported. That transcript/turn difference is a product decision beyond
  workload settlement.

**SDK blocker** (`kdlbs/acp-go-sdk
v0.13.6-0.20261029234457-5e124db622e9`): `ClientCapabilities` has no
`asyncTasks` field (`_meta` is the only carrier), and `SessionUpdate` has no
`async_task_state_update` variant. `SessionUpdate.UnmarshalJSON` tolerates an
unknown discriminator by falling through to the first variant whose unmarshal
succeeds, which is `user_message_chunk`; the `asyncTaskId` and `toolCallId` are
lost before `handleACPUpdate` can read them. Consuming the frames therefore
needs an SDK variant (or raw-frame access), which this task does not change.

Remaining evidence: the capture above (frames and differences) is recorded. The
pinned SDK has no `async_task_state_update` variant, and advertising the
capability changes transcript and follow-up-turn behavior. A revised proposal
requires SDK support or a safe raw-frame path plus an explicit product decision
on the task-notification follow-up turn. Tasks 01 to 04 stand alone.

Review rerun: the race-enabled `acpdbg` test passed for the new capability
advertisement flag. This verifies the probe can reproduce the capture; it does
not change the rejected protocol adoption.
