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
then delivers the dream prompt through `PromptTask`. `PromptTask` already waits for a `STARTING`
session to become promptable. After this change a Shadow dream reaches an answer and no longer
fails with `run_error` (issue #5388).

## In scope

- `apps/backend/internal/coordinator/dream_episode.go`: widen the `PromptDreamEpisode` parameter
  interface with `LaunchSession`. Before `PromptTask`, call
  `LaunchSession(ctx, &orchestrator.LaunchSessionRequest{TaskID, SessionID,
  Intent: orchestrator.IntentStartCreated, AutoStart: true, NoInitialPrompt: true,
  SkipMessageRecord: true})`. Return a launch error wrapped as
  `start dream episode session: %w`, and do not prompt in that case.
- Update the `PromptDreamEpisode` doc comment so it states the invariant: the prompter starts the
  prepared session, then prompts it.
- `apps/backend/internal/coordinator/dream_episode_test.go`: add the two regression tests listed
  below. They use a recording fake orchestrator that enforces the promptability rule: `PromptTask`
  fails with `orchestrator.ErrSessionNotPromptable` while the fake session is `CREATED`.

## Out of scope

- `dream/run.go`, the `dream.Episode` interface, `CreateSession` (it stays `AutoStart=false`), and
  the orchestrator's `checkSessionPromptable`.
- Wiring changes. `backendapp/coordinator_dream.go` passes `*orchestrator.Service`, which already
  implements `LaunchSession`. If the build shows otherwise, stop and report.

## TDD order

1. Write `TestPromptDreamEpisode_StartsPreparedSessionBeforePrompt` and run it. It must fail with
   `ErrSessionNotPromptable`, which reproduces the issue. Record that failure in Results.
2. Implement the launch call.
3. Add `TestPromptDreamEpisode_LaunchFailureSendsNoPrompt`.

## Acceptance

- The first test passes. It asserts one launch with `IntentStartCreated`, `NoInitialPrompt`,
  `SkipMessageRecord` and `AutoStart` set, an empty `Prompt`, an empty `ActivationSource`, and
  the launch recorded before `PromptTask`. The test also checks the returned answer.
- If the launch returns an error, it is returned, wrapped, and `PromptTask` is never
  called.
- The existing dream, coordinator and orchestrator launch-integrity tests pass, including the
  existing orchestrator test `no initial prompt creates no prompt turn`.

## Verification

```bash
(cd apps/backend && go test -trimpath ./internal/coordinator/ -run 'DreamEpisode' -count=1)
(cd apps/backend && go test -trimpath ./internal/coordinator/ ./internal/coordinator/dream/... -count=1)
(cd apps/backend && go test -trimpath ./internal/orchestrator/ -run 'TestLaunchSession|PromptIntegrity' -count=1)
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

## Dependencies

None.

## Parallelism

`sequential`

## Inputs

- Root cause and evidence in [plan.md](plan.md).
- The **Session start** paragraph in `docs/specs/coordinator/system-design/shadow-dream.md`.
- Precedent: `apps/backend/internal/task/handlers/config_chat_restart.go`, which starts an
  ephemeral utility session with `IntentStartCreated` and `NoInitialPrompt`.
- `loadPromptableSession` / `waitForStartingSessionPromptable` in
  `apps/backend/internal/orchestrator/task_operations.go`.

## Risks

- Setting `ActivationSource` to `session_open` would make `LaunchSession` treat this as a passive
  open. Leave the activation source empty.
- Dropping `NoInitialPrompt` would fall back to the task description and open an extra turn. That
  turn would create an extra `dream` ledger row. The first test asserts the flag.

## Results

Done. `TestPromptDreamEpisode_StartsPreparedSessionBeforePrompt` failed before
the fix with `ErrSessionNotPromptable`, then passed after the launch call was
added. `TestPromptDreamEpisode_LaunchFailureSendsNoPrompt` confirms that a
launch failure is wrapped and prevents `PromptTask` from running.

- `go test -trimpath ./internal/coordinator/ -run 'DreamEpisode' -count=1`: passed.
- `go test -trimpath ./internal/coordinator/ ./internal/coordinator/dream/... -count=1`: passed.
- `go test -trimpath ./internal/orchestrator/ -run 'TestLaunchSession|PromptIntegrity' -count=1`: passed.
- `go build -trimpath ./...`: passed.
- `bash scripts/lint-go-changed apps/backend/internal/coordinator/dream_episode.go apps/backend/internal/coordinator/dream_episode_test.go`: passed, `0 issues`.
- `python3 scripts/list-docs.py validate`: passed, 456 decisions and 2047 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
