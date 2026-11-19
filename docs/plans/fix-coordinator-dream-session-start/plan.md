---
created: 2026-11-19
status: done
requirements:
  - REQ-COORDINATOR-SHADOW-DREAM-002
  - REQ-COORDINATOR-SHADOW-DREAM-003
system_design:
  - ../../specs/coordinator/system-design/shadow-dream.md
legacy_specs: []
---

# Implementation Plan: Fix coordinator dream session start

## Overview

Issue [#5388](https://github.com/kdlbs/kandev/issues/5388): every Shadow dream fails with
`run_error` about one second after it starts, with `turn_count: 0` and no cost. This happens on
every attempt, so no dream report is ever produced.

**Root cause.** `DreamEpisode.CreateSession`
(`apps/backend/internal/coordinator/dream_episode.go`) calls `EnsureSession` with
`AutoStart=false`. The session is therefore prepared but stays in `CREATED`, and the only launch
is an asynchronous workspace launch with no agent. `Scheduler.episode`
(`apps/backend/internal/coordinator/dream/run.go`) then calls `Episode.Prompt` right away.
`PromptDreamEpisode` calls `orchestrator.PromptTask`. `loadPromptableSession` waits only for a
`STARTING` session. `checkSessionPromptable` rejects `CREATED` with `ErrSessionNotPromptable`, so
the episode returns `ReasonRunError`. Nothing in the dream path ever starts the agent. The
issue's backend log shows the same order: `rejected prompt: session not ready for input
{"session_state": "CREATED"}`.

The current tests replace the prompter with a stub (`TestDreamEpisode_PromptReturnsTheAgentMessage`
and the `dream` scheduler fakes), so nothing tests the orchestrator state contract.

**Classification.** The implementation violates active acceptance criteria. Under
`AC-COORDINATOR-SHADOW-DREAM-002.4` the episode is one bounded turn, and under
`AC-COORDINATOR-SHADOW-DREAM-003.2` every dream that reaches an answer stores a report. Today no
episode turn can run. No requirement changes. The system design did not say how the prepared
session becomes promptable. One paragraph, **Session start**, was added to
`docs/specs/coordinator/system-design/shadow-dream.md` (section "The episode") to state this.

**Correction (KISS).** The orchestrator already has the exact primitive. Config-chat restart
(`apps/backend/internal/task/handlers/config_chat_restart.go`) uses it to start an ephemeral
utility session:
`LaunchSession{Intent: IntentStartCreated, NoInitialPrompt: true, SkipMessageRecord: true}`.
`NoInitialPrompt` skips the task-description fallback and opens no initial turn. Existing coverage
for this is `no initial prompt creates no prompt turn` in
`orchestrator/session_launch_prompt_integrity_test.go`. Because no turn is opened, the coordinator
ledger gets no extra `dream` row. The executor persists `STARTING` synchronously before it starts the
agent process on both launch paths: `persistLaunchState` (`executor_resume.go`) for a fresh launch,
and `startAgentOnExistingWorkspaceWithRequest` (`executor_execute.go`) for the fast path. The following
`PromptTask` therefore takes its existing STARTING wait (`waitForStartingSessionPromptable`) and
delivers the one prompt once the agent is ready. Launches are serialized per session in
`LaunchPreparedSession`, so the asynchronous workspace-only launch that prepare starts cannot race
this start. The second launch takes the existing fast path on top of the launched workspace. This
is the same order the UI uses when a user sends the first message to a prepared session.

The start happens inside `PromptDreamEpisode`, which runs inside `run.go`'s 20-minute
`promptCtx`, so the agent launch counts against the episode bound as the design requires. The
`dream.Episode` interface, `run.go` and the dream scheduler do not change.

## Scope

### In scope

- `PromptDreamEpisode` starts the prepared session with no prompt, then prompts it.
- A launch error returns before any prompt is sent. The scheduler already maps it to `run_error`.
- Regression tests at the adapter boundary, using a fake that enforces the orchestrator's
  `CREATED`/`STARTING` promptability rule rather than a stub prompter.

### Out of scope

- Changing `checkSessionPromptable` or making `PromptTask` start `CREATED` sessions in general.
- Changing `CreateSession` to `AutoStart=true`. That would launch outside the episode bound and
  fall back to the task description.
- Retry or backoff changes for failed dreams (the 24-hour cadence stays as it is).
- Frontend, persistence, routes, localization.

## Technical approach

1. Widen the interface parameter of `PromptDreamEpisode` to also require
   `LaunchSession(ctx, *orchestrator.LaunchSessionRequest) (*orchestrator.LaunchSessionResponse, error)`.
   `*orchestrator.Service` already satisfies it, so the wiring in
   `backendapp/coordinator_dream.go` compiles unchanged.
2. In the returned prompter, before `PromptTask`, call `LaunchSession` with
   `TaskID`, `SessionID`, `Intent: orchestrator.IntentStartCreated`, `AutoStart: true`,
   `NoInitialPrompt: true` and `SkipMessageRecord: true`. Leave `ActivationSource` empty: this is a
   system launch, not `session_open`, which rejects prompts, and not a user action. Return an
   error wrapped as `start dream episode session: %w`. Only the dream starts this fresh session,
   so no other start can race it and no special case is needed.
3. Leave the `PromptTask` call and the result handling as they are.

## Tests

- `AC-COORDINATOR-SHADOW-DREAM-002.4`, `AC-COORDINATOR-SHADOW-DREAM-003.2`:
  `coordinator/dream_episode_test.go`,
  `TestPromptDreamEpisode_StartsPreparedSessionBeforePrompt`. Its fake orchestrator holds a
  session in `CREATED`. `PromptTask` rejects with `orchestrator.ErrSessionNotPromptable` unless
  the session is `STARTING` or `WAITING_FOR_INPUT`. `LaunchSession` with `IntentStartCreated`
  moves it to `STARTING`. The test asserts the answer, that exactly one launch was made, and the
  request fields (`IntentStartCreated`, `NoInitialPrompt`, `SkipMessageRecord`, `AutoStart`, no
  prompt text, empty activation source). **It fails before the correction** with
  `ErrSessionNotPromptable`, which is the behavior reported in the issue.
- `TestPromptDreamEpisode_LaunchFailureSendsNoPrompt`: the launch returns an error. The
  prompter returns it, and `PromptTask` is never called.
- The existing coverage `no initial prompt creates no prompt turn`
  (`orchestrator/session_launch_prompt_integrity_test.go`) already guarantees that the start
  records no message and opens no turn. It is unchanged and must still pass.

## Work orders

| # | Work order | Wave | Depends on |
|---|------------|------|------------|
| 01 | [Start the dream episode session before prompting](task-01-start-dream-episode-session.md) | 1 | none |

## Verification

```bash
(cd apps/backend && go test -trimpath ./internal/coordinator/ ./internal/coordinator/dream/... -count=1)
(cd apps/backend && go test -trimpath ./internal/orchestrator/ -run 'TestLaunchSession|PromptIntegrity' -count=1)
(cd apps/backend && go build -trimpath ./...)
bash scripts/lint-go-changed apps/backend/internal/coordinator/dream_episode.go apps/backend/internal/coordinator/dream_episode_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

Run from the repository root.

## Risks

- **Race with the prepare's asynchronous workspace launch.** The per-session lock in
  `LaunchPreparedSession` serializes it. The second caller sees the `executors_running` row and
  takes the fast path. This is the same race the UI first-message path already handles.
- **Session left in `CREATED` after the start returns.** It cannot be: both executor launch paths
  persist `STARTING` before they return, and the agent process starts asynchronously afterwards.
- **Agent start fails asynchronously.** The session moves to `FAILED`, and `waitForSessionReady`
  returns `session failed`. The episode fails with `run_error`, the correct reason, after the
  agent was actually attempted.
- **Extra ledger row.** `NoInitialPrompt` sets `noInitialTurn`, so no turn-start event fires for
  the launch. The one `dream` ledger row still comes from the prompt's turn.

## Results

Implementation complete. The regression failed before the change with
`ErrSessionNotPromptable`; after the prompter starts the prepared session first,
the regression and launch-failure coverage pass.

- Coordinator and dream tests passed: `go test -trimpath ./internal/coordinator/ ./internal/coordinator/dream/... -count=1`.
- Orchestrator launch-integrity tests passed: `go test -trimpath ./internal/orchestrator/ -run 'TestLaunchSession|PromptIntegrity' -count=1`.
- Backend build passed: `go build -trimpath ./...`.
- Changed-file Go lint passed with `0 issues`.
- Specification catalog validation and spec lint passed.
