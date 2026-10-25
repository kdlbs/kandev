---
id: "01-shutdown-failure-guard"
title: "Gate terminal failure on graceful shutdown"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/shutdown-turn-failure-suppression.md"
parallelism: sequential
---

# Task 01: Gate terminal failure on graceful shutdown

Suppress user-visible agent failure for turns aborted by backend graceful
shutdown. When `Manager.IsShuttingDown()` is true, an error completion / failing
`MarkCompleted` marks the execution `STOPPED` and publishes `events.AgentStopped`
instead of `FAILED` + `events.AgentFailed`, and skips `classifyAndMaybeRemediate`.

## Root cause
See `plan.md` "Root cause". SIGTERM mid-turn -> agent `error` event (`-32603 peer
disconnected before response`) -> `handleErrorEvent` -> `handleCompleteEvent` ->
`finishPromptCompletion` -> `handleCompleteEventMarkState(isError=true)` ->
`MarkCompleted(1, msg)` -> `FAILED` + `AgentFailed` + classifier
(`agent_runtime_error`) -> orchestrator terminal FAILED -> red UI banner.
`IsShuttingDown()` was already true but never consulted on this path.

## Acceptance
1. With `IsShuttingDown()` true, an error completion (via
   `handleCompleteEventMarkState`) and a failing `MarkCompleted` both leave the
   execution `v1.AgentStatusStopped`, publish `events.AgentStopped`, do NOT
   publish `events.AgentFailed`, and do NOT run `classifyAndMaybeRemediate`.
2. With `IsShuttingDown()` false, behavior is unchanged: error completion / failing
   `MarkCompleted` -> `FAILED` + `AgentFailed` + classifier (existing tests still
   pass).
3. The `promptDoneCh` completion signal still fires on the shutdown-race path so
   `waitForPromptDone` / `dispatchInitialPrompt` waiters unblock.

## Implementation notes
- Add unexported helper `(*Manager) markStoppedDuringShutdown(execution
  *AgentExecution, exitCode int, errorMessage string)` that stamps
  `AgentStatusStopped` + `FinishedAt`/`ExitCode`/`ErrorMessage`, runs the same
  terminal teardown as the failed branch (`EndSessionSpan`,
  `persistExecutorRunning`, `releaseActivity`), and publishes `events.AgentStopped`.
  Reuse it from both guards to avoid a duplicated ≥150-token block (backend lint).
- `manager_events.go` `handleCompleteEventMarkState`: at the top of the
  `if isError` branch, if `m.IsShuttingDown()` log WARN "error completion during
  shutdown, treating as cancellation" and call the helper, then `return` (skip
  `MarkCompleted`). Do not touch the `promptDoneCh` signal (emitted earlier in
  `finishPromptCompletion`).
- `manager_interaction.go` `MarkCompleted`: keep the duplicate-terminal guard.
  In the branch that would set `AgentStatusFailed`, if `m.IsShuttingDown()`, use
  the helper's status/publish decision (STOPPED + `AgentStopped`, no classifier)
  instead of `AgentFailed` + `classifyAndMaybeRemediate`.
- Keep each edited function within the 80-line / 50-statement / complexity limits;
  extract into the helper rather than inlining.

## Files likely touched
- `apps/backend/internal/agent/runtime/lifecycle/manager_events.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_events_shutdown_test.go` (new)

## Tests (must fail before, pass after)
- `manager_events_shutdown_test.go` (new file to respect the 800-line test-file
  rule): shutdown-race error completion -> `STOPPED`, `AgentStopped` published,
  no `AgentFailed`; normal error completion -> `FAILED`, `AgentFailed`; failing
  `MarkCompleted` under shutdown -> `STOPPED`/`AgentStopped`; `promptDoneCh`
  non-blocking receive succeeds on the shutdown path. Use `newTestManager(t)` /
  `createTestManagerWithTracking()`, `createTestExecution(...)`, and
  `MockEventBus.OnPublish` to capture published subjects; drive shutdown via
  `mgr.StopAllAgents(ctx)` (which sets `IsShuttingDown()` true).

## Verification
```bash
cd apps/backend && go test ./internal/agent/runtime/lifecycle/... && \
  golangci-lint run ./internal/agent/runtime/lifecycle/... --new-from-rev="$(git merge-base HEAD origin/main)" --timeout=5m
```

## Dependencies
None.

## Output contract
Summary of the guard + helper, reconciled files-changed list, the exact `go test`
and changed-file linter commands with pass counts, any risks, and status updates
to this task file and `plan.md`.

## Results
- Added `(*Manager) markStoppedDuringShutdown` in `manager_interaction.go`;
  guarded the failure branch of `MarkCompleted` and the `isError` branch of
  `handleCompleteEventMarkState` (`manager_events.go`) on `IsShuttingDown()`.
- New tests in `manager_events_shutdown_test.go` (new file):
  `TestHandleCompleteEventMarkState_ShutdownRaceMarksStopped`,
  `TestHandleCompleteEventMarkState_ErrorFailsWhenNotShuttingDown`,
  `TestMarkCompleted_ShutdownRaceMarksStopped`,
  `TestMarkCompleted_SuccessUnaffectedByShutdown`.
- `cd apps/backend && go test ./internal/agent/runtime/lifecycle/` -> `ok` (full
  package, ~48s). Targeted new tests -> PASS.
- `golangci-lint run ./internal/agent/runtime/lifecycle/... --new-from-rev="$(git merge-base HEAD origin/main)" --timeout=5m`
  -> `0 issues`. `gofmt -l` on the three changed files -> clean.
- External side effects: None.
