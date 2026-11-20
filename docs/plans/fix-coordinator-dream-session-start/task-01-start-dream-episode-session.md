---
id: "01-start-dream-episode-session"
title: "Start the dream episode session before prompting"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-SHADOW-DREAM-002
  - REQ-COORDINATOR-SHADOW-DREAM-003
acceptance_criteria:
  - AC-COORDINATOR-SHADOW-DREAM-002.4
  - AC-COORDINATOR-SHADOW-DREAM-003.2
system_design:
  - ../../specs/coordinator/system-design/shadow-dream.md
---

# Task 01: Start the dream episode session before prompting

## Summary

`PromptDreamEpisode` starts the prepared (`CREATED`) episode session with no prompt and no turn,
then delivers the dream prompt through `PromptTask`. If launch admission is queued because session
capacity is full, it waits until the persisted session and agent are promptable first. After this
change a Shadow dream reaches an answer and no longer fails with `run_error` (issue #5388).

## In scope

- `apps/backend/internal/coordinator/dream_episode.go`: widen the `PromptDreamEpisode` parameter
  interface with `LaunchSession` and `WaitForSessionPromptable`. Before `PromptTask`, call
  `LaunchSession(ctx, &orchestrator.LaunchSessionRequest{TaskID, SessionID,
  Intent: orchestrator.IntentStartCreated, AutoStart: true, NoInitialPrompt: true,
  SkipMessageRecord: true})`. If the response disposition is `queued`, wait with the same context
  until the session and agent are promptable. Return launch or readiness errors before any prompt.
- Update the `PromptDreamEpisode` doc comment so it states the invariant: the prompter starts the
  prepared session, waits after queued admission, then prompts it.
- `apps/backend/internal/coordinator/dream_episode_test.go`: cover immediate start, queued start,
  readiness failure and launch failure. The fake rejects prompts while the session is `CREATED`.
- `apps/backend/internal/orchestrator/task_operations.go`: expose the existing bounded readiness
  wait for callers that receive a queued launch response.
- `apps/backend/internal/orchestrator/coordinator_dream_queued_launch_test.go`: exercise a real
  queued launch with a persisted `CREATED` session and controlled executor readiness; assert no
  startup message or turn and exactly one evidence prompt after readiness.

## Out of scope

- `dream/run.go`, the `dream.Episode` interface, `CreateSession` (it stays `AutoStart=false`), and
  the orchestrator's `checkSessionPromptable`.
- Wiring changes. `backendapp/coordinator_dream.go` passes `*orchestrator.Service`, which already
  implements `LaunchSession`. If the build shows otherwise, stop and report.

## TDD order

1. Write `TestPromptDreamEpisode_StartsPreparedSessionBeforePrompt` and run it. It must fail with
   `ErrSessionNotPromptable`, which reproduces the issue. Record that failure in Results.
2. Implement the launch call.
3. Add queued-launch coverage. `TestPromptDreamEpisode_WaitsForQueuedSessionBeforePrompt` must
   fail before the wait is added because the queued fake remains `CREATED` and `PromptTask` returns
   `ErrSessionNotPromptable`; `TestPromptDreamEpisode_QueuedSessionWaitFailureSendsNoPrompt`
   verifies no prompt on readiness failure.
4. Add the real orchestrator test for a persisted queued session and controlled agent readiness.
5. Keep `TestPromptDreamEpisode_LaunchFailureSendsNoPrompt` to cover immediate launch failure.

## Acceptance

- The first test passes. It asserts one launch with `IntentStartCreated`, `NoInitialPrompt`,
  `SkipMessageRecord` and `AutoStart` set, an empty `Prompt`, an empty `ActivationSource`, and
  the launch recorded before `PromptTask`. The test also checks the returned answer.
- A queued response causes readiness wait before `PromptTask`; a readiness failure sends no prompt.
- The real orchestrator test proves a queued `CREATED` session does not receive an initial message
  or turn, and dispatches the evidence prompt once after session and agent readiness.
- If the launch returns an error, it is returned, wrapped, and `PromptTask` is never
  called.
- The existing dream, coordinator and orchestrator launch-integrity tests pass, including the
  existing orchestrator test `no initial prompt creates no prompt turn`.

## Verification

```bash
(cd apps/backend && go test -trimpath ./internal/coordinator/ -run 'DreamEpisode' -count=1)
(cd apps/backend && go test -trimpath ./internal/coordinator/ ./internal/coordinator/dream/... -count=1)
(cd apps/backend && go test -trimpath ./internal/orchestrator/ -run 'TestLaunchSession|PromptIntegrity|TestQueuedDreamLaunchWaitsForSessionAndExecutorReadinessBeforePrompt' -count=1)
(cd apps/backend && go build -trimpath ./...)
bash scripts/lint-go-changed apps/backend/internal/coordinator/dream_episode.go apps/backend/internal/coordinator/dream_episode_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- apps/backend/internal/coordinator docs/specs/coordinator docs/plans/fix-coordinator-dream-session-start
```

Run from the repository root.

## Files likely touched

- `apps/backend/internal/coordinator/dream_episode.go`
- `apps/backend/internal/coordinator/dream_episode_test.go`
- `apps/backend/internal/orchestrator/session_launch.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/coordinator_dream_queued_launch_test.go`
- `docs/specs/coordinator/system-design/shadow-dream.md`

## Dependencies

None.

## Parallelism

`sequential`

## Inputs

- Root cause and evidence in [plan.md](plan.md).
- The **Session start** paragraph in `docs/specs/coordinator/system-design/shadow-dream.md`.
- Precedent: `apps/backend/internal/task/handlers/config_chat_restart.go`, which starts an
  ephemeral utility session with `IntentStartCreated` and `NoInitialPrompt`.
- `loadPromptableSession` / `waitForSessionPromptable` in
  `apps/backend/internal/orchestrator/task_operations.go`.

## Risks

- Setting `ActivationSource` to `session_open` would make `LaunchSession` treat this as a passive
  open. Leave the activation source empty.
- Dropping `NoInitialPrompt` would fall back to the task description and open an extra turn. That
  turn would create an extra `dream` ledger row. The first test asserts the flag.
- A queued readiness wait uses the episode context and orchestrator launch timeout; timeout or
  terminal session state returns an error before `PromptTask`.

## Results

Done. The original `TestPromptDreamEpisode_StartsPreparedSessionBeforePrompt`
reproduced `ErrSessionNotPromptable` before the first fix. During PR fixup, the
queued-launch regression also failed before the readiness wait with
`ErrSessionNotPromptable`. The queued success and failure tests now verify
ordering and that readiness errors prevent prompt dispatch. The real
orchestrator regression verifies a persisted queued `CREATED` session adds no
message or turn before readiness, then dispatches the evidence prompt once and
opens one turn.

- `go test -trimpath ./internal/coordinator/ ./internal/coordinator/dream/... -count=1`: passed.
- `go test -trimpath ./internal/orchestrator/ -run 'TestLaunchSession|PromptIntegrity|TestQueuedDreamLaunchWaitsForSessionAndExecutorReadinessBeforePrompt' -count=1`: passed.
- `go test -race -trimpath ./internal/coordinator/ ./internal/orchestrator/ -run 'TestPromptDreamEpisode_|TestQueuedDreamLaunchWaitsForSessionAndExecutorReadinessBeforePrompt' -count=1`: passed.
- `go build -trimpath ./...`: passed.
- `bash scripts/lint-go-changed apps/backend/internal/coordinator/dream_episode.go apps/backend/internal/coordinator/dream_episode_test.go apps/backend/internal/orchestrator/session_launch.go apps/backend/internal/orchestrator/task_operations.go apps/backend/internal/orchestrator/coordinator_dream_queued_launch_test.go`: passed, `0 issues`.
- `python3 scripts/list-docs.py validate`: passed, 456 decisions and 2047 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
